import { booleanAttribute, Component, input } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TmAppIcon, TmBadge, TmCard } from '../../ui';
import { ContainerSummary, imageName, isCurrent, isOpenUpdate, isSnoozed } from '../containers';
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
 * `dimCurrent`: an up-to-date row or tile fades as a whole and drops its surface (the row shows the
 * page background, the tile already has none). Hover and keyboard focus bring it all back: full
 * opacity, the icon's colour and the name's colour (DIM_ICON / DIM_NAME, via the `group` here).
 */
export const DIM_CLASS = 'group opacity-60 hover:opacity-100 focus-within:opacity-100';
export const DIM_ICON = 'group-hover:opacity-100 group-hover:grayscale-0 group-focus-within:opacity-100 group-focus-within:grayscale-0';
export const DIM_NAME = 'group-hover:text-fg group-focus-within:text-fg';

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
  /**
   * Fade up-to-date rows as a whole (`data-dimmed`), so in a stack with something to do the rows
   * that need a look stand out. Stack grouping only; reviewed rows aren't dimmed (their badges fade).
   */
  readonly dimCurrent = input(false, { transform: booleanAttribute });
  /** Where each row links to; the styleguide points this somewhere harmless. */
  readonly link = input<ContainerLink>(DETAIL_LINK);

  protected readonly columns = COLUMNS;
  protected readonly dimClass = `transition-opacity ${DIM_CLASS}`;
  protected readonly dimIcon = DIM_ICON;
  protected readonly imageName = imageName;
  protected readonly isCurrent = isCurrent;

  /** Ignored, snoozed or up to date: toned down, so rows with an update stand out. */
  protected quiet(c: ContainerSummary): boolean {
    return this.ignored() || isCurrent(c) || isSnoozed(c);
  }

  protected dimmed(c: ContainerSummary): boolean {
    return this.dimCurrent() && isCurrent(c);
  }

  /** Bold only for an open update; a reviewed one has been looked at. */
  protected nameClass(c: ContainerSummary): string {
    if (this.quiet(c)) return `font-medium text-fg-muted${this.dimmed(c) ? ` ${DIM_NAME}` : ''}`;
    return isOpenUpdate(c) ? 'font-semibold text-fg' : 'font-medium text-fg';
  }
}
