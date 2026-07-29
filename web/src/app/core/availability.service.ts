import { HttpClient } from '@angular/common/http';
import { Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { AvailabilityInput, AvailabilityRule } from '../schedule.models';

@Injectable({ providedIn: 'root' })
export class AvailabilityService {
  readonly rules = signal<AvailabilityRule[]>([]);
  readonly loading = signal(false);
  readonly error = signal('');

  constructor(private readonly http: HttpClient) {}

  async load(): Promise<void> {
    this.loading.set(true);
    this.error.set('');
    try {
      this.rules.set(await firstValueFrom(this.http.get<AvailabilityRule[]>('/api/v1/availability')));
    } catch (error) {
      this.error.set(this.message(error, 'Unable to load availability.'));
    } finally {
      this.loading.set(false);
    }
  }

  async save(input: AvailabilityInput, ruleId: string | null): Promise<void> {
    try {
      if (ruleId) {
        await firstValueFrom(this.http.put<AvailabilityRule>(`/api/v1/availability/${ruleId}`, input));
      } else {
        await firstValueFrom(this.http.post<AvailabilityRule>('/api/v1/availability', input));
      }
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to save availability.'));
    }
  }

  async delete(ruleId: string): Promise<void> {
    try {
      await firstValueFrom(this.http.delete<void>(`/api/v1/availability/${ruleId}`));
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to delete availability.'));
    }
  }

  private message(error: unknown, fallback: string): string {
    const response = error as { error?: { message?: string } };
    return response.error?.message ?? fallback;
  }
}
