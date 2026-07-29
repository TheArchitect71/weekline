import { Component, EventEmitter, Input, Output } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Conflict, DayColumn, Employee, ShiftDraft } from '../schedule.models';

@Component({
  selector: 'app-shift-editor',
  imports: [FormsModule],
  templateUrl: './shift-editor.html',
  styleUrl: './shift-editor.css',
})
export class ShiftEditor {
  @Input({ required: true }) draft!: ShiftDraft;
  @Input({ required: true }) employees: Employee[] = [];
  @Input({ required: true }) days: DayColumn[] = [];
  @Input() isNew = false;
  @Input() conflicts: Conflict[] = [];
  @Input() saving = false;
  @Output() closed = new EventEmitter<void>();
  @Output() saved = new EventEmitter<ShiftDraft>();
  @Output() deleted = new EventEmitter<void>();
  @Output() changed = new EventEmitter<void>();
  @Output() confirmed = new EventEmitter<void>();

  protected submit(): void {
    if (!this.isTimeRangeValid()) return;
    this.saved.emit({ ...this.draft });
  }

  protected isTimeRangeValid(): boolean {
    return Boolean(this.draft.start && this.draft.end && this.draft.end > this.draft.start);
  }

  protected requestDelete(): void {
    this.deleted.emit();
  }

  protected hasUnresolvableConflict(): boolean {
    return this.conflicts.some((conflict) => conflict.severity === 'blocking' && !conflict.canOverride);
  }
}
