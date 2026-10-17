import { Component, computed, input } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TmAppIcon, TmBadge } from '../ui';
import { ContainerSummary, imageName, isIgnored } from './containers';
import { ContainerRuleProblems, ContainerUpdate, ContainerVersion } from './update-info';

/** One container in tile mode. */
@Component({
  selector: 'tm-container-tile',
  imports: [RouterLink, ContainerRuleProblems, ContainerUpdate, ContainerVersion, TmAppIcon, TmBadge],
  template: `
    <div class="mb-4 flex items-start gap-3">
      <tm-app-icon [icon]="container().icon" [name]="container().name" [size]="28" />
      <div class="min-w-0 flex-1">
        <div class="flex items-start justify-between gap-2">
          <h3 class="truncate font-medium" [title]="container().name">
            <!-- Stretched link: the whole tile is clickable, but it stays one link for keyboards and screen readers. -->
            <a
              [routerLink]="['/containers', container().name]"
              class="no-underline after:absolute after:inset-0 after:rounded-lg hover:no-underline"
              [class]="ignored() ? 'text-fg-muted' : 'text-fg'"
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
        <p class="mt-0.5 truncate font-mono text-xs text-fg-subtle" [title]="container().image.display">
          {{ image() }}:{{ container().image.tag || '—' }}
        </p>
        <p class="truncate text-xs text-fg-subtle" [title]="container().status">{{ container().status }}</p>
      </div>
    </div>

    <div class="mt-auto flex items-start justify-between gap-3 border-t border-line pt-3">
      <tm-container-version [container]="container()" [checking]="checking()" class="flex-1" />
      <tm-container-update [container]="container()" [checking]="checking()" class="shrink-0" />
    </div>
  `,
  host: {
    class:
      'relative flex min-w-0 flex-col rounded-lg border border-line bg-surface p-4 shadow-raised transition-colors hover:border-line-strong has-[a:focus-visible]:outline-2 has-[a:focus-visible]:outline-focus',
  },
})
export class ContainerTile {
  readonly container = input.required<ContainerSummary>();
  readonly checking = input(false);
  protected readonly image = computed(() => imageName(this.container()));
  protected readonly ignored = computed(() => isIgnored(this.container()));
}
