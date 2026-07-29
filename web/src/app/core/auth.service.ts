import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { User } from '../schedule.models';

@Injectable({ providedIn: 'root' })
export class AuthService {
  readonly user = signal<User | null>(null);
  readonly checking = signal(false);
  private sessionRequest?: Promise<User | null>;

  constructor(private readonly http: HttpClient) {}

  loadSession(force = false): Promise<User | null> {
    if (this.sessionRequest && !force) return this.sessionRequest;
    this.checking.set(true);
    this.sessionRequest = firstValueFrom(this.http.get<User>('/api/v1/auth/me'))
      .then((user) => {
        this.user.set(user);
        return user;
      })
      .catch((error: HttpErrorResponse) => {
        if (error.status !== 401) throw error;
        this.user.set(null);
        return null;
      })
      .finally(() => this.checking.set(false));
    return this.sessionRequest;
  }

  async login(email: string, password: string): Promise<User> {
    const user = await firstValueFrom(this.http.post<User>('/api/v1/auth/login', { email, password }));
    this.user.set(user);
    this.sessionRequest = Promise.resolve(user);
    return user;
  }

  async logout(): Promise<void> {
    await firstValueFrom(this.http.post<void>('/api/v1/auth/logout', {}));
    this.user.set(null);
    this.sessionRequest = Promise.resolve(null);
  }
}
