import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { AuthService } from '../core/auth.service';
import { ScheduleService } from '../core/schedule.service';
import { ShiftRequestService } from '../core/shift-request.service';
import { Shift, ShiftRequest, SwapOption } from '../schedule.models';

@Component({
  selector: 'app-my-requests-page',
  imports: [FormsModule, RouterLink],
  templateUrl: './my-requests-page.html',
  styleUrl: './my-requests-page.css',
})
export class MyRequestsPage implements OnInit {
  protected readonly schedule = inject(ScheduleService);
  protected readonly shiftRequests = inject(ShiftRequestService);
  private readonly auth = inject(AuthService);
  protected readonly sourceShiftId = signal('');
  protected readonly targetShiftId = signal('');
  protected readonly options = signal<SwapOption[]>([]);
  protected readonly loadingOptions = signal(false);
  protected readonly submitting = signal(false);
  protected readonly actionError = signal('');
  protected readonly ownShifts = computed(() =>
    this.schedule.shifts().filter((shift) => shift.employeeId === this.auth.user()?.id),
  );

  async ngOnInit(): Promise<void> {
    await Promise.all([this.schedule.load(), this.shiftRequests.load()]);
  }

  protected async sourceChanged(): Promise<void> {
    this.targetShiftId.set('');
    this.options.set([]);
    this.actionError.set('');
    if (!this.sourceShiftId()) return;
    this.loadingOptions.set(true);
    try {
      this.options.set(await this.shiftRequests.swapOptions(this.sourceShiftId()));
    } catch (error) {
      this.actionError.set(this.message(error, 'Unable to find eligible swap options.'));
    } finally {
      this.loadingOptions.set(false);
    }
  }

  protected async submit(): Promise<void> {
    if (!this.sourceShiftId() || !this.targetShiftId() || this.submitting()) return;
    this.submitting.set(true);
    this.actionError.set('');
    try {
      await this.shiftRequests.createSwap(this.sourceShiftId(), this.targetShiftId());
      this.sourceShiftId.set('');
      this.targetShiftId.set('');
      this.options.set([]);
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.submitting.set(false);
    }
  }

  protected async cancel(request: ShiftRequest): Promise<void> {
    if (this.submitting()) return;
    this.submitting.set(true);
    this.actionError.set('');
    try {
      await this.shiftRequests.cancel(request.id);
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.submitting.set(false);
    }
  }

  protected async claimOpenShift(openShiftId: string): Promise<void> {
    if (this.submitting()) return;
    this.submitting.set(true);
    this.actionError.set('');
    try {
      await this.shiftRequests.claimOpenShift(openShiftId);
      await this.schedule.load();
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.submitting.set(false);
    }
  }

  protected shiftLabel(shift: Shift): string {
    return `${this.dateLabel(shift.date)} · ${this.time(shift.start)}–${this.time(shift.end)}`;
  }

  protected optionLabel(option: SwapOption): string {
    return `${option.employeeName} · ${this.dateLabel(option.date)} · ${this.time(option.start)}–${this.time(option.end)}`;
  }

  protected requestSummary(request: ShiftRequest): string {
    if (request.requestType === 'open_claim') {
      return `Open shift · ${this.dateLabel(request.sourceDate)} ${this.time(request.sourceStart)}–${this.time(request.sourceEnd)}`;
    }
    return `${this.dateLabel(request.sourceDate)} ${this.time(request.sourceStart)} ↔ ${request.targetEmployeeName}, ${this.dateLabel(request.targetDate!)} ${this.time(request.targetStart!)}`;
  }

  protected statusMessage(status: ShiftRequest['status']): string {
    if (status === 'approved') return 'Approved. The manager will publish the updated schedule.';
    if (status === 'rejected') return 'The manager declined this request.';
    if (status === 'cancelled') return 'You cancelled this request.';
    if (status === 'expired') return 'This request expired after 72 hours.';
    return 'Waiting for manager review.';
  }

  protected dateLabel(value: string): string {
    return new Date(`${value}T12:00:00`).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
  }

  protected time(value: string): string {
    const [hours, minutes] = value.split(':').map(Number);
    return `${hours % 12 || 12}:${String(minutes).padStart(2, '0')} ${hours >= 12 ? 'PM' : 'AM'}`;
  }

  private message(error: unknown, fallback: string): string {
    const response = error as { error?: { message?: string } };
    return response.error?.message ?? fallback;
  }
}
