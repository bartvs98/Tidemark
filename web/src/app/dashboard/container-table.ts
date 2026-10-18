import { booleanAttribute, Component, input } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TmAppIcon, TmBadge, TmCard } from '../ui';
import { ContainerSummary, imageName } from './containers';
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
    class: 'sticky top-14 z-30 -mx-4 mb-1 hidden bg-canvas/95 px-4 backdrop-blur sm:-mx-6 sm:block sm:px-6',
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
              <tr class="tm-row-link">
                <td data-stack="full">
                  <div class="flex items-center gap-3">
                    <tm-app-icon [icon]="c.icon" [name]="c.name" [size]="20" />
                    <div class="min-w-0">
                      <div class="flex items-center gap-2">
                        <!-- Stretched link: the whole row opens the detail page, but stays one link for keyboards and screen readers. -->
                        <a
                          [routerLink]="link()(c)"
                          class="truncate font-medium no-underline after:absolute after:inset-0 hover:no-underline"
                          [class]="ignored() ? 'text-fg-muted' : 'text-fg'"
                          [title]="c.name"
                          >{{ c.name }}</a
                        >
                        @if (c.state !== 'running') {
                          <tm-badge>{{ c.state }}</tm-badge>
                        }
                        <tm-rule-problems [container]="c" />
                      </div>
                      <div class="truncate text-xs text-fg-subtle" [title]="c.status">
                        @if (ignored() && c.stack) {
                          {{ c.stack }} ·
                        }
                        {{ c.status }}
                      </div>
                    </div>
                  </div>
                </td>
                <td data-stack="hide">
                  <div class="truncate font-mono text-sm text-fg-muted" [title]="c.image.display">
                    {{ imageName(c) }}<span [class]="ignored() ? 'text-fg-muted' : 'text-fg'">:{{ c.image.tag || '—' }}</span>
                  </div>
                  @if (c.image.missing) {
                    <tm-badge tone="unknown" class="mt-1">image missing</tm-badge>
                  }
                </td>
                <td data-stack="indent"><tm-container-version [container]="c" [checking]="checking()" /></td>
                <td><tm-container-update [container]="c" [checking]="checking()" /></td>
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
  /** Where each row links to; the styleguide points this somewhere harmless. */
  readonly link = input<ContainerLink>(DETAIL_LINK);

  protected readonly columns = COLUMNS;
  protected readonly imageName = imageName;
}
