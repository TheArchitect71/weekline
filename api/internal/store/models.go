package store

import "errors"

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalid  = errors.New("invalid input")
)

type User struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	DisplayName  string `json:"displayName"`
	Role         string `json:"role"`
	PasswordHash string `json:"-"`
}

type BootstrapManagerInput struct {
	Email       string
	DisplayName string
	Password    string
}

type HostLeaseInput struct {
	InstanceID  string `json:"instanceId"`
	MachineName string `json:"machineName"`
	AppVersion  string `json:"appVersion"`
}

type HostLease struct {
	InstanceID  string `json:"instanceId"`
	MachineName string `json:"machineName"`
	AppVersion  string `json:"appVersion"`
	LastSeenAt  string `json:"lastSeenAt"`
	ExpiresAt   string `json:"expiresAt"`
}

type HostStatus struct {
	Available   bool        `json:"available"`
	ActiveCount int         `json:"activeCount"`
	Instances   []HostLease `json:"instances"`
}

type Employee struct {
	ID               string `json:"id"`
	Email            string `json:"email,omitempty"`
	DisplayName      string `json:"displayName"`
	Status           string `json:"status"`
	ScheduleEligible bool   `json:"scheduleEligible"`
	DeactivatedAt    string `json:"deactivatedAt,omitempty"`
}

type Shift struct {
	ID         string `json:"id"`
	EmployeeID string `json:"employeeId"`
	Date       string `json:"date"`
	Start      string `json:"start"`
	End        string `json:"end"`
	Note       string `json:"note"`
	Changed    bool   `json:"changed"`
}

type Schedule struct {
	WeekStart    string             `json:"weekStart"`
	Status       string             `json:"status"`
	Version      int                `json:"version"`
	Employees    []Employee         `json:"employees"`
	Shifts       []Shift            `json:"shifts"`
	Leave        []LeaveBlock       `json:"leave"`
	Holidays     []PublicHoliday    `json:"holidays"`
	OpenShifts   []OpenShift        `json:"openShifts"`
	Conflicts    []Conflict         `json:"conflicts"`
	Availability []AvailabilityRule `json:"availability"`
}

type ShiftInput struct {
	EmployeeID            string   `json:"employeeId"`
	Date                  string   `json:"date"`
	Start                 string   `json:"start"`
	End                   string   `json:"end"`
	Note                  string   `json:"note"`
	OverrideConflictCodes []string `json:"overrideConflictCodes,omitempty"`
}

type ShiftSwapInput struct {
	SourceShiftID string `json:"sourceShiftId"`
	TargetShiftID string `json:"targetShiftId"`
}

type ShiftSwapResult struct {
	Source Shift `json:"source"`
	Target Shift `json:"target"`
}

type ConflictEvaluationInput struct {
	ShiftID string `json:"shiftId,omitempty"`
	ShiftInput
}

type Conflict struct {
	Code               string `json:"code"`
	Severity           string `json:"severity"`
	Message            string `json:"message"`
	AffectedEmployeeID string `json:"affectedEmployeeId"`
	AffectedShiftID    string `json:"affectedShiftId,omitempty"`
	CanOverride        bool   `json:"canOverride"`
}

type EmployeeInput struct {
	Email            string `json:"email"`
	DisplayName      string `json:"displayName"`
	InitialPassword  string `json:"initialPassword,omitempty"`
	Status           string `json:"status"`
	ScheduleEligible *bool  `json:"scheduleEligible,omitempty"`
}

type SwapRequestInput struct {
	SourceShiftID string `json:"sourceShiftId"`
	TargetShiftID string `json:"targetShiftId"`
}

type SwapOption struct {
	ShiftID      string `json:"shiftId"`
	EmployeeID   string `json:"employeeId"`
	EmployeeName string `json:"employeeName"`
	Date         string `json:"date"`
	Start        string `json:"start"`
	End          string `json:"end"`
	Note         string `json:"note"`
}

