import { booleanAttribute, Component, computed, input } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TmAppIcon, TmBadge } from '../ui';
import { ContainerLink, DETAIL_LINK } from './container-table';
import { ContainerSummary, hasUpdate, imageName, isCurrent, isIgnored } from './containers';
import { ContainerRuleProblems, ContainerUpdate, ContainerVersion } from './update-info';

const BASE =
  'relative flex min-w-0 flex-col rounded-xl border p-5 transition-colors hover:border-line-strong has-[a:focus-visible]:outline-2 has-[a:focus-visible]:outline-focus';
/** Up to date or ignored: no surface or shadow, so tiles with an update stand out. */
const QUIET = `${BASE} border-line bg-transparent`;
const PROMINENT = `${BASE} border-line bg-surface shadow-raised`;

/** One container in tile mode. */
@Component({
  selector: 'tm-container-tile',
  imports: [RouterLink, ContainerRuleProblems, ContainerUpdate, ContainerVersion, TmAppIcon, TmBadge],
  template: `
    <div class="mb-5 flex items-start gap-3">
      <tm-app-icon
        [icon]="container().icon"
        [name]="container().name"
        [size]="32"
        [class.opacity-50]="quiet()"
        [class.grayscale]="quiet()"
      />
      <div class="min-w-0 flex-1">
        <div class="flex items-start justify-between gap-2">
          <h3 class="truncate">
            <!-- Stretched link: the whole tile is clickable, but it stays one link for keyboards and screen readers. -->
            <a
              [routerLink]="link()(container())"
              class="no-underline after:absolute after:inset-0 after:rounded-xl hover:no-underline"
              [class]="nameClass()"
              >{{ container().name }}</a
            >
          </h3>
          <div class="flex shrink-0 items-center gap-1.5 empty:hidden">
            <tm-rule-problems [container]="container()" />
            @if (container().state !== 'running') {
              <tm-badge>{{ container().state }}</tm-badge>
            }
          </div>
        </div>
        <p class="mt-0.5 truncate font-mono text-xs text-fg-subtle">
          {{ image() }}:{{ container().image.tag || '—' }}
        </p>
        <p class="truncate text-xs text-fg-subtle">
          @if (showStack() && container().stack) {
            <span data-testid="tile-stack">{{ container().stack }}</span> ·
          }
          {{ container().status }}
        </p>
      </div>
    </div>

    <!-- Version on its own line, badges below it: a long "2.17.2 → 3.5.1" never runs into them. -->
    <div class="mt-auto flex flex-col gap-2 border-t border-line pt-4">
      <tm-container-version [container]="container()" [checking]="checking()" />
      <tm-container-update [container]="container()" [checking]="checking()" class="empty:hidden" />
    </div>
  `,
  host: {
    '[class]': 'hostClass()',
    '[attr.data-current]': "current() ? '' : null",
  },
})
export class ContainerTile {
  readonly container = input.required<ContainerSummary>();
  readonly checking = input(false);
  /** Show the stack before the status, e.g. when grouped by severity. */
  readonly showStack = input(false, { transform: booleanAttribute });
  /** Where the tile links to; the styleguide points this somewhere harmless. */
  readonly link = input<ContainerLink>(DETAIL_LINK);
  protected readonly image = computed(() => imageName(this.container()));
  protected readonly current = computed(() => isCurrent(this.container()));
  /** Ignored or up to date: toned down. */
  protected readonly quiet = computed(() => this.current() || isIgnored(this.container()));
  protected readonly hostClass = computed(() => (this.quiet() ? QUIET : PROMINENT));
  protected readonly nameClass = computed(() => {
    if (this.quiet()) return 'font-medium text-fg-muted';
    return hasUpdate(this.container()) ? 'font-semibold text-fg' : 'font-medium text-fg';
  });
}
