import { booleanAttribute, Component, input } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TmAppIcon, TmBadge, TmCard } from '../../ui';
import { ContainerSummary, hasUpdate, imageName, isCurrent } from '../containers';
import { ContainerRuleProblems } from '../rule-problems/rule-problems';
import { ContainerUpdate } from '../container-update/container-update';
import { ContainerVersion } from '../container-version/container-version';

/** List columns. The pinned column bar and every stack's table share these widths. */
export const COLUMNS = [
  { label: 'Container', width: 'w-[25%]' },
  { label: 'Image', width: 'w-[28%]' },
  { label: 'Version', width: 'w-[26%]' },
  { label: 'Update', width: 'w-[21%]' },
];

/** Where a container row or tile links to: its detail page. */
export type ContainerLink = (c: ContainerSummary) => string | unknown[];
export const DETAIL_LINK: ContainerLink = (c) => ['/containers', c.name];

/**
 * One group of containers (a Compose stack, or "Ignored") as a card with a table.
 * Fixed column widths from COLUMNS, so tables line up across stacks under one tm-column-bar.
 */
@Component({
  selector: 'tm-container-table',
  imports: [RouterLink, ContainerRuleProblems, ContainerUpdate, ContainerVersion, TmAppIcon, TmBadge, TmCard],
  templateUrl: './container-table.html',
  host: { class: 'block' },
})
export class ContainerTable {
  readonly containers = input.required<ContainerSummary[]>();
  readonly checking = input(false);
  /** Rows of the "Ignored" section: names toned down, and the stack shown before the status. */
  readonly ignored = input(false, { transform: booleanAttribute });
  /** Show each container's stack before its status, e.g. when grouped by severity. */
  readonly showStack = input(false, { transform: booleanAttribute });
  /** Where each row links to; the styleguide points this somewhere harmless. */
  readonly link = input<ContainerLink>(DETAIL_LINK);

  protected readonly columns = COLUMNS;
  protected readonly imageName = imageName;
  protected readonly isCurrent = isCurrent;

  /** Ignored or up to date: toned down, so rows with an update stand out. */
  protected quiet(c: ContainerSummary): boolean {
    return this.ignored() || isCurrent(c);
  }

  protected nameClass(c: ContainerSummary): string {
    if (this.quiet(c)) return 'font-medium text-fg-muted';
    return hasUpdate(c) ? 'font-semibold text-fg' : 'font-medium text-fg';
  }
}
