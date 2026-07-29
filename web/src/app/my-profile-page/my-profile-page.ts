import { Component, inject } from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { AuthService } from '../core/auth.service';
import { initials } from '../schedule.data';

@Component({ selector: 'app-my-profile-page', imports: [RouterLink], templateUrl: './my-profile-page.html', styleUrl: './my-profile-page.css' })
export class MyProfilePage {
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  protected readonly user = this.auth.user;
  protected readonly initials = initials;
  protected async logout(): Promise<void> { await this.auth.logout(); await this.router.navigateByUrl('/login'); }
}
