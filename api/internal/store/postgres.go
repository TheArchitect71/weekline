package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Postgres struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Postgres {
	return &Postgres{db: db}
}

func (p *Postgres) UserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := p.db.QueryRow(ctx, `
		SELECT id::text, email, display_name, role, password_hash
		FROM users
		WHERE lower(email) = lower($1) AND is_active = TRUE`, strings.TrimSpace(email),
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return user, err
}

func (p *Postgres) CreateSession(ctx context.Context, rawToken string, userID string, expiresAt time.Time) error {
	hash := sha256.Sum256([]byte(rawToken))
	_, err := p.db.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`, hash[:], userID, expiresAt,
	)
	return err
}

func (p *Postgres) UserBySession(ctx context.Context, rawToken string) (User, error) {
	hash := sha256.Sum256([]byte(rawToken))
	var user User
	err := p.db.QueryRow(ctx, `
		SELECT u.id::text, u.email, u.display_name, u.role
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now() AND u.is_active = TRUE`, hash[:],
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return user, err
}

func (p *Postgres) DeleteSession(ctx context.Context, rawToken string) error {
	hash := sha256.Sum256([]byte(rawToken))
	_, err := p.db.Exec(ctx, "DELETE FROM sessions WHERE token_hash = $1", hash[:])
	return err
}

func (p *Postgres) DeleteExpiredSessions(ctx context.Context) error {
	_, err := p.db.Exec(ctx, "DELETE FROM sessions WHERE expires_at <= now()")
	return err
}

func (p *Postgres) ManagerSchedule(ctx context.Context, weekStart string) (Schedule, error) {
	if _, err := p.db.Exec(ctx, `
		INSERT INTO schedules (week_start) VALUES ($1::date)
		ON CONFLICT (week_start) DO NOTHING`, weekStart); err != nil {
		return Schedule{}, err
	}

	var scheduleID, status string
	var version int
	err := p.db.QueryRow(ctx, `
		SELECT id::text, status, version FROM schedules WHERE week_start = $1::date`, weekStart,
	).Scan(&scheduleID, &status, &version)
	if err != nil {
		return Schedule{}, err
	}

	employees, err := p.listEmployees(ctx, nil)
	if err != nil {
		return Schedule{}, err
	}
	shifts, err := p.queryShifts(ctx, `
		SELECT id::text, employee_id::text, to_char(shift_date, 'YYYY-MM-DD'),
		       to_char(starts_at, 'HH24:MI'), to_char(ends_at, 'HH24:MI'), notes,
		       change_version > $2
		FROM shifts WHERE schedule_id = $1::uuid ORDER BY shift_date, starts_at, employee_id`, scheduleID, version)
	if err != nil {
		return Schedule{}, err
	}
	leave, err := p.LeaveBlocks(ctx, weekStart, "")
	if err != nil {
		return Schedule{}, err
	}
	holidays, err := p.ListPublicHolidays(ctx, weekStart, endOfWeek(weekStart))
	if err != nil {
		return Schedule{}, err
	}
	openShifts, err := p.ManagerOpenShifts(ctx, scheduleID)
	if err != nil {
		return Schedule{}, err
	}
	conflicts, err := scheduleConflicts(ctx, p.db, scheduleID)
	if err != nil {
		return Schedule{}, err
	}
	availability, err := p.AvailabilityForWeek(ctx, weekStart, "")
	if err != nil {
		return Schedule{}, err
	}
	return Schedule{WeekStart: weekStart, Status: status, Version: version, Employees: employees, Shifts: shifts, Leave: leave, Holidays: holidays, OpenShifts: openShifts, Conflicts: conflicts, Availability: availability}, nil
}

func (p *Postgres) WorkerSchedule(ctx context.Context, weekStart string, userID string) (Schedule, error) {
	var scheduleID, status string
	var version, lastSeen int
	err := p.db.QueryRow(ctx, `
		SELECT s.id::text, s.status, s.version,
		       COALESCE((SELECT last_seen_version FROM schedule_views WHERE schedule_id = s.id AND user_id = $2::uuid), 0)
		FROM schedules s WHERE s.week_start = $1::date AND s.version > 0`, weekStart, userID,
	).Scan(&scheduleID, &status, &version, &lastSeen)
	if errors.Is(err, pgx.ErrNoRows) {
		return Schedule{}, ErrNotFound
	}
	if err != nil {
		return Schedule{}, err
	}

	shifts, err := p.queryShifts(ctx, `
		SELECT ps.source_shift_id::text, ps.employee_id::text, to_char(ps.shift_date, 'YYYY-MM-DD'),
		       to_char(ps.starts_at, 'HH24:MI'), to_char(ps.ends_at, 'HH24:MI'), ps.notes,
		       ps.change_version > $3
		FROM published_shifts ps
		WHERE ps.schedule_id = $1::uuid
		  AND (
		    ps.employee_id = $2::uuid OR EXISTS (
		      SELECT 1 FROM published_shifts own
		      WHERE own.schedule_id = ps.schedule_id AND own.employee_id = $2::uuid
		        AND own.shift_date = ps.shift_date AND own.starts_at = ps.starts_at AND own.ends_at = ps.ends_at
		    )
		  )
		ORDER BY ps.shift_date, ps.starts_at, ps.employee_id`, scheduleID, userID, lastSeen)
	if err != nil {
		return Schedule{}, err
	}

	ids := make([]string, 0, len(shifts)+1)
	ids = append(ids, userID)
	for _, shift := range shifts {
		ids = append(ids, shift.EmployeeID)
	}
	employees, err := p.listEmployees(ctx, ids)
	if err != nil {
		return Schedule{}, err
	}
	leave, err := p.LeaveBlocks(ctx, weekStart, userID)
	if err != nil {
		return Schedule{}, err
	}
	holidays, err := p.ListPublicHolidays(ctx, weekStart, endOfWeek(weekStart))
	if err != nil {
		return Schedule{}, err
	}
	openShifts, err := p.WorkerOpenShifts(ctx, scheduleID, userID)
	if err != nil {
		return Schedule{}, err
	}
	availability, err := p.AvailabilityForWeek(ctx, weekStart, userID)
	if err != nil {
		return Schedule{}, err
	}

	_, err = p.db.Exec(ctx, `
		INSERT INTO schedule_views (schedule_id, user_id, last_seen_version, viewed_at)
		VALUES ($1::uuid, $2::uuid, $3, now())
		ON CONFLICT (schedule_id, user_id) DO UPDATE
		SET last_seen_version = EXCLUDED.last_seen_version, viewed_at = now()`, scheduleID, userID, version)
	if err != nil {
		return Schedule{}, err
	}

	return Schedule{WeekStart: weekStart, Status: status, Version: version, Employees: employees, Shifts: shifts, Leave: leave, Holidays: holidays, OpenShifts: openShifts, Availability: availability}, nil
}

func (p *Postgres) CreateShift(ctx context.Context, weekStart string, actorID string, input ShiftInput) (Shift, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return Shift{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var scheduleID string
	var version int
	err = tx.QueryRow(ctx, `
		SELECT id::text, version FROM schedules WHERE week_start = $1::date FOR UPDATE`, weekStart,
	).Scan(&scheduleID, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Shift{}, ErrNotFound
	}
	if err != nil {
		return Shift{}, err
	}
	conflicts, err := evaluateShiftConflicts(ctx, tx, scheduleID, input, []string{}, "", true)
	if err != nil {
		return Shift{}, err
	}
	if err = enforceShiftConflicts(conflicts, input.OverrideConflictCodes); err != nil {
		return Shift{}, err
	}

	var shift Shift
	err = tx.QueryRow(ctx, `
		INSERT INTO shifts (schedule_id, employee_id, shift_date, starts_at, ends_at, notes, change_version)
		VALUES ($1::uuid, $2::uuid, $3::date, $4::time, $5::time, $6, $7)
		RETURNING id::text, employee_id::text, to_char(shift_date, 'YYYY-MM-DD'),
		          to_char(starts_at, 'HH24:MI'), to_char(ends_at, 'HH24:MI'), notes`,
		scheduleID, input.EmployeeID, input.Date, input.Start, input.End, input.Note, version+1,
	).Scan(&shift.ID, &shift.EmployeeID, &shift.Date, &shift.Start, &shift.End, &shift.Note)
	if err != nil {
		return Shift{}, err
	}
	shift.Changed = true
	if err = recordShiftConflicts(ctx, tx, scheduleID, shift.ID, actorID, conflicts, input.OverrideConflictCodes); err != nil {
		return Shift{}, err
	}
	if _, err = tx.Exec(ctx, "UPDATE schedules SET status = 'draft', updated_at = now() WHERE id = $1::uuid", scheduleID); err != nil {
		return Shift{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "shift", shift.ID, "created", nil, shift); err != nil {
		return Shift{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Shift{}, err
	}
	return shift, nil
}

func (p *Postgres) UpdateShift(ctx context.Context, shiftID string, actorID string, input ShiftInput) (Shift, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return Shift{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var scheduleID, weekStart string
	var version int
	var before Shift
	err = tx.QueryRow(ctx, `
		SELECT s.id::text, to_char(s.week_start, 'YYYY-MM-DD'), s.version, sh.id::text, sh.employee_id::text,
		       to_char(sh.shift_date, 'YYYY-MM-DD'), to_char(sh.starts_at, 'HH24:MI'),
		       to_char(sh.ends_at, 'HH24:MI'), sh.notes
		FROM shifts sh JOIN schedules s ON s.id = sh.schedule_id
		WHERE sh.id = $1::uuid FOR UPDATE OF s, sh`, shiftID,
	).Scan(&scheduleID, &weekStart, &version, &before.ID, &before.EmployeeID, &before.Date, &before.Start, &before.End, &before.Note)
	if errors.Is(err, pgx.ErrNoRows) {
		return Shift{}, ErrNotFound
	}
	if err != nil {
		return Shift{}, err
	}
	week, _ := time.Parse("2006-01-02", weekStart)
	date, dateErr := time.Parse("2006-01-02", input.Date)
	if dateErr != nil || date.Before(week) || date.After(week.AddDate(0, 0, 6)) {
		return Shift{}, fmt.Errorf("%w: shift date must fall within the schedule week", ErrInvalid)
	}
	conflicts, err := evaluateShiftConflicts(ctx, tx, scheduleID, input, []string{shiftID}, shiftID, true)
	if err != nil {
		return Shift{}, err
	}
	if err = enforceShiftConflicts(conflicts, input.OverrideConflictCodes); err != nil {
		return Shift{}, err
	}

	var shift Shift
	err = tx.QueryRow(ctx, `
		UPDATE shifts SET employee_id = $2::uuid, shift_date = $3::date, starts_at = $4::time,
		       ends_at = $5::time, notes = $6, change_version = $7, updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, employee_id::text, to_char(shift_date, 'YYYY-MM-DD'),
		          to_char(starts_at, 'HH24:MI'), to_char(ends_at, 'HH24:MI'), notes`,
		shiftID, input.EmployeeID, input.Date, input.Start, input.End, input.Note, version+1,
	).Scan(&shift.ID, &shift.EmployeeID, &shift.Date, &shift.Start, &shift.End, &shift.Note)
	if err != nil {
		return Shift{}, err
	}
	shift.Changed = true
	if err = recordShiftConflicts(ctx, tx, scheduleID, shift.ID, actorID, conflicts, input.OverrideConflictCodes); err != nil {
		return Shift{}, err
	}
	if _, err = tx.Exec(ctx, "UPDATE schedules SET status = 'draft', updated_at = now() WHERE id = $1::uuid", scheduleID); err != nil {
		return Shift{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "shift", shift.ID, "updated", before, shift); err != nil {
		return Shift{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Shift{}, err
	}
	return shift, nil
}

func (p *Postgres) DeleteShift(ctx context.Context, shiftID string, actorID string) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var scheduleID string
	var before Shift
	err = tx.QueryRow(ctx, `
		SELECT s.id::text, sh.id::text, sh.employee_id::text,
		       to_char(sh.shift_date, 'YYYY-MM-DD'), to_char(sh.starts_at, 'HH24:MI'),
		       to_char(sh.ends_at, 'HH24:MI'), sh.notes
		FROM shifts sh JOIN schedules s ON s.id = sh.schedule_id
		WHERE sh.id = $1::uuid FOR UPDATE OF s, sh`, shiftID,
	).Scan(&scheduleID, &before.ID, &before.EmployeeID, &before.Date, &before.Start, &before.End, &before.Note)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM shifts WHERE id = $1::uuid", shiftID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE schedules SET status = 'draft', updated_at = now() WHERE id = $1::uuid", scheduleID); err != nil {
		return err
	}
	if err = insertAudit(ctx, tx, actorID, "shift", before.ID, "deleted", before, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) SwapOptions(ctx context.Context, requesterID, sourceShiftID string) ([]SwapOption, error) {
	var scheduleID string
	err := p.db.QueryRow(ctx, `
		SELECT schedule_id::text
		FROM published_shifts
		WHERE source_shift_id = $1::uuid AND employee_id = $2::uuid`, sourceShiftID, requesterID,
	).Scan(&scheduleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := p.db.Query(ctx, `
		SELECT target.source_shift_id::text, target.employee_id::text, employee.display_name,
		       to_char(target.shift_date, 'YYYY-MM-DD'), to_char(target.starts_at, 'HH24:MI'),
		       to_char(target.ends_at, 'HH24:MI'), target.notes
		FROM published_shifts target
		JOIN published_shifts source
		  ON source.schedule_id = target.schedule_id AND source.source_shift_id = $2::uuid
		JOIN users employee ON employee.id = target.employee_id
		WHERE target.schedule_id = $1::uuid
		  AND target.source_shift_id <> source.source_shift_id
		  AND target.employee_id <> $3::uuid
		  AND employee.employee_status = 'active' AND employee.schedule_eligible = TRUE
		  AND NOT EXISTS (
		    SELECT 1 FROM published_shifts own_conflict
		    WHERE own_conflict.schedule_id = target.schedule_id
		      AND own_conflict.employee_id = $3::uuid
		      AND own_conflict.source_shift_id NOT IN (source.source_shift_id, target.source_shift_id)
		      AND own_conflict.shift_date = target.shift_date
		      AND own_conflict.starts_at < target.ends_at AND own_conflict.ends_at > target.starts_at
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM published_shifts target_conflict
		    WHERE target_conflict.schedule_id = target.schedule_id
		      AND target_conflict.employee_id = target.employee_id
		      AND target_conflict.source_shift_id NOT IN (source.source_shift_id, target.source_shift_id)
		      AND target_conflict.shift_date = source.shift_date
		      AND target_conflict.starts_at < source.ends_at AND target_conflict.ends_at > source.starts_at
		  )
		ORDER BY target.shift_date, target.starts_at, employee.display_name`, scheduleID, sourceShiftID, requesterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	options := []SwapOption{}
	for rows.Next() {
		var option SwapOption
		if err := rows.Scan(&option.ShiftID, &option.EmployeeID, &option.EmployeeName, &option.Date, &option.Start, &option.End, &option.Note); err != nil {
			return nil, err
		}
		options = append(options, option)
	}
	return options, rows.Err()
}

func (p *Postgres) CreateSwapRequest(ctx context.Context, requesterID, actorID string, input SwapRequestInput) (ShiftRequest, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return ShiftRequest{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var request ShiftRequest
	err = tx.QueryRow(ctx, `
		SELECT source.employee_id::text, requester.display_name,
		       to_char(source.shift_date, 'YYYY-MM-DD'), to_char(source.starts_at, 'HH24:MI'),
		       to_char(source.ends_at, 'HH24:MI'), source.notes,
		       target.employee_id::text, target_employee.display_name,
		       to_char(target.shift_date, 'YYYY-MM-DD'), to_char(target.starts_at, 'HH24:MI'),
		       to_char(target.ends_at, 'HH24:MI'), target.notes
		FROM published_shifts source
		JOIN published_shifts target ON target.schedule_id = source.schedule_id
		JOIN users requester ON requester.id = source.employee_id
		JOIN users target_employee ON target_employee.id = target.employee_id
		WHERE source.source_shift_id = $1::uuid AND target.source_shift_id = $2::uuid
		  AND source.employee_id = $3::uuid AND target.employee_id <> $3::uuid
		  AND target_employee.employee_status = 'active' AND target_employee.schedule_eligible = TRUE
		FOR SHARE OF source, target`, input.SourceShiftID, input.TargetShiftID, requesterID,
	).Scan(&request.SourceEmployeeID, &request.RequestedByName,
		&request.SourceDate, &request.SourceStart, &request.SourceEnd, &request.SourceNote,
		&request.TargetEmployeeID, &request.TargetEmployeeName,
		&request.TargetDate, &request.TargetStart, &request.TargetEnd, &request.TargetNote)
	if errors.Is(err, pgx.ErrNoRows) {
		return ShiftRequest{}, fmt.Errorf("%w: source and target must be published shifts in the same week", ErrInvalid)
	}
	if err != nil {
		return ShiftRequest{}, err
	}

	request.RequestType = "swap"
	request.RequestedByEmployeeID = requesterID
	request.SourceShiftID = input.SourceShiftID
	request.TargetShiftID = input.TargetShiftID
	err = tx.QueryRow(ctx, `
		INSERT INTO shift_requests (
		  request_type, requested_by_employee_id, source_shift_id, source_employee_id,
		  source_date, source_starts_at, source_ends_at, source_note,
		  target_shift_id, target_employee_id, target_date, target_starts_at, target_ends_at, target_note,
		  expires_at
		) VALUES (
		  'swap', $1::uuid, $2::uuid, $3::uuid, $4::date, $5::time, $6::time, $7,
		  $8::uuid, $9::uuid, $10::date, $11::time, $12::time, $13, now() + interval '72 hours'
		)
		RETURNING id::text, status,
		          to_char(expires_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS'),
		          to_char(created_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')`,
		requesterID, input.SourceShiftID, request.SourceEmployeeID,
		request.SourceDate, request.SourceStart, request.SourceEnd, request.SourceNote,
		input.TargetShiftID, request.TargetEmployeeID,
		request.TargetDate, request.TargetStart, request.TargetEnd, request.TargetNote,
	).Scan(&request.ID, &request.Status, &request.ExpiresAt, &request.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ShiftRequest{}, fmt.Errorf("%w: this shift already has a pending request", ErrConflict)
		}
		return ShiftRequest{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "shift_request", request.ID, "created", nil, request); err != nil {
		return ShiftRequest{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ShiftRequest{}, err
	}
	return request, nil
}

func (p *Postgres) ListShiftRequests(ctx context.Context, user User) ([]ShiftRequest, error) {
	if _, err := p.db.Exec(ctx, `
		UPDATE shift_requests SET status = 'expired', resolved_at = now(), updated_at = now()
		WHERE status = 'pending' AND expires_at <= now()`); err != nil {
		return nil, err
	}
	query := `
		SELECT request.id::text, request.request_type, request.requested_by_employee_id::text,
		       requester.display_name, request.source_shift_id::text, request.source_employee_id::text,
		       to_char(request.source_date, 'YYYY-MM-DD'), to_char(request.source_starts_at, 'HH24:MI'),
		       to_char(request.source_ends_at, 'HH24:MI'), request.source_note,
		       COALESCE(request.target_shift_id::text, ''), COALESCE(request.target_employee_id::text, ''),
		       COALESCE(target.display_name, ''), COALESCE(to_char(request.target_date, 'YYYY-MM-DD'), ''),
		       COALESCE(to_char(request.target_starts_at, 'HH24:MI'), ''),
		       COALESCE(to_char(request.target_ends_at, 'HH24:MI'), ''), request.target_note,
		       COALESCE(request.open_shift_id::text, ''),
		       request.status,
		       to_char(request.expires_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS'),
		       COALESCE(to_char(request.resolved_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS'), ''),
		       to_char(request.created_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')
		FROM shift_requests request
		JOIN users requester ON requester.id = request.requested_by_employee_id
		LEFT JOIN users target ON target.id = request.target_employee_id`
	args := []any{}
	if user.Role != "manager" {
		query += " WHERE request.requested_by_employee_id = $1::uuid"
		args = append(args, user.ID)
	}
	query += " ORDER BY CASE request.status WHEN 'pending' THEN 0 ELSE 1 END, request.created_at DESC"
	rows, err := p.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	requests := []ShiftRequest{}
	for rows.Next() {
		var request ShiftRequest
		if err := rows.Scan(
			&request.ID, &request.RequestType, &request.RequestedByEmployeeID, &request.RequestedByName,
			&request.SourceShiftID, &request.SourceEmployeeID, &request.SourceDate, &request.SourceStart,
			&request.SourceEnd, &request.SourceNote, &request.TargetShiftID, &request.TargetEmployeeID,
			&request.TargetEmployeeName, &request.TargetDate, &request.TargetStart, &request.TargetEnd,
			&request.TargetNote, &request.OpenShiftID, &request.Status, &request.ExpiresAt, &request.ResolvedAt, &request.CreatedAt,
		); err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

func (p *Postgres) ResolveShiftRequest(ctx context.Context, requestID, actorID, decision string) (ShiftRequest, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return ShiftRequest{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	request, err := shiftRequestForUpdate(ctx, tx, requestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ShiftRequest{}, ErrNotFound
	}
	if err != nil {
		return ShiftRequest{}, err
	}
	if request.Status != "pending" {
		return ShiftRequest{}, fmt.Errorf("%w: request is no longer pending", ErrConflict)
	}
	if decision == "rejected" {
		request.Status = "rejected"
		if err = finishShiftRequest(ctx, tx, &request, actorID); err != nil {
			return ShiftRequest{}, err
		}
	} else if decision == "approved" {
		if request.RequestType == "open_claim" {
			if _, err = assignOpenShiftTx(ctx, tx, request.OpenShiftID, request.RequestedByEmployeeID, actorID); err != nil {
				return ShiftRequest{}, err
			}
		} else {
			if err = applySwap(ctx, tx, request, actorID); err != nil {
				return ShiftRequest{}, err
			}
		}
		request.Status = "approved"
		if err = finishShiftRequest(ctx, tx, &request, actorID); err != nil {
			return ShiftRequest{}, err
		}
	} else {
		return ShiftRequest{}, fmt.Errorf("%w: decision must be approved or rejected", ErrInvalid)
	}
	if err = tx.Commit(ctx); err != nil {
		return ShiftRequest{}, err
	}
	return request, nil
}

func (p *Postgres) CancelShiftRequest(ctx context.Context, requestID, requesterID string) (ShiftRequest, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return ShiftRequest{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	request, err := shiftRequestForUpdate(ctx, tx, requestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ShiftRequest{}, ErrNotFound
	}
	if err != nil {
		return ShiftRequest{}, err
	}
	if request.RequestedByEmployeeID != requesterID {
		return ShiftRequest{}, ErrNotFound
	}
	if request.Status != "pending" {
		return ShiftRequest{}, fmt.Errorf("%w: request is no longer pending", ErrConflict)
	}
	request.Status = "cancelled"
	if err = finishShiftRequest(ctx, tx, &request, requesterID); err != nil {
		return ShiftRequest{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ShiftRequest{}, err
	}
	return request, nil
}

func (p *Postgres) Publish(ctx context.Context, weekStart string, actorID string) (Schedule, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return Schedule{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var scheduleID, status string
	var version int
	err = tx.QueryRow(ctx, `
		SELECT id::text, status, version FROM schedules WHERE week_start = $1::date FOR UPDATE`, weekStart,
	).Scan(&scheduleID, &status, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Schedule{}, ErrNotFound
	}
	if err != nil {
		return Schedule{}, err
	}
	conflicts, err := scheduleConflicts(ctx, tx, scheduleID)
	if err != nil {
		return Schedule{}, err
	}
	if err = enforcePublishConflicts(ctx, tx, scheduleID, conflicts); err != nil {
		return Schedule{}, err
	}
	if status == "draft" || version == 0 {
		if _, err = tx.Exec(ctx, "DELETE FROM published_open_shifts WHERE schedule_id = $1::uuid", scheduleID); err != nil {
			return Schedule{}, err
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO published_open_shifts
			    (schedule_id, source_open_shift_id, shift_date, starts_at, ends_at, notes, required_headcount, status, change_version)
			SELECT schedule_id, id, shift_date, starts_at, ends_at, notes, required_headcount, status, change_version
			FROM open_shifts WHERE schedule_id = $1::uuid`, scheduleID); err != nil {
			return Schedule{}, err
		}
		if _, err = tx.Exec(ctx, "DELETE FROM published_shifts WHERE schedule_id = $1::uuid", scheduleID); err != nil {
			return Schedule{}, err
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO published_shifts
			    (schedule_id, source_shift_id, employee_id, shift_date, starts_at, ends_at, notes, change_version, open_shift_id)
			SELECT schedule_id, id, employee_id, shift_date, starts_at, ends_at, notes, change_version, open_shift_id
			FROM shifts WHERE schedule_id = $1::uuid`, scheduleID); err != nil {
			return Schedule{}, err
		}
		version++
		if _, err = tx.Exec(ctx, `
			UPDATE schedules SET status = 'published', version = $2, published_at = now(),
			published_by = $3::uuid, updated_at = now() WHERE id = $1::uuid`, scheduleID, version, actorID); err != nil {
			return Schedule{}, err
		}
		if err = insertAudit(ctx, tx, actorID, "schedule", scheduleID, "published", nil, map[string]any{"version": version, "weekStart": weekStart, "conflicts": conflicts}); err != nil {
			return Schedule{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Schedule{}, err
	}
	return p.ManagerSchedule(ctx, weekStart)
}

func (p *Postgres) ListPeople(ctx context.Context) ([]Employee, error) {
	return p.listAllEmployees(ctx)
}

func (p *Postgres) CreateEmployee(ctx context.Context, actorID string, input EmployeeInput) (Employee, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(input.InitialPassword), bcrypt.DefaultCost)
	if err != nil {
		return Employee{}, err
	}
	status := normalizedEmployeeStatus(input.Status)
	scheduleEligible := normalizedScheduleEligible(status, input.ScheduleEligible)
	isActive := status != "deactivated"
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return Employee{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var employee Employee
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name, role, is_active, employee_status, schedule_eligible, deactivated_at)
		VALUES ($1, $2, $3, 'worker', $4, $5, $6, CASE WHEN $5 = 'deactivated' THEN now() ELSE NULL END)
		RETURNING id::text, email, display_name, employee_status, schedule_eligible,
		          to_char(deactivated_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')`,
		strings.TrimSpace(input.Email), string(hash), strings.TrimSpace(input.DisplayName), isActive, status, scheduleEligible,
	).Scan(&employee.ID, &employee.Email, &employee.DisplayName, &employee.Status, &employee.ScheduleEligible, nullableString(&employee.DeactivatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return Employee{}, fmt.Errorf("%w: email is already in use", ErrConflict)
		}
		return Employee{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "employee", employee.ID, "created", nil, employee); err != nil {
		return Employee{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Employee{}, err
	}
	return employee, nil
}

func (p *Postgres) UpdateEmployee(ctx context.Context, employeeID string, actorID string, input EmployeeInput) (Employee, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return Employee{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	before, err := employeeByID(ctx, tx, employeeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Employee{}, ErrNotFound
	}
	if err != nil {
		return Employee{}, err
	}
	status := normalizedEmployeeStatus(input.Status)
	scheduleEligible := normalizedScheduleEligible(status, input.ScheduleEligible)
	isActive := status != "deactivated"

	var employee Employee
	err = tx.QueryRow(ctx, `
		UPDATE users
		SET email = $2,
		    display_name = $3,
		    is_active = $4,
		    employee_status = $5,
		    schedule_eligible = $6,
		    deactivated_at = CASE
		        WHEN $5 = 'deactivated' THEN COALESCE(deactivated_at, now())
		        ELSE NULL
		    END,
		    updated_at = now()
		WHERE id = $1::uuid AND role = 'worker'
		RETURNING id::text, email, display_name, employee_status, schedule_eligible,
		          to_char(deactivated_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')`,
		employeeID, strings.TrimSpace(input.Email), strings.TrimSpace(input.DisplayName), isActive, status, scheduleEligible,
	).Scan(&employee.ID, &employee.Email, &employee.DisplayName, &employee.Status, &employee.ScheduleEligible, nullableString(&employee.DeactivatedAt))
	if errors.Is(err, pgx.ErrNoRows) {
		return Employee{}, ErrNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return Employee{}, fmt.Errorf("%w: email is already in use", ErrConflict)
		}
		return Employee{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "employee", employee.ID, "updated", before, employee); err != nil {
		return Employee{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Employee{}, err
	}
	return employee, nil
}

func (p *Postgres) DeactivateEmployee(ctx context.Context, employeeID string, actorID string) (Employee, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return Employee{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	before, err := employeeByID(ctx, tx, employeeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Employee{}, ErrNotFound
	}
	if err != nil {
		return Employee{}, err
	}

	var employee Employee
	err = tx.QueryRow(ctx, `
		UPDATE users
		SET is_active = FALSE,
		    employee_status = 'deactivated',
		    schedule_eligible = FALSE,
		    deactivated_at = COALESCE(deactivated_at, now()),
		    updated_at = now()
		WHERE id = $1::uuid AND role = 'worker'
		RETURNING id::text, email, display_name, employee_status, schedule_eligible,
		          to_char(deactivated_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')`, employeeID,
	).Scan(&employee.ID, &employee.Email, &employee.DisplayName, &employee.Status, &employee.ScheduleEligible, nullableString(&employee.DeactivatedAt))
	if errors.Is(err, pgx.ErrNoRows) {
		return Employee{}, ErrNotFound
	}
	if err != nil {
		return Employee{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "employee", employee.ID, "deactivated", before, employee); err != nil {
		return Employee{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Employee{}, err
	}
	return employee, nil
}

func (p *Postgres) ListAudit(ctx context.Context, limit int) ([]AuditEvent, error) {
	rows, err := p.db.Query(ctx, `
		SELECT ae.id, COALESCE(u.display_name, 'System'), ae.entity_type, ae.action,
		       to_char(ae.occurred_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS'),
		       CASE
		         WHEN ae.entity_type = 'schedule' THEN 'Published the weekly schedule'
		         WHEN ae.entity_type = 'shift' AND ae.action = 'created' THEN 'Created a shift'
		         WHEN ae.entity_type = 'shift' AND ae.action = 'updated' THEN 'Updated a shift'
		         WHEN ae.entity_type = 'shift' AND ae.action = 'deleted' THEN 'Deleted a shift'
		         WHEN ae.entity_type = 'shift' AND ae.action = 'swapped' THEN 'Swapped a shift assignment'
		         WHEN ae.entity_type = 'shift_request' AND ae.action = 'created' THEN 'Requested a shift swap'
		         WHEN ae.entity_type = 'shift_request' AND ae.action = 'approved' THEN 'Approved a shift swap'
		         WHEN ae.entity_type = 'shift_request' AND ae.action = 'rejected' THEN 'Rejected a shift swap'
		         WHEN ae.entity_type = 'shift_request' AND ae.action = 'cancelled' THEN 'Cancelled a shift swap'
		         WHEN ae.entity_type = 'leave_request' AND ae.action = 'created' THEN 'Requested time off'
		         WHEN ae.entity_type = 'leave_request' AND ae.action = 'approved' THEN 'Approved time off'
		         WHEN ae.entity_type = 'leave_request' AND ae.action = 'rejected' THEN 'Rejected time off'
		         WHEN ae.entity_type = 'leave_request' AND ae.action = 'cancelled' THEN 'Cancelled time off'
		         WHEN ae.entity_type = 'employee' AND ae.action = 'leave_balance_updated' THEN 'Updated a leave balance'
		         WHEN ae.entity_type = 'public_holiday' AND ae.action = 'created' THEN 'Added a public holiday'
		         WHEN ae.entity_type = 'public_holiday' AND ae.action = 'deleted' THEN 'Removed a public holiday'
		         WHEN ae.entity_type = 'open_shift' AND ae.action = 'created' THEN 'Created an open shift'
		         WHEN ae.entity_type = 'open_shift' AND ae.action = 'updated' THEN 'Updated an open shift'
		         WHEN ae.entity_type = 'open_shift' AND ae.action = 'deleted' THEN 'Deleted an open shift'
		         WHEN ae.entity_type = 'shift' AND ae.action = 'assigned_open_shift' THEN 'Assigned an open shift'
		         WHEN ae.entity_type = 'employee' AND ae.action = 'created' THEN 'Added employee'
		         WHEN ae.entity_type = 'employee' AND ae.action = 'updated' THEN 'Updated employee'
		         WHEN ae.entity_type = 'employee' AND ae.action = 'deactivated' THEN 'Deactivated employee'
		         WHEN ae.entity_type = 'availability' AND ae.action = 'created' THEN 'Added recurring availability'
		         WHEN ae.entity_type = 'availability' AND ae.action = 'updated' THEN 'Updated recurring availability'
		         WHEN ae.entity_type = 'availability' AND ae.action = 'deleted' THEN 'Deleted recurring availability'
		         WHEN ae.entity_type = 'attendance_station' AND ae.action = 'created' THEN 'Created an attendance station'
		         WHEN ae.entity_type = 'attendance_station' AND ae.action = 'updated' THEN 'Updated an attendance station'
		         WHEN ae.entity_type = 'attendance_station' AND ae.action = 'token_rotated' THEN 'Rotated an attendance station token'
		         WHEN ae.entity_type = 'attendance_correction' AND ae.action = 'created' THEN 'Requested an attendance correction'
		         WHEN ae.entity_type = 'attendance_correction' AND ae.action = 'approved' THEN 'Approved an attendance correction'
		         WHEN ae.entity_type = 'attendance_correction' AND ae.action = 'rejected' THEN 'Rejected an attendance correction'
		         WHEN ae.entity_type = 'attendance_correction' AND ae.action = 'cancelled' THEN 'Cancelled an attendance correction'
		         ELSE initcap(ae.action) || ' ' || ae.entity_type
		       END
		FROM audit_events ae LEFT JOIN users u ON u.id = ae.actor_id
		ORDER BY ae.occurred_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []AuditEvent{}
	for rows.Next() {
		var event AuditEvent
		if err := rows.Scan(&event.ID, &event.ActorName, &event.EntityType, &event.Action, &event.OccurredAt, &event.Summary); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (p *Postgres) SeedDemo(ctx context.Context, password string) error {
	var count int
	if err := p.db.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	people := []struct{ email, name, role string }{
		{"manager@weekline.local", "Ana Zoila", "manager"},
		{"elena@weekline.local", "Elena Ruiz", "worker"},
		{"marcus@weekline.local", "Marcus Hill", "worker"},
		{"jordan@weekline.local", "Jordan Lee", "worker"},
		{"priya@weekline.local", "Priya Shah", "worker"},
		{"sam@weekline.local", "Sam Carter", "worker"},
	}
	ids := map[string]string{}
	for _, person := range people {
		var id string
		if err := tx.QueryRow(ctx, `
			INSERT INTO users (email, password_hash, display_name, role)
			VALUES ($1, $2, $3, $4) RETURNING id::text`, person.email, string(hash), person.name, person.role,
		).Scan(&id); err != nil {
			return err
		}
		ids[person.email] = id
	}

	var scheduleID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO schedules (week_start, status, version, published_at, published_by)
		VALUES ('2026-07-06', 'published', 1, now(), $1::uuid) RETURNING id::text`, ids["manager@weekline.local"],
	).Scan(&scheduleID); err != nil {
		return err
	}

	type demoShift struct {
		email            string
		day              int
		start, end, note string
	}
	demo := []demoShift{
		{"elena@weekline.local", 0, "08:00", "16:00", "Front desk"}, {"elena@weekline.local", 1, "08:00", "16:00", "Front desk"},
		{"elena@weekline.local", 2, "08:00", "16:00", "Front desk"}, {"elena@weekline.local", 3, "08:00", "16:00", "Front desk"},
		{"elena@weekline.local", 4, "08:00", "16:00", "Front desk"}, {"elena@weekline.local", 5, "09:00", "17:00", "Front desk"},
		{"marcus@weekline.local", 0, "10:00", "18:00", "Stock"}, {"marcus@weekline.local", 1, "10:00", "18:00", "Stock"},
		{"marcus@weekline.local", 3, "12:00", "20:00", "Stock"}, {"marcus@weekline.local", 4, "10:00", "18:00", "Stock"},
		{"marcus@weekline.local", 5, "10:00", "18:00", "Stock"},
		{"jordan@weekline.local", 1, "12:00", "20:00", "Sales floor"}, {"jordan@weekline.local", 2, "12:00", "20:00", "Sales floor"},
		{"jordan@weekline.local", 3, "12:00", "20:00", "Sales floor"}, {"jordan@weekline.local", 5, "12:00", "20:00", "Sales floor"},
		{"priya@weekline.local", 0, "08:00", "16:00", "Stock"}, {"priya@weekline.local", 2, "08:00", "16:00", "Stock"},
		{"priya@weekline.local", 3, "08:00", "16:00", "Stock"}, {"priya@weekline.local", 4, "08:00", "16:00", "Stock"},
		{"priya@weekline.local", 5, "09:00", "17:00", "Stock"},
		{"sam@weekline.local", 2, "16:00", "20:00", "Front desk"}, {"sam@weekline.local", 3, "16:00", "20:00", "Front desk"},
		{"sam@weekline.local", 4, "16:00", "20:00", "Front desk"},
	}
	for _, shift := range demo {
		if _, err := tx.Exec(ctx, `
			INSERT INTO shifts (schedule_id, employee_id, shift_date, starts_at, ends_at, notes, change_version)
			VALUES ($1::uuid, $2::uuid, DATE '2026-07-06' + $3::int, $4::time, $5::time, $6, 1)`,
			scheduleID, ids[shift.email], shift.day, shift.start, shift.end, shift.note); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO published_shifts
		    (schedule_id, source_shift_id, employee_id, shift_date, starts_at, ends_at, notes, change_version)
		SELECT schedule_id, id, employee_id, shift_date, starts_at, ends_at, notes, change_version
		FROM shifts WHERE schedule_id = $1::uuid`, scheduleID); err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, ids["manager@weekline.local"], "schedule", scheduleID, "published", nil, map[string]any{"version": 1, "weekStart": "2026-07-06"}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) listEmployees(ctx context.Context, ids []string) ([]Employee, error) {
	query := `SELECT id::text, email, display_name, employee_status, schedule_eligible,
	                 to_char(deactivated_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')
	          FROM users WHERE role = 'worker' AND employee_status = 'active' AND schedule_eligible = TRUE`
	args := []any{}
	if len(ids) > 0 {
		query += " AND id = ANY($1::uuid[])"
		args = append(args, ids)
	}
	query += " ORDER BY display_name"
	rows, err := p.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	employees := []Employee{}
	for rows.Next() {
		var employee Employee
		if err := rows.Scan(&employee.ID, &employee.Email, &employee.DisplayName, &employee.Status, &employee.ScheduleEligible, nullableString(&employee.DeactivatedAt)); err != nil {
			return nil, err
		}
		employees = append(employees, employee)
	}
	return employees, rows.Err()
}

func (p *Postgres) listAllEmployees(ctx context.Context) ([]Employee, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id::text, email, display_name, employee_status, schedule_eligible,
		       to_char(deactivated_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')
		FROM users
		WHERE role = 'worker'
		ORDER BY CASE employee_status WHEN 'active' THEN 0 WHEN 'inactive' THEN 1 ELSE 2 END, display_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	employees := []Employee{}
	for rows.Next() {
		var employee Employee
		if err := rows.Scan(&employee.ID, &employee.Email, &employee.DisplayName, &employee.Status, &employee.ScheduleEligible, nullableString(&employee.DeactivatedAt)); err != nil {
			return nil, err
		}
		employees = append(employees, employee)
	}
	return employees, rows.Err()
}

func employeeByID(ctx context.Context, tx pgx.Tx, employeeID string) (Employee, error) {
	var employee Employee
	err := tx.QueryRow(ctx, `
		SELECT id::text, email, display_name, employee_status, schedule_eligible,
		       to_char(deactivated_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')
		FROM users WHERE id = $1::uuid AND role = 'worker'`, employeeID,
	).Scan(&employee.ID, &employee.Email, &employee.DisplayName, &employee.Status, &employee.ScheduleEligible, nullableString(&employee.DeactivatedAt))
	return employee, err
}

func (p *Postgres) queryShifts(ctx context.Context, query string, args ...any) ([]Shift, error) {
	rows, err := p.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	shifts := []Shift{}
	for rows.Next() {
		var shift Shift
		if err := rows.Scan(&shift.ID, &shift.EmployeeID, &shift.Date, &shift.Start, &shift.End, &shift.Note, &shift.Changed); err != nil {
			return nil, err
		}
		shifts = append(shifts, shift)
	}
	return shifts, rows.Err()
}

func shiftRequestForUpdate(ctx context.Context, tx pgx.Tx, requestID string) (ShiftRequest, error) {
	var request ShiftRequest
	err := tx.QueryRow(ctx, `
		SELECT request.id::text, request.request_type, request.requested_by_employee_id::text,
		       requester.display_name, request.source_shift_id::text, request.source_employee_id::text,
		       to_char(request.source_date, 'YYYY-MM-DD'), to_char(request.source_starts_at, 'HH24:MI'),
		       to_char(request.source_ends_at, 'HH24:MI'), request.source_note,
		       COALESCE(request.target_shift_id::text, ''), COALESCE(request.target_employee_id::text, ''),
		       COALESCE(target.display_name, ''), COALESCE(to_char(request.target_date, 'YYYY-MM-DD'), ''),
		       COALESCE(to_char(request.target_starts_at, 'HH24:MI'), ''),
		       COALESCE(to_char(request.target_ends_at, 'HH24:MI'), ''), request.target_note,
		       COALESCE(request.open_shift_id::text, ''),
		       CASE WHEN request.status = 'pending' AND request.expires_at <= now() THEN 'expired' ELSE request.status END,
		       to_char(request.expires_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS'),
		       COALESCE(to_char(request.resolved_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS'), ''),
		       to_char(request.created_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')
		FROM shift_requests request
		JOIN users requester ON requester.id = request.requested_by_employee_id
		LEFT JOIN users target ON target.id = request.target_employee_id
		WHERE request.id = $1::uuid
		FOR UPDATE OF request`, requestID,
	).Scan(
		&request.ID, &request.RequestType, &request.RequestedByEmployeeID, &request.RequestedByName,
		&request.SourceShiftID, &request.SourceEmployeeID, &request.SourceDate, &request.SourceStart,
		&request.SourceEnd, &request.SourceNote, &request.TargetShiftID, &request.TargetEmployeeID,
		&request.TargetEmployeeName, &request.TargetDate, &request.TargetStart, &request.TargetEnd,
		&request.TargetNote, &request.OpenShiftID, &request.Status, &request.ExpiresAt, &request.ResolvedAt, &request.CreatedAt,
	)
	return request, err
}

func finishShiftRequest(ctx context.Context, tx pgx.Tx, request *ShiftRequest, actorID string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE shift_requests
		SET status = $2, resolved_at = now(), approved_by_user_id = CASE WHEN $2 IN ('approved', 'rejected') THEN $3::uuid ELSE NULL END,
		    updated_at = now()
		WHERE id = $1::uuid`, request.ID, request.Status, actorID); err != nil {
		return err
	}
	request.ResolvedAt = time.Now().Format("2006-01-02T15:04:05")
	return insertAudit(ctx, tx, actorID, "shift_request", request.ID, request.Status, nil, request)
}

func applySwap(ctx context.Context, tx pgx.Tx, request ShiftRequest, actorID string) error {
	if request.RequestType != "swap" {
		return fmt.Errorf("%w: only swap requests can be approved right now", ErrInvalid)
	}
	var scheduleID string
	var version int
	var sourceBefore, targetBefore Shift
	err := tx.QueryRow(ctx, `
		SELECT schedule.id::text, schedule.version, shift.id::text, shift.employee_id::text,
		       to_char(shift.shift_date, 'YYYY-MM-DD'), to_char(shift.starts_at, 'HH24:MI'),
		       to_char(shift.ends_at, 'HH24:MI'), shift.notes
		FROM shifts shift JOIN schedules schedule ON schedule.id = shift.schedule_id
		WHERE shift.id = $1::uuid FOR UPDATE OF schedule, shift`, request.SourceShiftID,
	).Scan(&scheduleID, &version, &sourceBefore.ID, &sourceBefore.EmployeeID,
		&sourceBefore.Date, &sourceBefore.Start, &sourceBefore.End, &sourceBefore.Note)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: source shift no longer exists", ErrConflict)
	}
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `
		SELECT shift.id::text, shift.employee_id::text, to_char(shift.shift_date, 'YYYY-MM-DD'),
		       to_char(shift.starts_at, 'HH24:MI'), to_char(shift.ends_at, 'HH24:MI'), shift.notes
		FROM shifts shift
		WHERE shift.id = $1::uuid AND shift.schedule_id = $2::uuid FOR UPDATE`, request.TargetShiftID, scheduleID,
	).Scan(&targetBefore.ID, &targetBefore.EmployeeID, &targetBefore.Date, &targetBefore.Start, &targetBefore.End, &targetBefore.Note)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: target shift no longer exists", ErrConflict)
	}
	if err != nil {
		return err
	}
	if sourceBefore.EmployeeID != request.SourceEmployeeID || targetBefore.EmployeeID != request.TargetEmployeeID ||
		sourceBefore.Date != request.SourceDate || sourceBefore.Start != request.SourceStart || sourceBefore.End != request.SourceEnd ||
		targetBefore.Date != request.TargetDate || targetBefore.Start != request.TargetStart || targetBefore.End != request.TargetEnd {
		return fmt.Errorf("%w: one of the shifts changed after the request was created", ErrConflict)
	}
	excluded := []string{request.SourceShiftID, request.TargetShiftID}
	sourceConflicts, err := evaluateShiftConflicts(ctx, tx, scheduleID, ShiftInput{
		EmployeeID: request.TargetEmployeeID, Date: sourceBefore.Date, Start: sourceBefore.Start, End: sourceBefore.End,
	}, excluded, sourceBefore.ID, true)
	if err != nil {
		return err
	}
	if err = enforceShiftConflicts(sourceConflicts, nil); err != nil {
		return err
	}
	targetConflicts, err := evaluateShiftConflicts(ctx, tx, scheduleID, ShiftInput{
		EmployeeID: request.SourceEmployeeID, Date: targetBefore.Date, Start: targetBefore.Start, End: targetBefore.End,
	}, excluded, targetBefore.ID, true)
	if err != nil {
		return err
	}
	if err = enforceShiftConflicts(targetConflicts, nil); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		UPDATE shifts SET employee_id = $2::uuid, change_version = $3, updated_at = now()
		WHERE id = $1::uuid`, sourceBefore.ID, request.TargetEmployeeID, version+1); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		UPDATE shifts SET employee_id = $2::uuid, change_version = $3, updated_at = now()
		WHERE id = $1::uuid`, targetBefore.ID, request.SourceEmployeeID, version+1); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE schedules SET status = 'draft', updated_at = now() WHERE id = $1::uuid", scheduleID); err != nil {
		return err
	}
	sourceAfter := sourceBefore
	sourceAfter.EmployeeID = request.TargetEmployeeID
	sourceAfter.Changed = true
	targetAfter := targetBefore
	targetAfter.EmployeeID = request.SourceEmployeeID
	targetAfter.Changed = true
	if err = recordShiftConflicts(ctx, tx, scheduleID, sourceBefore.ID, actorID, sourceConflicts, nil); err != nil {
		return err
	}
	if err = recordShiftConflicts(ctx, tx, scheduleID, targetBefore.ID, actorID, targetConflicts, nil); err != nil {
		return err
	}
	if err = insertAudit(ctx, tx, actorID, "shift", sourceBefore.ID, "swapped", sourceBefore, sourceAfter); err != nil {
		return err
	}
	return insertAudit(ctx, tx, actorID, "shift", targetBefore.ID, "swapped", targetBefore, targetAfter)
}

func ensureNoOverlap(ctx context.Context, tx pgx.Tx, scheduleID, excludedShiftID string, input ShiftInput) error {
	excluded := []string{}
	if excludedShiftID != "" {
		excluded = append(excluded, excludedShiftID)
	}
	return ensureNoOverlapExcluding(ctx, tx, scheduleID, excluded, input)
}

func ensureNoOverlapExcluding(ctx context.Context, tx pgx.Tx, scheduleID string, excludedShiftIDs []string, input ShiftInput) error {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM shifts
		  WHERE schedule_id = $1::uuid AND employee_id = $2::uuid AND shift_date = $3::date
		    AND NOT (id = ANY($6::uuid[])) AND starts_at < $5::time AND ends_at > $4::time
		)`, scheduleID, input.EmployeeID, input.Date, input.Start, input.End, excludedShiftIDs).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: employee already has an overlapping shift", ErrConflict)
	}
	return nil
}

func ensureSchedulableEmployee(ctx context.Context, tx pgx.Tx, employeeID string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM users
		  WHERE id = $1::uuid AND role = 'worker' AND employee_status = 'active' AND schedule_eligible = TRUE
		)`, employeeID,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: employee is not active or schedulable", ErrInvalid)
	}
	return nil
}

func insertAudit(ctx context.Context, tx pgx.Tx, actorID, entityType, entityID, action string, before, after any) error {
	var beforeJSON, afterJSON []byte
	var err error
	if before != nil {
		beforeJSON, err = json.Marshal(before)
		if err != nil {
			return err
		}
	}
	if after != nil {
		afterJSON, err = json.Marshal(after)
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (actor_id, entity_type, entity_id, action, before_data, after_data)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6)`, actorID, entityType, entityID, action, beforeJSON, afterJSON)
	return err
}

func normalizedEmployeeStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "active"
	}
	return status
}

func normalizedScheduleEligible(status string, eligible *bool) bool {
	if status != "active" {
		return false
	}
	if eligible == nil {
		return true
	}
	return *eligible
}

type nullStringTarget struct {
	value *string
}

func nullableString(target *string) nullStringTarget {
	return nullStringTarget{value: target}
}

func (target nullStringTarget) Scan(src any) error {
	var value sql.NullString
	if err := value.Scan(src); err != nil {
		return err
	}
	if value.Valid {
		*target.value = value.String
	} else {
		*target.value = ""
	}
	return nil
}

func isUniqueViolation(err error) bool {
	type sqlState interface {
		SQLState() string
	}
	var state sqlState
	return errors.As(err, &state) && state.SQLState() == "23505"
}
