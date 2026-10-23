import { formatDate } from '@angular/common';

const MIN = 60_000;
const HOUR = 60 * MIN;
const DAY = 24 * HOUR;

/** A span, roughly: "12 min", "3 h", "2 days", "5 weeks", "3 months", "2 years". */
export function longDuration(ms: number): string {
  if (ms < MIN) return 'under a minute';
  if (ms < HOUR) return `${Math.round(ms / MIN)} min`;
  if (ms < DAY) return `${Math.round(ms / HOUR)} h`;
  const days = Math.round(ms / DAY);
  if (days < 14) return days === 1 ? '1 day' : `${days} days`;
  if (days < 60) return `${Math.floor(days / 7)} weeks`;
  if (days < 730) return `${Math.floor(days / 30)} months`;
  return `${Math.floor(days / 365)} years`;
}

/** 0.874 → "87%". */
export function percent(ratio: number): string {
  return `${Math.round(ratio * 100)}%`;
}

/** "2026-09" → "Sep", or "Sep 26" when `withYear`. */
export function monthLabel(month: string, locale: string, withYear = false): string {
  return formatDate(`${month}-01T12:00:00Z`, withYear ? 'MMM yy' : 'MMM', locale, 'UTC');
}

/** "2026-10-12" → "12 Oct". */
export function dayLabel(day: string, locale: string): string {
  return formatDate(`${day.slice(0, 10)}T12:00:00Z`, 'd MMM', locale, 'UTC');
}

/** The first letter capitalised: "rate limited" → "Rate limited". */
export function capitalize(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1);
}

/** "1 container", "3 releases". */
export function count(n: number, one: string, many = one + 's'): string {
  return `${n.toLocaleString('en-US')} ${n === 1 ? one : many}`;
}

/** The time since the oldest newer release, for "behind for …"; empty when unknown. */
export function behindFor(since: string | undefined, now: number): string {
  if (!since) return '';
  const ms = now - new Date(since).getTime();
  return ms < DAY ? 'less than a day' : longDuration(ms);
}
