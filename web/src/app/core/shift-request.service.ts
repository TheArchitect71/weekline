import { HttpClient } from '@angular/common/http';
import { Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { ShiftRequest, SwapOption } from '../schedule.models';

@Injectable({ providedIn: 'root' })
export class ShiftRequestService {
  readonly requests = signal<ShiftRequest[]>([]);
  readonly loading = signal(false);
  readonly error = signal('');

  constructor(private readonly http: HttpClient) {}

  async load(): Promise<void> {
    this.loading.set(true);
    this.error.set('');
    try {
      this.requests.set(await firstValueFrom(this.http.get<ShiftRequest[]>('/api/v1/shift-requests')));
    } catch (error) {
      this.error.set(this.message(error, 'Unable to load shift requests.'));
    } finally {
      this.loading.set(false);
    }
  }

  async swapOptions(sourceShiftId: string): Promise<SwapOption[]> {
    return firstValueFrom(
      this.http.get<SwapOption[]>('/api/v1/shift-requests/swap-options', { params: { sourceShiftId } }),
    );
  }

  async createSwap(sourceShiftId: string, targetShiftId: string): Promise<void> {
    try {
      await firstValueFrom(
        this.http.post<ShiftRequest>('/api/v1/shift-requests/swaps', { sourceShiftId, targetShiftId }),
      );
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to request the shift swap.'));
    }
  }

  async decide(requestId: string, decision: 'approve' | 'reject'): Promise<void> {
    try {
      await firstValueFrom(
        this.http.post<ShiftRequest>(`/api/v1/shift-requests/${requestId}/${decision}`, {}),
      );
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, `Unable to ${decision} the shift request.`));
    }
  }

  async cancel(requestId: string): Promise<void> {
    try {
      await firstValueFrom(
        this.http.post<ShiftRequest>(`/api/v1/shift-requests/${requestId}/cancel`, {}),
      );
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to cancel the shift request.'));
    }
  }

  async claimOpenShift(openShiftId: string): Promise<void> {
    try {
      await firstValueFrom(
        this.http.post<ShiftRequest>(`/api/v1/open-shifts/${openShiftId}/claim`, {}),
      );
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to claim the open shift.'));
    }
  }

  private message(error: unknown, fallback: string): string {
    const response = error as { error?: { message?: string } };
    return response.error?.message ?? fallback;
  }
}
