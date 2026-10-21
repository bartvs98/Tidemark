import { booleanAttribute, Component, ElementRef, inject, input, model, output, signal, viewChild } from '@angular/core';
import { FormValueControl } from '@angular/forms/signals';
import { TM_FIELD } from '../field/field';
import { TmIcon } from '../icon/icon';
import { InputSize, TmInput } from '../input/input';

/**
 * A secret field (tokens): a `tmInput` `type="password"` with an eye button that shows the
 * value as text and hides it again. Works with `[formField]` (a `FormValueControl<string>`) or
 * standalone with `[(value)]`; inside a `tm-field` the input takes the field's id.
 *
 *   <tm-field label="Bot token"><tm-password-input placeholder="123456:ABC…" [formField]="f.botToken" /></tm-field>
 *
 * On its own, `label` is the input's `aria-label`. Mono by default, `autocomplete="off"`.
 * `invalid` draws the attention border once `touched`
 * (both are bound by `[formField]`). Public `focus()`.
 */
@Component({
  selector: 'tm-password-input',
  imports: [TmIcon, TmInput],
  templateUrl: './password-input.html',
  host: { class: 'relative block min-w-0' },
})
export class TmPasswordInput implements FormValueControl<string> {
  readonly value = model('');
  readonly disabled = input(false, { transform: booleanAttribute });
  readonly readonly = input(false, { transform: booleanAttribute });
  readonly invalid = input(false, { transform: booleanAttribute });
  readonly touched = input(false, { transform: booleanAttribute });
  readonly name = input('');
  /** Accessible name when the input isn't in a `tm-field`. */
  readonly label = input<string>();
  readonly placeholder = input('');
  readonly autocomplete = input('off');
  readonly mono = input(true, { transform: booleanAttribute });
  readonly size = input<InputSize>('md');

  /** Emitted on blur, so `[formField]` marks the field touched. */
  readonly touch = output<void>();

  protected readonly tmField = inject(TM_FIELD, { optional: true });
  protected readonly shown = signal(false);
  private readonly field = viewChild.required<ElementRef<HTMLInputElement>>('field');

  focus(options?: FocusOptions): void {
    this.field().nativeElement.focus(options);
  }

  protected toggle(): void {
    this.shown.update((v) => !v);
  }
}
