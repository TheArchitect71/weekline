import { Component, EventEmitter, Input, Output } from '@angular/core';
import { AvailabilityRule, Conflict, DayColumn, Employee, LeaveBlock, OpenShift, PublicHoliday, Shift, ShiftDropAction } from '../schedule.models';

@Component({
  selector: 'app-manager-schedule',
  templateUrl: './manager-schedule.html',
  styleUrl: './manager-schedule.css',
})
export class ManagerSchedule {
  @Input({ required: true }) shifts: Shift[] = [];
  @Input({ required: true }) employees: Employee[] = [];
  @Input({ required: true }) days: DayColumn[] = [];
  @Input() leave: LeaveBlock[] = [];
  @Input() holidays: PublicHoliday[] = [];
  @Input() openShifts: OpenShift[] = [];
  @Input() conflicts: Conflict[] = [];
  @Input() availability: AvailabilityRule[] = [];
  @Input() selectedShiftId: string | null = null;
  @Output() shiftSelected = new EventEmitter<Shift>();
  @Output() emptySelected = new EventEmitter<{ employeeId: string; dayIndex: number }>();
  @Output() shiftDropped = new EventEmitter<ShiftDropAction>();
  protected filterChanged = false;
  protected draggingShiftId: string | null = null;
  protected dropTarget = '';

  protected filteredEmployees(): Employee[] {
    if (!this.filterChanged) return this.employees;
    return this.employees.filter((employee) => this.shifts.some((shift) => shift.employeeId === employee.id && shift.changed));
  }

  protected getShift(employeeId: string, dayIndex: number): Shift | undefined {
    return this.shifts.find((shift) => shift.employeeId === employeeId && shift.dayIndex === dayIndex);
  }

  protected getLeave(employeeId: string, dayIndex: number): LeaveBlock | undefined {
    return this.leave.find((block) => block.employeeId === employeeId && block.date === this.days[dayIndex].isoDate);
  }

  protected holiday(dayIndex: number): PublicHoliday | undefined {
    return this.holidays.find((holiday) => holiday.date === this.days[dayIndex].isoDate);
  }

  protected coverageGap(dayIndex: number): number {
    return this.openShifts
      .filter((openShift) => openShift.date === this.days[dayIndex].isoDate)
      .reduce((total, openShift) => total + openShift.coverageGap, 0);
  }

  protected totalCoverageGap(): number {
    return this.openShifts.reduce((total, openShift) => total + openShift.coverageGap, 0);
  }

  protected conflictsForShift(shift: Shift): Conflict[] {
	return this.conflicts.filter((conflict) =>
	  conflict.affectedShiftId === shift.id ||
	  (!conflict.affectedShiftId && conflict.affectedEmployeeId === shift.employeeId),
	);
  }

  protected conflictLabel(shift: Shift): string {
	const conflicts = this.conflictsForShift(shift);
	return conflicts.map((conflict) => conflict.message).join(' ');
  }

  protected availabilityFor(employeeId: string, dayIndex: number): AvailabilityRule | undefined {
    const date = this.days[dayIndex].isoDate;
    return this.availability
      .filter((rule) => rule.employeeId === employeeId && rule.dayOfWeek === dayIndex + 1)
      .filter((rule) => (!rule.effectiveFrom || rule.effectiveFrom <= date) && (!rule.effectiveUntil || rule.effectiveUntil >= date))
      .sort((a, b) => (a.kind === 'unavailable' ? -1 : 1) - (b.kind === 'unavailable' ? -1 : 1))[0];
  }

  protected weeklyHours(employee: Employee): number {
    return this.shifts
      .filter((shift) => shift.employeeId === employee.id)
      .reduce((total, shift) => total + this.duration(shift), 0);
  }

  protected displayTime(value: string): string {
    const [hours, minutes] = value.split(':').map(Number);
    const hour = hours % 12 || 12;
    return `${hour}:${String(minutes).padStart(2, '0')}`;
  }

  protected dragStart(event: DragEvent, shift: Shift): void {
    this.draggingShiftId = shift.id;
    event.dataTransfer?.setData('text/plain', shift.id);
    if (event.dataTransfer) event.dataTransfer.effectAllowed = 'copyMove';
  }

  protected dragOver(event: DragEvent, employeeId: string, dayIndex: number, target?: Shift): void {
    if (!this.draggingShiftId || this.draggingShiftId === target?.id) return;
    event.preventDefault();
    this.dropTarget = `${employeeId}:${dayIndex}`;
    if (event.dataTransfer) event.dataTransfer.dropEffect = target ? 'move' : event.altKey ? 'copy' : 'move';
  }

  protected dragLeave(event: DragEvent): void {
    const next = event.relatedTarget as Node | null;
    if (!next || !(event.currentTarget as HTMLElement).contains(next)) this.dropTarget = '';
  }

  protected drop(event: DragEvent, source: Shift, employeeId: string, dayIndex: number, target?: Shift): void {
    event.preventDefault();
    event.stopPropagation();
    this.dropTarget = '';
    if (source.id === target?.id || (source.employeeId === employeeId && source.dayIndex === dayIndex && !target)) return;
    this.shiftDropped.emit({
      mode: target ? 'swap' : event.altKey ? 'copy' : 'move',
      source,
      targetEmployeeId: employeeId,
      targetDayIndex: dayIndex,
      target,
    });
  }

  protected dragEnd(): void {
    this.draggingShiftId = null;
    this.dropTarget = '';
  }

  protected draggedShift(): Shift | undefined {
    return this.shifts.find((shift) => shift.id === this.draggingShiftId);
  }

  private duration(shift: Shift): number {
    const [startHour, startMinute] = shift.start.split(':').map(Number);
    const [endHour, endMinute] = shift.end.split(':').map(Number);
    return endHour + endMinute / 60 - startHour - startMinute / 60;
  }
}
