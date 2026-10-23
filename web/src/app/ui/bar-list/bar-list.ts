import { Component, computed, inject, input, LOCALE_ID } from '@angular/core';
import { RouterLink } from '@angular/router';
import { ChartTone, TONE_COLOR } from '../chart-tip/chart';

export interface BarListItem {
  label: string;
  value: number;
  /** Shown instead of the value ("3 h", "12%"); the bar still follows `value`. */
  display?: string;
  /** Muted text after the label (a stack, `1.2 → 1.4`). */
  hint?: string;
  /** Makes the label a link: a router link for paths starting with '/', else an external link. */
  href?: string;
  tone?: ChartTone;
  /** Label in mono (versions, image refs, repos). */
  mono?: boolean;
}

/**
 * A ranked list with horizontal bars: label (and hint) left, value right, and under them a
 * bar proportional to the largest value (accent unless the item has a tone). For top-N lists
 * like "Updates per stack". The page sorts it. Plain text, so it's read as a list named
 * `label`; the bars are decoration.
 *
 *   <tm-bar-list label="Updates per stack" [items]="stacks" />
 */
@Component({
  selector: 'tm-bar-list',
  imports: [RouterLink],
  templateUrl: './bar-list.html',
  host: { class: 'block' },
})
export class TmBarList {
  private readonly locale = inject(LOCALE_ID);
  readonly label = input.required<string>();
  readonly items = input.required<BarListItem[]>();
  readonly empty = input('Nothing yet');

  protected readonly rows = computed(() => {
    const items = this.items();
    const max = Math.max(0, ...items.map((i) => i.value));
    return items.map((item) => ({
      ...item,
      text: item.display ?? item.value.toLocaleString(this.locale),
      pct: max > 0 ? Math.max(0, item.value) / max * 100 : 0,
      color: TONE_COLOR[item.tone ?? 'accent'],
      internal: !!item.href?.startsWith('/'),
    }));
  });
}
