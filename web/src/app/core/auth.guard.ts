import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { AuthService } from './auth.service';

export const authGuard: CanActivateFn = async (_route, state) => {
  const auth = inject(AuthService);
  const router = inject(Router);
  const user = await auth.loadSession();
  return user ? true : router.createUrlTree(['/login'], { queryParams: { returnUrl: state.url } });
};

export const managerGuard: CanActivateFn = async () => {
  const auth = inject(AuthService);
  const router = inject(Router);
  const user = await auth.loadSession();
  if (!user) return router.createUrlTree(['/login']);
  return user.role === 'manager' ? true : router.createUrlTree(['/my-schedule']);
};

export const workerGuard: CanActivateFn = async () => {
  const auth = inject(AuthService);
  const router = inject(Router);
  const user = await auth.loadSession();
  if (!user) return router.createUrlTree(['/login']);
  return user.role === 'worker' ? true : router.createUrlTree(['/schedule']);
};
