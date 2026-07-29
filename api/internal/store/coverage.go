package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (p *Postgres) ManagerOpenShifts(ctx context.Context, scheduleID string) ([]OpenShift, error) {
	return p.queryOpenShifts(ctx, `
		SELECT open_shift.id::text, to_char(open_shift.shift_date, 'YYYY-MM-DD'),
		       to_char(open_shift.starts_at, 'HH24:MI'), to_char(open_shift.ends_at, 'HH24:MI'),
		       open_shift.notes, open_shift.required_headcount, count(shift.id)::int,
		       GREATEST(open_shift.required_headcount - count(shift.id)::int, 0),
		       CASE WHEN open_shift.status = 'closed' THEN 'closed'
		            WHEN count(shift.id) >= open_shift.required_headcount THEN 'filled' ELSE 'open' END,
		       ''
		FROM open_shifts open_shift
		LEFT JOIN shifts shift ON shift.open_shift_id = open_shift.id
		WHERE open_shift.schedule_id = $1::uuid
		GROUP BY open_shift.id
		ORDER BY open_shift.shift_date, open_shift.starts_at`, scheduleID)
}

func (p *Postgres) WorkerOpenShifts(ctx context.Context, scheduleID, employeeID string) ([]OpenShift, error) {
	return p.queryOpenShifts(ctx, `
		SELECT open_shift.source_open_shift_id::text, to_char(open_shift.shift_date, 'YYYY-MM-DD'),
		       to_char(open_shift.starts_at, 'HH24:MI'), to_char(open_shift.ends_at, 'HH24:MI'),
		       open_shift.notes, open_shift.required_headcount, count(shift.source_shift_id)::int,
		       GREATEST(open_shift.required_headcount - count(shift.source_shift_id)::int, 0),
		       CASE WHEN open_shift.status = 'closed' THEN 'closed'
		            WHEN count(shift.source_shift_id) >= open_shift.required_headcount THEN 'filled' ELSE 'open' END,
		       COALESCE((
		         SELECT request.status FROM shift_requests request
		         WHERE request.open_shift_id = open_shift.source_open_shift_id
		           AND request.requested_by_employee_id = $2::uuid
		         ORDER BY request.created_at DESC LIMIT 1
		       ), '')
		FROM published_open_shifts open_shift
		LEFT JOIN published_shifts shift ON shift.open_shift_id = open_shift.source_open_shift_id
		WHERE open_shift.schedule_id = $1::uuid
		GROUP BY open_shift.schedule_id, open_shift.source_open_shift_id, open_shift.shift_date,
		         open_shift.starts_at, open_shift.ends_at, open_shift.notes,
		         open_shift.required_headcount, open_shift.status
		ORDER BY open_shift.shift_date, open_shift.starts_at`, scheduleID, employeeID)
}

func (p *Postgres) queryOpenShifts(ctx context.Context, query string, args ...any) ([]OpenShift, error) {
	rows, err := p.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	openShifts := []OpenShift{}
	for rows.Next() {
		var openShift OpenShift
		if err := rows.Scan(&openShift.ID, &openShift.Date, &openShift.Start, &openShift.End,
			&openShift.Note, &openShift.RequiredHeadcount, &openShift.AssignedCount,
			&openShift.CoverageGap, &openShift.Status, &openShift.ClaimStatus); err != nil {
			return nil, err
		}
		openShifts = append(openShifts, openShift)
	}
	return openShifts, rows.Err()
}

