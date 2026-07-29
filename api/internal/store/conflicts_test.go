package store

import (
	"errors"
	"testing"
)

func TestEnforceShiftConflicts(t *testing.T) {
	tests := []struct {
		name      string
		conflicts []Conflict
		overrides []string
		wantError bool
	}{
		{
			name:      "warnings do not block",
			conflicts: []Conflict{{Code: conflictMinimumRest, Severity: "warning", CanOverride: true}},
		},
		{
			name:      "non overrideable blocker",
			conflicts: []Conflict{{Code: conflictApprovedLeave, Severity: "blocking", CanOverride: false}},
			overrides: []string{conflictApprovedLeave}, wantError: true,
		},
		{
			name:      "overrideable blocker without approval",
			conflicts: []Conflict{{Code: conflictMaximumWeeklyHours, Severity: "blocking", CanOverride: true}},
			wantError: true,
		},
		{
			name:      "overrideable blocker with approval",
			conflicts: []Conflict{{Code: conflictMaximumWeeklyHours, Severity: "blocking", CanOverride: true}},
			overrides: []string{conflictMaximumWeeklyHours},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := enforceShiftConflicts(test.conflicts, test.overrides)
			if (err != nil) != test.wantError {
				t.Fatalf("enforceShiftConflicts() error = %v, wantError %v", err, test.wantError)
			}
			if err != nil && !errors.Is(err, ErrConflict) {
				t.Fatalf("error %v does not wrap ErrConflict", err)
			}
		})
	}
}

func TestShiftTimes(t *testing.T) {
	start, end, err := shiftTimes(ShiftInput{Date: "2026-07-09", Start: "09:15", End: "17:45"})
	if err != nil {
		t.Fatal(err)
	}
	if got := end.Sub(start).Hours(); got != 8.5 {
		t.Fatalf("duration = %v, want 8.5", got)
	}

	if _, _, err := shiftTimes(ShiftInput{Date: "bad", Start: "09:00", End: "17:00"}); err == nil {
		t.Fatal("shiftTimes() accepted an invalid date")
	}
}
