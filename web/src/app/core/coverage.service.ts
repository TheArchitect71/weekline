import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { EmployeeSuggestion, OpenShift, OpenShiftInput } from '../schedule.models';
import { ScheduleService } from './schedule.service';

@Injectable({ providedIn: 'root' })
export class CoverageService {
  constructor(
    private readonly http: HttpClient,
    private readonly schedule: ScheduleService,
  ) {}

  async create(input: OpenShiftInput): Promise<void> {
    try {
      await firstValueFrom(this.http.post<OpenShift>(`/api/v1/schedules/${this.schedule.weekStart()}/open-shifts`, input));
      await this.schedule.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to create the open shift.'));
    }
  }

  async update(id: string, input: OpenShiftInput): Promise<void> {
    try {
      await firstValueFrom(this.http.put<OpenShift>(`/api/v1/open-shifts/${id}`, input));
      await this.schedule.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to update the open shift.'));
    }
  }

  async delete(id: string): Promise<void> {
    try {
      await firstValueFrom(this.http.delete<void>(`/api/v1/open-shifts/${id}`));
      await this.schedule.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to delete the open shift.'));
    }
  }

  async suggestions(id: string): Promise<EmployeeSuggestion[]> {
    try {
      return await firstValueFrom(this.http.get<EmployeeSuggestion[]>(`/api/v1/open-shifts/${id}/suggestions`));
    } catch (error) {
      throw new Error(this.message(error, 'Unable to find available employees.'));
    }
  }

  async assign(id: string, employeeId: string): Promise<void> {
    try {
      await firstValueFrom(this.http.post(`/api/v1/open-shifts/${id}/assign`, { employeeId }));
      await this.schedule.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to assign the open shift.'));
    }
  }

  private message(error: unknown, fallback: string): string {
    const response = error as { error?: { message?: string } };
    return response.error?.message ?? fallback;
  }
}
