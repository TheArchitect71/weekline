package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (p *Postgres) ListLeaveTypes(ctx context.Context) ([]LeaveType, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id::text, code, name, paid, tracks_balance, accrual_hours_per_month::float8
		FROM leave_types WHERE active = TRUE ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	types := []LeaveType{}
	for rows.Next() {
		var leaveType LeaveType
		if err := rows.Scan(&leaveType.ID, &leaveType.Code, &leaveType.Name, &leaveType.Paid,
			&leaveType.TracksBalance, &leaveType.AccrualHoursPerMonth); err != nil {
			return nil, err
		}
		types = append(types, leaveType)
	}
	return types, rows.Err()
}

func (p *Postgres) ListLeaveBalances(ctx context.Context, user User) ([]LeaveBalance, error) {
	if _, err := p.db.Exec(ctx, `
		INSERT INTO leave_balances (employee_id, leave_type_id, balance_hours)
		SELECT employee.id, leave_type.id, leave_type.default_balance_hours
		FROM users employee CROSS JOIN leave_types leave_type
		WHERE employee.role = 'worker' AND leave_type.active = TRUE AND leave_type.tracks_balance = TRUE
		ON CONFLICT (employee_id, leave_type_id) DO NOTHING`); err != nil {
		return nil, err
	}
	query := `
		SELECT balance.employee_id::text, employee.display_name, balance.leave_type_id::text,
		       leave_type.name, balance.balance_hours::float8, balance.used_hours::float8,
		       (balance.balance_hours - balance.used_hours)::float8
		FROM leave_balances balance
		JOIN users employee ON employee.id = balance.employee_id
		JOIN leave_types leave_type ON leave_type.id = balance.leave_type_id`
	args := []any{}
	if user.Role != "manager" {
		query += " WHERE balance.employee_id = $1::uuid"
		args = append(args, user.ID)
	}
	query += " ORDER BY employee.display_name, leave_type.name"
	rows, err := p.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	balances := []LeaveBalance{}
	for rows.Next() {
		var balance LeaveBalance
		if err := rows.Scan(&balance.EmployeeID, &balance.EmployeeName, &balance.LeaveTypeID,
			&balance.LeaveTypeName, &balance.BalanceHours, &balance.UsedHours, &balance.AvailableHours); err != nil {
			return nil, err
		}
		balances = append(balances, balance)
	}
	return balances, rows.Err()
}

