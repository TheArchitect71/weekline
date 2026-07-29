import { Component, OnInit, computed, inject } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { AuthService } from '../core/auth.service';
import { ScheduleService } from '../core/schedule.service';
import { Employee } from '../schedule.models';
import { WorkerSchedule } from '../worker-schedule/worker-schedule';

@Component({
  selector: 'app-worker-page',
  imports: [WorkerSchedule],
  templateUrl: './worker-page.html',
  styleUrl: './worker-page.css',
})
export class WorkerPage implements OnInit {
  protected readonly schedule = inject(ScheduleService);
  protected readonly auth = inject(AuthService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  protected readonly preview = this.route.snapshot.data['preview'] === true;
  protected readonly currentEmployee = computed<Employee | null>(() => {
    const employees = this.schedule.employees();
    const user = this.auth.user();
    return this.preview ? employees[0] ?? null : employees.find((employee) => employee.id === user?.id) ?? null;
  });

  async ngOnInit(): Promise<void> { await this.schedule.load(); }

  protected async exit(): Promise<void> { await this.router.navigateByUrl(this.preview ? '/schedule' : '/my-profile'); }

  protected async openRequests(): Promise<void> {
    await this.router.navigateByUrl(this.preview ? '/requests' : '/my-requests');
  }

  protected async openLeave(): Promise<void> {
    await this.router.navigateByUrl(this.preview ? '/leave' : '/my-leave');
  }

  protected async openAvailability(): Promise<void> {
    await this.router.navigateByUrl(this.preview ? '/availability' : '/my-availability');
  }

}
