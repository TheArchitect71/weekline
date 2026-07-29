package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	conflictEmployeeIneligible  = "employee_ineligible"
	conflictApprovedLeave       = "approved_leave"
	conflictShiftOverlap        = "shift_overlap"
	conflictOvertimeRisk        = "overtime_risk"
	conflictMaximumWeeklyHours  = "maximum_weekly_hours"
	conflictMinimumRest         = "minimum_rest"
	conflictMinimumShift        = "minimum_shift"
	conflictPublishedEdit       = "published_edit"
	conflictEmployeeUnavailable = "employee_unavailable"
	conflictOutsidePreferred    = "outside_preferred_availability"
)

type conflictQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type ConflictError struct {
	Conflicts []Conflict
}

func (e ConflictError) Error() string {
	if len(e.Conflicts) == 0 {
		return ErrConflict.Error()
	}
	return fmt.Sprintf("%s: %s", ErrConflict, e.Conflicts[0].Message)
}

func (e ConflictError) Unwrap() error { return ErrConflict }

type schedulingPolicy struct {
	OvertimeWarningHours float64
	MaximumWeeklyHours   float64
	MinimumRestHours     float64
	MinimumShiftHours    float64
}

func (p *Postgres) EvaluateShiftConflicts(ctx context.Context, weekStart string, input ConflictEvaluationInput) ([]Conflict, error) {
	var scheduleID string
	err := p.db.QueryRow(ctx, `SELECT id::text FROM schedules WHERE week_start = $1::date`, weekStart).Scan(&scheduleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	excluded := []string{}
	if input.ShiftID != "" {
		excluded = append(excluded, input.ShiftID)
	}
	return evaluateShiftConflicts(ctx, p.db, scheduleID, input.ShiftInput, excluded, input.ShiftID, true)
}

func evaluateShiftConflicts(
	ctx context.Context,
	q conflictQueryer,
	scheduleID string,
	input ShiftInput,
	excludedShiftIDs []string,
	affectedShiftID string,
	includePublishedEdit bool,
) ([]Conflict, error) {
	conflicts := make([]Conflict, 0, 8)
	add := func(code, severity, message string, canOverride bool, shiftID string) {
		conflicts = append(conflicts, Conflict{
			Code: code, Severity: severity, Message: message, AffectedEmployeeID: input.EmployeeID,
			AffectedShiftID: shiftID, CanOverride: canOverride,
		})
	}

	var status string
	var eligible bool
	err := q.QueryRow(ctx, `
		SELECT employee_status, schedule_eligible
		FROM users WHERE id = $1::uuid AND role = 'worker'`, input.EmployeeID,
	).Scan(&status, &eligible)
	if errors.Is(err, pgx.ErrNoRows) {
		add(conflictEmployeeIneligible, "blocking", "Employee does not exist or is not a worker.", false, affectedShiftID)
	} else if err != nil {
		return nil, err
	} else if status != "active" || !eligible {
		add(conflictEmployeeIneligible, "blocking", "Employee is inactive or not eligible for scheduling.", false, affectedShiftID)
	}

	var policy schedulingPolicy
	if err := q.QueryRow(ctx, `
		SELECT overtime_warning_hours::float8, maximum_weekly_hours::float8,
		       minimum_rest_hours::float8, minimum_shift_hours::float8
		FROM scheduling_policies WHERE id = TRUE`,
	).Scan(&policy.OvertimeWarningHours, &policy.MaximumWeeklyHours, &policy.MinimumRestHours, &policy.MinimumShiftHours); err != nil {
		return nil, err
	}

	startAt, endAt, err := shiftTimes(input)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid shift date or time", ErrInvalid)
	}
	duration := endAt.Sub(startAt).Hours()
	if duration < policy.MinimumShiftHours {
		add(conflictMinimumShift, "warning",
			fmt.Sprintf("Shift is %.1f hours; policy minimum is %.1f hours.", duration, policy.MinimumShiftHours), true, affectedShiftID)
	}

	var leaveType string
	err = q.QueryRow(ctx, `
		SELECT leave_type.name
		FROM leave_requests request
		JOIN leave_types leave_type ON leave_type.id = request.leave_type_id
		WHERE request.employee_id = $1::uuid AND request.status = 'approved'
		  AND $2::date BETWEEN request.starts_on AND request.ends_on
		ORDER BY request.created_at DESC LIMIT 1`, input.EmployeeID, input.Date,
	).Scan(&leaveType)
	if err == nil {
		add(conflictApprovedLeave, "blocking", fmt.Sprintf("Employee has approved %s leave on this date.", leaveType), false, affectedShiftID)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	var unavailable bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM availability_rules rule
		  WHERE rule.employee_id = $1::uuid
		    AND rule.availability_kind = 'unavailable'
		    AND rule.day_of_week = EXTRACT(ISODOW FROM $2::date)::int
		    AND (rule.effective_from IS NULL OR rule.effective_from <= $2::date)
		    AND (rule.effective_until IS NULL OR rule.effective_until >= $2::date)
		    AND rule.starts_at < $4::time AND rule.ends_at > $3::time
		)`, input.EmployeeID, input.Date, input.Start, input.End).Scan(&unavailable); err != nil {
		return nil, err
	}
	if unavailable {
		add(conflictEmployeeUnavailable, "blocking", "Employee marked this time as unavailable.", true, affectedShiftID)
	}

	var hasPreference, withinPreference bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM availability_rules rule
		  WHERE rule.employee_id = $1::uuid
		    AND rule.availability_kind = 'preferred'
		    AND rule.day_of_week = EXTRACT(ISODOW FROM $2::date)::int
		    AND (rule.effective_from IS NULL OR rule.effective_from <= $2::date)
		    AND (rule.effective_until IS NULL OR rule.effective_until >= $2::date)
		), EXISTS (
		  SELECT 1 FROM availability_rules rule
		  WHERE rule.employee_id = $1::uuid
		    AND rule.availability_kind = 'preferred'
		    AND rule.day_of_week = EXTRACT(ISODOW FROM $2::date)::int
		    AND (rule.effective_from IS NULL OR rule.effective_from <= $2::date)
		    AND (rule.effective_until IS NULL OR rule.effective_until >= $2::date)
		    AND rule.starts_at <= $3::time AND rule.ends_at >= $4::time
		)`, input.EmployeeID, input.Date, input.Start, input.End).Scan(&hasPreference, &withinPreference); err != nil {
		return nil, err
	}
	if hasPreference && !withinPreference {
		add(conflictOutsidePreferred, "warning", "Shift falls outside the employee's preferred hours.", true, affectedShiftID)
	}

	var overlapID string
	err = q.QueryRow(ctx, `
		SELECT id::text FROM shifts
		WHERE schedule_id = $1::uuid AND employee_id = $2::uuid AND shift_date = $3::date
		  AND NOT (id = ANY($6::uuid[])) AND starts_at < $5::time AND ends_at > $4::time
		ORDER BY starts_at LIMIT 1`, scheduleID, input.EmployeeID, input.Date, input.Start, input.End, excludedShiftIDs,
	).Scan(&overlapID)
	if err == nil {
		add(conflictShiftOverlap, "blocking", "Employee already has an overlapping shift.", false, overlapID)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	var existingHours float64
	if err := q.QueryRow(ctx, `
		SELECT COALESCE(sum(EXTRACT(EPOCH FROM (ends_at - starts_at)) / 3600), 0)::float8
		FROM shifts
		WHERE schedule_id = $1::uuid AND employee_id = $2::uuid
		  AND NOT (id = ANY($3::uuid[]))`, scheduleID, input.EmployeeID, excludedShiftIDs,
	).Scan(&existingHours); err != nil {
		return nil, err
	}
	weeklyHours := existingHours + duration
	if weeklyHours > policy.MaximumWeeklyHours {
		add(conflictMaximumWeeklyHours, "blocking",
			fmt.Sprintf("Assignment raises weekly hours to %.1f; policy maximum is %.1f hours.", weeklyHours, policy.MaximumWeeklyHours), true, affectedShiftID)
	} else if weeklyHours > policy.OvertimeWarningHours {
		add(conflictOvertimeRisk, "warning",
			fmt.Sprintf("Assignment raises weekly hours to %.1f, above the %.1f-hour overtime threshold.", weeklyHours, policy.OvertimeWarningHours), true, affectedShiftID)
	}

	rows, err := q.Query(ctx, `
		SELECT id::text, shift_date::text, starts_at::text, ends_at::text
		FROM shifts
		WHERE employee_id = $1::uuid AND NOT (id = ANY($2::uuid[]))
		  AND shift_date BETWEEN ($3::date - 1) AND ($3::date + 1)
		ORDER BY shift_date, starts_at`, input.EmployeeID, excludedShiftIDs, input.Date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var otherID, date, start, end string
		if err := rows.Scan(&otherID, &date, &start, &end); err != nil {
			return nil, err
		}
		otherStart, otherEnd, err := shiftTimes(ShiftInput{
			Date: date, Start: strings.TrimSuffix(start, ":00"), End: strings.TrimSuffix(end, ":00"),
		})
		if err != nil {
			return nil, err
		}
		var rest float64
		if !otherEnd.After(startAt) {
			rest = startAt.Sub(otherEnd).Hours()
		} else if !endAt.After(otherStart) {
			rest = otherStart.Sub(endAt).Hours()
		} else {
			continue
		}
		if rest < policy.MinimumRestHours {
			add(conflictMinimumRest, "warning",
				fmt.Sprintf("Only %.1f hours of rest separate this assignment from another shift; policy minimum is %.1f hours.", rest, policy.MinimumRestHours), true, affectedShiftID)
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if includePublishedEdit {
		var published bool
		if err := q.QueryRow(ctx, `SELECT version > 0 FROM schedules WHERE id = $1::uuid`, scheduleID).Scan(&published); err != nil {
			return nil, err
		}
		if published {
			add(conflictPublishedEdit, "warning", "This changes a schedule that employees have already received.", true, affectedShiftID)
		}
	}

	sort.SliceStable(conflicts, func(i, j int) bool {
		return conflictSeverityRank(conflicts[i].Severity) > conflictSeverityRank(conflicts[j].Severity)
	})
	return conflicts, nil
}

func shiftTimes(input ShiftInput) (time.Time, time.Time, error) {
	start, err := time.Parse("2006-01-02 15:04", input.Date+" "+input.Start)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := time.Parse("2006-01-02 15:04", input.Date+" "+input.End)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return start, end, nil
}

func enforceShiftConflicts(conflicts []Conflict, overrideCodes []string) error {
	overrides := make(map[string]bool, len(overrideCodes))
	for _, code := range overrideCodes {
		overrides[code] = true
	}
	blocked := make([]Conflict, 0)
	for _, conflict := range conflicts {
		if conflict.Severity == "blocking" && (!conflict.CanOverride || !overrides[conflict.Code]) {
			blocked = append(blocked, conflict)
		}
	}
	if len(blocked) > 0 {
		return ConflictError{Conflicts: blocked}
	}
	return nil
}

func recordShiftConflicts(ctx context.Context, tx pgx.Tx, scheduleID, shiftID, actorID string, conflicts []Conflict, overrideCodes []string) error {
	overrides := make(map[string]bool, len(overrideCodes))
	for _, code := range overrideCodes {
		overrides[code] = true
	}
	for _, conflict := range conflicts {
		resolution := "accepted"
		if conflict.Severity == "blocking" && overrides[conflict.Code] {
			resolution = "overridden"
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO conflict_overrides
			    (schedule_id, shift_id, employee_id, conflict_code, severity, message, can_override, resolution, actor_id)
			VALUES ($1::uuid, NULLIF($2, '')::uuid, $3::uuid, $4, $5, $6, $7, $8, $9::uuid)`,
			scheduleID, shiftID, conflict.AffectedEmployeeID, conflict.Code, conflict.Severity,
			conflict.Message, conflict.CanOverride, resolution, actorID)
		if err != nil {
			return err
		}
	}
	return nil
}

func enforcePublishConflicts(ctx context.Context, q conflictQueryer, scheduleID string, conflicts []Conflict) error {
	blocked := make([]Conflict, 0)
	for _, conflict := range conflicts {
		if conflict.Severity != "blocking" {
			continue
		}
		if !conflict.CanOverride || conflict.AffectedShiftID == "" {
			blocked = append(blocked, conflict)
			continue
		}
		var overridden bool
		err := q.QueryRow(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM conflict_overrides
			  WHERE schedule_id = $1::uuid AND shift_id = $2::uuid AND conflict_code = $3
			    AND resolution = 'overridden'
			)`, scheduleID, conflict.AffectedShiftID, conflict.Code).Scan(&overridden)
		if err != nil {
			return err
		}
		if !overridden {
			blocked = append(blocked, conflict)
		}
	}
	if len(blocked) > 0 {
		return ConflictError{Conflicts: blocked}
	}
	return nil
}

func scheduleConflicts(ctx context.Context, q conflictQueryer, scheduleID string) ([]Conflict, error) {
	rows, err := q.Query(ctx, `
		SELECT id::text, employee_id::text, shift_date::text,
		       to_char(starts_at, 'HH24:MI'), to_char(ends_at, 'HH24:MI'), notes
		FROM shifts WHERE schedule_id = $1::uuid ORDER BY shift_date, starts_at, id`, scheduleID)
	if err != nil {
		return nil, err
	}
	shifts := make([]Shift, 0)
	for rows.Next() {
		var shift Shift
		if err := rows.Scan(&shift.ID, &shift.EmployeeID, &shift.Date, &shift.Start, &shift.End, &shift.Note); err != nil {
			rows.Close()
			return nil, err
		}
		shifts = append(shifts, shift)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	result := make([]Conflict, 0)
	seen := map[string]bool{}
	for _, shift := range shifts {
		conflicts, err := evaluateShiftConflicts(ctx, q, scheduleID, ShiftInput{
			EmployeeID: shift.EmployeeID, Date: shift.Date, Start: shift.Start, End: shift.End, Note: shift.Note,
		}, []string{shift.ID}, shift.ID, false)
		if err != nil {
			return nil, err
		}
		for _, conflict := range conflicts {
			key := conflict.Code + "|" + conflict.AffectedEmployeeID + "|" + conflict.AffectedShiftID
			if conflict.Code == conflictOvertimeRisk || conflict.Code == conflictMaximumWeeklyHours {
				key = conflict.Code + "|" + conflict.AffectedEmployeeID
			}
			if !seen[key] {
				seen[key] = true
				result = append(result, conflict)
			}
		}
	}
	return result, nil
}

func conflictSeverityRank(severity string) int {
	switch severity {
	case "blocking":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}
