import { booleanAttribute, Component, computed, inject, input, LOCALE_ID } from '@angular/core';
import { IconName, TmIcon } from '../icon/icon';

/**
 * A headline number: a muted label, the value (large, proportional figures), an optional
 * small unit after it and a muted hint line under it. No box of its own: put it in a grid,
 * or a card. A number is grouped (1,284); a string is shown as given.
 *
 *   <tm-stat label="Median time to update" value="3" unit="days" hint="12 updates" />
 *   <tm-stat label="Fastest update" value="1.6.0" mono hint="jellyfin" icon="clock" />
 */
@Component({
  selector: 'tm-stat',
  imports: [TmIcon],
  templateUrl: './stat.html',
  host: { class: 'block min-w-0' },
})
export class TmStat {
  private readonly locale = inject(LOCALE_ID);
  readonly label = input.required<string>();
  readonly value = input.required<string | number>();
  readonly unit = input<string>();
  readonly hint = input<string>();
  /** Value in mono (versions). */
  readonly mono = input(false, { transform: booleanAttribute });
  readonly icon = input<IconName>();

  protected readonly text = computed(() => {
    const v = this.value();
    return typeof v === 'number' ? v.toLocaleString(this.locale) : v;
  });
}
