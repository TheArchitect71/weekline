package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"weekline/api/internal/store"
)

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	recorder := httptest.NewRecorder()

	NewHandler(nil, Config{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"status":"ok"`) {
		t.Fatalf("expected healthy response, got %s", recorder.Body.String())
	}
}

func TestProtectedRouteRequiresSession(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/schedules/2026-07-06", nil)
	recorder := httptest.NewRecorder()

	NewHandler(nil, Config{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
}

func TestAttendanceRoutesAreDisabledByDefault(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/attendance", nil)
	recorder := httptest.NewRecorder()

	NewHandler(nil, Config{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func TestHostControlRequiresToken(t *testing.T) {
	request := httptest.NewRequest(http.MethodPut, "/api/host/leases/550e8400-e29b-41d4-a716-446655440000", strings.NewReader(`{"machineName":"Office"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	NewHandler(nil, Config{HostControlToken: strings.Repeat("a", 32)}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
}

func TestValidUUID(t *testing.T) {
	if !validUUID("550e8400-e29b-41d4-a716-446655440000") {
		t.Fatal("expected UUID to be valid")
	}
	for _, value := range []string{"", "not-a-uuid", "550e8400-e29b-41d4-a716-44665544000z"} {
		if validUUID(value) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}

func TestValidateSwapRequest(t *testing.T) {
	tests := []struct {
		name  string
		input store.SwapRequestInput
		valid bool
	}{
		{name: "valid", input: store.SwapRequestInput{SourceShiftID: "source", TargetShiftID: "target"}, valid: true},
		{name: "missing source", input: store.SwapRequestInput{TargetShiftID: "target"}},
		{name: "missing target", input: store.SwapRequestInput{SourceShiftID: "source"}},
		{name: "same shift", input: store.SwapRequestInput{SourceShiftID: "same", TargetShiftID: "same"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSwapRequest(test.input)
			if test.valid && err != nil {
				t.Fatalf("expected valid request, got %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateLeaveRequest(t *testing.T) {
	tests := []struct {
		name  string
		input store.LeaveRequestInput
		valid bool
	}{
		{name: "valid", input: store.LeaveRequestInput{LeaveTypeID: "type", StartsOn: "2026-07-09", EndsOn: "2026-07-10", HoursPerDay: 8}, valid: true},
		{name: "missing type", input: store.LeaveRequestInput{StartsOn: "2026-07-09", EndsOn: "2026-07-10", HoursPerDay: 8}},
		{name: "reversed dates", input: store.LeaveRequestInput{LeaveTypeID: "type", StartsOn: "2026-07-10", EndsOn: "2026-07-09", HoursPerDay: 8}},
		{name: "invalid hours", input: store.LeaveRequestInput{LeaveTypeID: "type", StartsOn: "2026-07-09", EndsOn: "2026-07-09", HoursPerDay: 25}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateLeaveRequest(test.input)
			if test.valid && err != nil {
				t.Fatalf("expected valid request, got %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateOpenShift(t *testing.T) {
	tests := []struct {
		name  string
		input store.OpenShiftInput
		valid bool
	}{
		{name: "valid", input: store.OpenShiftInput{Date: "2026-07-09", Start: "09:00", End: "17:00", RequiredHeadcount: 2}, valid: true},
		{name: "outside week", input: store.OpenShiftInput{Date: "2026-07-13", Start: "09:00", End: "17:00", RequiredHeadcount: 2}},
		{name: "reversed time", input: store.OpenShiftInput{Date: "2026-07-09", Start: "17:00", End: "09:00", RequiredHeadcount: 2}},
		{name: "zero headcount", input: store.OpenShiftInput{Date: "2026-07-09", Start: "09:00", End: "17:00"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateOpenShift(test.input, "2026-07-06")
			if test.valid && err != nil {
				t.Fatalf("expected valid open shift, got %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateAvailability(t *testing.T) {
	tests := []struct {
		name  string
		input store.AvailabilityInput
		valid bool
	}{
		{name: "valid recurring rule", input: store.AvailabilityInput{EmployeeID: "employee", Kind: "unavailable", DayOfWeek: 2, Start: "08:00", End: "12:00"}, valid: true},
		{name: "valid dated preference", input: store.AvailabilityInput{EmployeeID: "employee", Kind: "preferred", DayOfWeek: 5, Start: "09:00", End: "17:00", EffectiveFrom: "2026-07-01", EffectiveUntil: "2026-08-01"}, valid: true},
		{name: "missing employee", input: store.AvailabilityInput{Kind: "unavailable", DayOfWeek: 2, Start: "08:00", End: "12:00"}},
		{name: "invalid day", input: store.AvailabilityInput{EmployeeID: "employee", Kind: "unavailable", DayOfWeek: 8, Start: "08:00", End: "12:00"}},
		{name: "reversed time", input: store.AvailabilityInput{EmployeeID: "employee", Kind: "preferred", DayOfWeek: 2, Start: "17:00", End: "09:00"}},
		{name: "reversed dates", input: store.AvailabilityInput{EmployeeID: "employee", Kind: "unavailable", DayOfWeek: 2, Start: "08:00", End: "12:00", EffectiveFrom: "2026-08-01", EffectiveUntil: "2026-07-01"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateAvailability(test.input, true)
			if test.valid && err != nil {
				t.Fatalf("expected valid availability, got %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidatePunch(t *testing.T) {
	validID := "550e8400-e29b-41d4-a716-446655440000"
	tests := []struct {
		name  string
		input store.PunchInput
		valid bool
	}{
		{name: "online punch", input: store.PunchInput{EmployeeID: validID, EventType: "in", ClientEventID: validID}, valid: true},
		{name: "recent offline punch", input: store.PunchInput{EmployeeID: validID, EventType: "out", ClientEventID: validID, OccurredAt: time.Now().Add(-time.Hour).Format(time.RFC3339)}, valid: true},
		{name: "bad event", input: store.PunchInput{EmployeeID: validID, EventType: "break", ClientEventID: validID}},
		{name: "bad uuid", input: store.PunchInput{EmployeeID: "employee", EventType: "in", ClientEventID: validID}},
		{name: "stale offline punch", input: store.PunchInput{EmployeeID: validID, EventType: "in", ClientEventID: validID, OccurredAt: time.Now().Add(-25 * time.Hour).Format(time.RFC3339)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePunch(test.input)
			if test.valid && err != nil {
				t.Fatalf("expected valid punch, got %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateAttendanceCorrection(t *testing.T) {
	validID := "550e8400-e29b-41d4-a716-446655440000"
	tests := []struct {
		name  string
		input store.AttendanceCorrectionInput
		valid bool
	}{
		{name: "valid", input: store.AttendanceCorrectionInput{EmployeeID: validID, Date: "2026-07-09", ClockIn: "08:00", ClockOut: "16:00", Reason: "Missed kiosk punch"}, valid: true},
		{name: "bad times", input: store.AttendanceCorrectionInput{EmployeeID: validID, Date: "2026-07-09", ClockIn: "16:00", ClockOut: "08:00", Reason: "Missed kiosk punch"}},
		{name: "short reason", input: store.AttendanceCorrectionInput{EmployeeID: validID, Date: "2026-07-09", ClockIn: "08:00", ClockOut: "16:00", Reason: "x"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateAttendanceCorrection(test.input, true)
			if test.valid && err != nil {
				t.Fatalf("expected valid correction, got %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