type ShiftRequest struct {
	ID                    string `json:"id"`
	RequestType           string `json:"requestType"`
	RequestedByEmployeeID string `json:"requestedByEmployeeId"`
	RequestedByName       string `json:"requestedByName"`
	SourceShiftID         string `json:"sourceShiftId"`
	SourceEmployeeID      string `json:"sourceEmployeeId"`
	SourceDate            string `json:"sourceDate"`
	SourceStart           string `json:"sourceStart"`
	SourceEnd             string `json:"sourceEnd"`
	SourceNote            string `json:"sourceNote"`
	TargetShiftID         string `json:"targetShiftId,omitempty"`
	TargetEmployeeID      string `json:"targetEmployeeId,omitempty"`
	TargetEmployeeName    string `json:"targetEmployeeName,omitempty"`
	TargetDate            string `json:"targetDate,omitempty"`
	TargetStart           string `json:"targetStart,omitempty"`
	TargetEnd             string `json:"targetEnd,omitempty"`
	TargetNote            string `json:"targetNote,omitempty"`
	OpenShiftID           string `json:"openShiftId,omitempty"`
	Status                string `json:"status"`
	ExpiresAt             string `json:"expiresAt"`
	ResolvedAt            string `json:"resolvedAt,omitempty"`
	CreatedAt             string `json:"createdAt"`
}

type OpenShift struct {
	ID                string `json:"id"`
	Date              string `json:"date"`
	Start             string `json:"start"`
	End               string `json:"end"`
	Note              string `json:"note"`
	RequiredHeadcount int    `json:"requiredHeadcount"`
	AssignedCount     int    `json:"assignedCount"`
	CoverageGap       int    `json:"coverageGap"`
	Status            string `json:"status"`
	ClaimStatus       string `json:"claimStatus,omitempty"`
}

type OpenShiftInput struct {
	Date              string `json:"date"`
	Start             string `json:"start"`
	End               string `json:"end"`
	Note              string `json:"note"`
	RequiredHeadcount int    `json:"requiredHeadcount"`
}

type EmployeeSuggestion struct {
	EmployeeID   string  `json:"employeeId"`
	EmployeeName string  `json:"employeeName"`
	WeeklyHours  float64 `json:"weeklyHours"`
	Preferred    bool    `json:"preferred"`
}

type AvailabilityInput struct {
	EmployeeID     string `json:"employeeId,omitempty"`
	Kind           string `json:"kind"`
	DayOfWeek      int    `json:"dayOfWeek"`
	Start          string `json:"start"`
	End            string `json:"end"`
	EffectiveFrom  string `json:"effectiveFrom,omitempty"`
	EffectiveUntil string `json:"effectiveUntil,omitempty"`
	Note           string `json:"note"`
}

type AvailabilityRule struct {
	ID             string `json:"id"`
	EmployeeID     string `json:"employeeId"`
	EmployeeName   string `json:"employeeName"`
	Kind           string `json:"kind"`
	DayOfWeek      int    `json:"dayOfWeek"`
	Start          string `json:"start"`
	End            string `json:"end"`
	EffectiveFrom  string `json:"effectiveFrom,omitempty"`
	EffectiveUntil string `json:"effectiveUntil,omitempty"`
	Note           string `json:"note"`
}

type LeaveType struct {
	ID                   string  `json:"id"`
	Code                 string  `json:"code"`
	Name                 string  `json:"name"`
	Paid                 bool    `json:"paid"`
	TracksBalance        bool    `json:"tracksBalance"`
	AccrualHoursPerMonth float64 `json:"accrualHoursPerMonth"`
}

type LeaveBalance struct {
	EmployeeID     string  `json:"employeeId"`
	EmployeeName   string  `json:"employeeName"`
	LeaveTypeID    string  `json:"leaveTypeId"`
	LeaveTypeName  string  `json:"leaveTypeName"`
	BalanceHours   float64 `json:"balanceHours"`
	UsedHours      float64 `json:"usedHours"`
	AvailableHours float64 `json:"availableHours"`
}

type LeaveRequestInput struct {
	EmployeeID  string  `json:"employeeId,omitempty"`
	LeaveTypeID string  `json:"leaveTypeId"`
	StartsOn    string  `json:"startsOn"`
	EndsOn      string  `json:"endsOn"`
	HoursPerDay float64 `json:"hoursPerDay"`
	Reason      string  `json:"reason"`
}

