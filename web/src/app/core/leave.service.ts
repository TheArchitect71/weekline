import { HttpClient } from '@angular/common/http';
import { Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { LeaveBalance, LeaveRequest, LeaveRequestInput, LeaveType, PublicHoliday } from '../schedule.models';

@Injectable({ providedIn: 'root' })
export class LeaveService {
  readonly types = signal<LeaveType[]>([]);
  readonly balances = signal<LeaveBalance[]>([]);
  readonly requests = signal<LeaveRequest[]>([]);
  readonly holidays = signal<PublicHoliday[]>([]);
  readonly loading = signal(false);
  readonly error = signal('');

  constructor(private readonly http: HttpClient) {}

  async load(): Promise<void> {
    this.loading.set(true);
    this.error.set('');
    try {
      const [types, balances, requests, holidays] = await Promise.all([
        firstValueFrom(this.http.get<LeaveType[]>('/api/v1/leave/types')),
        firstValueFrom(this.http.get<LeaveBalance[]>('/api/v1/leave/balances')),
        firstValueFrom(this.http.get<LeaveRequest[]>('/api/v1/leave/requests')),
        firstValueFrom(this.http.get<PublicHoliday[]>('/api/v1/holidays')),
      ]);
      this.types.set(types);
      this.balances.set(balances);
      this.requests.set(requests);
      this.holidays.set(holidays);
    } catch (error) {
      this.error.set(this.message(error, 'Unable to load time-off information.'));
    } finally {
      this.loading.set(false);
    }
  }

  async create(input: LeaveRequestInput): Promise<void> {
    try {
      await firstValueFrom(this.http.post<LeaveRequest>('/api/v1/leave/requests', input));
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to create the leave request.'));
    }
  }

  async decide(requestId: string, decision: 'approve' | 'reject'): Promise<void> {
    try {
      await firstValueFrom(this.http.post<LeaveRequest>(`/api/v1/leave/requests/${requestId}/${decision}`, {}));
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, `Unable to ${decision} the leave request.`));
    }
  }

  async cancel(requestId: string): Promise<void> {
    try {
      await firstValueFrom(this.http.post<LeaveRequest>(`/api/v1/leave/requests/${requestId}/cancel`, {}));
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to cancel the leave request.'));
    }
  }

  async setBalance(employeeId: string, leaveTypeId: string, balanceHours: number): Promise<void> {
    try {
      await firstValueFrom(this.http.put<LeaveBalance>(`/api/v1/leave/balances/${employeeId}/${leaveTypeId}`, { balanceHours }));
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to update the leave balance.'));
    }
  }

  async addHoliday(input: Omit<PublicHoliday, 'id'>): Promise<void> {
    try {
      await firstValueFrom(this.http.post<PublicHoliday>('/api/v1/holidays', input));
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to add the holiday.'));
    }
  }

  async deleteHoliday(id: string): Promise<void> {
    try {
      await firstValueFrom(this.http.delete<void>(`/api/v1/holidays/${id}`));
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to remove the holiday.'));
    }
  }

  private message(error: unknown, fallback: string): string {
    const response = error as { error?: { message?: string } };
    return response.error?.message ?? fallback;
  }
}
