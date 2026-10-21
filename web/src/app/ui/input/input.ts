import { booleanAttribute, computed, Directive, ElementRef, inject, input } from '@angular/core';
import { TM_FIELD } from '../field/field';

export type InputSize = 'sm' | 'md';

/** The look shared by text inputs and the `tm-select` trigger. */
export const INPUT_BASE =
  'box-border w-full min-w-0 rounded-md border border-line-strong bg-surface text-fg transition-colors ' +
  'placeholder:text-fg-subtle hover:border-fg-subtle ' +
  'aria-invalid:border-attention aria-invalid:hover:border-attention ' +
  'disabled:cursor-not-allowed disabled:border-line disabled:bg-canvas disabled:text-fg-muted disabled:hover:border-line ' +
  'aria-disabled:cursor-not-allowed aria-disabled:border-line aria-disabled:bg-canvas aria-disabled:text-fg-muted aria-disabled:hover:border-line';

/** Height and text size per size: md is 36 px (a md button, `tm-search`), sm 32 px. */
export const INPUT_SIZES: Record<InputSize, string> = {
  md: 'h-9 px-3 text-base',
  sm: 'h-8 px-2.5 text-sm',
};

/**
 * Styles a native `<input>` (text, url, password, number…) or `<textarea>`, so `[formField]`,
 * `disabled`, `readonly` and `placeholder` all work as usual. Inside a `tm-field` it takes the
 * field's id, description and error state.
 *
 *   <tm-field label="Public URL" hint="Links in notifications point here.">
 *     <input tmInput mono type="url" placeholder="https://tidemark.lan" [formField]="f.publicUrl" />
 *   </tm-field>
 *
 * `mono` for cron specs, URLs, IDs and tokens. An error on the field, or `invalid` once `touched`
 * (both bound by `[formField]`), sets `aria-invalid="true"`, which draws the attention border.
 */
@Directive({
  selector: 'input[tmInput], textarea[tmInput]',
  host: {
    '[class]': 'classes()',
    '[attr.id]': 'controlId()',
    '[attr.aria-describedby]': 'field?.describedBy() ?? null',
    '[attr.aria-invalid]': "(invalid() && touched()) || field?.invalid() ? 'true' : null",
    '[attr.aria-required]': "field?.required() ? 'true' : null",
  },
})
export class TmInput {
  readonly size = input<InputSize>('md');
  readonly mono = input(false, { transform: booleanAttribute });
  readonly invalid = input(false, { transform: booleanAttribute });
  readonly touched = input(false, { transform: booleanAttribute });
  /** Used outside a `tm-field`; inside one the field's id wins (its label points there). */
  readonly id = input<string>();

  protected readonly field = inject(TM_FIELD, { optional: true });
  private readonly textarea = inject<ElementRef<HTMLElement>>(ElementRef).nativeElement.tagName === 'TEXTAREA';

  protected readonly controlId = computed(() => this.field?.controlId() ?? this.id() ?? null);

  protected readonly classes = computed(() => {
    const size = this.textarea
      ? this.size() === 'sm'
        ? 'min-h-16 px-2.5 py-1.5 text-sm'
        : 'min-h-20 px-3 py-2 text-base'
      : INPUT_SIZES[this.size()];
    return [INPUT_BASE, 'read-only:bg-canvas', size, this.mono() ? 'font-mono' : 'font-sans'].join(' ');
  });
}
