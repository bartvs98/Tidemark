import { Component, effect, ElementRef, input, model, viewChild } from '@angular/core';
import { TmButton } from './button';

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
  template: `
    <dialog #dialog class="tm-dialog" [attr.aria-labelledby]="headingId" (close)="open.set(false)" (click)="backdrop($event)">
      <div class="flex max-h-[inherit] flex-col">
        <header class="flex min-h-14 shrink-0 items-center justify-between gap-3 border-b border-line py-2 pr-3 pl-5">
          <h2 [id]="headingId" class="text-lg font-semibold">{{ heading() }}</h2>
          <button tmButton square variant="ghost" icon="x" aria-label="Close" (click)="open.set(false)"></button>
        </header>
        <div class="overflow-y-auto px-5 py-4"><ng-content /></div>
      </div>
    </dialog>
  `,
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
