export type UserRole = 'manager' | 'worker';

export interface User {
  id: string;
  email: string;
  displayName: string;
  role: UserRole;
}

export interface Employee {
  id: string;
  name: string;
  email: string;
  status: EmployeeStatus;
  scheduleEligible: boolean;
  deactivatedAt?: string;
  initials: string;
  accent: string;
}

export type EmployeeStatus = 'active' | 'inactive' | 'deactivated';

export interface Shift {
  id: string;
  employeeId: string;
  dayIndex: number;
  date: string;
  start: string;
  end: string;
  note: string;
  changed?: boolean;
}

export interface DayColumn {
  short: string;
  label: string;
  date: string;
  isoDate: string;
}

export interface ShiftDraft {
  employeeId: string;
  dayIndex: number;
  start: string;
  end: string;
  note: string;
}

export type ShiftDropMode = 'move' | 'copy' | 'swap';

export interface ShiftDropAction {
  mode: ShiftDropMode;
  source: Shift;
  targetEmployeeId: string;
  targetDayIndex: number;
  target?: Shift;
}

export interface ApiEmployee {
  id: string;
  email?: string;
  displayName: string;
  status?: EmployeeStatus;
  scheduleEligible?: boolean;
  deactivatedAt?: string;
}

export interface EmployeeInput {
  email: string;
  displayName: string;
  initialPassword?: string;
  status: EmployeeStatus;
  scheduleEligible: boolean;
}

export interface ApiShift {
  id: string;
  employeeId: string;
  date: string;
  start: string;
  end: string;
  note: string;
  changed: boolean;
}

export interface ApiSchedule {
  weekStart: string;
  status: 'draft' | 'published';
  version: number;
  employees: ApiEmployee[];
  shifts: ApiShift[];
  leave: LeaveBlock[];
  holidays: PublicHoliday[];
  openShifts: OpenShift[];
  conflicts: Conflict[];
  availability: AvailabilityRule[];
}

export type ConflictSeverity = 'info' | 'warning' | 'blocking';

export interface Conflict {
  code: string;
  severity: ConflictSeverity;
  message: string;
  affectedEmployeeId: string;
  affectedShiftId?: string;
  canOverride: boolean;
}

export interface AuditEvent {
  id: number;
  actorName: string;
  entityType: string;
  action: string;
  occurredAt: string;
  summary: string;
}

export interface ApiProblem {
  code: string;
  message: string;
  conflicts?: Conflict[];
}

export type ShiftRequestStatus = 'pending' | 'approved' | 'rejected' | 'cancelled' | 'expired';

export interface SwapOption {
  shiftId: string;
  employeeId: string;
  employeeName: string;
  date: string;
  start: string;
  end: string;
  note: string;
}

export interface ShiftRequest {
  id: string;
  requestType: 'swap' | 'open_claim' | 'shift_change';
  requestedByEmployeeId: string;
  requestedByName: string;
  sourceShiftId: string;
  sourceEmployeeId: string;
  sourceDate: string;
  sourceStart: string;
  sourceEnd: string;
  sourceNote: string;
  targetShiftId?: string;
  targetEmployeeId?: string;
  targetEmployeeName?: string;
  targetDate?: string;
  targetStart?: string;
  targetEnd?: string;
  targetNote?: string;
  openShiftId?: string;
  status: ShiftRequestStatus;
  expiresAt: string;
  resolvedAt?: string;
  createdAt: string;
}

export interface OpenShift {
  id: string;
  date: string;
  start: string;
  end: string;
  note: string;
  requiredHeadcount: number;
  assignedCount: number;
  coverageGap: number;
  status: 'open' | 'filled' | 'closed';
  claimStatus?: ShiftRequestStatus;
}

export interface OpenShiftInput {
  date: string;
  start: string;
  end: string;
  note: string;
  requiredHeadcount: number;
}

export interface EmployeeSuggestion {
  employeeId: string;
  employeeName: string;
  weeklyHours: number;
  preferred: boolean;
}

export type AvailabilityKind = 'unavailable' | 'preferred';

export interface AvailabilityInput {
  employeeId?: string;
  kind: AvailabilityKind;
  dayOfWeek: number;
  start: string;
  end: string;
  effectiveFrom: string;
  effectiveUntil: string;
  note: string;
}

export interface AvailabilityRule extends AvailabilityInput {
  id: string;
  employeeId: string;
  employeeName: string;
}

export interface LeaveType {
  id: string;
  code: string;
  name: string;
  paid: boolean;
  tracksBalance: boolean;
  accrualHoursPerMonth: number;
}

export interface LeaveBalance {
  employeeId: string;
  employeeName: string;
  leaveTypeId: string;
  leaveTypeName: string;
  balanceHours: number;
  usedHours: number;
  availableHours: number;
}

export type LeaveRequestStatus = 'pending' | 'approved' | 'rejected' | 'cancelled';

export interface LeaveRequest {
  id: string;
  employeeId: string;
  employeeName: string;
  leaveTypeId: string;
  leaveTypeName: string;
  startsOn: string;
  endsOn: string;
  hoursPerDay: number;
  requestedHours: number;
  reason: string;
  status: LeaveRequestStatus;
  conflictingShiftCount: number;
  approvedAt?: string;
  resolvedAt?: string;
  createdAt: string;
}

export interface LeaveRequestInput {
  employeeId?: string;
  leaveTypeId: string;
  startsOn: string;
  endsOn: string;
  hoursPerDay: number;
  reason: string;
}

export interface LeaveBlock {
  requestId: string;
  employeeId: string;
  date: string;
  leaveTypeName: string;
  hours: number;
}

export interface PublicHoliday {
  id: string;
  date: string;
  name: string;
  paid: boolean;
}

export interface AttendanceStation {
  id: string;
  name: string;
  active: boolean;
  kioskToken?: string;
}

export interface KioskEmployee {
  id: string;
  displayName: string;
  lastEventType?: 'in' | 'out';
}

export interface KioskSession {
  stationId: string;
  stationName: string;
  employees: KioskEmployee[];
}

export interface PunchInput {
  employeeId: string;
  eventType: 'in' | 'out';
  clientEventId: string;
  occurredAt: string;
}

export interface AttendanceEvent {
  id: string;
  employeeId: string;
  eventType: 'in' | 'out';
  occurredAt: string;
}

export type AttendanceStatus = 'complete' | 'missing_in' | 'missing_out' | 'missing_punches';

export interface AttendanceRecord {
  employeeId: string;
  employeeName: string;
  date: string;
  scheduledStart?: string;
  scheduledEnd?: string;
  clockIn?: string;
  clockOut?: string;
  workedHours: number;
  status: AttendanceStatus;
  eventCount: number;
}

export interface AttendanceCorrectionInput {
  employeeId?: string;
  date: string;
  clockIn: string;
  clockOut: string;
  reason: string;
}

export interface AttendanceCorrection extends AttendanceCorrectionInput {
  id: string;
  employeeId: string;
  employeeName: string;
  status: 'pending' | 'approved' | 'rejected' | 'cancelled';
  createdAt: string;
  resolvedAt?: string;
}
