import { Component, OnInit, signal } from '@angular/core';
import { ShiftRequestService } from '../core/shift-request.service';
import { ScheduleService } from '../core/schedule.service';
import { ShiftRequest } from '../schedule.models';

@Component({
  selector: 'app-requests-page',
  templateUrl: './requests-page.html',
  styleUrl: './requests-page.css',
})
export class RequestsPage implements OnInit {
  protected readonly actingOn = signal<string | null>(null);
  protected readonly actionError = signal('');

  constructor(
    protected readonly shiftRequests: ShiftRequestService,
    private readonly schedule: ScheduleService,
  ) {}

  async ngOnInit(): Promise<void> {
    await this.shiftRequests.load();
  }

  protected pendingCount(): number {
    return this.shiftRequests.requests().filter((request) => request.status === 'pending').length;
  }

  protected async decide(request: ShiftRequest, decision: 'approve' | 'reject'): Promise<void> {
    if (this.actingOn()) return;
    this.actingOn.set(request.id);
    this.actionError.set('');
    try {
      await this.shiftRequests.decide(request.id, decision);
      if (decision === 'approve') await this.schedule.load();
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.actingOn.set(null);
    }
  }

  protected dateTime(date: string, start: string, end: string): string {
    return `${this.dateLabel(date)} · ${this.time(start)}–${this.time(end)}`;
  }

  protected dateLabel(value: string): string {
    return new Date(`${value}T12:00:00`).toLocaleDateString('en-US', {
      weekday: 'short', month: 'short', day: 'numeric',
    });
  }

  protected time(value: string): string {
    const [hours, minutes] = value.split(':').map(Number);
    return `${hours % 12 || 12}:${String(minutes).padStart(2, '0')} ${hours >= 12 ? 'PM' : 'AM'}`;
  }
}
