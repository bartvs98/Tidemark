import { Component, computed, inject, input, LOCALE_ID, output, signal } from '@angular/core';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';
import { MenuOption, TmButton, TmIcon, TmMenu, TmTooltip, Toasts } from '../../ui';
import { ContainerSummary, isBuildKey, ReviewRequest } from '../../dashboard/containers';
import { daysFromNow, shortDate } from '../dates';

type SnoozeChoice = 'day' | 'week' | 'month' | 'newer';
type Pending = 'reviewed' | 'snoozed' | 'delete';

const SNOOZE_DAYS: Record<Exclude<SnoozeChoice, 'newer'>, { days: number; label: string }> = {
  day: { days: 1, label: '1 day' },
  week: { days: 7, label: '1 week' },
  month: { days: 30, label: '1 month' },
};

const NO_KEY = "There's no version or build to mark it against.";

/**
 * Mark reviewed / Snooze ▾ on the detail page's tab row, or the quiet "Reviewed up to …" / "Snoozed until …"
 * status with Undo / Unsnooze. Calls PUT/DELETE /api/containers/{name}/review, toasts the outcome
 * and emits `changed` after a success so the page reloads. Only shown for a container with an update.
 */
@Component({
  selector: 'tm-review-actions',
  imports: [TmButton, TmIcon, TmMenu, TmTooltip],
  templateUrl: './review-actions.html',
  host: { class: 'flex flex-wrap items-center gap-2 sm:justify-end', 'data-testid': 'review-actions' },
})
export class ReviewActions {
  readonly container = input.required<ContainerSummary>();
  /** A review was saved or removed. */
  readonly changed = output<void>();

  private readonly http = inject(HttpClient);
  private readonly toasts = inject(Toasts);
  private readonly locale = inject(LOCALE_ID);

  protected readonly pending = signal<Pending | null>(null);
  protected readonly review = computed(() => this.container().review);
  /** What a review is set against: the latest version, or the build (digest) for a channel tag or an unknown latest. */
  protected readonly key = computed(() => this.container().update?.reviewKey);
  /** The stored review is against a build, not a version. */
  protected readonly buildReview = computed(() => isBuildKey(this.review()?.version ?? ''));
  protected readonly isBuild = computed(() => isBuildKey(this.key() ?? ''));
  protected readonly reviewHint = computed(() => {
    const key = this.key();
    if (!key) return NO_KEY;
    return this.isBuild()
      ? "You've looked at this build. The alert ends when a new build comes out."
      : `You've read the changes up to ${key}. The alert ends when a newer version comes out.`;
  });
  protected readonly snoozeOptions = computed<MenuOption<SnoozeChoice>[]>(() => [
    ...(Object.keys(SNOOZE_DAYS) as (keyof typeof SNOOZE_DAYS)[]).map((value) => ({ value, label: SNOOZE_DAYS[value].label })),
    { value: 'newer', label: this.isBuild() ? 'Until a new build' : 'Until a newer version', disabled: !this.key(), hint: this.key() ? undefined : NO_KEY },
  ]);

  protected date(iso: string): string {
    return shortDate(iso, this.locale);
  }

  protected markReviewed(): void {
    const version = this.key();
    if (!version) return;
    void this.save({ kind: 'reviewed', version }, this.isBuild() ? 'Marked this build as reviewed' : `Marked as reviewed up to ${version}`);
  }

  protected snooze(choice: SnoozeChoice): void {
    if (choice === 'newer') {
      const version = this.key();
      if (version) void this.save({ kind: 'snoozed', version }, this.isBuild() ? 'Snoozed until a new build' : 'Snoozed until a newer version');
      return;
    }
    const { days, label } = SNOOZE_DAYS[choice];
    void this.save({ kind: 'snoozed', until: daysFromNow(days) }, `Snoozed for ${label}`);
  }

  protected undo(): void {
    const ok = this.review()?.kind === 'snoozed' ? 'Unsnoozed' : 'No longer marked as reviewed';
    void this.send('delete', () => this.http.delete(this.url()), ok);
  }

  private save(body: ReviewRequest, ok: string): Promise<void> {
    return this.send(body.kind, () => this.http.put(this.url(), body), ok);
  }

  private url(): string {
    return `/api/containers/${encodeURIComponent(this.container().name)}/review`;
  }

  private async send(what: Pending, request: () => ReturnType<HttpClient['delete']>, ok: string): Promise<void> {
    if (this.pending()) return;
    this.pending.set(what);
    try {
      await firstValueFrom(request());
      this.toasts.show(ok, 'ok');
      this.changed.emit();
    } catch (err) {
      const reason = err instanceof HttpErrorResponse ? ((err.error as { error?: string } | null)?.error ?? err.message) : String(err);
      this.toasts.show(`Couldn't save that: ${reason}`, 'attention');
    } finally {
      this.pending.set(null);
    }
  }
}
