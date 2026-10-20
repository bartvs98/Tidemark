import { formatDate } from '@angular/common';
import { ContainerSummary, ErrorKind, isBuildKey, ReviewState, UpdateInfo } from './containers';

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

/** A date as "20 Oct", or just the time ("14:00") when it is today. */
function shortDate(iso: string, locale: string): string {
  const today = new Date(iso).toDateString() === new Date().toDateString();
  return formatDate(iso, today ? 'HH:mm' : 'd MMM', locale);
}

/** Tooltip of the "Reviewed" marker: "Reviewed up to 1.6.0 on 20 Oct." */
export function reviewedTitle(r: ReviewState, locale: string): string {
  const on = formatDate(r.at, 'd MMM', locale);
  if (r.version && isBuildKey(r.version)) return `Reviewed this build on ${on}. It shows as new again when a new build comes out.`;
  const upTo = r.version ? ` up to ${r.version}` : '';
  return `Reviewed${upTo} on ${on}. It shows as new again when a newer version comes out.`;
}

/**
 * The quiet line on a snoozed row: "until 3 Nov" (`value` empty) or "until newer than" + `value`
 * "1.6.0" (in mono). `title` says when it was snoozed.
 */
export function snoozedText(r: ReviewState, locale: string): { text: string; value?: string; title: string } {
  const on = `Snoozed on ${formatDate(r.at, 'd MMM', locale)}`;
  if (r.until) {
    const until = shortDate(r.until, locale);
    return { text: `until ${until}`, title: `${on}. It shows again from ${formatDate(r.until, 'd MMM, HH:mm', locale)}.` };
  }
  if (r.version && isBuildKey(r.version)) {
    return { text: 'until a new build', title: `${on}. It shows again when a new build comes out.` };
  }
  return {
    text: 'until newer than',
    value: r.version,
    title: `${on}. It shows again when a version newer than ${r.version ?? 'the current latest'} comes out.`,
  };
}

const ERROR_KIND_LABELS: Record<ErrorKind, string> = {
  'rate-limit': 'rate limited',
  'not-found': 'not found or private',
  timeout: 'timed out',
  network: 'network error',
  registry: 'registry error',
};

/** Short, quiet reason for a failed check ("rate limited"); empty when the backend couldn't classify it. */
export function errorKindLabel(kind: ErrorKind | undefined): string {
  return (kind && ERROR_KIND_LABELS[kind]) || '';
}

// not-found has none: its message already says only public images are supported.
const ERROR_KIND_HINTS: Partial<Record<ErrorKind, string>> = {
  'rate-limit': 'Tidemark tries again at the next check.',
  timeout: "Check the server's network or DNS.",
  network: "Check the server's network or DNS.",
  registry: 'The registry had a problem; Tidemark tries again at the next check.',
};

/** One-line advice for a failed check; empty without a kind. */
export function errorKindHint(kind: ErrorKind | undefined): string {
  return (kind && ERROR_KIND_HINTS[kind]) || '';
}
