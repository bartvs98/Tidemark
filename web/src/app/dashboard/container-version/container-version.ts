import { Component, computed, inject, input, LOCALE_ID } from '@angular/core';
import { TmSkeleton, TmTooltip, TmVersionDiff } from '../../ui';
import { ContainerSummary, hasResult, isIgnored, isSnoozed, UpdateSource } from '../containers';
import { heldText, snoozedText } from '../format';

const SOURCE_HINT: Record<UpdateSource, string> = {
  tag: '',
  label: 'detected from image label',
  digest: 'detected by image digest',
  env: 'detected from image metadata',
  unknown: 'running version unknown',
};

/** Running → latest version, how the running version was found, the in-major option, and the held / snoozed lines. */
@Component({
  selector: 'tm-container-version',
  imports: [TmVersionDiff, TmSkeleton, TmTooltip],
  templateUrl: './container-version.html',
  host: { class: 'block min-w-0' },
})
export class ContainerVersion {
  readonly container = input.required<ContainerSummary>();
  readonly checking = input(false);
  /** Show the snoozed line; off where the snooze is shown elsewhere (detail header). */
  readonly showReview = input(true);
  private readonly locale = inject(LOCALE_ID);

  protected readonly ignored = computed(() => isIgnored(this.container()));
  /** The last successful result; a failed attempt without one shows just the tag. */
  protected readonly update = computed(() => {
    const u = this.container().update;
    return hasResult(u) ? u : undefined;
  });
  /** The only attempt so far failed: no skeleton, "check failed" next to it says why. */
  protected readonly failed = computed(() => !!this.container().update?.error);
  protected readonly held = computed(() => {
    const u = this.update();
    return u?.held ? heldText(u, this.container()) : undefined;
  });
  /** Snoozed: "until 3 Nov" / "until newer than 1.6.0". */
  protected readonly snoozed = computed(() => {
    const c = this.container();
    return this.showReview() && isSnoozed(c) ? snoozedText(c.review!, this.locale) : undefined;
  });
  protected readonly showLatest = computed(() => {
    const u = this.update();
    return !!u?.latest && u.latest !== u.running && u.class !== 'none';
  });
  protected readonly hint = computed(() => {
    const u = this.update();
    return u ? SOURCE_HINT[u.source] : '';
  });
}
