import { Component, ElementRef, input, model, viewChild } from '@angular/core';
import { TmIcon } from '../icon/icon';

/**
 * A search field: leading magnifier, a clear button while there is text, Escape clears.
 * 36 px tall, level with md buttons and `tm-segmented size="md"` in a toolbar.
 *
 *   <tm-search label="Search containers" placeholder="Search…" [(value)]="query" />
 *
 * `label` is the accessible name (there is no visible label). `focus()` moves focus into the field.
 */
@Component({
  selector: 'tm-search',
  imports: [TmIcon],
  templateUrl: './search.html',
  styleUrl: './search.css',
  host: { class: 'relative block h-9 min-w-0' },
})
export class TmSearch {
  readonly value = model('');
  readonly label = input.required<string>();
  readonly placeholder = input('Search');

  private readonly field = viewChild.required<ElementRef<HTMLInputElement>>('field');

  focus(): void {
    this.field().nativeElement.focus();
  }

  protected clear(): void {
    this.value.set('');
    this.field().nativeElement.focus();
  }

  protected onEscape(event: Event): void {
    if (!this.value()) return;
    event.preventDefault();
    this.value.set('');
  }
}
