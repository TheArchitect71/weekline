import { Component, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { LeaveService } from '../core/leave.service';
import { ScheduleService } from '../core/schedule.service';
import { LeaveBalance, LeaveRequest, LeaveRequestInput } from '../schedule.models';
import { currentWeekStart } from '../schedule.data';

@Component({
  selector: 'app-leave-page',
  imports: [FormsModule],
  templateUrl: './leave-page.html',
  styleUrl: './leave-page.css',
})
export class LeavePage implements OnInit {
  protected readonly acting = signal('');
  protected readonly actionError = signal('');
  protected requestFormOpen = false;
  protected holidayFormOpen = false;
  protected requestForm: LeaveRequestInput = this.emptyRequest();
  protected holidayForm = { date: '', name: '', paid: true };
  protected balanceDrafts: Record<string, number> = {};

  constructor(
    protected readonly leave: LeaveService,
    protected readonly schedule: ScheduleService,
  ) {}

  async ngOnInit(): Promise<void> {
    await Promise.all([this.schedule.load(), this.leave.load()]);
    this.requestForm = this.emptyRequest();
  }

  protected pendingCount(): number {
    return this.leave.requests().filter((request) => request.status === 'pending').length;
  }

  protected async createRequest(): Promise<void> {
    if (this.acting()) return;
    this.acting.set('request');
    this.actionError.set('');
    try {
      await this.leave.create(this.requestForm);
      this.requestForm = this.emptyRequest();
      this.requestFormOpen = false;
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set('');
    }
  }

  protected async decide(request: LeaveRequest, decision: 'approve' | 'reject'): Promise<void> {
    if (this.acting()) return;
    this.acting.set(request.id);
    this.actionError.set('');
    try {
      await this.leave.decide(request.id, decision);
      if (decision === 'approve') await this.schedule.load();
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set('');
    }
  }

  protected async cancelApproved(request: LeaveRequest): Promise<void> {
    if (this.acting()) return;
    this.acting.set(request.id);
    this.actionError.set('');
    try {
      await this.leave.cancel(request.id);
      await this.schedule.load();
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set('');
    }
  }

  protected balanceKey(balance: LeaveBalance): string {
    return `${balance.employeeId}:${balance.leaveTypeId}`;
  }

  protected balanceValue(balance: LeaveBalance): number {
    return this.balanceDrafts[this.balanceKey(balance)] ?? balance.balanceHours;
  }

  protected setBalanceDraft(balance: LeaveBalance, value: number): void {
    this.balanceDrafts[this.balanceKey(balance)] = value;
  }

  protected async saveBalance(balance: LeaveBalance): Promise<void> {
    const key = this.balanceKey(balance);
    if (this.acting()) return;
    this.acting.set(key);
    this.actionError.set('');
    try {
      await this.leave.setBalance(balance.employeeId, balance.leaveTypeId, this.balanceValue(balance));
      delete this.balanceDrafts[key];
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set('');
    }
  }

  protected async addHoliday(): Promise<void> {
    if (this.acting()) return;
    this.acting.set('holiday');
    this.actionError.set('');
    try {
      await this.leave.addHoliday(this.holidayForm);
      this.holidayForm = { date: '', name: '', paid: true };
      this.holidayFormOpen = false;
      await this.schedule.load();
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set('');
    }
  }

  protected async deleteHoliday(id: string): Promise<void> {
    if (this.acting()) return;
    this.acting.set(id);
    this.actionError.set('');
    try {
      await this.leave.deleteHoliday(id);
      await this.schedule.load();
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set('');
    }
  }

  protected dateRange(request: LeaveRequest): string {
    const start = this.dateLabel(request.startsOn);
    const end = this.dateLabel(request.endsOn);
    return start === end ? start : `${start} – ${end}`;
  }

  protected dateLabel(value: string): string {
    return new Date(`${value}T12:00:00`).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  }

  private emptyRequest(): LeaveRequestInput {
    const date = currentWeekStart();
    return { employeeId: '', leaveTypeId: '', startsOn: date, endsOn: date, hoursPerDay: 8, reason: '' };
  }
}
