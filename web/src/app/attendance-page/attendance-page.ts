import { Component, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AttendanceService } from '../core/attendance.service';
import { ScheduleService } from '../core/schedule.service';
import { currentWeekStart, isoDate } from '../schedule.data';
import { AttendanceCorrection, AttendanceStation } from '../schedule.models';

@Component({
  selector: 'app-attendance-page',
  imports: [FormsModule],
  templateUrl: './attendance-page.html',
  styleUrl: './attendance-page.css',
})
export class AttendancePage implements OnInit {
  protected readonly activeTab = signal<'records' | 'corrections' | 'stations'>('records');
  protected readonly acting = signal('');
  protected readonly actionError = signal('');
  protected from = currentWeekStart();
  protected to = this.weekEnd(this.from);
  protected employeeId = '';
  protected stationName = '';

  constructor(
    protected readonly attendance: AttendanceService,
    protected readonly schedule: ScheduleService,
  ) {}

  async ngOnInit(): Promise<void> {
    await Promise.all([
      this.schedule.load(),
      this.attendance.loadRecords(this.from, this.to),
      this.attendance.loadCorrections(),
      this.attendance.loadStations(),
    ]);
  }

  protected async loadRecords(): Promise<void> {
    await this.attendance.loadRecords(this.from, this.to, this.employeeId);
  }

  protected async decide(correction: AttendanceCorrection, decision: 'approve' | 'reject'): Promise<void> {
    if (this.acting()) return;
    this.acting.set(correction.id);
    this.actionError.set('');
    try {
      await this.attendance.decideCorrection(correction.id, decision);
      if (decision === 'approve') await this.loadRecords();
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set('');
    }
  }

  protected async createStation(): Promise<void> {
    if (this.acting() || !this.stationName.trim()) return;
    this.acting.set('station');
    this.actionError.set('');
    try {
      const station = await this.attendance.createStation(this.stationName);
      this.stationName = '';
      this.openToken(station.kioskToken ?? '');
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set('');
    }
  }

  protected async toggleStation(station: AttendanceStation): Promise<void> {
    if (this.acting()) return;
    this.acting.set(station.id);
    this.actionError.set('');
    try {
      await this.attendance.updateStation(station, !station.active);
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set('');
    }
  }

  protected async openKiosk(station: AttendanceStation): Promise<void> {
    if (this.acting() || !station.active) return;
    this.acting.set(station.id);
    this.actionError.set('');
    try {
      let token = this.attendance.tokenFor(station.id);
      if (!token) token = (await this.attendance.rotateStation(station.id)).kioskToken ?? '';
      this.openToken(token);
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set('');
    }
  }

  protected statusLabel(status: string): string {
    return status.replaceAll('_', ' ');
  }

  protected pendingCorrectionCount(): number {
    return this.attendance.corrections().filter((correction) => correction.status === 'pending').length;
  }

  protected time(value?: string): string {
    if (!value) return '—';
    const [hours, minutes] = value.split(':').map(Number);
    return `${hours % 12 || 12}:${String(minutes).padStart(2, '0')} ${hours >= 12 ? 'PM' : 'AM'}`;
  }

  protected dateLabel(value: string): string {
    return new Date(`${value}T12:00:00`).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
  }

  private openToken(token: string): void {
    if (token) window.open(`/kiosk/${encodeURIComponent(token)}`, '_blank', 'noopener');
  }

  private weekEnd(start: string): string {
    const date = new Date(`${start}T12:00:00`);
    date.setDate(date.getDate() + 6);
    return isoDate(date);
  }
}
