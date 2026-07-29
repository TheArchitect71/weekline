import { HttpClient } from '@angular/common/http';
import { Component, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { firstValueFrom } from 'rxjs';
import { initials, mapEmployees } from '../schedule.data';
import { ApiEmployee, Employee, EmployeeInput, EmployeeStatus } from '../schedule.models';

@Component({
  selector: 'app-people-page',
  imports: [FormsModule],
  templateUrl: './people-page.html',
})
export class PeoplePage implements OnInit {
  protected readonly people = signal<Employee[]>([]);
  protected readonly loading = signal(true);
  protected readonly saving = signal(false);
  protected readonly error = signal('');
  protected readonly initials = initials;
  protected formOpen = false;
  protected editingId: string | null = null;
  protected form = this.emptyForm();

  constructor(private readonly http: HttpClient) {}

  async ngOnInit(): Promise<void> {
    await this.load();
  }

  protected startCreate(): void {
    this.formOpen = true;
    this.editingId = null;
    this.form = this.emptyForm();
    this.error.set('');
  }

  protected startEdit(person: Employee): void {
    this.formOpen = true;
    this.editingId = person.id;
    this.form = {
      displayName: person.name,
      email: person.email,
      initialPassword: '',
      status: person.status,
      scheduleEligible: person.scheduleEligible,
    };
    this.error.set('');
  }

  protected cancelEdit(): void {
    this.formOpen = false;
    this.editingId = null;
    this.form = this.emptyForm();
    this.error.set('');
  }

  protected onStatusChange(): void {
    if (this.form.status !== 'active') {
      this.form.scheduleEligible = false;
    }
  }

  protected async saveEmployee(): Promise<void> {
    this.saving.set(true);
    this.error.set('');
    const body: EmployeeInput = {
      email: this.form.email.trim(),
      displayName: this.form.displayName.trim(),
      status: this.form.status,
      scheduleEligible: this.form.status === 'active' && this.form.scheduleEligible,
    };
    if (!this.editingId) {
      body.initialPassword = this.form.initialPassword;
    }
    try {
      if (this.editingId) {
        await firstValueFrom(this.http.put<ApiEmployee>(`/api/v1/people/${this.editingId}`, body));
      } else {
        await firstValueFrom(this.http.post<ApiEmployee>('/api/v1/people', body));
      }
      this.cancelEdit();
      await this.load();
    } catch (error) {
      this.error.set(this.errorMessage(error));
    } finally {
      this.saving.set(false);
    }
  }

  protected async deactivate(person: Employee): Promise<void> {
    if (person.status === 'deactivated') return;
    const confirmed = window.confirm(
      `Deactivate ${person.name}? Existing schedules stay in history, but they cannot receive new shifts.`,
    );
    if (!confirmed) return;
    this.saving.set(true);
    this.error.set('');
    try {
      await firstValueFrom(this.http.delete<ApiEmployee>(`/api/v1/people/${person.id}`));
      await this.load();
    } catch (error) {
      this.error.set(this.errorMessage(error));
    } finally {
      this.saving.set(false);
    }
  }

  protected activeCount(): number {
    return this.people().filter((person) => person.status === 'active' && person.scheduleEligible).length;
  }

  protected statusLabel(status: EmployeeStatus): string {
    if (status === 'deactivated') return 'Deactivated';
    if (status === 'inactive') return 'Inactive';
    return 'Active';
  }

  private async load(): Promise<void> {
    this.loading.set(true);
    try {
      this.people.set(mapEmployees(await firstValueFrom(this.http.get<ApiEmployee[]>('/api/v1/people'))));
    } finally {
      this.loading.set(false);
    }
  }

  private emptyForm(): EmployeeInput {
    return {
      displayName: '',
      email: '',
      initialPassword: '',
      status: 'active',
      scheduleEligible: true,
    };
  }

  private errorMessage(error: unknown): string {
    const response = error as { error?: { message?: string } };
    return response.error?.message ?? 'Unable to save employee changes.';
  }
}
