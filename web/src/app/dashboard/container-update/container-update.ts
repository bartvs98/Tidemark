import { Component, computed, input } from '@angular/core';
import { TmBadge, TmIcon, TmLevel, TmSkeleton, TmTooltip, TmUpdateClass, UpdateClass } from '../../ui';
import { ContainerSummary, hasResult, isIgnored, ruleOrigin } from '../containers';
import { errorKindLabel } from '../format';
import { ContainerRefreshFailed } from '../refresh-failed/refresh-failed';

/** The update class and risk level badges, "check failed", or a placeholder while checking. */
@Component({
  selector: 'tm-container-update',
  imports: [ContainerRefreshFailed, TmBadge, TmIcon, TmLevel, TmSkeleton, TmTooltip, TmUpdateClass],
  templateUrl: './container-update.html',
  host: { class: 'inline-flex flex-wrap items-center gap-1.5' },
})
export class ContainerUpdate {
  readonly container = input.required<ContainerSummary>();
  readonly checking = input(false);

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
  protected readonly error = computed(() => this.container().update?.error);
  protected readonly reason = computed(() => errorKindLabel(this.container().update?.errorKind));
  protected readonly kind = computed(() => (this.result()?.class ?? 'none') as UpdateClass);
  protected readonly reasons = computed(() => this.result()?.reasons?.join('\n') || null);
}