type LeaveRequest struct {
	ID                    string  `json:"id"`
	EmployeeID            string  `json:"employeeId"`
	EmployeeName          string  `json:"employeeName"`
	LeaveTypeID           string  `json:"leaveTypeId"`
	LeaveTypeName         string  `json:"leaveTypeName"`
	StartsOn              string  `json:"startsOn"`
	EndsOn                string  `json:"endsOn"`
	HoursPerDay           float64 `json:"hoursPerDay"`
	RequestedHours        float64 `json:"requestedHours"`
	Reason                string  `json:"reason"`
	Status                string  `json:"status"`
	ConflictingShiftCount int     `json:"conflictingShiftCount"`
	ApprovedAt            string  `json:"approvedAt,omitempty"`
	ResolvedAt            string  `json:"resolvedAt,omitempty"`
	CreatedAt             string  `json:"createdAt"`
}

type LeaveBlock struct {
	RequestID     string  `json:"requestId"`
	EmployeeID    string  `json:"employeeId"`
	Date          string  `json:"date"`
	LeaveTypeName string  `json:"leaveTypeName"`
	Hours         float64 `json:"hours"`
}

type PublicHoliday struct {
	ID   string `json:"id"`
	Date string `json:"date"`
	Name string `json:"name"`
	Paid bool   `json:"paid"`
}

type PublicHolidayInput struct {
	Date string `json:"date"`
	Name string `json:"name"`
	Paid bool   `json:"paid"`
}

type AuditEvent struct {
	ID         int64  `json:"id"`
	ActorName  string `json:"actorName"`
	EntityType string `json:"entityType"`
	Action     string `json:"action"`
	OccurredAt string `json:"occurredAt"`
	Summary    string `json:"summary"`
}

type AttendanceStation struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Active     bool   `json:"active"`
	KioskToken string `json:"kioskToken,omitempty"`
}

type KioskEmployee struct {
	ID            string `json:"id"`
	DisplayName   string `json:"displayName"`
	LastEventType string `json:"lastEventType,omitempty"`
}

type KioskSession struct {
	StationID   string          `json:"stationId"`
	StationName string          `json:"stationName"`
	Employees   []KioskEmployee `json:"employees"`
}

type PunchInput struct {
	EmployeeID    string `json:"employeeId"`
	EventType     string `json:"eventType"`
	ClientEventID string `json:"clientEventId"`
	OccurredAt    string `json:"occurredAt,omitempty"`
}

type AttendanceEvent struct {
	ID         string `json:"id"`
	EmployeeID string `json:"employeeId"`
	EventType  string `json:"eventType"`
	OccurredAt string `json:"occurredAt"`
}

type AttendanceRecord struct {
	EmployeeID     string  `json:"employeeId"`
	EmployeeName   string  `json:"employeeName"`
	Date           string  `json:"date"`
	ScheduledStart string  `json:"scheduledStart,omitempty"`
	ScheduledEnd   string  `json:"scheduledEnd,omitempty"`
	ClockIn        string  `json:"clockIn,omitempty"`
	ClockOut       string  `json:"clockOut,omitempty"`
	WorkedHours    float64 `json:"workedHours"`
	Status         string  `json:"status"`
	EventCount     int     `json:"eventCount"`
}

type AttendanceCorrectionInput struct {
	EmployeeID string `json:"employeeId,omitempty"`
	Date       string `json:"date"`
	ClockIn    string `json:"clockIn"`
	ClockOut   string `json:"clockOut"`
	Reason     string `json:"reason"`
}

type AttendanceCorrection struct {
	ID           string `json:"id"`
	EmployeeID   string `json:"employeeId"`
	EmployeeName string `json:"employeeName"`
	Date         string `json:"date"`
	ClockIn      string `json:"clockIn"`
	ClockOut     string `json:"clockOut"`
	Reason       string `json:"reason"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
	ResolvedAt   string `json:"resolvedAt,omitempty"`
}
