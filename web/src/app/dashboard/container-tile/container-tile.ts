import { booleanAttribute, Component, computed, input } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TmAppIcon, TmBadge } from '../../ui';
import { ContainerLink, DETAIL_LINK, DIM_CLASS, DIM_ICON, DIM_NAME } from '../container-table/container-table';
import { ContainerSummary, imageName, isCurrent, isIgnored, isOpenUpdate, isSnoozed } from '../containers';
import { ContainerRuleProblems } from '../rule-problems/rule-problems';
import { ContainerUpdate } from '../container-update/container-update';
import { ContainerVersion } from '../container-version/container-version';

const BASE =
  'relative flex min-w-0 flex-col rounded-xl border p-5 transition-[border-color,opacity] hover:border-line-strong has-[a:focus-visible]:outline-2 has-[a:focus-visible]:outline-focus';
/** Up to date, ignored or snoozed: no surface or shadow, so tiles with an update stand out. */
const QUIET = `${BASE} border-line bg-transparent`;
const PROMINENT = `${BASE} border-line bg-surface shadow-raised`;

/** One container in tile mode. */
@Component({
  selector: 'tm-container-tile',
  imports: [RouterLink, ContainerRuleProblems, ContainerUpdate, ContainerVersion, TmAppIcon, TmBadge],
  templateUrl: './container-tile.html',
  host: {
    '[class]': 'hostClass()',
    '[attr.data-current]': "current() ? '' : null",
    '[attr.data-dimmed]': "dimmed() ? '' : null",
  },
})
export class ContainerTile {
  readonly container = input.required<ContainerSummary>();
  readonly checking = input(false);
  /** Show the stack before the status, e.g. when grouped by severity. */
  readonly showStack = input(false, { transform: booleanAttribute });
  /** Fade the tile as a whole when it's quiet (`data-dimmed`); see tm-container-table's `dimQuiet`. */
  readonly dimQuiet = input(false, { transform: booleanAttribute });
  /** Where the tile links to; the styleguide points this somewhere harmless. */
  readonly link = input<ContainerLink>(DETAIL_LINK);
  protected readonly image = computed(() => imageName(this.container()));
  protected readonly current = computed(() => isCurrent(this.container()));
  /** Ignored, snoozed or up to date: toned down. */
  protected readonly quiet = computed(() => this.current() || isIgnored(this.container()) || isSnoozed(this.container()));
  protected readonly dimmed = computed(() => this.dimQuiet() && this.quiet());
  protected readonly hostClass = computed(() => `${this.quiet() ? QUIET : PROMINENT}${this.dimmed() ? ` ${DIM_CLASS}` : ''}`);
  protected readonly dimIcon = computed(() => (this.dimmed() ? DIM_ICON : ''));
  protected readonly nameClass = computed(() => {
    if (this.quiet()) return `font-medium text-fg-muted${this.dimmed() ? ` ${DIM_NAME}` : ''}`;
    return isOpenUpdate(this.container()) ? 'font-semibold text-fg' : 'font-medium text-fg';
  });
}
