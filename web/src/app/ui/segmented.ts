import { booleanAttribute, Component, input, model } from '@angular/core';
import { IconName, TmIcon } from './icon';

export interface SegmentOption<T extends string> {
  value: T;
  label: string;
  icon?: IconName;
  /** Shown muted after the label ("Updates 11"), e.g. for filters. */
  count?: number;
}

/**
 * A small set of mutually exclusive toggle buttons (view mode, theme, filters…).
 *
 *   <tm-segmented label="View" [options]="views" [(value)]="view" />
 *
 * `iconOnly` hides the labels visually; they stay as aria-label and tooltip.
 */
@Component({
  selector: 'tm-segmented',
  imports: [TmIcon],
  template: `
    @for (option of options(); track option.value) {
      <button
        type="button"
        class="inline-flex h-7 cursor-pointer items-center justify-center gap-1.5 rounded-sm px-2 text-sm font-medium text-fg-muted transition-colors hover:text-fg aria-pressed:bg-hover aria-pressed:text-fg"
        [class.w-7]="iconOnly()"
        [class.px-0]="iconOnly()"
        [attr.aria-pressed]="value() === option.value"
        [attr.aria-label]="iconOnly() ? option.label : null"
        [attr.title]="iconOnly() ? option.label : null"
        (click)="value.set(option.value)"
      >
        @if (option.icon) {
          <tm-icon [name]="option.icon" [size]="14" />
        }
        @if (!iconOnly()) {
          {{ option.label }}
          @if (option.count !== undefined) {
            <span class="text-xs text-fg-subtle tabular-nums">{{ option.count }}</span>
          }
        }
      </button>
    }
  `,
  host: {
    class: 'inline-flex gap-0.5 rounded-md border border-line bg-surface p-0.5',
    role: 'group',
    '[attr.aria-label]': 'label()',
  },
})
export class TmSegmented<T extends string> {
  readonly options = input.required<SegmentOption<T>[]>();
  readonly value = model.required<T>();
  readonly label = input.required<string>();
  readonly iconOnly = input(false, { transform: booleanAttribute });
}
