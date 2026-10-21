import {
  booleanAttribute,
  Component,
  computed,
  effect,
  ElementRef,
  inject,
  input,
  model,
  output,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { Combobox, ComboboxPopup, ComboboxWidget } from '@angular/aria/combobox';
import { Listbox, Option } from '@angular/aria/listbox';
import { FormValueControl } from '@angular/forms/signals';
import { TM_FIELD } from '../field/field';
import { TmIcon } from '../icon/icon';
import { INPUT_BASE, INPUT_SIZES, InputSize } from '../input/input';

export interface SelectOption<T extends string = string> {
  value: T;
  label: string;
  /** A muted line under the label. */
  hint?: string;
}

/**
 * Pick one of a few options: a trigger that looks like a `tmInput` and opens a listbox. Built on
 * Angular Aria's combobox (select pattern) and listbox: arrow keys, Home/End, typeahead,
 * Enter/Space choose, Escape closes; focus stays on the trigger. Works with `[formField]` (a
 * `FormValueControl<T>`) or standalone with `[(value)]`; `''` means nothing chosen (the
 * placeholder shows).
 *
 *   <tm-field label="Notify about">
 *     <tm-select [options]="levels" [formField]="f.notifyLevel" />
 *   </tm-field>
 *   <tm-select label="Sort by" size="sm" [options]="sorts" [(value)]="sort" />
 *
 * In a `tm-field` the field's label names it (`aria-labelledby`); on its own, `label` is its
 * `aria-label`. The popup is absolutely positioned, so don't put it in a `tm-card` (clips overflow).
 */
@Component({
  selector: 'tm-select',
  imports: [Combobox, ComboboxPopup, ComboboxWidget, Listbox, Option, TmIcon],
  templateUrl: './select.html',
  host: { class: 'relative block min-w-0' },
})
export class TmSelect<T extends string> implements FormValueControl<T> {
  readonly value = model<T>('' as T);
  readonly options = input.required<SelectOption<T>[]>();
  readonly placeholder = input('Choose…');
  /** Accessible name when the select isn't in a `tm-field`. */
  readonly label = input<string>();
  readonly size = input<InputSize>('md');
  readonly disabled = input(false, { transform: booleanAttribute });
  readonly invalid = input(false, { transform: booleanAttribute });
  readonly touched = input(false, { transform: booleanAttribute });

  /** Emitted when focus leaves the select, so `[formField]` marks the field touched. */
  readonly touch = output<void>();

  protected readonly field = inject(TM_FIELD, { optional: true });
  protected readonly expanded = signal(false);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly trigger = viewChild.required<Combobox>(Combobox);
  private readonly listbox = viewChild(Listbox);

  protected readonly selected = computed(() => this.options().find((o) => o.value === this.value()));
  /** The listbox works on arrays. */
  protected readonly listValue = computed(() => (this.value() ? [this.value()] : []));
  protected readonly isInvalid = computed(() => (this.invalid() && this.touched()) || !!this.field?.invalid());
  protected readonly triggerClass = computed(
    () => `${INPUT_BASE} ${INPUT_SIZES[this.size()]} flex cursor-pointer items-center gap-2 pr-2 text-left select-none`,
  );

  constructor() {
    // Keep the active option in view while moving through a long list.
    effect(() => {
      const listbox = this.listbox();
      if (!listbox?.activeDescendant()) return;
      untracked(() => {
        try {
          listbox.scrollActiveItemIntoView({ block: 'nearest' });
        } catch {
          // jsdom has no scrollIntoView
        }
      });
    });
  }

  focus(options?: FocusOptions): void {
    this.trigger().element.focus(options);
  }

  protected choose(values: T[]): void {
    // Choosing the selected option again toggles it off in the listbox: keep the value then.
    if (values.length) this.value.set(values[0]);
    this.expanded.set(false);
    this.focus();
  }

  protected onFocusout(event: FocusEvent): void {
    if (!this.host.nativeElement.contains(event.relatedTarget as Node | null)) this.touch.emit();
  }
}
