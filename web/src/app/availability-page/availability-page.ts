import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { AvailabilityService } from '../core/availability.service';
import { AuthService } from '../core/auth.service';
import { ScheduleService } from '../core/schedule.service';
import { AvailabilityInput, AvailabilityKind, AvailabilityRule } from '../schedule.models';

@Component({
  selector: 'app-availability-page',
  imports: [FormsModule, RouterLink],
  templateUrl: './availability-page.html',
  styleUrl: './availability-page.css',
})
export class AvailabilityPage implements OnInit {
  protected readonly availability = inject(AvailabilityService);
  protected readonly schedule = inject(ScheduleService);
  private readonly auth = inject(AuthService);
  protected readonly isManager = computed(() => this.auth.user()?.role === 'manager');
  protected readonly acting = signal(false);
  protected readonly actionError = signal('');
  protected readonly days = [
    { value: 1, label: 'Monday' }, { value: 2, label: 'Tuesday' },
    { value: 3, label: 'Wednesday' }, { value: 4, label: 'Thursday' },
    { value: 5, label: 'Friday' }, { value: 6, label: 'Saturday' },
    { value: 7, label: 'Sunday' },
  ];
  protected formOpen = false;
  protected editingId: string | null = null;
  protected form: AvailabilityInput = this.emptyForm();

  async ngOnInit(): Promise<void> {
    const requests: Promise<void>[] = [this.availability.load()];
    if (this.isManager()) requests.push(this.schedule.load());
    await Promise.all(requests);
    this.form = this.emptyForm();
    this.formOpen = !this.isManager();
  }

  protected startCreate(): void {
    this.editingId = null;
    this.form = this.emptyForm();
    this.formOpen = true;
    this.actionError.set('');
  }

  protected startEdit(rule: AvailabilityRule): void {
    this.editingId = rule.id;
    this.form = {
      employeeId: rule.employeeId,
      kind: rule.kind,
      dayOfWeek: rule.dayOfWeek,
      start: rule.start,
      end: rule.end,
      effectiveFrom: rule.effectiveFrom ?? '',
      effectiveUntil: rule.effectiveUntil ?? '',
      note: rule.note,
    };
    this.formOpen = true;
    this.actionError.set('');
  }

  protected setKind(kind: AvailabilityKind): void {
    this.form.kind = kind;
  }

  protected async save(): Promise<void> {
    if (this.acting()) return;
    this.acting.set(true);
    this.actionError.set('');
    try {
      await this.availability.save(this.form, this.editingId);
      this.editingId = null;
      this.form = this.emptyForm();
      this.formOpen = !this.isManager();
      await this.schedule.load();
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set(false);
    }
  }

  protected async remove(rule: AvailabilityRule): Promise<void> {
    if (this.acting() || !window.confirm('Delete this availability rule?')) return;
    this.acting.set(true);
    this.actionError.set('');
    try {
      await this.availability.delete(rule.id);
      await this.schedule.load();
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set(false);
    }
  }

  protected dayName(value: number): string {
    return this.days.find((day) => day.value === value)?.label ?? '';
  }

  protected time(value: string): string {
    const [hours, minutes] = value.split(':').map(Number);
    return `${hours % 12 || 12}:${String(minutes).padStart(2, '0')} ${hours >= 12 ? 'PM' : 'AM'}`;
  }

  protected dateScope(rule: AvailabilityRule): string {
    if (rule.effectiveFrom && rule.effectiveUntil) return `${rule.effectiveFrom} to ${rule.effectiveUntil}`;
    if (rule.effectiveFrom) return `Starting ${rule.effectiveFrom}`;
    if (rule.effectiveUntil) return `Through ${rule.effectiveUntil}`;
    return 'Every week';
  }

  private emptyForm(): AvailabilityInput {
    return {
      employeeId: this.isManager() ? this.schedule.employees()[0]?.id ?? '' : undefined,
      kind: 'unavailable', dayOfWeek: 1, start: '09:00', end: '17:00',
      effectiveFrom: '', effectiveUntil: '', note: '',
    };
  }
}