func (p *Postgres) CreateOpenShift(ctx context.Context, weekStart, actorID string, input OpenShiftInput) (OpenShift, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return OpenShift{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var scheduleID string
	var version int
	if err = tx.QueryRow(ctx, `
		SELECT id::text, version FROM schedules WHERE week_start = $1::date FOR UPDATE`, weekStart,
	).Scan(&scheduleID, &version); errors.Is(err, pgx.ErrNoRows) {
		return OpenShift{}, ErrNotFound
	}
	if err != nil {
		return OpenShift{}, err
	}
	var openShift OpenShift
	err = tx.QueryRow(ctx, `
		INSERT INTO open_shifts (schedule_id, shift_date, starts_at, ends_at, notes, required_headcount, change_version)
		VALUES ($1::uuid, $2::date, $3::time, $4::time, $5, $6, $7)
		RETURNING id::text, to_char(shift_date, 'YYYY-MM-DD'), to_char(starts_at, 'HH24:MI'),
		          to_char(ends_at, 'HH24:MI'), notes, required_headcount, status`,
		scheduleID, input.Date, input.Start, input.End, input.Note, input.RequiredHeadcount, version+1,
	).Scan(&openShift.ID, &openShift.Date, &openShift.Start, &openShift.End,
		&openShift.Note, &openShift.RequiredHeadcount, &openShift.Status)
	if err != nil {
		return OpenShift{}, err
	}
	openShift.CoverageGap = openShift.RequiredHeadcount
	if _, err = tx.Exec(ctx, `UPDATE schedules SET status = 'draft', updated_at = now() WHERE id = $1::uuid`, scheduleID); err != nil {
		return OpenShift{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "open_shift", openShift.ID, "created", nil, openShift); err != nil {
		return OpenShift{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OpenShift{}, err
	}
	return openShift, nil
}

func (p *Postgres) UpdateOpenShift(ctx context.Context, openShiftID, actorID string, input OpenShiftInput) (OpenShift, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return OpenShift{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var scheduleID, weekStart string
	var version, assigned int
	var before OpenShift
	err = tx.QueryRow(ctx, `
		SELECT schedule.id::text, to_char(schedule.week_start, 'YYYY-MM-DD'), schedule.version,
		       open_shift.id::text, to_char(open_shift.shift_date, 'YYYY-MM-DD'),
		       to_char(open_shift.starts_at, 'HH24:MI'), to_char(open_shift.ends_at, 'HH24:MI'),
		       open_shift.notes, open_shift.required_headcount, open_shift.status,
		       (SELECT count(*)::int FROM shifts WHERE open_shift_id = open_shift.id)
		FROM open_shifts open_shift JOIN schedules schedule ON schedule.id = open_shift.schedule_id
		WHERE open_shift.id = $1::uuid FOR UPDATE OF schedule, open_shift`, openShiftID,
	).Scan(&scheduleID, &weekStart, &version, &before.ID, &before.Date, &before.Start,
		&before.End, &before.Note, &before.RequiredHeadcount, &before.Status, &assigned)
	if errors.Is(err, pgx.ErrNoRows) {
		return OpenShift{}, ErrNotFound
	}
	if err != nil {
		return OpenShift{}, err
	}
	if assigned > 0 {
		return OpenShift{}, fmt.Errorf("%w: remove assigned shifts before changing this open shift", ErrConflict)
	}
	if !dateWithinWeek(input.Date, weekStart) {
		return OpenShift{}, fmt.Errorf("%w: open shift date must fall within the schedule week", ErrInvalid)
	}
	var openShift OpenShift
	err = tx.QueryRow(ctx, `
		UPDATE open_shifts SET shift_date = $2::date, starts_at = $3::time, ends_at = $4::time,
		       notes = $5, required_headcount = $6, change_version = $7, updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, to_char(shift_date, 'YYYY-MM-DD'), to_char(starts_at, 'HH24:MI'),
		          to_char(ends_at, 'HH24:MI'), notes, required_headcount, status`,
		openShiftID, input.Date, input.Start, input.End, input.Note, input.RequiredHeadcount, version+1,
	).Scan(&openShift.ID, &openShift.Date, &openShift.Start, &openShift.End,
		&openShift.Note, &openShift.RequiredHeadcount, &openShift.Status)
	if err != nil {
		return OpenShift{}, err
	}
	openShift.CoverageGap = openShift.RequiredHeadcount
	if _, err = tx.Exec(ctx, `UPDATE schedules SET status = 'draft', updated_at = now() WHERE id = $1::uuid`, scheduleID); err != nil {
		return OpenShift{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "open_shift", openShift.ID, "updated", before, openShift); err != nil {
		return OpenShift{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OpenShift{}, err
	}
	return openShift, nil
}

func (p *Postgres) DeleteOpenShift(ctx context.Context, openShiftID, actorID string) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var scheduleID string
	var assigned int
	var before OpenShift
	err = tx.QueryRow(ctx, `
		SELECT schedule.id::text, open_shift.id::text, to_char(open_shift.shift_date, 'YYYY-MM-DD'),
		       to_char(open_shift.starts_at, 'HH24:MI'), to_char(open_shift.ends_at, 'HH24:MI'),
		       open_shift.notes, open_shift.required_headcount, open_shift.status,
		       (SELECT count(*)::int FROM shifts WHERE open_shift_id = open_shift.id)
		FROM open_shifts open_shift JOIN schedules schedule ON schedule.id = open_shift.schedule_id
		WHERE open_shift.id = $1::uuid FOR UPDATE OF schedule, open_shift`, openShiftID,
	).Scan(&scheduleID, &before.ID, &before.Date, &before.Start, &before.End,
		&before.Note, &before.RequiredHeadcount, &before.Status, &assigned)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if assigned > 0 {
		return fmt.Errorf("%w: remove assigned shifts before deleting this open shift", ErrConflict)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM open_shifts WHERE id = $1::uuid`, openShiftID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE schedules SET status = 'draft', updated_at = now() WHERE id = $1::uuid`, scheduleID); err != nil {
		return err
	}
	if err = insertAudit(ctx, tx, actorID, "open_shift", before.ID, "deleted", before, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) OpenShiftSuggestions(ctx context.Context, openShiftID string) ([]EmployeeSuggestion, error) {
	rows, err := p.db.Query(ctx, `
		SELECT employee.id::text, employee.display_name,
		       COALESCE(sum(EXTRACT(epoch FROM (shift.ends_at - shift.starts_at))) / 3600, 0)::float8,
		       EXISTS (
		         SELECT 1 FROM availability_rules preferred
		         WHERE preferred.employee_id = employee.id AND preferred.availability_kind = 'preferred'
		           AND preferred.day_of_week = EXTRACT(ISODOW FROM open_shift.shift_date)::int
		           AND (preferred.effective_from IS NULL OR preferred.effective_from <= open_shift.shift_date)
		           AND (preferred.effective_until IS NULL OR preferred.effective_until >= open_shift.shift_date)
		           AND preferred.starts_at <= open_shift.starts_at AND preferred.ends_at >= open_shift.ends_at
		       )
		FROM open_shifts open_shift
		JOIN users employee ON employee.role = 'worker' AND employee.employee_status = 'active' AND employee.schedule_eligible = TRUE
		LEFT JOIN shifts shift ON shift.schedule_id = open_shift.schedule_id AND shift.employee_id = employee.id
		WHERE open_shift.id = $1::uuid
		  AND NOT EXISTS (
		    SELECT 1 FROM shifts conflict
		    WHERE conflict.schedule_id = open_shift.schedule_id AND conflict.employee_id = employee.id
		      AND conflict.shift_date = open_shift.shift_date
		      AND conflict.starts_at < open_shift.ends_at AND conflict.ends_at > open_shift.starts_at
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM leave_requests leave
		    WHERE leave.employee_id = employee.id AND leave.status = 'approved'
		      AND open_shift.shift_date BETWEEN leave.starts_on AND leave.ends_on
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM availability_rules unavailable
		    WHERE unavailable.employee_id = employee.id AND unavailable.availability_kind = 'unavailable'
		      AND unavailable.day_of_week = EXTRACT(ISODOW FROM open_shift.shift_date)::int
		      AND (unavailable.effective_from IS NULL OR unavailable.effective_from <= open_shift.shift_date)
		      AND (unavailable.effective_until IS NULL OR unavailable.effective_until >= open_shift.shift_date)
		      AND unavailable.starts_at < open_shift.ends_at AND unavailable.ends_at > open_shift.starts_at
		  )
		GROUP BY open_shift.id, employee.id, employee.display_name
		ORDER BY 4 DESC, 3, employee.display_name`, openShiftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	suggestions := []EmployeeSuggestion{}
	for rows.Next() {
		var suggestion EmployeeSuggestion
		if err := rows.Scan(&suggestion.EmployeeID, &suggestion.EmployeeName, &suggestion.WeeklyHours, &suggestion.Preferred); err != nil {
			return nil, err
		}
		suggestions = append(suggestions, suggestion)
	}
	return suggestions, rows.Err()
}

func (p *Postgres) AssignOpenShift(ctx context.Context, openShiftID, employeeID, actorID string) (Shift, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return Shift{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	shift, err := assignOpenShiftTx(ctx, tx, openShiftID, employeeID, actorID)
	if err != nil {
		return Shift{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Shift{}, err
	}
	return shift, nil
}

func assignOpenShiftTx(ctx context.Context, tx pgx.Tx, openShiftID, employeeID, actorID string) (Shift, error) {
	var scheduleID string
	var version, required, assigned int
	var date, start, end, note, status string
	err := tx.QueryRow(ctx, `
		SELECT schedule.id::text, schedule.version, to_char(open_shift.shift_date, 'YYYY-MM-DD'),
		       to_char(open_shift.starts_at, 'HH24:MI'), to_char(open_shift.ends_at, 'HH24:MI'),
		       open_shift.notes, open_shift.required_headcount, open_shift.status,
		       (SELECT count(*)::int FROM shifts WHERE open_shift_id = open_shift.id)
		FROM open_shifts open_shift JOIN schedules schedule ON schedule.id = open_shift.schedule_id
		WHERE open_shift.id = $1::uuid FOR UPDATE OF schedule, open_shift`, openShiftID,
	).Scan(&scheduleID, &version, &date, &start, &end, &note, &required, &status, &assigned)
	if errors.Is(err, pgx.ErrNoRows) {
		return Shift{}, ErrNotFound
	}
	if err != nil {
		return Shift{}, err
	}
	if status != "open" || assigned >= required {
		return Shift{}, fmt.Errorf("%w: open shift has no remaining coverage gap", ErrConflict)
	}
	input := ShiftInput{EmployeeID: employeeID, Date: date, Start: start, End: end, Note: note}
	conflicts, err := evaluateShiftConflicts(ctx, tx, scheduleID, input, []string{}, "", true)
	if err != nil {
		return Shift{}, err
	}
	if err = enforceShiftConflicts(conflicts, nil); err != nil {
		return Shift{}, err
	}
	var shift Shift
	err = tx.QueryRow(ctx, `
		INSERT INTO shifts (schedule_id, employee_id, shift_date, starts_at, ends_at, notes, change_version, open_shift_id)
		VALUES ($1::uuid, $2::uuid, $3::date, $4::time, $5::time, $6, $7, $8::uuid)
		RETURNING id::text, employee_id::text, to_char(shift_date, 'YYYY-MM-DD'),
		          to_char(starts_at, 'HH24:MI'), to_char(ends_at, 'HH24:MI'), notes`,
		scheduleID, employeeID, date, start, end, note, version+1, openShiftID,
	).Scan(&shift.ID, &shift.EmployeeID, &shift.Date, &shift.Start, &shift.End, &shift.Note)
	if err != nil {
		return Shift{}, err
	}
	shift.Changed = true
	if err = recordShiftConflicts(ctx, tx, scheduleID, shift.ID, actorID, conflicts, nil); err != nil {
		return Shift{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE schedules SET status = 'draft', updated_at = now() WHERE id = $1::uuid`, scheduleID); err != nil {
		return Shift{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "shift", shift.ID, "assigned_open_shift", nil, shift); err != nil {
		return Shift{}, err
	}
	return shift, nil
}

func (p *Postgres) CreateOpenClaim(ctx context.Context, openShiftID string, user User) (ShiftRequest, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return ShiftRequest{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = ensureSchedulableEmployee(ctx, tx, user.ID); err != nil {
		return ShiftRequest{}, err
	}
	var scheduleID, date, start, end, note string
	var required, assigned int
	err = tx.QueryRow(ctx, `
		SELECT open_shift.schedule_id::text, to_char(open_shift.shift_date, 'YYYY-MM-DD'),
		       to_char(open_shift.starts_at, 'HH24:MI'), to_char(open_shift.ends_at, 'HH24:MI'),
		       open_shift.notes, open_shift.required_headcount, count(shift.source_shift_id)::int
		FROM published_open_shifts open_shift
		LEFT JOIN published_shifts shift ON shift.open_shift_id = open_shift.source_open_shift_id
		WHERE open_shift.source_open_shift_id = $1::uuid AND open_shift.status = 'open'
		GROUP BY open_shift.schedule_id, open_shift.source_open_shift_id, open_shift.shift_date,
		         open_shift.starts_at, open_shift.ends_at, open_shift.notes, open_shift.required_headcount`, openShiftID,
	).Scan(&scheduleID, &date, &start, &end, &note, &required, &assigned)
	if errors.Is(err, pgx.ErrNoRows) {
		return ShiftRequest{}, ErrNotFound
	}
	if err != nil {
		return ShiftRequest{}, err
	}
	if assigned >= required {
		return ShiftRequest{}, fmt.Errorf("%w: open shift is already filled", ErrConflict)
	}
	if err = ensureNoApprovedLeave(ctx, tx, user.ID, date); err != nil {
		return ShiftRequest{}, err
	}
	var overlap bool
	if err = tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM published_shifts
		  WHERE schedule_id = $1::uuid AND employee_id = $2::uuid AND shift_date = $3::date
		    AND starts_at < $5::time AND ends_at > $4::time
		)`, scheduleID, user.ID, date, start, end).Scan(&overlap); err != nil {
		return ShiftRequest{}, err
	}
	if overlap {
		return ShiftRequest{}, fmt.Errorf("%w: this open shift overlaps an assigned shift", ErrConflict)
	}
	var request ShiftRequest
	err = tx.QueryRow(ctx, `
		INSERT INTO shift_requests (
		  request_type, requested_by_employee_id, source_shift_id, source_employee_id,
		  source_date, source_starts_at, source_ends_at, source_note, open_shift_id, expires_at
		) VALUES ('open_claim', $1::uuid, $2::uuid, $1::uuid, $3::date, $4::time, $5::time, $6, $2::uuid, now() + interval '72 hours')
		RETURNING id::text, status,
		          to_char(expires_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS'),
		          to_char(created_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')`,
		user.ID, openShiftID, date, start, end, note,
	).Scan(&request.ID, &request.Status, &request.ExpiresAt, &request.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ShiftRequest{}, fmt.Errorf("%w: you already have a pending claim for this open shift", ErrConflict)
		}
		return ShiftRequest{}, err
	}
	request.RequestType = "open_claim"
	request.RequestedByEmployeeID = user.ID
	request.RequestedByName = user.DisplayName
	request.SourceShiftID = openShiftID
	request.SourceEmployeeID = user.ID
	request.SourceDate, request.SourceStart, request.SourceEnd, request.SourceNote = date, start, end, note
	request.OpenShiftID = openShiftID
	if err = insertAudit(ctx, tx, user.ID, "shift_request", request.ID, "created", nil, request); err != nil {
		return ShiftRequest{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ShiftRequest{}, err
	}
	return request, nil
}

func dateWithinWeek(value, weekStart string) bool {
	week, weekErr := time.Parse("2006-01-02", weekStart)
	date, dateErr := time.Parse("2006-01-02", value)
	return weekErr == nil && dateErr == nil && !date.Before(week) && !date.After(week.AddDate(0, 0, 6))
}
