import { booleanAttribute, Component, input } from '@angular/core';

/**
 * Surface for grouped content.
 *
 *   <tm-card heading="Backend">
 *     <button tmCardActions tmButton size="sm">Refresh</button>
 *     Body…
 *   </tm-card>
 *
 * `flush` removes body padding, e.g. for a table that runs edge to edge.
 */
@Component({
  selector: 'tm-card',
  template: `
    @if (heading()) {
      <header class="flex min-h-12 items-center justify-between gap-3 border-b border-line px-4 py-2">
        <h3 class="text-base font-semibold">{{ heading() }}</h3>
        <div class="flex items-center gap-2"><ng-content select="[tmCardActions]" /></div>
      </header>
    }
    <div [class.p-4]="!flush()"><ng-content /></div>
  `,
  host: { class: 'block overflow-hidden rounded-lg border border-line bg-surface shadow-raised' },
})
export class TmCard {
  readonly heading = input<string>();
  readonly flush = input(false, { transform: booleanAttribute });
}
