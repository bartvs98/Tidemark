import { booleanAttribute, Component, input, model } from '@angular/core';
import { IconName, TmIcon } from '../icon/icon';
import { TmTooltip } from '../tooltip/tooltip';

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
  imports: [TmIcon, TmTooltip],
  templateUrl: './segmented.html',
  host: {
    class: 'box-border inline-flex items-center gap-0.5 rounded-md border border-line bg-surface',
    '[class.p-0.5]': "size() === 'sm'",
    '[class.p-px]': "size() === 'md'",
    '[class.h-9]': "size() === 'md'",
    role: 'group',
    '[attr.aria-label]': 'label()',
  },
})
export class TmSegmented<T extends string> {
  readonly options = input.required<SegmentOption<T>[]>();
  readonly value = model.required<T>();
  readonly label = input.required<string>();
  readonly iconOnly = input(false, { transform: booleanAttribute });
  /** sm: 32 px tall (inline use); md: 36 px, matching a md button (toolbars). */
  readonly size = input<'sm' | 'md'>('sm');
}
