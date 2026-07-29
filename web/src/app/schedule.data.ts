import { ApiEmployee, DayColumn, Employee } from './schedule.models';

const accents = ['#dbeafe', '#ede9fe', '#dcfce7', '#ffedd5', '#fce7f3', '#e0f2fe'];

export function buildDays(weekStart: string): DayColumn[] {
  const start = new Date(`${weekStart}T12:00:00`);
  return Array.from({ length: 7 }, (_, index) => {
    const date = new Date(start);
    date.setDate(start.getDate() + index);
    return {
      short: date.toLocaleDateString('en-US', { weekday: 'short' }),
      label: date.toLocaleDateString('en-US', { weekday: 'long' }),
      date: date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
      isoDate: isoDate(date),
    };
  });
}

export function mapEmployees(employees: ApiEmployee[]): Employee[] {
  return employees.map((employee, index) => {
    const status = employee.status ?? 'active';
    return {
      id: employee.id,
      name: employee.displayName,
      email: employee.email ?? '',
      status,
      scheduleEligible: employee.scheduleEligible ?? (status === 'active'),
      deactivatedAt: employee.deactivatedAt,
      initials: initials(employee.displayName),
      accent: accents[index % accents.length],
    };
  });
}

export function initials(name: string): string {
  return name.split(/\s+/).filter(Boolean).slice(0, 2).map((part) => part[0]).join('').toUpperCase();
}

export function isoDate(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function currentWeekStart(): string {
  const today = new Date();
  const monday = new Date(today);
  const daysSinceMonday = (today.getDay() + 6) % 7;
  monday.setDate(today.getDate() - daysSinceMonday);
  return isoDate(monday);
}

export const INITIAL_WEEK = currentWeekStart();
