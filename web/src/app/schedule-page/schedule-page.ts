import { Component, OnInit, signal } from '@angular/core';
import { ScheduleService } from '../core/schedule.service';
import { ManagerSchedule } from '../manager-schedule/manager-schedule';
import { Conflict, Shift, ShiftDraft, ShiftDropAction } from '../schedule.models';
import { ShiftEditor } from '../shift-editor/shift-editor';

@Component({
  selector: 'app-schedule-page',
  imports: [ManagerSchedule, ShiftEditor],
  templateUrl: './schedule-page.html',
  styleUrl: './schedule-page.css',
})
export class SchedulePage implements OnInit {
  protected readonly selectedShiftId = signal<string | null>(null);
  protected readonly editorOpen = signal(false);
  protected readonly isCreating = signal(false);
  protected readonly draft = signal<ShiftDraft>({ employeeId: '', dayIndex: 0, start: '09:00', end: '17:00', note: '' });
  protected readonly saveError = signal('');
  protected readonly saving = signal(false);
  protected readonly preflightConflicts = signal<Conflict[]>([]);
  protected readonly dragMessage = signal('');
  protected readonly dragError = signal('');
  protected readonly draggingAction = signal(false);
  private pendingDraft: ShiftDraft | null = null;

  constructor(protected readonly schedule: ScheduleService) {}

  async ngOnInit(): Promise<void> { await this.schedule.load(); }

  protected selectShift(shift: Shift): void {
    this.selectedShiftId.set(shift.id);
    this.isCreating.set(false);
    this.saveError.set('');
    this.clearConflictResults();
    this.draft.set({ employeeId: shift.employeeId, dayIndex: shift.dayIndex, start: shift.start, end: shift.end, note: shift.note });
    this.editorOpen.set(true);
  }

  protected createShift(location: { employeeId: string; dayIndex: number }): void {
    this.selectedShiftId.set(null);
    this.isCreating.set(true);
    this.saveError.set('');
    this.clearConflictResults();
    this.draft.set({ employeeId: location.employeeId, dayIndex: location.dayIndex, start: '09:00', end: '17:00', note: '' });
    this.editorOpen.set(true);
  }

  protected async saveShift(draft: ShiftDraft): Promise<void> {
    this.saving.set(true);
    this.saveError.set('');
    try {
	  const conflicts = await this.schedule.evaluate(draft, this.selectedShiftId());
	  if (conflicts.length) {
	    this.pendingDraft = { ...draft };
	    this.preflightConflicts.set(conflicts);
	    return;
	  }
	  await this.commitShift(draft, []);
    } catch (error) {
      this.saveError.set((error as Error).message);
    } finally {
      this.saving.set(false);
    }
  }

  protected async confirmConflictSave(): Promise<void> {
	if (!this.pendingDraft || this.hasUnresolvableConflict()) return;
	this.saving.set(true);
	this.saveError.set('');
	try {
	  const overrideCodes = this.preflightConflicts()
	    .filter((conflict) => conflict.severity === 'blocking' && conflict.canOverride)
	    .map((conflict) => conflict.code);
	  await this.commitShift(this.pendingDraft, overrideCodes);
	} catch (error) {
	  this.saveError.set((error as Error).message);
	} finally {
	  this.saving.set(false);
	}
  }

  protected hasUnresolvableConflict(): boolean {
	return this.preflightConflicts().some((conflict) => conflict.severity === 'blocking' && !conflict.canOverride);
  }

  protected clearConflictResults(): void {
	this.pendingDraft = null;
	this.preflightConflicts.set([]);
  }

  private async commitShift(draft: ShiftDraft, overrideCodes: string[]): Promise<void> {
	await this.schedule.save(draft, this.selectedShiftId(), overrideCodes);
	this.editorOpen.set(false);
	this.selectedShiftId.set(null);
	this.clearConflictResults();
  }

  protected async deleteShift(): Promise<void> {
    const shiftId = this.selectedShiftId();
    if (!shiftId || !window.confirm('Delete this shift?')) return;

    this.saving.set(true);
    this.saveError.set('');
    try {
      await this.schedule.deleteShift(shiftId);
      this.editorOpen.set(false);
      this.selectedShiftId.set(null);
    } catch (error) {
      this.saveError.set((error as Error).message);
    } finally {
      this.saving.set(false);
    }
  }

  protected async handleShiftDrop(action: ShiftDropAction): Promise<void> {
    if (this.draggingAction()) return;
    this.draggingAction.set(true);
    this.dragError.set('');
    this.dragMessage.set('');
    try {
      if (action.mode === 'swap' && action.target) {
        await this.schedule.swapShifts(action.source.id, action.target.id);
        this.dragMessage.set('Shifts swapped. Publish when the schedule is ready.');
        return;
      }
      const draft: ShiftDraft = {
        employeeId: action.targetEmployeeId,
        dayIndex: action.targetDayIndex,
        start: action.source.start,
        end: action.source.end,
        note: action.source.note,
      };
      await this.schedule.save(draft, action.mode === 'move' ? action.source.id : null);
      this.dragMessage.set(action.mode === 'copy'
        ? 'Shift copied. Publish when the schedule is ready.'
        : 'Shift moved. Publish when the schedule is ready.');
    } catch (error) {
      this.dragError.set((error as Error).message);
    } finally {
      this.draggingAction.set(false);
    }
  }
}
