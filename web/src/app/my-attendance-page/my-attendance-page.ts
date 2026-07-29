import { Component, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { AttendanceService } from '../core/attendance.service';
import { currentWeekStart, isoDate } from '../schedule.data';
import { AttendanceCorrection, AttendanceCorrectionInput, AttendanceRecord } from '../schedule.models';

@Component({
  selector: 'app-my-attendance-page',
  imports: [FormsModule, RouterLink],
  templateUrl: './my-attendance-page.html',
  styleUrl: './my-attendance-page.css',
})
export class MyAttendancePage implements OnInit {
  protected readonly acting = signal(false);
  protected readonly actionError = signal('');
  protected form: AttendanceCorrectionInput = this.emptyForm();
  private readonly weekStart = currentWeekStart();
  private readonly weekEnd = this.endOfWeek(this.weekStart);

  constructor(protected readonly attendance: AttendanceService) {}

  async ngOnInit(): Promise<void> {
    await Promise.all([
      this.attendance.loadRecords(this.weekStart, this.weekEnd),
      this.attendance.loadCorrections(),
    ]);
  }

  protected async submit(): Promise<void> {
    if (this.acting()) return;
    this.acting.set(true);
    this.actionError.set('');
    try {
      await this.attendance.createCorrection(this.form);
      this.form = this.emptyForm();
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set(false);
    }
  }

  protected async cancel(correction: AttendanceCorrection): Promise<void> {
    if (this.acting()) return;
    this.acting.set(true);
    this.actionError.set('');
    try {
      await this.attendance.cancelCorrection(correction.id);
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set(false);
    }
  }

  protected time(value?: string): string {
    if (!value) return '—';
    const [hours, minutes] = value.split(':').map(Number);
    return `${hours % 12 || 12}:${String(minutes).padStart(2, '0')} ${hours >= 12 ? 'PM' : 'AM'}`;
  }

  protected dateLabel(value: string): string {
    return new Date(`${value}T12:00:00`).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
  }

  protected statusText(record: AttendanceRecord): string {
    if (record.status === 'complete') return `${record.workedHours} hours recorded`;
    if (record.status === 'missing_in') return 'Clock-in is missing';
    if (record.status === 'missing_out') return 'Clock-out is missing';
    return 'No punches recorded';
  }

  private emptyForm(): AttendanceCorrectionInput {
    return { date: isoDate(new Date()), clockIn: '09:00', clockOut: '17:00', reason: '' };
  }

  private endOfWeek(start: string): string {
    const date = new Date(`${start}T12:00:00`);
    date.setDate(date.getDate() + 6);
    return isoDate(date);
  }
}
