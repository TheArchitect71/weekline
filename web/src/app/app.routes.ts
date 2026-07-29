import { Routes } from '@angular/router';
import { authGuard, managerGuard, workerGuard } from './core/auth.guard';

export const routes: Routes = [
  { path: 'login', loadComponent: () => import('./login-page/login-page').then((m) => m.LoginPage) },
  {
    path: '',
    loadComponent: () => import('./manager-layout/manager-layout').then((m) => m.ManagerLayout),
    canActivate: [managerGuard],
    children: [
      { path: '', pathMatch: 'full', redirectTo: 'schedule' },
      { path: 'schedule', loadComponent: () => import('./schedule-page/schedule-page').then((m) => m.SchedulePage) },
      { path: 'people', loadComponent: () => import('./people-page/people-page').then((m) => m.PeoplePage) },
      { path: 'requests', loadComponent: () => import('./requests-page/requests-page').then((m) => m.RequestsPage) },
      { path: 'leave', loadComponent: () => import('./leave-page/leave-page').then((m) => m.LeavePage) },
      { path: 'coverage', loadComponent: () => import('./coverage-page/coverage-page').then((m) => m.CoveragePage) },
      { path: 'availability', loadComponent: () => import('./availability-page/availability-page').then((m) => m.AvailabilityPage) },
      { path: 'history', loadComponent: () => import('./history-page/history-page').then((m) => m.HistoryPage) },
      { path: 'profile', loadComponent: () => import('./profile-page/profile-page').then((m) => m.ProfilePage) },
      { path: 'help', loadComponent: () => import('./help-page/help-page').then((m) => m.HelpPage) },
      { path: 'settings', loadComponent: () => import('./settings-page/settings-page').then((m) => m.SettingsPage) },
    ],
  },
  { path: 'worker-preview', canActivate: [managerGuard], data: { preview: true }, loadComponent: () => import('./worker-page/worker-page').then((m) => m.WorkerPage) },
  { path: 'my-schedule', canActivate: [workerGuard], loadComponent: () => import('./worker-page/worker-page').then((m) => m.WorkerPage) },
  { path: 'my-requests', canActivate: [workerGuard], loadComponent: () => import('./my-requests-page/my-requests-page').then((m) => m.MyRequestsPage) },
  { path: 'my-leave', canActivate: [workerGuard], loadComponent: () => import('./my-leave-page/my-leave-page').then((m) => m.MyLeavePage) },
  { path: 'my-availability', canActivate: [workerGuard], loadComponent: () => import('./availability-page/availability-page').then((m) => m.AvailabilityPage) },
  { path: 'my-profile', canActivate: [authGuard], loadComponent: () => import('./my-profile-page/my-profile-page').then((m) => m.MyProfilePage) },
  { path: '**', redirectTo: 'schedule' },
];
