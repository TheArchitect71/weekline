package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (p *Postgres) SwapDraftShifts(ctx context.Context, actorID string, input ShiftSwapInput) (ShiftSwapResult, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return ShiftSwapResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var scheduleID string
	var version int
	err = tx.QueryRow(ctx, `
		SELECT schedule.id::text, schedule.version
		FROM schedules schedule
		JOIN shifts source ON source.schedule_id = schedule.id AND source.id = $1::uuid
		JOIN shifts target ON target.schedule_id = schedule.id AND target.id = $2::uuid
		FOR UPDATE OF schedule`, input.SourceShiftID, input.TargetShiftID,
	).Scan(&scheduleID, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ShiftSwapResult{}, fmt.Errorf("%w: shifts must belong to the same draft schedule", ErrInvalid)
	}
	if err != nil {
		return ShiftSwapResult{}, err
	}

	rows, err := tx.Query(ctx, `
		SELECT id::text, employee_id::text, to_char(shift_date, 'YYYY-MM-DD'),
		       to_char(starts_at, 'HH24:MI'), to_char(ends_at, 'HH24:MI'), notes
		FROM shifts
		WHERE id = ANY($1::uuid[])
		ORDER BY id
		FOR UPDATE`, []string{input.SourceShiftID, input.TargetShiftID})
	if err != nil {
		return ShiftSwapResult{}, err
	}
	defer rows.Close()
	locked := map[string]Shift{}
	for rows.Next() {
		var shift Shift
		if err := rows.Scan(&shift.ID, &shift.EmployeeID, &shift.Date, &shift.Start, &shift.End, &shift.Note); err != nil {
			return ShiftSwapResult{}, err
		}
		locked[shift.ID] = shift
	}
	if err := rows.Err(); err != nil {
		return ShiftSwapResult{}, err
	}
	source, sourceOK := locked[input.SourceShiftID]
	target, targetOK := locked[input.TargetShiftID]
	if !sourceOK || !targetOK {
		return ShiftSwapResult{}, ErrNotFound
	}

	sourceAfter := ShiftInput{EmployeeID: target.EmployeeID, Date: target.Date, Start: source.Start, End: source.End, Note: source.Note}
	targetAfter := ShiftInput{EmployeeID: source.EmployeeID, Date: source.Date, Start: target.Start, End: target.End, Note: target.Note}
	excluded := []string{source.ID, target.ID}
	sourceConflicts, err := evaluateShiftConflicts(ctx, tx, scheduleID, sourceAfter, excluded, source.ID, true)
	if err != nil {
		return ShiftSwapResult{}, err
	}
	targetConflicts, err := evaluateShiftConflicts(ctx, tx, scheduleID, targetAfter, excluded, target.ID, true)
	if err != nil {
		return ShiftSwapResult{}, err
	}
	if err = enforceShiftConflicts(sourceConflicts, nil); err != nil {
		return ShiftSwapResult{}, err
	}
	if err = enforceShiftConflicts(targetConflicts, nil); err != nil {
		return ShiftSwapResult{}, err
	}

	updatedSource, err := updateShiftPosition(ctx, tx, source.ID, sourceAfter, version+1)
	if err != nil {
		return ShiftSwapResult{}, err
	}
	updatedTarget, err := updateShiftPosition(ctx, tx, target.ID, targetAfter, version+1)
	if err != nil {
		return ShiftSwapResult{}, err
	}
	if err = recordShiftConflicts(ctx, tx, scheduleID, source.ID, actorID, sourceConflicts, nil); err != nil {
		return ShiftSwapResult{}, err
	}
	if err = recordShiftConflicts(ctx, tx, scheduleID, target.ID, actorID, targetConflicts, nil); err != nil {
		return ShiftSwapResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE schedules SET status = 'draft', updated_at = now() WHERE id = $1::uuid`, scheduleID); err != nil {
		return ShiftSwapResult{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "shift", source.ID, "drag_swapped", source, updatedSource); err != nil {
		return ShiftSwapResult{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "shift", target.ID, "drag_swapped", target, updatedTarget); err != nil {
		return ShiftSwapResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ShiftSwapResult{}, err
	}
	return ShiftSwapResult{Source: updatedSource, Target: updatedTarget}, nil
}

func updateShiftPosition(ctx context.Context, tx pgx.Tx, shiftID string, input ShiftInput, changeVersion int) (Shift, error) {
	var shift Shift
	err := tx.QueryRow(ctx, `
		UPDATE shifts SET employee_id = $2::uuid, shift_date = $3::date, starts_at = $4::time,
		       ends_at = $5::time, notes = $6, change_version = $7, updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, employee_id::text, to_char(shift_date, 'YYYY-MM-DD'),
		          to_char(starts_at, 'HH24:MI'), to_char(ends_at, 'HH24:MI'), notes`,
		shiftID, input.EmployeeID, input.Date, input.Start, input.End, input.Note, changeVersion,
	).Scan(&shift.ID, &shift.EmployeeID, &shift.Date, &shift.Start, &shift.End, &shift.Note)
	shift.Changed = true
	return shift, err
}
