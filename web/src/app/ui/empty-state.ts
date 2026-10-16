import { Component, input } from '@angular/core';
import { IconName, TmIcon } from './icon';

/** Shown when a list has nothing in it. Project buttons as content for the next step. */
@Component({
  selector: 'tm-empty-state',
  imports: [TmIcon],
  template: `
    <div class="mb-3 grid size-10 place-items-center rounded-lg bg-hover text-fg-muted">
      <tm-icon [name]="icon()" [size]="20" />
    </div>
    <p class="text-lg font-semibold">{{ heading() }}</p>
    @if (description()) {
      <p class="mt-1 max-w-sm text-fg-muted">{{ description() }}</p>
    }
    <div class="mt-4 flex gap-2 empty:hidden"><ng-content /></div>
  `,
  host: { class: 'flex flex-col items-center px-6 py-12 text-center' },
})
export class TmEmptyState {
  readonly heading = input.required<string>();
  readonly description = input<string>();
  readonly icon = input<IconName>('box');
}
