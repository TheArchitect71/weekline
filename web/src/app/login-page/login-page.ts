import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { AuthService } from '../core/auth.service';
import { ApiProblem } from '../schedule.models';

@Component({
  selector: 'app-login-page',
  imports: [FormsModule],
  templateUrl: './login-page.html',
  styleUrl: './login-page.css',
})
export class LoginPage implements OnInit {
  protected email = '';
  protected password = '';
  protected error = '';
  protected submitting = false;

  constructor(
    private readonly auth: AuthService,
    private readonly router: Router,
    private readonly route: ActivatedRoute,
  ) {}

  async ngOnInit(): Promise<void> {
    const user = await this.auth.loadSession();
    if (user) await this.router.navigateByUrl(user.role === 'manager' ? '/schedule' : '/my-schedule');
  }

  protected useDemo(role: 'manager' | 'worker'): void {
    this.email = role === 'manager' ? 'manager@weekline.local' : 'elena@weekline.local';
    this.password = 'WeeklineDemo!';
    this.error = '';
  }

  protected async submit(): Promise<void> {
    if (this.submitting) return;
    this.submitting = true;
    this.error = '';
    try {
      const user = await this.auth.login(this.email, this.password);
      const returnUrl = this.route.snapshot.queryParamMap.get('returnUrl');
      const destination = returnUrl && returnUrl.startsWith('/') ? returnUrl : user.role === 'manager' ? '/schedule' : '/my-schedule';
      await this.router.navigateByUrl(destination);
    } catch (error) {
      const response = error as HttpErrorResponse;
      this.error = (response.error as ApiProblem | undefined)?.message ?? 'Unable to sign in.';
    } finally {
      this.submitting = false;
    }
  }
}
