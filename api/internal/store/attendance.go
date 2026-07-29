package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (p *Postgres) ListAttendanceStations(ctx context.Context) ([]AttendanceStation, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id::text, name, is_active FROM attendance_stations
		ORDER BY CASE WHEN is_active THEN 0 ELSE 1 END, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stations := []AttendanceStation{}
	for rows.Next() {
		var station AttendanceStation
		if err := rows.Scan(&station.ID, &station.Name, &station.Active); err != nil {
			return nil, err
		}
		stations = append(stations, station)
	}
	return stations, rows.Err()
}

func (p *Postgres) CreateAttendanceStation(ctx context.Context, actorID, name string) (AttendanceStation, error) {
	token, hash, err := newStationToken()
	if err != nil {
		return AttendanceStation{}, err
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return AttendanceStation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var station AttendanceStation
	err = tx.QueryRow(ctx, `
		INSERT INTO attendance_stations (name, token_hash, created_by_user_id)
		VALUES ($1, $2, $3::uuid)
		RETURNING id::text, name, is_active`, strings.TrimSpace(name), hash, actorID,
	).Scan(&station.ID, &station.Name, &station.Active)
	if err != nil {
		return AttendanceStation{}, err
	}
	station.KioskToken = token
	if err = insertAudit(ctx, tx, actorID, "attendance_station", station.ID, "created", nil, station); err != nil {
		return AttendanceStation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AttendanceStation{}, err
	}
	return station, nil
}

func (p *Postgres) UpdateAttendanceStation(ctx context.Context, stationID, actorID, name string, active bool) (AttendanceStation, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return AttendanceStation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var before, station AttendanceStation
	err = tx.QueryRow(ctx, `SELECT id::text, name, is_active FROM attendance_stations WHERE id = $1::uuid FOR UPDATE`, stationID).
		Scan(&before.ID, &before.Name, &before.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return AttendanceStation{}, ErrNotFound
	}
	if err != nil {
		return AttendanceStation{}, err
	}
	err = tx.QueryRow(ctx, `
		UPDATE attendance_stations SET name = $2, is_active = $3, updated_at = now()
		WHERE id = $1::uuid RETURNING id::text, name, is_active`, stationID, strings.TrimSpace(name), active,
	).Scan(&station.ID, &station.Name, &station.Active)
	if err != nil {
		return AttendanceStation{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "attendance_station", station.ID, "updated", before, station); err != nil {
		return AttendanceStation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AttendanceStation{}, err
	}
	return station, nil
}

func (p *Postgres) RotateAttendanceStationToken(ctx context.Context, stationID, actorID string) (AttendanceStation, error) {
	token, hash, err := newStationToken()
	if err != nil {
		return AttendanceStation{}, err
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return AttendanceStation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var station AttendanceStation
	err = tx.QueryRow(ctx, `
		UPDATE attendance_stations SET token_hash = $2, updated_at = now()
		WHERE id = $1::uuid RETURNING id::text, name, is_active`, stationID, hash,
	).Scan(&station.ID, &station.Name, &station.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return AttendanceStation{}, ErrNotFound
	}
	if err != nil {
		return AttendanceStation{}, err
	}
	station.KioskToken = token
	if err = insertAudit(ctx, tx, actorID, "attendance_station", station.ID, "token_rotated", nil, map[string]any{"stationId": station.ID}); err != nil {
		return AttendanceStation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AttendanceStation{}, err
	}
	return station, nil
}

func (p *Postgres) KioskSession(ctx context.Context, token string) (KioskSession, error) {
	hash := sha256.Sum256([]byte(token))
	var session KioskSession
	err := p.db.QueryRow(ctx, `
		SELECT id::text, name FROM attendance_stations
		WHERE token_hash = $1 AND is_active = TRUE`, hash[:],
	).Scan(&session.StationID, &session.StationName)
	if errors.Is(err, pgx.ErrNoRows) {
		return KioskSession{}, ErrNotFound
	}
	if err != nil {
		return KioskSession{}, err
	}
	rows, err := p.db.Query(ctx, `
		SELECT employee.id::text, employee.display_name,
		       COALESCE((
		         SELECT event.event_type FROM attendance_events event
		         WHERE event.employee_id = employee.id
		           AND (event.occurred_at AT TIME ZONE 'America/Chicago')::date =
		               (now() AT TIME ZONE 'America/Chicago')::date
		         ORDER BY event.occurred_at DESC LIMIT 1
		       ), '')
		FROM users employee
		WHERE employee.role = 'worker' AND employee.employee_status = 'active'
		ORDER BY employee.display_name`)
	if err != nil {
		return KioskSession{}, err
	}
	defer rows.Close()
	session.Employees = []KioskEmployee{}
	for rows.Next() {
		var employee KioskEmployee
		if err := rows.Scan(&employee.ID, &employee.DisplayName, &employee.LastEventType); err != nil {
			return KioskSession{}, err
		}
		session.Employees = append(session.Employees, employee)
	}
	return session, rows.Err()
}

func (p *Postgres) CreatePunch(ctx context.Context, token string, input PunchInput) (AttendanceEvent, error) {
	hash := sha256.Sum256([]byte(token))
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return AttendanceEvent{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var stationID string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM attendance_stations
		WHERE token_hash = $1 AND is_active = TRUE FOR SHARE`, hash[:],
	).Scan(&stationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AttendanceEvent{}, ErrNotFound
	}
	if err != nil {
		return AttendanceEvent{}, err
	}
	if err = ensureSchedulableEmployee(ctx, tx, input.EmployeeID); err != nil {
		return AttendanceEvent{}, err
	}

	var event AttendanceEvent
	err = tx.QueryRow(ctx, `
		SELECT id::text, employee_id::text, event_type,
		       to_char(occurred_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')
		FROM attendance_events WHERE client_event_id = $1::uuid`, input.ClientEventID,
	).Scan(&event.ID, &event.EmployeeID, &event.EventType, &event.OccurredAt)
	if err == nil {
		return event, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return AttendanceEvent{}, err
	}

	occurredAt := time.Now()
	if input.OccurredAt != "" {
		occurredAt, err = time.Parse(time.RFC3339, input.OccurredAt)
		if err != nil {
			return AttendanceEvent{}, fmt.Errorf("%w: invalid punch timestamp", ErrInvalid)
		}
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO attendance_events
		    (station_id, employee_id, event_type, event_source, occurred_at, client_event_id)
		VALUES ($1::uuid, $2::uuid, $3, 'station', $4, $5::uuid)
		RETURNING id::text, employee_id::text, event_type,
		          to_char(occurred_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')`,
		stationID, input.EmployeeID, input.EventType, occurredAt, input.ClientEventID,
	).Scan(&event.ID, &event.EmployeeID, &event.EventType, &event.OccurredAt)
	if err != nil {
		return AttendanceEvent{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AttendanceEvent{}, err
	}
	return event, nil
}

func (p *Postgres) ListAttendance(ctx context.Context, from, to string, user User, employeeID string) ([]AttendanceRecord, error) {
	if user.Role != "manager" {
		employeeID = user.ID
	}
	rows, err := p.db.Query(ctx, `
		WITH scheduled AS (
		  SELECT employee_id, shift_date AS attendance_date, min(starts_at) AS scheduled_start, max(ends_at) AS scheduled_end
		  FROM published_shifts
		  WHERE shift_date BETWEEN $1::date AND $2::date
		    AND (NULLIF($3, '') IS NULL OR employee_id = NULLIF($3, '')::uuid)
		  GROUP BY employee_id, shift_date
		), event_local AS (
		  SELECT employee_id, (occurred_at AT TIME ZONE 'America/Chicago')::date AS attendance_date,
		         occurred_at AT TIME ZONE 'America/Chicago' AS local_time, event_type
		  FROM attendance_events
		  WHERE (occurred_at AT TIME ZONE 'America/Chicago')::date BETWEEN $1::date AND $2::date
		    AND (NULLIF($3, '') IS NULL OR employee_id = NULLIF($3, '')::uuid)
		), events AS (
		  SELECT employee_id, attendance_date,
		         min(local_time) FILTER (WHERE event_type = 'in') AS clock_in,
		         max(local_time) FILTER (WHERE event_type = 'out') AS clock_out,
		         count(*)::int AS event_count
		  FROM event_local GROUP BY employee_id, attendance_date
		), keys AS (
		  SELECT employee_id, attendance_date FROM scheduled
		  UNION
		  SELECT employee_id, attendance_date FROM events
		)
		SELECT keys.employee_id::text, employee.display_name, to_char(keys.attendance_date, 'YYYY-MM-DD'),
		       COALESCE(to_char(scheduled.scheduled_start, 'HH24:MI'), ''),
		       COALESCE(to_char(scheduled.scheduled_end, 'HH24:MI'), ''),
		       COALESCE(to_char(events.clock_in, 'HH24:MI'), ''),
		       COALESCE(to_char(events.clock_out, 'HH24:MI'), ''),
		       CASE WHEN events.clock_in IS NOT NULL AND events.clock_out IS NOT NULL
		            THEN round((EXTRACT(EPOCH FROM (events.clock_out - events.clock_in)) / 3600)::numeric, 2)::float8
		            ELSE 0::float8 END,
		       CASE WHEN events.employee_id IS NULL THEN 'missing_punches'
		            WHEN events.clock_in IS NULL THEN 'missing_in'
		            WHEN events.clock_out IS NULL THEN 'missing_out'
		            ELSE 'complete' END,
		       COALESCE(events.event_count, 0)
		FROM keys
		JOIN users employee ON employee.id = keys.employee_id
		LEFT JOIN scheduled USING (employee_id, attendance_date)
		LEFT JOIN events USING (employee_id, attendance_date)
		ORDER BY keys.attendance_date DESC, employee.display_name`, from, to, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []AttendanceRecord{}
	for rows.Next() {
		var record AttendanceRecord
		if err := rows.Scan(&record.EmployeeID, &record.EmployeeName, &record.Date,
			&record.ScheduledStart, &record.ScheduledEnd, &record.ClockIn, &record.ClockOut,
			&record.WorkedHours, &record.Status, &record.EventCount); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (p *Postgres) ListAttendanceCorrections(ctx context.Context, user User) ([]AttendanceCorrection, error) {
	query := `
		SELECT correction.id::text, correction.employee_id::text, employee.display_name,
		       to_char(correction.attendance_date, 'YYYY-MM-DD'),
		       to_char(correction.proposed_clock_in, 'HH24:MI'), to_char(correction.proposed_clock_out, 'HH24:MI'),
		       correction.reason, correction.status,
		       to_char(correction.created_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS'),
		       COALESCE(to_char(correction.resolved_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS'), '')
		FROM attendance_corrections correction
		JOIN users employee ON employee.id = correction.employee_id`
	args := []any{}
	if user.Role != "manager" {
		query += " WHERE correction.employee_id = $1::uuid"
		args = append(args, user.ID)
	}
	query += " ORDER BY CASE correction.status WHEN 'pending' THEN 0 ELSE 1 END, correction.created_at DESC"
	rows, err := p.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	corrections := []AttendanceCorrection{}
	for rows.Next() {
		var correction AttendanceCorrection
		if err := rows.Scan(&correction.ID, &correction.EmployeeID, &correction.EmployeeName, &correction.Date,
			&correction.ClockIn, &correction.ClockOut, &correction.Reason, &correction.Status,
			&correction.CreatedAt, &correction.ResolvedAt); err != nil {
			return nil, err
		}
		corrections = append(corrections, correction)
	}
	return corrections, rows.Err()
}

func (p *Postgres) CreateAttendanceCorrection(ctx context.Context, user User, input AttendanceCorrectionInput) (AttendanceCorrection, error) {
	employeeID := input.EmployeeID
	if user.Role != "manager" {
		employeeID = user.ID
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return AttendanceCorrection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = ensureWorkerExists(ctx, tx, employeeID); err != nil {
		return AttendanceCorrection{}, err
	}
	var correction AttendanceCorrection
	err = tx.QueryRow(ctx, `
		INSERT INTO attendance_corrections
		    (employee_id, attendance_date, proposed_clock_in, proposed_clock_out, reason, requested_by_user_id)
		VALUES ($1::uuid, $2::date, $3::time, $4::time, $5, $6::uuid)
		RETURNING id::text, employee_id::text, (SELECT display_name FROM users WHERE id = employee_id),
		          to_char(attendance_date, 'YYYY-MM-DD'), to_char(proposed_clock_in, 'HH24:MI'),
		          to_char(proposed_clock_out, 'HH24:MI'), reason, status,
		          to_char(created_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS'), ''`,
		employeeID, input.Date, input.ClockIn, input.ClockOut, strings.TrimSpace(input.Reason), user.ID,
	).Scan(&correction.ID, &correction.EmployeeID, &correction.EmployeeName, &correction.Date,
		&correction.ClockIn, &correction.ClockOut, &correction.Reason, &correction.Status,
		&correction.CreatedAt, &correction.ResolvedAt)
	if err != nil {
		return AttendanceCorrection{}, err
	}
	if err = insertAudit(ctx, tx, user.ID, "attendance_correction", correction.ID, "created", nil, correction); err != nil {
		return AttendanceCorrection{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AttendanceCorrection{}, err
	}
	return correction, nil
}

func (p *Postgres) ResolveAttendanceCorrection(ctx context.Context, correctionID, actorID, decision string) (AttendanceCorrection, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return AttendanceCorrection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	correction, err := attendanceCorrectionForUpdate(ctx, tx, correctionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AttendanceCorrection{}, ErrNotFound
	}
	if err != nil {
		return AttendanceCorrection{}, err
	}
	if correction.Status != "pending" {
		return AttendanceCorrection{}, fmt.Errorf("%w: correction is no longer pending", ErrConflict)
	}
	if decision == "approved" {
		if _, err = tx.Exec(ctx, `
			INSERT INTO attendance_events
			    (employee_id, event_type, event_source, occurred_at, client_event_id, correction_id)
			VALUES
			    ($1::uuid, 'in', 'correction', ($2::date + $3::time) AT TIME ZONE 'America/Chicago', gen_random_uuid(), $5::uuid),
			    ($1::uuid, 'out', 'correction', ($2::date + $4::time) AT TIME ZONE 'America/Chicago', gen_random_uuid(), $5::uuid)`,
			correction.EmployeeID, correction.Date, correction.ClockIn, correction.ClockOut, correction.ID); err != nil {
			return AttendanceCorrection{}, err
		}
	} else if decision != "rejected" {
		return AttendanceCorrection{}, fmt.Errorf("%w: decision must be approved or rejected", ErrInvalid)
	}
	correction.Status = decision
	err = tx.QueryRow(ctx, `
		UPDATE attendance_corrections SET status = $2, resolved_by_user_id = $3::uuid,
		       resolved_at = now(), updated_at = now()
		WHERE id = $1::uuid
		RETURNING to_char(resolved_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')`,
		correction.ID, decision, actorID).Scan(&correction.ResolvedAt)
	if err != nil {
		return AttendanceCorrection{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "attendance_correction", correction.ID, decision, nil, correction); err != nil {
		return AttendanceCorrection{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AttendanceCorrection{}, err
	}
	return correction, nil
}

func (p *Postgres) CancelAttendanceCorrection(ctx context.Context, correctionID, employeeID string) (AttendanceCorrection, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return AttendanceCorrection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	correction, err := attendanceCorrectionForUpdate(ctx, tx, correctionID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && correction.EmployeeID != employeeID) {
		return AttendanceCorrection{}, ErrNotFound
	}
	if err != nil {
		return AttendanceCorrection{}, err
	}
	if correction.Status != "pending" {
		return AttendanceCorrection{}, fmt.Errorf("%w: correction is no longer pending", ErrConflict)
	}
	correction.Status = "cancelled"
	err = tx.QueryRow(ctx, `
		UPDATE attendance_corrections SET status = 'cancelled', resolved_at = now(), updated_at = now()
		WHERE id = $1::uuid
		RETURNING to_char(resolved_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')`, correction.ID,
	).Scan(&correction.ResolvedAt)
	if err != nil {
		return AttendanceCorrection{}, err
	}
	if err = insertAudit(ctx, tx, employeeID, "attendance_correction", correction.ID, "cancelled", nil, correction); err != nil {
		return AttendanceCorrection{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AttendanceCorrection{}, err
	}
	return correction, nil
}

func attendanceCorrectionForUpdate(ctx context.Context, tx pgx.Tx, correctionID string) (AttendanceCorrection, error) {
	var correction AttendanceCorrection
	err := tx.QueryRow(ctx, `
		SELECT correction.id::text, correction.employee_id::text, employee.display_name,
		       to_char(correction.attendance_date, 'YYYY-MM-DD'),
		       to_char(correction.proposed_clock_in, 'HH24:MI'), to_char(correction.proposed_clock_out, 'HH24:MI'),
		       correction.reason, correction.status,
		       to_char(correction.created_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS'),
		       COALESCE(to_char(correction.resolved_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS'), '')
		FROM attendance_corrections correction JOIN users employee ON employee.id = correction.employee_id
		WHERE correction.id = $1::uuid FOR UPDATE OF correction`, correctionID,
	).Scan(&correction.ID, &correction.EmployeeID, &correction.EmployeeName, &correction.Date,
		&correction.ClockIn, &correction.ClockOut, &correction.Reason, &correction.Status,
		&correction.CreatedAt, &correction.ResolvedAt)
	return correction, err
}

func newStationToken() (string, []byte, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	return token, hash[:], nil
}
