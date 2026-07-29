package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (p *Postgres) ListAvailability(ctx context.Context, user User) ([]AvailabilityRule, error) {
	query := availabilitySelect
	args := []any{}
	if user.Role != "manager" {
		query += " WHERE rule.employee_id = $1::uuid"
		args = append(args, user.ID)
	}
	query += " ORDER BY employee.display_name, rule.day_of_week, rule.starts_at"
	return p.queryAvailability(ctx, query, args...)
}

func (p *Postgres) AvailabilityForWeek(ctx context.Context, weekStart, employeeID string) ([]AvailabilityRule, error) {
	query := availabilitySelect + `
		WHERE (rule.effective_from IS NULL OR rule.effective_from <= ($1::date + 6))
		  AND (rule.effective_until IS NULL OR rule.effective_until >= $1::date)`
	args := []any{weekStart}
	if employeeID != "" {
		query += " AND rule.employee_id = $2::uuid"
		args = append(args, employeeID)
	}
	query += " ORDER BY employee.display_name, rule.day_of_week, rule.starts_at"
	return p.queryAvailability(ctx, query, args...)
}

const availabilitySelect = `
	SELECT rule.id::text, rule.employee_id::text, employee.display_name, rule.availability_kind,
	       rule.day_of_week, to_char(rule.starts_at, 'HH24:MI'), to_char(rule.ends_at, 'HH24:MI'),
	       COALESCE(to_char(rule.effective_from, 'YYYY-MM-DD'), ''),
	       COALESCE(to_char(rule.effective_until, 'YYYY-MM-DD'), ''), rule.notes
	FROM availability_rules rule
	JOIN users employee ON employee.id = rule.employee_id`

func (p *Postgres) queryAvailability(ctx context.Context, query string, args ...any) ([]AvailabilityRule, error) {
	rows, err := p.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := []AvailabilityRule{}
	for rows.Next() {
		var rule AvailabilityRule
		if err := rows.Scan(&rule.ID, &rule.EmployeeID, &rule.EmployeeName, &rule.Kind, &rule.DayOfWeek,
			&rule.Start, &rule.End, &rule.EffectiveFrom, &rule.EffectiveUntil, &rule.Note); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (p *Postgres) CreateAvailability(ctx context.Context, user User, input AvailabilityInput) (AvailabilityRule, error) {
	employeeID := input.EmployeeID
	if user.Role != "manager" {
		employeeID = user.ID
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return AvailabilityRule{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = ensureWorkerExists(ctx, tx, employeeID); err != nil {
		return AvailabilityRule{}, err
	}
	rule, err := insertAvailability(ctx, tx, employeeID, user.ID, input)
	if err != nil {
		return AvailabilityRule{}, err
	}
	if err = insertAudit(ctx, tx, user.ID, "availability", rule.ID, "created", nil, rule); err != nil {
		return AvailabilityRule{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AvailabilityRule{}, err
	}
	return rule, nil
}

func insertAvailability(ctx context.Context, tx pgx.Tx, employeeID, actorID string, input AvailabilityInput) (AvailabilityRule, error) {
	var rule AvailabilityRule
	err := tx.QueryRow(ctx, `
		INSERT INTO availability_rules
		    (employee_id, availability_kind, day_of_week, starts_at, ends_at, effective_from, effective_until, notes, created_by_user_id)
		VALUES ($1::uuid, $2, $3, $4::time, $5::time, NULLIF($6, '')::date, NULLIF($7, '')::date, $8, $9::uuid)
		RETURNING id::text, employee_id::text,
		          (SELECT display_name FROM users WHERE id = employee_id), availability_kind, day_of_week,
		          to_char(starts_at, 'HH24:MI'), to_char(ends_at, 'HH24:MI'),
		          COALESCE(to_char(effective_from, 'YYYY-MM-DD'), ''),
		          COALESCE(to_char(effective_until, 'YYYY-MM-DD'), ''), notes`,
		employeeID, input.Kind, input.DayOfWeek, input.Start, input.End, input.EffectiveFrom, input.EffectiveUntil, input.Note, actorID,
	).Scan(&rule.ID, &rule.EmployeeID, &rule.EmployeeName, &rule.Kind, &rule.DayOfWeek,
		&rule.Start, &rule.End, &rule.EffectiveFrom, &rule.EffectiveUntil, &rule.Note)
	return rule, err
}

func (p *Postgres) UpdateAvailability(ctx context.Context, ruleID string, user User, input AvailabilityInput) (AvailabilityRule, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return AvailabilityRule{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	before, err := availabilityForUpdate(ctx, tx, ruleID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && user.Role != "manager" && before.EmployeeID != user.ID) {
		return AvailabilityRule{}, ErrNotFound
	}
	if err != nil {
		return AvailabilityRule{}, err
	}
	employeeID := input.EmployeeID
	if user.Role != "manager" {
		employeeID = user.ID
	}
	if err = ensureWorkerExists(ctx, tx, employeeID); err != nil {
		return AvailabilityRule{}, err
	}
	var rule AvailabilityRule
	err = tx.QueryRow(ctx, `
		UPDATE availability_rules SET employee_id = $2::uuid, availability_kind = $3, day_of_week = $4,
		       starts_at = $5::time, ends_at = $6::time, effective_from = NULLIF($7, '')::date,
		       effective_until = NULLIF($8, '')::date, notes = $9, updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, employee_id::text,
		          (SELECT display_name FROM users WHERE id = employee_id), availability_kind, day_of_week,
		          to_char(starts_at, 'HH24:MI'), to_char(ends_at, 'HH24:MI'),
		          COALESCE(to_char(effective_from, 'YYYY-MM-DD'), ''),
		          COALESCE(to_char(effective_until, 'YYYY-MM-DD'), ''), notes`,
		ruleID, employeeID, input.Kind, input.DayOfWeek, input.Start, input.End,
		input.EffectiveFrom, input.EffectiveUntil, input.Note,
	).Scan(&rule.ID, &rule.EmployeeID, &rule.EmployeeName, &rule.Kind, &rule.DayOfWeek,
		&rule.Start, &rule.End, &rule.EffectiveFrom, &rule.EffectiveUntil, &rule.Note)
	if err != nil {
		return AvailabilityRule{}, err
	}
	if err = insertAudit(ctx, tx, user.ID, "availability", rule.ID, "updated", before, rule); err != nil {
		return AvailabilityRule{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AvailabilityRule{}, err
	}
	return rule, nil
}

func (p *Postgres) DeleteAvailability(ctx context.Context, ruleID string, user User) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rule, err := availabilityForUpdate(ctx, tx, ruleID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && user.Role != "manager" && rule.EmployeeID != user.ID) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM availability_rules WHERE id = $1::uuid`, ruleID); err != nil {
		return err
	}
	if err = insertAudit(ctx, tx, user.ID, "availability", rule.ID, "deleted", rule, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func availabilityForUpdate(ctx context.Context, tx pgx.Tx, ruleID string) (AvailabilityRule, error) {
	var rule AvailabilityRule
	err := tx.QueryRow(ctx, availabilitySelect+`
		WHERE rule.id = $1::uuid FOR UPDATE OF rule`, ruleID,
	).Scan(&rule.ID, &rule.EmployeeID, &rule.EmployeeName, &rule.Kind, &rule.DayOfWeek,
		&rule.Start, &rule.End, &rule.EffectiveFrom, &rule.EffectiveUntil, &rule.Note)
	return rule, err
}

func ensureWorkerExists(ctx context.Context, tx pgx.Tx, employeeID string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1::uuid AND role = 'worker')`, employeeID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: employee is required", ErrInvalid)
	}
	return nil
}
