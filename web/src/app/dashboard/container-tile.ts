import { Component, computed, input } from '@angular/core';
import { TmAppIcon, TmBadge } from '../ui';
import { ContainerSummary, imageName } from './containers';
import { ContainerUpdate, ContainerVersion } from './update-info';

/** One container in tile mode. */
@Component({
  selector: 'tm-container-tile',
  imports: [ContainerUpdate, ContainerVersion, TmAppIcon, TmBadge],
  template: `
    <div class="mb-4 flex items-start gap-3">
      <tm-app-icon [icon]="container().icon" [name]="container().name" [size]="28" />
      <div class="min-w-0 flex-1">
        <div class="flex items-start justify-between gap-2">
          <h3 class="truncate font-medium" [title]="container().name">{{ container().name }}</h3>
          @if (container().state !== 'running') {
            <tm-badge>{{ container().state }}</tm-badge>
          }
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
  host: { class: 'flex min-w-0 flex-col rounded-lg border border-line bg-surface p-4 shadow-raised' },
})
export class ContainerTile {
  readonly container = input.required<ContainerSummary>();
  readonly checking = input(false);
  protected readonly image = computed(() => imageName(this.container()));
}
