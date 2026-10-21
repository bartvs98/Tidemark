import { booleanAttribute, Component, computed, ElementRef, inject, InjectionToken, input, Signal } from '@angular/core';
import { TmIcon } from '../icon/icon';

/**
 * What a `tm-field` tells the control inside it. `tmInput`, `tm-password-input`, `tm-select` and
 * `tm-switch` inject it (optionally) and put these on their focusable element.
 */
export interface FieldContext {
  /** `id` for the focusable element; the `<label for>` points at it. */
  readonly controlId: Signal<string>;
  /** The label's `id`, for controls `<label for>` can't reach (`aria-labelledby`). */
  readonly labelId: Signal<string>;
  /** The id of the message under the control (error, locked note or hint), or null. */
  readonly describedBy: Signal<string | null>;
  /** True while the field shows an error. */
  readonly invalid: Signal<boolean>;
  /** The field is marked required (`aria-required` on the control). */
  readonly required: Signal<boolean>;
}

export const TM_FIELD = new InjectionToken<FieldContext>('TM_FIELD');

let nextId = 0;

/** Elements a `<label for>` focuses or activates by itself. */
const LABELABLE = new Set(['INPUT', 'TEXTAREA', 'SELECT', 'BUTTON']);

/**
 * A form row: a label, one projected control, and under it a hint, an error or why it's locked.
 * The control picks up the field's ids, so `<label for>`, `aria-describedby` and `aria-invalid`
 * just work.
 *
 *   <tm-field label="Schedule" hint="Cron, or off." [error]="errors().schedule" [locked]="lockedBy('schedule')">
 *     <input tmInput mono [formField]="f.schedule" />
 *   </tm-field>
 *   <tm-field label="Icons" inline><tm-switch [(checked)]="icons" /></tm-field>
 *
 * `error` replaces the hint (attention colour, with an icon); `locked` ("Set by TIDEMARK_SCHEDULE")
 * replaces both with a lock icon and the env var in mono; the page disables the control itself.
 * `inline` puts the label left and the control right (for switches).
 */
@Component({
  selector: 'tm-field',
  imports: [TmIcon],
  templateUrl: './field.html',
  providers: [{ provide: TM_FIELD, useExisting: TmField }],
  host: { '[class]': 'inline() ? "grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-0.5" : "grid min-w-0 content-start gap-1.5"' },
})
export class TmField implements FieldContext {
  readonly label = input.required<string>();
  readonly hint = input<string | null | undefined>();
  readonly error = input<string | null | undefined>();
  /** Why the control can't be changed here, e.g. "Set by TIDEMARK_SCHEDULE". */
  readonly locked = input<string | null | undefined>();
  readonly required = input(false, { transform: booleanAttribute });
  readonly inline = input(false, { transform: booleanAttribute });

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly id = `tm-field-${++nextId}`;

  readonly controlId = computed(() => `${this.id}-control`);
  readonly labelId = computed(() => `${this.id}-label`);
  protected readonly messageId = `${this.id}-message`;

  /** Which message shows: an error wins over the locked note, which wins over the hint. */
  protected readonly message = computed<'error' | 'locked' | 'hint' | null>(() =>
    this.error() ? 'error' : this.locked() ? 'locked' : this.hint() ? 'hint' : null,
  );
  readonly describedBy = computed(() => (this.message() ? this.messageId : null));
  readonly invalid = computed(() => !!this.error());

  /** The locked text split into plain and env-var parts (`TIDEMARK_X` is shown mono). */
  protected readonly lockedParts = computed(() =>
    (this.locked() ?? '')
      .split(/([A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+)/)
      .map((text, i) => ({ text, mono: i % 2 === 1 }))
      .filter((p) => p.text),
  );

  /** A label can't focus a `div[role=combobox]`; do it by hand. */
  protected focusControl(): void {
    const control = this.host.nativeElement.querySelector<HTMLElement>(`#${this.controlId()}`);
    if (control && !LABELABLE.has(control.tagName)) control.focus();
  }
}
