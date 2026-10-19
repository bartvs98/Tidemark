import { formatDate } from '@angular/common';
import { ContainerSummary, UpdateInfo } from './containers';

/**
 * The quiet line under the versions when a rule keeps a newer version back:
 * "held back by pin 1.4" / "held back, majors off". The version itself goes in front, in mono.
 */
export function heldText(u: UpdateInfo, c: ContainerSummary): { text: string; value?: string; title: string } {
  if (u.heldBy === 'pin') {
    const pin = c.rules?.pin;
    return {
      text: 'held back by pin',
      value: pin,
      title: pin ? `Pinned to ${pin}: only versions starting with ${pin} count.` : 'Pinned: only versions in the pin count.',
    };
  }
  if (u.heldBy === 'allow-major') {
    return { text: 'held back, majors off', title: 'Major updates are off for this container: only versions in the running major count.' };
  }
  return { text: 'held back by a rule', title: 'A rule for this container keeps this version back.' };
}

/** When a date-time is today, just the time ("07:02"); otherwise with the date ("17 Oct, 23:00"). */
export function shortTime(iso: string, locale: string): string {
  const today = new Date(iso).toDateString() === new Date().toDateString();
  return formatDate(iso, today ? 'HH:mm' : 'd MMM, HH:mm', locale);
}
