import { booleanAttribute, Component, input } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TmAppIcon, TmBadge, TmCard } from '../ui';
import { ContainerSummary, hasUpdate, imageName, isCurrent } from './containers';
import { ContainerRuleProblems, ContainerUpdate, ContainerVersion } from './update-info';

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
 * The one column header bar above all stacks in list view: sticky just under the app header,
 * hidden below `sm` (rows stack there). Decorative (`aria-hidden`): each table keeps its own
 * `<thead>` for screen readers.
 */
@Component({
  selector: 'tm-column-bar',
  template: `
    <div class="border-x border-transparent">
      <table class="tm-table min-w-[44rem] table-fixed">
        <colgroup>
          @for (col of columns; track col.label) {
            <col [class]="col.width" />
          }
        </colgroup>
        <thead>
          <tr>
            @for (col of columns; track col.label) {
              <th>{{ col.label }}</th>
            }
          </tr>
        </thead>
      </table>
    </div>
  `,
  host: {
    class: 'sticky top-16 z-30 -mx-4 mb-1 hidden bg-canvas/95 px-4 backdrop-blur sm:-mx-8 sm:block sm:px-8',
    'aria-hidden': 'true',
    'data-testid': 'column-bar',
  },
})
export class ContainerColumnBar {
  protected readonly columns = COLUMNS;
}

/**
 * One group of containers (a Compose stack, or "Ignored") as a card with a table.
 * Fixed column widths from COLUMNS, so tables line up across stacks under one tm-column-bar.
 */
@Component({
  selector: 'tm-container-table',
  imports: [RouterLink, ContainerRuleProblems, ContainerUpdate, ContainerVersion, TmAppIcon, TmBadge, TmCard],
  template: `
    <tm-card flush>
      <div class="overflow-x-auto">
        <!-- tm-table-stack: below 640px each row stacks (name, then version + update), see components.css -->
        <table class="tm-table tm-table-stack table-fixed sm:min-w-[44rem]">
          <colgroup>
            @for (col of columns; track col.label) {
              <col [class]="col.width" />
            }
          </colgroup>
          <thead class="sm:sr-only">
            <tr>
              @for (col of columns; track col.label) {
                <th scope="col">{{ col.label }}</th>
              }
            </tr>
          </thead>
          <tbody>
            @for (c of containers(); track c.id) {
              <!-- data-current: up to date, so the row is toned down; rows with an update stay prominent. -->
              <tr class="tm-row-link" [attr.data-current]="isCurrent(c) ? '' : null">
                <td data-stack="full" class="sm:py-3.5">
                  <div class="flex items-center gap-3">
                    <tm-app-icon [icon]="c.icon" [name]="c.name" [size]="20" [class.opacity-50]="quiet(c)" [class.grayscale]="quiet(c)" />
                    <div class="min-w-0">
                      <div class="flex items-center gap-2">
                        <!-- Stretched link: the whole row opens the detail page, but stays one link for keyboards and screen readers. -->
                        <a
                          [routerLink]="link()(c)"
                          class="truncate no-underline after:absolute after:inset-0 hover:no-underline"
                          [class]="nameClass(c)"
                          >{{ c.name }}</a
                        >
                        @if (c.state !== 'running') {
                          <tm-badge>{{ c.state }}</tm-badge>
                        }
                        <tm-rule-problems [container]="c" />
                      </div>
                      <div class="truncate text-xs text-fg-subtle">
                        @if ((ignored() || showStack()) && c.stack) {
                          <span data-testid="row-stack">{{ c.stack }}</span> ·
                        }
                        {{ c.status }}
                      </div>
                    </div>
                  </div>
                </td>
                <td data-stack="hide" class="sm:py-3.5">
                  <div class="truncate font-mono text-sm text-fg-muted">
                    {{ imageName(c) }}<span [class]="quiet(c) ? 'text-fg-muted' : 'text-fg'">:{{ c.image.tag || '—' }}</span>
                  </div>
                  @if (c.image.missing) {
                    <tm-badge tone="unknown" class="mt-1">image missing</tm-badge>
                  }
                </td>
                <td data-stack="indent" class="sm:py-3.5"><tm-container-version [container]="c" [checking]="checking()" /></td>
                <td class="sm:py-3.5"><tm-container-update [container]="c" [checking]="checking()" /></td>
              </tr>
            }
          </tbody>
        </table>
      </div>
    </tm-card>
  `,
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
