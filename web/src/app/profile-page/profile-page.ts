import { Component, computed, inject } from '@angular/core';
import { Router } from '@angular/router';
import { AuthService } from '../core/auth.service';
import { initials } from '../schedule.data';

@Component({ selector: 'app-profile-page', templateUrl: './profile-page.html' })
export class ProfilePage {
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  protected readonly user = this.auth.user;
  protected readonly userInitials = computed(() => initials(this.user()?.displayName ?? 'User'));
  protected signingOut = false;
  protected async logout(): Promise<void> {
    this.signingOut = true;
    try { await this.auth.logout(); await this.router.navigateByUrl('/login'); }
    finally { this.signingOut = false; }
  }
}
