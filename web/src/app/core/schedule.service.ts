import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { computed, Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { buildDays, currentWeekStart, INITIAL_WEEK, isoDate, mapEmployees } from '../schedule.data';
import { ApiProblem, ApiSchedule, ApiShift, AvailabilityRule, Conflict, Employee, LeaveBlock, OpenShift, PublicHoliday, Shift, ShiftDraft } from '../schedule.models';

@Injectable({ providedIn: 'root' })
export class ScheduleService {
  readonly weekStart = signal(INITIAL_WEEK);
  readonly days = computed(() => buildDays(this.weekStart()));
  readonly employees = signal<Employee[]>([]);
  readonly shifts = signal<Shift[]>([]);
  readonly leave = signal<LeaveBlock[]>([]);
  readonly holidays = signal<PublicHoliday[]>([]);
  readonly openShifts = signal<OpenShift[]>([]);
  readonly conflicts = signal<Conflict[]>([]);
  readonly availability = signal<AvailabilityRule[]>([]);
  readonly status = signal<'draft' | 'published'>('draft');
  readonly version = signal(0);
  readonly loading = signal(false);
  readonly error = signal('');
  readonly hasUnpublishedChanges = computed(() => this.status() === 'draft');
  readonly isCurrentWeek = computed(() => this.weekStart() === currentWeekStart());
  private currentLoad?: Promise<void>;

  constructor(private readonly http: HttpClient) {}

  load(): Promise<void> {
    if (this.currentLoad) return this.currentLoad;
    this.currentLoad = this.performLoad().finally(() => { this.currentLoad = undefined; });
    return this.currentLoad;
  }

  private async performLoad(): Promise<void> {
    this.loading.set(true);
    this.error.set('');
    try {
      const schedule = await firstValueFrom(this.http.get<ApiSchedule>(`/api/v1/schedules/${this.weekStart()}`));
      this.apply(schedule);
    } catch (error) {
      const response = error as HttpErrorResponse;
      if (response.status === 404) {
        this.employees.set([]);
        this.shifts.set([]);
        this.leave.set([]);
        this.holidays.set([]);
        this.openShifts.set([]);
        this.conflicts.set([]);
        this.availability.set([]);
        this.status.set('published');
        this.version.set(0);
        this.error.set('No published schedule is available for this week.');
      } else {
        this.error.set(this.message(error, 'Unable to load the schedule.'));
      }
    } finally {
      this.loading.set(false);
    }
  }

  async moveWeek(offset: number): Promise<void> {
    const date = new Date(`${this.weekStart()}T12:00:00`);
    date.setDate(date.getDate() + offset * 7);
    this.weekStart.set(isoDate(date));
    await this.load();
  }

  async goToCurrentWeek(): Promise<void> {
    if (this.isCurrentWeek()) return;
    this.weekStart.set(currentWeekStart());
    await this.load();
  }

  async evaluate(draft: ShiftDraft, shiftId: string | null): Promise<Conflict[]> {
    const body = this.shiftBody(draft);
    if (shiftId) body['shiftId'] = shiftId;
    try {
      return await firstValueFrom(this.http.post<Conflict[]>(`/api/v1/schedules/${this.weekStart()}/conflicts/evaluate`, body));
    } catch (error) {
      throw new Error(this.message(error, 'Unable to evaluate the shift.'));
    }
  }

  async save(draft: ShiftDraft, shiftId: string | null, overrideConflictCodes: string[] = []): Promise<void> {
    const body = {
      ...this.shiftBody(draft),
      overrideConflictCodes,
    };
    try {
      if (shiftId) {
        await firstValueFrom(this.http.put<ApiShift>(`/api/v1/shifts/${shiftId}`, body));
      } else {
        await firstValueFrom(this.http.post<ApiShift>(`/api/v1/schedules/${this.weekStart()}/shifts`, body));
      }
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to save the shift.'));
    }
  }

  async deleteShift(shiftId: string): Promise<void> {
    try {
      await firstValueFrom(this.http.delete<void>(`/api/v1/shifts/${shiftId}`));
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to delete the shift.'));
    }
  }

  async swapShifts(sourceShiftId: string, targetShiftId: string): Promise<void> {
    try {
      await firstValueFrom(this.http.post('/api/v1/shifts/swap', { sourceShiftId, targetShiftId }));
      await this.load();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to swap the shifts.'));
    }
  }

  async publish(): Promise<void> {
    try {
      const schedule = await firstValueFrom(this.http.post<ApiSchedule>(`/api/v1/schedules/${this.weekStart()}/publish`, {}));
      this.apply(schedule);
    } catch (error) {
      throw new Error(this.message(error, 'Unable to publish the schedule.'));
    }
  }

  private apply(schedule: ApiSchedule): void {
    const days = buildDays(schedule.weekStart);
    this.weekStart.set(schedule.weekStart);
    this.employees.set(mapEmployees(schedule.employees));
    this.shifts.set(schedule.shifts.map((shift) => this.mapShift(shift, days.map((day) => day.isoDate))));
    this.leave.set(schedule.leave ?? []);
    this.holidays.set(schedule.holidays ?? []);
    this.openShifts.set(schedule.openShifts ?? []);
    this.conflicts.set(schedule.conflicts ?? []);
    this.availability.set(schedule.availability ?? []);
    this.status.set(schedule.status);
    this.version.set(schedule.version);
  }

  private mapShift(shift: ApiShift, dates: string[]): Shift {
    return { ...shift, dayIndex: dates.indexOf(shift.date) };
  }

  private shiftBody(draft: ShiftDraft): Record<string, string> {
    return {
      employeeId: draft.employeeId,
      date: this.days()[draft.dayIndex].isoDate,
      start: draft.start,
      end: draft.end,
      note: draft.note,
    };
  }

  private message(error: unknown, fallback: string): string {
    const response = error as HttpErrorResponse;
    return (response.error as ApiProblem | undefined)?.message ?? fallback;
  }
}
