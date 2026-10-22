import { formatDate } from '@angular/common';

const DAY_MS = 24 * 60 * 60 * 1000;

/** A date without the time: "20 Oct", with the year when it isn't this year ("20 Oct 2025"). */
export function shortDate(iso: string, locale: string, now = new Date()): string {
  const sameYear = new Date(iso).getFullYear() === now.getFullYear();
  return formatDate(iso, sameYear ? 'd MMM' : 'd MMM y', locale);
}

/** How long between two moments, roughly: "less than a day", "1 day", "12 days", "3 months", "2 years". */
export function roughDuration(fromIso: string, toIso: string): string {
  const days = Math.floor((new Date(toIso).getTime() - new Date(fromIso).getTime()) / DAY_MS);
  if (days < 1) return 'less than a day';
  if (days < 60) return days === 1 ? '1 day' : `${days} days`;
  if (days < 730) return `${Math.floor(days / 30)} months`;
  return `${Math.floor(days / 365)} years`;
}

/** now + n days, as RFC 3339. */
export function daysFromNow(days: number, now = Date.now()): string {
  return new Date(now + days * DAY_MS).toISOString();
}

/** How long ago, roughly: "just now", "5 minutes ago", "3 hours ago", "2 days ago"; a date after two weeks. */
export function timeAgo(iso: string, locale: string, now = Date.now()): string {
  const minutes = Math.floor((now - new Date(iso).getTime()) / 60000);
  if (minutes < 1) return 'just now';
  if (minutes < 60) return `${minutes} ${minutes === 1 ? 'minute' : 'minutes'} ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} ${hours === 1 ? 'hour' : 'hours'} ago`;
  const days = Math.floor(hours / 24);
  if (days < 14) return `${days} ${days === 1 ? 'day' : 'days'} ago`;
  return shortDate(iso, locale, new Date(now));
}
