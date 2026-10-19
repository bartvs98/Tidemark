import { Component, computed, inject, input, LOCALE_ID } from '@angular/core';
import { TmIcon, TmTooltip } from '../../ui';
import { ContainerSummary, hasResult, refreshFailed } from '../containers';
import { shortTime } from '../format';

/**
 * Quiet note that the result shown is from an earlier check, because the latest refresh failed.
 * Not a risk, so no colour and no badge: icon + "refresh failed", details on hover.
 * Renders nothing unless there's a kept result and an error.
 */
@Component({
  selector: 'tm-refresh-failed',
  imports: [TmIcon, TmTooltip],
  templateUrl: './refresh-failed.html',
  host: { class: 'inline-flex empty:hidden' },
})
export class ContainerRefreshFailed {
  readonly container = input.required<ContainerSummary>();
  private readonly locale = inject(LOCALE_ID);

  protected readonly title = computed(() => {
    const u = this.container().update;
    if (!u || !hasResult(u) || !refreshFailed(u)) return null;
    const failed = u.failedAt ? ` at ${shortTime(u.failedAt, this.locale)}` : '';
    return `Showing the result from ${shortTime(u.checkedAt, this.locale)}. The refresh${failed} failed:\n${u.error}`;
  });
}
