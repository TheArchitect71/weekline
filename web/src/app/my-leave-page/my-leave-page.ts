import { Component, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { LeaveService } from '../core/leave.service';
import { ScheduleService } from '../core/schedule.service';
import { LeaveRequest, LeaveRequestInput } from '../schedule.models';
import { currentWeekStart } from '../schedule.data';

@Component({
  selector: 'app-my-leave-page',
  imports: [FormsModule, RouterLink],
  templateUrl: './my-leave-page.html',
  styleUrl: './my-leave-page.css',
})
export class MyLeavePage implements OnInit {
  protected readonly submitting = signal(false);
  protected readonly actionError = signal('');
  protected form: LeaveRequestInput = this.emptyForm();

  constructor(
    protected readonly leave: LeaveService,
    private readonly schedule: ScheduleService,
  ) {}

  async ngOnInit(): Promise<void> {
    await Promise.all([this.schedule.load(), this.leave.load()]);
    this.form = this.emptyForm();
  }

  protected async submit(): Promise<void> {
    if (this.submitting()) return;
    this.submitting.set(true);
    this.actionError.set('');
    try {
      await this.leave.create(this.form);
      this.form = this.emptyForm();
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.submitting.set(false);
    }
  }

  protected async cancel(request: LeaveRequest): Promise<void> {
    if (this.submitting()) return;
    this.submitting.set(true);
    this.actionError.set('');
    try {
      await this.leave.cancel(request.id);
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.submitting.set(false);
    }
  }

  protected dateRange(request: LeaveRequest): string {
    const start = this.dateLabel(request.startsOn);
    const end = this.dateLabel(request.endsOn);
    return start === end ? start : `${start} – ${end}`;
  }

  protected statusMessage(request: LeaveRequest): string {
    if (request.status === 'approved') return 'Approved and added to the schedule.';
    if (request.status === 'rejected') return 'Your manager declined this request.';
    if (request.status === 'cancelled') return 'You cancelled this request.';
    return 'Waiting for manager review.';
  }

  protected dateLabel(value: string): string {
    return new Date(`${value}T12:00:00`).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  }

  private emptyForm(): LeaveRequestInput {
    const date = currentWeekStart();
    return { leaveTypeId: '', startsOn: date, endsOn: date, hoursPerDay: 8, reason: '' };
  }
}