func (p *Postgres) SetLeaveBalance(ctx context.Context, employeeID, leaveTypeID, actorID string, balanceHours float64) (LeaveBalance, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return LeaveBalance{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = ensureSchedulableEmployee(ctx, tx, employeeID); err != nil {
		return LeaveBalance{}, err
	}
	var balance LeaveBalance
	err = tx.QueryRow(ctx, `
		INSERT INTO leave_balances (employee_id, leave_type_id, balance_hours)
		VALUES ($1::uuid, $2::uuid, $3)
		ON CONFLICT (employee_id, leave_type_id) DO UPDATE
		SET balance_hours = EXCLUDED.balance_hours, updated_at = now()
		WHERE leave_balances.used_hours <= EXCLUDED.balance_hours
		RETURNING employee_id::text, leave_type_id::text, balance_hours::float8,
		          used_hours::float8, (balance_hours - used_hours)::float8`, employeeID, leaveTypeID, balanceHours,
	).Scan(&balance.EmployeeID, &balance.LeaveTypeID, &balance.BalanceHours, &balance.UsedHours, &balance.AvailableHours)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeaveBalance{}, fmt.Errorf("%w: balance cannot be lower than hours already used", ErrInvalid)
	}
	if err != nil {
		return LeaveBalance{}, err
	}
	if err = tx.QueryRow(ctx, `
		SELECT employee.display_name, leave_type.name
		FROM users employee CROSS JOIN leave_types leave_type
		WHERE employee.id = $1::uuid AND leave_type.id = $2::uuid`, employeeID, leaveTypeID,
	).Scan(&balance.EmployeeName, &balance.LeaveTypeName); err != nil {
		return LeaveBalance{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "employee", employeeID, "leave_balance_updated", nil, balance); err != nil {
		return LeaveBalance{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return LeaveBalance{}, err
	}
	return balance, nil
}

func (p *Postgres) CreateLeaveRequest(ctx context.Context, employeeID, actorID string, input LeaveRequestInput) (LeaveRequest, error) {
	start, _ := time.Parse("2006-01-02", input.StartsOn)
	end, _ := time.Parse("2006-01-02", input.EndsOn)
	requestedHours := float64(int(end.Sub(start).Hours()/24)+1) * input.HoursPerDay
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return LeaveRequest{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = ensureSchedulableEmployee(ctx, tx, employeeID); err != nil {
		return LeaveRequest{}, err
	}
	var leaveTypeName string
	if err = tx.QueryRow(ctx, `SELECT name FROM leave_types WHERE id = $1::uuid AND active = TRUE`, input.LeaveTypeID).Scan(&leaveTypeName); errors.Is(err, pgx.ErrNoRows) {
		return LeaveRequest{}, fmt.Errorf("%w: leave type does not exist", ErrInvalid)
	}
	if err != nil {
		return LeaveRequest{}, err
	}
	var overlaps bool
	if err = tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM leave_requests
		  WHERE employee_id = $1::uuid AND status IN ('pending', 'approved')
		    AND starts_on <= $3::date AND ends_on >= $2::date
		)`, employeeID, input.StartsOn, input.EndsOn).Scan(&overlaps); err != nil {
		return LeaveRequest{}, err
	}
	if overlaps {
		return LeaveRequest{}, fmt.Errorf("%w: employee already has a leave request for these dates", ErrConflict)
	}
	var request LeaveRequest
	err = tx.QueryRow(ctx, `
		INSERT INTO leave_requests (
		  employee_id, leave_type_id, starts_on, ends_on, hours_per_day, requested_hours, reason
		) VALUES ($1::uuid, $2::uuid, $3::date, $4::date, $5, $6, $7)
		RETURNING id::text, to_char(starts_on, 'YYYY-MM-DD'), to_char(ends_on, 'YYYY-MM-DD'),
		          hours_per_day::float8, requested_hours::float8, reason, status,
		          to_char(created_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')`,
		employeeID, input.LeaveTypeID, input.StartsOn, input.EndsOn, input.HoursPerDay, requestedHours, input.Reason,
	).Scan(&request.ID, &request.StartsOn, &request.EndsOn, &request.HoursPerDay,
		&request.RequestedHours, &request.Reason, &request.Status, &request.CreatedAt)
	if err != nil {
		return LeaveRequest{}, err
	}
	request.EmployeeID = employeeID
	request.LeaveTypeID = input.LeaveTypeID
	request.LeaveTypeName = leaveTypeName
	if err = tx.QueryRow(ctx, `SELECT display_name FROM users WHERE id = $1::uuid`, employeeID).Scan(&request.EmployeeName); err != nil {
		return LeaveRequest{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "leave_request", request.ID, "created", nil, request); err != nil {
		return LeaveRequest{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return LeaveRequest{}, err
	}
	return request, nil
}

func (p *Postgres) ListLeaveRequests(ctx context.Context, user User) ([]LeaveRequest, error) {
	query := `
		SELECT request.id::text, request.employee_id::text, employee.display_name,
		       request.leave_type_id::text, leave_type.name,
		       to_char(request.starts_on, 'YYYY-MM-DD'), to_char(request.ends_on, 'YYYY-MM-DD'),
		       request.hours_per_day::float8, request.requested_hours::float8, request.reason, request.status,
		       (SELECT count(*)::int FROM shifts shift
		        WHERE shift.employee_id = request.employee_id
		          AND shift.shift_date BETWEEN request.starts_on AND request.ends_on),
		       COALESCE(to_char(request.approved_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS'), ''),
		       COALESCE(to_char(request.resolved_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS'), ''),
		       to_char(request.created_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')
		FROM leave_requests request
		JOIN users employee ON employee.id = request.employee_id
		JOIN leave_types leave_type ON leave_type.id = request.leave_type_id`
	args := []any{}
	if user.Role != "manager" {
		query += " WHERE request.employee_id = $1::uuid"
		args = append(args, user.ID)
	}
	query += " ORDER BY CASE request.status WHEN 'pending' THEN 0 WHEN 'approved' THEN 1 ELSE 2 END, request.starts_on DESC, request.created_at DESC"
	rows, err := p.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	requests := []LeaveRequest{}
	for rows.Next() {
		var request LeaveRequest
		if err := rows.Scan(&request.ID, &request.EmployeeID, &request.EmployeeName,
			&request.LeaveTypeID, &request.LeaveTypeName, &request.StartsOn, &request.EndsOn,
			&request.HoursPerDay, &request.RequestedHours, &request.Reason, &request.Status,
			&request.ConflictingShiftCount, &request.ApprovedAt, &request.ResolvedAt, &request.CreatedAt); err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

func (p *Postgres) ResolveLeaveRequest(ctx context.Context, requestID, actorID, decision string) (LeaveRequest, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return LeaveRequest{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	request, tracksBalance, err := leaveRequestForUpdate(ctx, tx, requestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeaveRequest{}, ErrNotFound
	}
	if err != nil {
		return LeaveRequest{}, err
	}
	if request.Status != "pending" {
		return LeaveRequest{}, fmt.Errorf("%w: leave request is no longer pending", ErrConflict)
	}
	if decision == "approved" {
		var overlaps bool
		if err = tx.QueryRow(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM leave_requests
			  WHERE employee_id = $1::uuid AND id <> $2::uuid AND status = 'approved'
			    AND starts_on <= $4::date AND ends_on >= $3::date
			)`, request.EmployeeID, request.ID, request.StartsOn, request.EndsOn).Scan(&overlaps); err != nil {
			return LeaveRequest{}, err
		}
		if overlaps {
			return LeaveRequest{}, fmt.Errorf("%w: approved leave already covers these dates", ErrConflict)
		}
		if tracksBalance {
			if _, err = tx.Exec(ctx, `
				INSERT INTO leave_balances (employee_id, leave_type_id, balance_hours)
				SELECT $1::uuid, id, default_balance_hours FROM leave_types WHERE id = $2::uuid
				ON CONFLICT (employee_id, leave_type_id) DO NOTHING`, request.EmployeeID, request.LeaveTypeID); err != nil {
				return LeaveRequest{}, err
			}
			command, updateErr := tx.Exec(ctx, `
				UPDATE leave_balances SET used_hours = used_hours + $3, updated_at = now()
				WHERE employee_id = $1::uuid AND leave_type_id = $2::uuid
				  AND balance_hours - used_hours >= $3`, request.EmployeeID, request.LeaveTypeID, request.RequestedHours)
			if updateErr != nil {
				return LeaveRequest{}, updateErr
			}
			if command.RowsAffected() != 1 {
				return LeaveRequest{}, fmt.Errorf("%w: insufficient leave balance", ErrConflict)
			}
		}
		request.Status = "approved"
	} else if decision == "rejected" {
		request.Status = "rejected"
	} else {
		return LeaveRequest{}, fmt.Errorf("%w: decision must be approved or rejected", ErrInvalid)
	}
	if _, err = tx.Exec(ctx, `
		UPDATE leave_requests SET status = $2, approved_by_user_id = $3::uuid,
		       approved_at = CASE WHEN $2 = 'approved' THEN now() ELSE NULL END,
		       resolved_at = now(), updated_at = now()
		WHERE id = $1::uuid`, request.ID, request.Status, actorID); err != nil {
		return LeaveRequest{}, err
	}
	request.ResolvedAt = time.Now().Format("2006-01-02T15:04:05")
	if request.Status == "approved" {
		request.ApprovedAt = request.ResolvedAt
	}
	if err = insertAudit(ctx, tx, actorID, "leave_request", request.ID, request.Status, nil, request); err != nil {
		return LeaveRequest{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return LeaveRequest{}, err
	}
	return request, nil
}

func (p *Postgres) CancelLeaveRequest(ctx context.Context, requestID string, user User) (LeaveRequest, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return LeaveRequest{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	request, tracksBalance, err := leaveRequestForUpdate(ctx, tx, requestID)
	if errors.Is(err, pgx.ErrNoRows) || (user.Role != "manager" && request.EmployeeID != user.ID) {
		return LeaveRequest{}, ErrNotFound
	}
	if err != nil {
		return LeaveRequest{}, err
	}
	if request.Status == "approved" {
		if user.Role != "manager" {
			return LeaveRequest{}, fmt.Errorf("%w: manager approval is required to cancel approved leave", ErrConflict)
		}
		if tracksBalance {
			if _, err = tx.Exec(ctx, `
				UPDATE leave_balances
				SET used_hours = GREATEST(0, used_hours - $3), updated_at = now()
				WHERE employee_id = $1::uuid AND leave_type_id = $2::uuid`,
				request.EmployeeID, request.LeaveTypeID, request.RequestedHours); err != nil {
				return LeaveRequest{}, err
			}
		}
	} else if request.Status != "pending" {
		return LeaveRequest{}, fmt.Errorf("%w: leave request is no longer cancellable", ErrConflict)
	}
	request.Status = "cancelled"
	if _, err = tx.Exec(ctx, `UPDATE leave_requests SET status = 'cancelled', resolved_at = now(), updated_at = now() WHERE id = $1::uuid`, request.ID); err != nil {
		return LeaveRequest{}, err
	}
	if err = insertAudit(ctx, tx, user.ID, "leave_request", request.ID, "cancelled", nil, request); err != nil {
		return LeaveRequest{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return LeaveRequest{}, err
	}
	return request, nil
}

func leaveRequestForUpdate(ctx context.Context, tx pgx.Tx, requestID string) (LeaveRequest, bool, error) {
	var request LeaveRequest
	var tracksBalance bool
	err := tx.QueryRow(ctx, `
		SELECT request.id::text, request.employee_id::text, employee.display_name,
		       request.leave_type_id::text, leave_type.name, leave_type.tracks_balance,
		       to_char(request.starts_on, 'YYYY-MM-DD'), to_char(request.ends_on, 'YYYY-MM-DD'),
		       request.hours_per_day::float8, request.requested_hours::float8, request.reason, request.status,
		       to_char(request.created_at AT TIME ZONE 'America/Chicago', 'YYYY-MM-DD"T"HH24:MI:SS')
		FROM leave_requests request
		JOIN users employee ON employee.id = request.employee_id
		JOIN leave_types leave_type ON leave_type.id = request.leave_type_id
		WHERE request.id = $1::uuid FOR UPDATE OF request`, requestID,
	).Scan(&request.ID, &request.EmployeeID, &request.EmployeeName, &request.LeaveTypeID,
		&request.LeaveTypeName, &tracksBalance, &request.StartsOn, &request.EndsOn,
		&request.HoursPerDay, &request.RequestedHours, &request.Reason, &request.Status, &request.CreatedAt)
	return request, tracksBalance, err
}

func (p *Postgres) LeaveBlocks(ctx context.Context, weekStart, employeeID string) ([]LeaveBlock, error) {
	query := `
		SELECT request.id::text, request.employee_id::text, to_char(day::date, 'YYYY-MM-DD'),
		       leave_type.name, request.hours_per_day::float8
		FROM leave_requests request
		JOIN leave_types leave_type ON leave_type.id = request.leave_type_id
		CROSS JOIN LATERAL generate_series(
		  GREATEST(request.starts_on, $1::date), LEAST(request.ends_on, $1::date + 6), interval '1 day'
		) day
		WHERE request.status = 'approved'
		  AND request.starts_on <= $1::date + 6 AND request.ends_on >= $1::date`
	args := []any{weekStart}
	if employeeID != "" {
		query += " AND request.employee_id = $2::uuid"
		args = append(args, employeeID)
	}
	query += " ORDER BY day, request.employee_id"
	rows, err := p.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	blocks := []LeaveBlock{}
	for rows.Next() {
		var block LeaveBlock
		if err := rows.Scan(&block.RequestID, &block.EmployeeID, &block.Date, &block.LeaveTypeName, &block.Hours); err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, rows.Err()
}

func (p *Postgres) ListPublicHolidays(ctx context.Context, startsOn, endsOn string) ([]PublicHoliday, error) {
	query := `SELECT id::text, to_char(holiday_date, 'YYYY-MM-DD'), name, paid FROM public_holidays`
	args := []any{}
	if startsOn != "" && endsOn != "" {
		query += " WHERE holiday_date BETWEEN $1::date AND $2::date"
		args = append(args, startsOn, endsOn)
	}
	query += " ORDER BY holiday_date"
	rows, err := p.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	holidays := []PublicHoliday{}
	for rows.Next() {
		var holiday PublicHoliday
		if err := rows.Scan(&holiday.ID, &holiday.Date, &holiday.Name, &holiday.Paid); err != nil {
			return nil, err
		}
		holidays = append(holidays, holiday)
	}
	return holidays, rows.Err()
}

func (p *Postgres) CreatePublicHoliday(ctx context.Context, actorID string, input PublicHolidayInput) (PublicHoliday, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return PublicHoliday{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var holiday PublicHoliday
	err = tx.QueryRow(ctx, `
		INSERT INTO public_holidays (holiday_date, name, paid, created_by_user_id)
		VALUES ($1::date, $2, $3, $4::uuid)
		RETURNING id::text, to_char(holiday_date, 'YYYY-MM-DD'), name, paid`, input.Date, input.Name, input.Paid, actorID,
	).Scan(&holiday.ID, &holiday.Date, &holiday.Name, &holiday.Paid)
	if err != nil {
		if isUniqueViolation(err) {
			return PublicHoliday{}, fmt.Errorf("%w: a holiday already exists on this date", ErrConflict)
		}
		return PublicHoliday{}, err
	}
	if err = insertAudit(ctx, tx, actorID, "public_holiday", holiday.ID, "created", nil, holiday); err != nil {
		return PublicHoliday{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PublicHoliday{}, err
	}
	return holiday, nil
}

func (p *Postgres) DeletePublicHoliday(ctx context.Context, holidayID, actorID string) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var before PublicHoliday
	if err = tx.QueryRow(ctx, `
		SELECT id::text, to_char(holiday_date, 'YYYY-MM-DD'), name, paid
		FROM public_holidays WHERE id = $1::uuid FOR UPDATE`, holidayID,
	).Scan(&before.ID, &before.Date, &before.Name, &before.Paid); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	command, err := tx.Exec(ctx, `DELETE FROM public_holidays WHERE id = $1::uuid`, holidayID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err = insertAudit(ctx, tx, actorID, "public_holiday", before.ID, "deleted", before, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func ensureNoApprovedLeave(ctx context.Context, tx pgx.Tx, employeeID, shiftDate string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM leave_requests
		  WHERE employee_id = $1::uuid AND status = 'approved'
		    AND $2::date BETWEEN starts_on AND ends_on
		)`, employeeID, shiftDate).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: employee has approved leave on this date", ErrConflict)
	}
	return nil
}

func endOfWeek(weekStart string) string {
	start, _ := time.Parse("2006-01-02", weekStart)
	return start.AddDate(0, 0, 6).Format("2006-01-02")
}
