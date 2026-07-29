import { HttpClient } from '@angular/common/http';
import { Component, OnInit, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { AuditEvent } from '../schedule.models';

@Component({ selector: 'app-history-page', templateUrl: './history-page.html' })
export class HistoryPage implements OnInit {
  protected readonly events = signal<AuditEvent[]>([]);
  protected readonly loading = signal(true);
  constructor(private readonly http: HttpClient) {}
  async ngOnInit(): Promise<void> {
    try { this.events.set(await firstValueFrom(this.http.get<AuditEvent[]>('/api/v1/audit'))); }
    finally { this.loading.set(false); }
  }
  protected format(value: string): string {
    return new Date(`${value}-06:00`).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });
  }
}
