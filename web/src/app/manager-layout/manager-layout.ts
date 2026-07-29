import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { AuthService } from '../core/auth.service';
import { ScheduleService } from '../core/schedule.service';
import { initials } from '../schedule.data';

@Component({
  selector: 'app-manager-layout',
  imports: [RouterLink, RouterLinkActive, RouterOutlet],
  templateUrl: './manager-layout.html',
  styleUrl: './manager-layout.css',
})
export class ManagerLayout implements OnInit {
  protected readonly schedule = inject(ScheduleService);
  private readonly auth = inject(AuthService);
  protected readonly days = this.schedule.days;
  protected readonly hasUnpublishedChanges = this.schedule.hasUnpublishedChanges;
  protected readonly isCurrentWeek = this.schedule.isCurrentWeek;
  protected readonly user = this.auth.user;
  protected readonly publishLabel = computed(() => this.hasUnpublishedChanges() ? 'Publish schedule' : 'Published');
  protected readonly publishError = signal('');
  protected readonly weekLabel = computed(() => {
    const days = this.days();
    return `${days[0].date} – ${days[6].date}, ${days[6].isoDate.slice(0, 4)}`;
  });
  protected readonly userInitials = computed(() => initials(this.user()?.displayName ?? 'Manager'));
  protected publishing = false;

  async ngOnInit(): Promise<void> { await this.schedule.load(); }

  protected async moveWeek(offset: number): Promise<void> {
    await this.schedule.moveWeek(offset);
  }

  protected async goToCurrentWeek(): Promise<void> {
    await this.schedule.goToCurrentWeek();
  }

  protected async publish(): Promise<void> {
    if (!this.hasUnpublishedChanges() || this.publishing) return;
    this.publishing = true;
    this.publishError.set('');
    try {
      await this.schedule.publish();
    } catch (error) {
      this.publishError.set((error as Error).message);
    } finally {
      this.publishing = false;
    }
  }
}
