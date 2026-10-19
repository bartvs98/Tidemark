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
  templateUrl: './card.html',
  host: { class: 'block overflow-hidden rounded-lg border border-line bg-surface shadow-raised' },
})
export class TmCard {
  readonly heading = input<string>();
  readonly flush = input(false, { transform: booleanAttribute });
}
