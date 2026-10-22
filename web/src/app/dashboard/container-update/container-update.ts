import { Component, computed, inject, input, LOCALE_ID } from '@angular/core';
import { TmBadge, TmIcon, TmLevel, TmSkeleton, TmTooltip, TmUpdateClass, UpdateClass } from '../../ui';
import { ContainerSummary, hasResult, isIgnored, isReviewed, isSnoozed, REVIEW_ICONS, ruleOrigin } from '../containers';
import { errorKindLabel, reviewedTitle } from '../format';
import { ContainerRefreshFailed } from '../refresh-failed/refresh-failed';

const TOOLTIP_LINE = 140;
/** Quoted lines the tooltip shows; the detail page lists them all. */
const TOOLTIP_QUOTES = 3;
const clip = (s: string) => (s.length > TOOLTIP_LINE ? s.slice(0, TOOLTIP_LINE - 1).trimEnd() + '…' : s);

/**
 * The update class and risk level badges, "check failed", or a placeholder while checking.
 * Reviewed: badges faded plus a quiet "Reviewed" marker. Snoozed: badges faded (the row says until when).
 */
@Component({
  selector: 'tm-container-update',
  imports: [ContainerRefreshFailed, TmBadge, TmIcon, TmLevel, TmSkeleton, TmTooltip, TmUpdateClass],
  templateUrl: './container-update.html',
  host: { class: 'inline-flex flex-wrap items-center gap-1.5' },
})
export class ContainerUpdate {
  readonly container = input.required<ContainerSummary>();
  readonly checking = input(false);
  /** Show the "Reviewed" marker; off where the review is shown elsewhere (detail header). */
  readonly showReview = input(true);
  private readonly locale = inject(LOCALE_ID);
  protected readonly icons = REVIEW_ICONS;

  protected readonly ignored = computed(() => isIgnored(this.container()));
  protected readonly ignoredTitle = computed(() => {
    const rules = this.container().rules;
    const origin = rules && ruleOrigin(rules, 'ignore');
    const by = origin?.label ? ` by the ${origin.label} label` : origin ? ' in the config file' : '';
    return `Not checked for updates: ignored${by}.`;
  });
  /** The last successful result (kept when a later refresh failed). */
  protected readonly result = computed(() => {
    const u = this.container().update;
    return hasResult(u) ? u : undefined;
  });
  /** Tooltip of the "Reviewed" marker, or null when the update isn't reviewed. */
  protected readonly reviewed = computed(() => {
    const c = this.container();
    return this.showReview() && isReviewed(c) ? reviewedTitle(c.review!, this.locale) : null;
  });
  protected readonly faded = computed(() => isReviewed(this.container()) || isSnoozed(this.container()));
  protected readonly error = computed(() => this.container().update?.error);
  protected readonly reason = computed(() => errorKindLabel(this.container().update?.errorKind));
  protected readonly kind = computed(() => (this.result()?.class ?? 'none') as UpdateClass);
  /**
   * One line per reason, quotes prefixed with their release; cut short, since the tooltip is small:
   * the first three quotes (breaking ones come first), then "and N more flagged lines".
   */
  protected readonly reasons = computed(() => {
    let quotes = 0;
    const lines = (this.result()?.reasons ?? [])
      .filter((r) => !r.line || ++quotes <= TOOLTIP_QUOTES)
      .map((r) => clip((r.source === 'ai' ? 'AI: ' : '') + (r.tag ? `${r.tag}: ${r.text}` : r.text)));
    const more = quotes - TOOLTIP_QUOTES;
    if (more > 0) lines.push(`and ${more} more flagged ${more === 1 ? 'line' : 'lines'}`);
    return lines.join('\n') || null;
  });
}
