import { Component, computed, input } from '@angular/core';
import { TmBadge, TmTooltip } from '../../ui';
import { ContainerSummary } from '../containers';

/**
 * Small neutral marker for rules that were skipped because their value was invalid.
 * Not a risk, so no colour; the problems show on hover and on the detail page.
 */
@Component({
  selector: 'tm-rule-problems',
  imports: [TmBadge, TmTooltip],
  templateUrl: './rule-problems.html',
  host: { class: 'inline-flex empty:hidden' },
})
export class ContainerRuleProblems {
  readonly container = input.required<ContainerSummary>();
  protected readonly problems = computed(() => this.container().rules?.problems ?? []);
  protected readonly title = computed(() => this.problems().join('\n'));
}
