import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnDestroy, OnInit, signal } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { AttendanceService } from '../core/attendance.service';
import { KioskEmployee, KioskSession, PunchInput } from '../schedule.models';

@Component({
  selector: 'app-kiosk-page',
  templateUrl: './kiosk-page.html',
  styleUrl: './kiosk-page.css',
})
export class KioskPage implements OnInit, OnDestroy {
  protected readonly session = signal<KioskSession | null>(null);
  protected readonly selected = signal<KioskEmployee | null>(null);
  protected readonly currentTime = signal(new Date());
  protected readonly status = signal('');
  protected readonly error = signal('');
  protected readonly sending = signal(false);
  protected readonly queuedCount = signal(0);
  private readonly token: string;
  private readonly clockTimer: number;
  private readonly onlineHandler = () => void this.flushQueue();

  constructor(
    route: ActivatedRoute,
    private readonly attendance: AttendanceService,
  ) {
    this.token = route.snapshot.paramMap.get('token') ?? '';
    this.clockTimer = window.setInterval(() => this.currentTime.set(new Date()), 1000);
  }

  async ngOnInit(): Promise<void> {
    window.addEventListener('online', this.onlineHandler);
    const cached = localStorage.getItem(this.sessionKey());
    if (cached) this.session.set(JSON.parse(cached) as KioskSession);
    this.updateQueueCount();
    try {
      await this.refreshSession();
      await this.flushQueue();
    } catch (error) {
      const response = error as HttpErrorResponse;
      if (response.status !== 0 && navigator.onLine) {
        this.session.set(null);
        localStorage.removeItem(this.sessionKey());
      }
      if (!this.session()) this.error.set(this.message(error, 'Station is unavailable.'));
    }
  }

  ngOnDestroy(): void {
    window.clearInterval(this.clockTimer);
    window.removeEventListener('online', this.onlineHandler);
  }

  protected choose(employee: KioskEmployee): void {
    this.selected.set(employee);
    this.status.set('');
    this.error.set('');
  }

  protected async punch(eventType: 'in' | 'out'): Promise<void> {
    const employee = this.selected();
    if (!employee || this.sending()) return;
    this.sending.set(true);
    this.error.set('');
    const input: PunchInput = {
      employeeId: employee.id,
      eventType,
      clientEventId: crypto.randomUUID(),
      occurredAt: new Date().toISOString(),
    };
    try {
      await this.attendance.punch(this.token, input);
      this.markPunched(employee.id, eventType);
      this.status.set(`${employee.displayName} clocked ${eventType}.`);
    } catch (error) {
      const response = error as HttpErrorResponse;
      if (response.status === 0 || !navigator.onLine) {
        this.enqueue(input);
        this.markPunched(employee.id, eventType);
        this.status.set(`${employee.displayName} punch queued.`);
      } else {
        this.error.set(this.message(error, 'Punch could not be recorded.'));
      }
    } finally {
      this.selected.set(null);
      this.sending.set(false);
    }
  }

  protected dateLabel(): string {
    return this.currentTime().toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric' });
  }

  protected timeLabel(): string {
    return this.currentTime().toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit', second: '2-digit' });
  }

  private async refreshSession(): Promise<void> {
    const session = await this.attendance.loadKiosk(this.token);
    this.session.set(session);
    localStorage.setItem(this.sessionKey(), JSON.stringify(session));
  }

  private enqueue(input: PunchInput): void {
    const queue = this.queue();
    queue.push(input);
    localStorage.setItem(this.queueKey(), JSON.stringify(queue));
    this.updateQueueCount();
  }

  private async flushQueue(): Promise<void> {
    if (!navigator.onLine) return;
    const queue = this.queue();
    const remaining: PunchInput[] = [];
    for (let index = 0; index < queue.length; index++) {
      try {
        await this.attendance.punch(this.token, queue[index]);
      } catch (error) {
        const response = error as HttpErrorResponse;
        if (response.status === 0) {
          remaining.push(...queue.slice(index));
          break;
        }
        this.error.set(this.message(error, 'A queued punch was rejected.'));
      }
    }
    localStorage.setItem(this.queueKey(), JSON.stringify(remaining));
    this.updateQueueCount();
    if (!remaining.length && queue.length) await this.refreshSession();
  }

  private markPunched(employeeId: string, eventType: 'in' | 'out'): void {
    const session = this.session();
    if (!session) return;
    const updated = {
      ...session,
      employees: session.employees.map((employee) => employee.id === employeeId ? { ...employee, lastEventType: eventType } : employee),
    } as KioskSession;
    this.session.set(updated);
    localStorage.setItem(this.sessionKey(), JSON.stringify(updated));
  }

  private queue(): PunchInput[] {
    return JSON.parse(localStorage.getItem(this.queueKey()) ?? '[]') as PunchInput[];
  }

  private updateQueueCount(): void {
    this.queuedCount.set(this.queue().length);
  }

  private queueKey(): string { return `weekline-kiosk-queue-${this.token}`; }
  private sessionKey(): string { return `weekline-kiosk-session-${this.token}`; }

  private message(error: unknown, fallback: string): string {
    const response = error as HttpErrorResponse;
    return (response.error as { message?: string } | undefined)?.message ?? fallback;
  }
}
