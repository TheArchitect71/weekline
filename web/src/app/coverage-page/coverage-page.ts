import { Component, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { CoverageService } from '../core/coverage.service';
import { ScheduleService } from '../core/schedule.service';
import { EmployeeSuggestion, OpenShift, OpenShiftInput } from '../schedule.models';
import { currentWeekStart } from '../schedule.data';

@Component({
  selector: 'app-coverage-page',
  imports: [FormsModule],
  templateUrl: './coverage-page.html',
  styleUrl: './coverage-page.css',
})
export class CoveragePage implements OnInit {
  protected readonly acting = signal('');
  protected readonly actionError = signal('');
  protected readonly suggestions = signal<EmployeeSuggestion[]>([]);
  protected formOpen = false;
  protected editingId: string | null = null;
  protected assigningId: string | null = null;
  protected selectedEmployeeId = '';
  protected form: OpenShiftInput = this.emptyForm();

  constructor(
    protected readonly schedule: ScheduleService,
    private readonly coverage: CoverageService,
  ) {}

  async ngOnInit(): Promise<void> {
    await this.schedule.load();
    this.form = this.emptyForm();
  }

  protected totalGap(): number {
    return this.schedule.openShifts().reduce((sum, shift) => sum + shift.coverageGap, 0);
  }

  protected startCreate(): void {
    this.form = this.emptyForm();
    this.editingId = null;
    this.formOpen = true;
    this.actionError.set('');
  }

  protected startEdit(openShift: OpenShift): void {
    this.form = {
      date: openShift.date, start: openShift.start, end: openShift.end,
      note: openShift.note, requiredHeadcount: openShift.requiredHeadcount,
    };
    this.editingId = openShift.id;
    this.formOpen = true;
    this.assigningId = null;
    this.actionError.set('');
  }

  protected async save(): Promise<void> {
    if (this.acting()) return;
    this.acting.set('save');
    this.actionError.set('');
    try {
      if (this.editingId) await this.coverage.update(this.editingId, this.form);
      else await this.coverage.create(this.form);
      this.formOpen = false;
      this.editingId = null;
      this.form = this.emptyForm();
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set('');
    }
  }

  protected async remove(openShift: OpenShift): Promise<void> {
    if (this.acting()) return;
    this.acting.set(openShift.id);
    this.actionError.set('');
    try {
      await this.coverage.delete(openShift.id);
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set('');
    }
  }

  protected async startAssign(openShift: OpenShift): Promise<void> {
    if (this.acting()) return;
    this.acting.set(openShift.id);
    this.actionError.set('');
    try {
      this.suggestions.set(await this.coverage.suggestions(openShift.id));
      this.assigningId = openShift.id;
      this.selectedEmployeeId = '';
      this.formOpen = false;
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set('');
    }
  }

  protected async assign(): Promise<void> {
    if (!this.assigningId || !this.selectedEmployeeId || this.acting()) return;
    this.acting.set(this.assigningId);
    this.actionError.set('');
    try {
      await this.coverage.assign(this.assigningId, this.selectedEmployeeId);
      this.assigningId = null;
      this.selectedEmployeeId = '';
      this.suggestions.set([]);
    } catch (error) {
      this.actionError.set((error as Error).message);
    } finally {
      this.acting.set('');
    }
  }

  protected dateLabel(value: string): string {
    return new Date(`${value}T12:00:00`).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
  }

  protected time(value: string): string {
    const [hours, minutes] = value.split(':').map(Number);
    return `${hours % 12 || 12}:${String(minutes).padStart(2, '0')} ${hours >= 12 ? 'PM' : 'AM'}`;
  }

  private emptyForm(): OpenShiftInput {
    return { date: currentWeekStart(), start: '09:00', end: '17:00', note: '', requiredHeadcount: 1 };
  }
}
