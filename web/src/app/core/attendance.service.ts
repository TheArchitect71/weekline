import { HttpClient } from '@angular/common/http';
import { Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import {
  AttendanceCorrection,
  AttendanceCorrectionInput,
  AttendanceEvent,
  AttendanceRecord,
  AttendanceStation,
  KioskSession,
  PunchInput,
} from '../schedule.models';

@Injectable({ providedIn: 'root' })
export class AttendanceService {
  readonly records = signal<AttendanceRecord[]>([]);
  readonly corrections = signal<AttendanceCorrection[]>([]);
  readonly stations = signal<AttendanceStation[]>([]);
  readonly loading = signal(false);
  readonly error = signal('');

  constructor(private readonly http: HttpClient) {}

  async loadRecords(from: string, to: string, employeeId = ''): Promise<void> {
    this.loading.set(true);
    this.error.set('');
    try {
      const params: Record<string, string> = { from, to };
      if (employeeId) params['employeeId'] = employeeId;
      this.records.set(await firstValueFrom(this.http.get<AttendanceRecord[]>('/api/v1/attendance', { params })));
    } catch (error) {
      this.error.set(this.message(error, 'Unable to load attendance.'));
    } finally {
      this.loading.set(false);
    }
  }

  async loadCorrections(): Promise<void> {
    try {
      this.corrections.set(await firstValueFrom(this.http.get<AttendanceCorrection[]>('/api/v1/attendance/corrections')));
    } catch (error) {
      this.error.set(this.message(error, 'Unable to load corrections.'));
    }
  }

  async loadStations(): Promise<void> {
    try {
      this.stations.set(await firstValueFrom(this.http.get<AttendanceStation[]>('/api/v1/attendance/stations')));
    } catch (error) {
      this.error.set(this.message(error, 'Unable to load stations.'));
    }
  }

  async createStation(name: string): Promise<AttendanceStation> {
    try {
      const station = await firstValueFrom(this.http.post<AttendanceStation>('/api/v1/attendance/stations', { name }));
      this.rememberToken(station);
      await this.loadStations();
      return station;
    } catch (error) {
      throw new Error(this.message(error, 'Unable to create the station.'));
    }
  }

  async updateStation(station: AttendanceStation, active: boolean): Promise<void> {
    try {
      await firstValueFrom(this.http.put<AttendanceStation>(`/api/v1/attendance/stations/${station.id}`, { name: station.name, active }));
      await this.loadStations();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to update the station.'));
    }
  }

  async rotateStation(stationId: string): Promise<AttendanceStation> {
    try {
      const station = await firstValueFrom(this.http.post<AttendanceStation>(`/api/v1/attendance/stations/${stationId}/rotate-token`, {}));
      this.rememberToken(station);
      return station;
    } catch (error) {
      throw new Error(this.message(error, 'Unable to rotate station access.'));
    }
  }

  tokenFor(stationId: string): string {
    return localStorage.getItem(`weekline-station-${stationId}`) ?? '';
  }

  async createCorrection(input: AttendanceCorrectionInput): Promise<void> {
    try {
      await firstValueFrom(this.http.post<AttendanceCorrection>('/api/v1/attendance/corrections', input));
      await this.loadCorrections();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to request the correction.'));
    }
  }

  async decideCorrection(correctionId: string, decision: 'approve' | 'reject'): Promise<void> {
    try {
      await firstValueFrom(this.http.post<AttendanceCorrection>(`/api/v1/attendance/corrections/${correctionId}/${decision}`, {}));
      await this.loadCorrections();
    } catch (error) {
      throw new Error(this.message(error, `Unable to ${decision} the correction.`));
    }
  }

  async cancelCorrection(correctionId: string): Promise<void> {
    try {
      await firstValueFrom(this.http.post<AttendanceCorrection>(`/api/v1/attendance/corrections/${correctionId}/cancel`, {}));
      await this.loadCorrections();
    } catch (error) {
      throw new Error(this.message(error, 'Unable to cancel the correction.'));
    }
  }

  loadKiosk(token: string): Promise<KioskSession> {
    return firstValueFrom(this.http.get<KioskSession>(`/api/v1/kiosk/${encodeURIComponent(token)}`));
  }

  punch(token: string, input: PunchInput): Promise<AttendanceEvent> {
    return firstValueFrom(this.http.post<AttendanceEvent>(`/api/v1/kiosk/${encodeURIComponent(token)}/punch`, input));
  }

  private rememberToken(station: AttendanceStation): void {
    if (station.kioskToken) localStorage.setItem(`weekline-station-${station.id}`, station.kioskToken);
  }

  private message(error: unknown, fallback: string): string {
    const response = error as { error?: { message?: string } };
    return response.error?.message ?? fallback;
  }
}
