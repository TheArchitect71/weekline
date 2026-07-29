import { Component, EventEmitter, Input, Output, signal } from '@angular/core';
import { DayColumn, Employee, LeaveBlock, PublicHoliday, Shift } from '../schedule.models';

@Component({
  selector: 'app-worker-schedule',
  templateUrl: './worker-schedule.html',
  styleUrl: './worker-schedule.css',
})
export class WorkerSchedule {
  @Input({ required: true }) shifts: Shift[] = [];
  @Input({ required: true }) employees: Employee[] = [];
  @Input({ required: true }) days: DayColumn[] = [];
  @Input({ required: true }) currentEmployee!: Employee;
  @Input() leave: LeaveBlock[] = [];
  @Input() holidays: PublicHoliday[] = [];
  @Output() profileRequested = new EventEmitter<void>();
  @Output() previousWeekRequested = new EventEmitter<void>();
  @Output() nextWeekRequested = new EventEmitter<void>();
  @Output() requestsRequested = new EventEmitter<void>();
  @Output() leaveRequested = new EventEmitter<void>();
  @Output() availabilityRequested = new EventEmitter<void>();
  protected readonly changesFocused = signal(false);

  protected weekLabel(): string {
    return `${this.days[0].date} – ${this.days[6].date}, ${this.days[6].isoDate.slice(0, 4)}`;
  }

  protected changedCount(): number {
    return this.shifts.filter((shift) => shift.employeeId === this.currentEmployee.id && shift.changed).length;
  }

  protected shiftFor(dayIndex: number): Shift | undefined {
    return this.shifts.find((shift) => shift.employeeId === this.currentEmployee.id && shift.dayIndex === dayIndex);
  }

  protected leaveFor(dayIndex: number): LeaveBlock | undefined {
    return this.leave.find((block) => block.employeeId === this.currentEmployee.id && block.date === this.days[dayIndex].isoDate);
  }

  protected holidayFor(dayIndex: number): PublicHoliday | undefined {
    return this.holidays.find((holiday) => holiday.date === this.days[dayIndex].isoDate);
  }

  protected coworkersFor(shift: Shift): string[] {
    const ids = this.shifts
      .filter((candidate) =>
        candidate.dayIndex === shift.dayIndex &&
        candidate.employeeId !== shift.employeeId &&
        candidate.start === shift.start &&
        candidate.end === shift.end
      )
      .map((candidate) => candidate.employeeId);
    return this.employees.filter((employee) => ids.includes(employee.id)).map((employee) => employee.name);
  }

  protected displayTime(value: string): string {
    const [hours, minutes] = value.split(':').map(Number);
    return `${hours % 12 || 12}:${String(minutes).padStart(2, '0')} ${hours >= 12 ? 'PM' : 'AM'}`;
  }
}
