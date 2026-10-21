import { booleanAttribute, Component, computed, ElementRef, inject, input, model, output, viewChild } from '@angular/core';
import { FormCheckboxControl } from '@angular/forms/signals';
import { TM_FIELD } from '../field/field';

export type SwitchSize = 'sm' | 'md';

/**
 * An on/off switch: a native `<button role="switch" aria-checked>` with a track and a thumb.
 * Works with `[formField]` (a `FormCheckboxControl`) or standalone with `[(checked)]`.
 *
 *   <tm-field label="App icons" hint="Loaded from the icons URL." inline>
 *     <tm-switch [formField]="f.icons" />
 *   </tm-field>
 *   <tm-switch label="Dim quiet rows" [(checked)]="dim" />
 *
 * In a `tm-field` the field's label names it (`<label for>`); on its own, `label` is its
 * `aria-label`. `size`: md (20 px tall) or sm (16 px). The thumb doesn't slide under reduced motion.
 */
@Component({
  selector: 'tm-switch',
  templateUrl: './switch.html',
  host: { class: 'inline-flex' },
})
export class TmSwitch implements FormCheckboxControl {
  readonly checked = model(false);
  readonly disabled = input(false, { transform: booleanAttribute });
  readonly invalid = input(false, { transform: booleanAttribute });
  readonly touched = input(false, { transform: booleanAttribute });
  /** Accessible name when the switch isn't in a `tm-field`. */
  readonly label = input<string>();
  readonly size = input<SwitchSize>('md');

  /** Emitted on blur, so `[formField]` marks the field touched. */
  readonly touch = output<void>();

  protected readonly field = inject(TM_FIELD, { optional: true });
  private readonly button = viewChild.required<ElementRef<HTMLButtonElement>>('button');

  protected readonly isInvalid = computed(() => (this.invalid() && this.touched()) || !!this.field?.invalid());

  focus(options?: FocusOptions): void {
    this.button().nativeElement.focus(options);
  }

  protected toggle(): void {
    this.checked.update((v) => !v);
  }
}
