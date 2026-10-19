import { Component, effect, ElementRef, input, model, viewChild } from '@angular/core';
import { TmButton } from '../button/button';

let nextId = 0;

/**
 * A modal dialog on the native <dialog> element (showModal): the browser traps focus, makes
 * the page behind it inert, closes on Escape and returns focus to the opener. A click on the
 * backdrop or the close button closes it too. Two-way `[(open)]`; content is projected, so
 * wrap anything that should only load while open in `@if (open)`.
 *
 *   <button tmButton square variant="ghost" icon="settings" aria-label="Settings" (click)="open.set(true)"></button>
 *   <tm-dialog heading="Settings" [(open)]="open">…</tm-dialog>
 */
@Component({
  selector: 'tm-dialog',
  imports: [TmButton],
  templateUrl: './dialog.html',
  host: { class: 'contents' },
})
export class TmDialog {
  readonly heading = input.required<string>();
  readonly open = model(false);

  protected readonly headingId = `tm-dialog-${nextId++}`;
  private readonly dialog = viewChild.required<ElementRef<HTMLDialogElement>>('dialog');

  constructor() {
    effect(() => {
      const el = this.dialog().nativeElement;
      if (this.open() && !el.open) el.showModal();
      else if (!this.open() && el.open) el.close();
    });
  }

  /** Clicks on the dialog element itself (not its content) land on the backdrop. */
  protected backdrop(event: MouseEvent) {
    if (event.target === this.dialog().nativeElement) this.open.set(false);
  }
}
