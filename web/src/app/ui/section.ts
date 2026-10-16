import { Component, input } from '@angular/core';
import { IconName, TmIcon } from './icon';

let nextId = 0;

/**
 * A titled group of content, e.g. one Compose stack on the dashboard.
 *
 *   <tm-section heading="media" icon="layers" meta="3 containers">
 *     <button tmSectionActions tmButton size="sm">…</button>
 *     …content…
 *   </tm-section>
 */
@Component({
  selector: 'tm-section',
  imports: [TmIcon],
  template: `
    <header class="mb-3 flex min-h-7 items-center gap-2">
      @if (icon(); as icon) {
        <tm-icon [name]="icon" class="text-fg-subtle" />
      }
      <h2 [id]="headingId" class="text-lg font-semibold tracking-tight">{{ heading() }}</h2>
      @if (meta()) {
        <span class="text-sm text-fg-subtle">{{ meta() }}</span>
      }
      <div class="ml-auto flex items-center gap-2 empty:hidden"><ng-content select="[tmSectionActions]" /></div>
    </header>
    <ng-content />
  `,
  host: { class: 'block', role: 'region', '[attr.aria-labelledby]': 'headingId' },
})
export class TmSection {
  readonly heading = input.required<string>();
  readonly icon = input<IconName>();
  readonly meta = input<string>();
  protected readonly headingId = `tm-section-${nextId++}`;
}
