import { booleanAttribute, Component, input, model } from '@angular/core';
import { IconName, TmIcon } from './icon';

let nextId = 0;

/**
 * A titled group of content, e.g. one Compose stack on the dashboard.
 *
 *   <tm-section heading="media" icon="layers" meta="3 containers">
 *     <button tmSectionActions tmButton size="sm">…</button>
 *     …content…
 *   </tm-section>
 *
 * `collapsible` turns the heading into a disclosure button (`[(open)]`, closed by default);
 * `muted` tones the header down for groups that matter less, like ignored containers.
 */
@Component({
  selector: 'tm-section',
  imports: [TmIcon],
  template: `
    <header class="mb-4 flex min-h-8 items-center gap-2.5" [class.text-fg-muted]="muted()">
      @if (collapsible()) {
        <h2 [id]="headingId" class="text-xl font-semibold tracking-tight">
          <button
            type="button"
            class="-mx-1 flex cursor-pointer items-center gap-2 rounded-sm px-1 hover:text-fg"
            [attr.aria-expanded]="open()"
            [attr.aria-controls]="bodyId"
            (click)="open.set(!open())"
          >
            <tm-icon name="chevron-right" [size]="14" class="text-fg-subtle transition-transform" [class.rotate-90]="open()" />
            @if (icon(); as icon) {
              <tm-icon [name]="icon" class="text-fg-subtle" />
            }
            {{ heading() }}
          </button>
        </h2>
      } @else {
        @if (icon(); as icon) {
          <tm-icon [name]="icon" class="text-fg-subtle" />
        }
        <h2 [id]="headingId" class="text-xl font-semibold tracking-tight">{{ heading() }}</h2>
      }
      @if (meta()) {
        <span class="text-sm text-fg-subtle">{{ meta() }}</span>
      }
      <div class="ml-auto flex items-center gap-2 empty:hidden"><ng-content select="[tmSectionActions]" /></div>
    </header>
    <div [id]="bodyId" [hidden]="collapsible() && !open()"><ng-content /></div>
  `,
  host: { class: 'block', role: 'region', '[attr.aria-labelledby]': 'headingId' },
})
export class TmSection {
  readonly heading = input.required<string>();
  readonly icon = input<IconName>();
  readonly meta = input<string>();
  readonly collapsible = input(false, { transform: booleanAttribute });
  readonly muted = input(false, { transform: booleanAttribute });
  /** Only used when collapsible. */
  readonly open = model(false);
  protected readonly headingId = `tm-section-${nextId++}`;
  protected readonly bodyId = `${this.headingId}-body`;
}
