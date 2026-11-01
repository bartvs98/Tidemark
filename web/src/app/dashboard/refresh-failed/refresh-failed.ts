import { Component, computed, inject, input, LOCALE_ID } from '@angular/core';
import { TmIcon, TmTooltip } from '../../ui';
import { ContainerSummary, hasResult, refreshFailed, versionUnknown } from '../containers';
import { errorKindLabel, shortTime } from '../format';

/**
 * Quiet note that the result shown is from an earlier check, because the latest refresh failed.
 * Not a risk, so no colour and no badge: icon + "refresh failed" (+ the short reason when known), full details on hover.
 * A check that succeeded on an older tag list (the registry rate-limited it) is half a failed refresh: "older tag list".
 * Renders nothing unless there's a kept result and an error, or a result from an older tag list.
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

  protected readonly reason = computed(() => errorKindLabel(this.container().update?.errorKind));

  protected readonly note = computed(() => {
    const u = this.container().update;
    if (!u || !hasResult(u)) return null;
    if (refreshFailed(u)) {
      const failed = u.failedAt ? ` at ${shortTime(u.failedAt, this.locale)}` : '';
      return {
        text: 'refresh failed',
        reason: this.reason(),
        title: `Showing the result from ${shortTime(u.checkedAt, this.locale)}. The refresh${failed} failed:\n${u.error}`,
      };
    }
    if (u.tagsFrom) {
      const missing = versionUnknown(u)
        ? "The image under this tag changed, but that list doesn't have its version yet."
        : 'A newer version can be missing from it.';
      return {
        text: 'older tag list',
        reason: '',
        title:
          `The registry rate-limited the tag list, so the check at ${shortTime(u.checkedAt, this.locale)} used the one from ` +
          `${shortTime(u.tagsFrom, this.locale)}. ${missing} Tidemark asks again at the next check.`,
      };
    }
    return null;
  });
}
