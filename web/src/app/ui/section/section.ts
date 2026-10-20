import { booleanAttribute, Component, computed, input, model } from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { IconName, TmIcon } from '../icon/icon';

let nextId = 0;

/**
 * A titled group of content, e.g. one Compose stack on the dashboard.
 *
 *   <tm-section heading="media" icon="layers" meta="3 containers">
 *     <button tmSectionActions tmButton size="sm">…</button>
 *     …content…
 *   </tm-section>
 *
 * `collapsible` turns the heading into a disclosure button (`[(open)]`, closed by default);
 * `muted` tones the header down for groups that matter less, like ignored containers.
 * `level` 3 makes it a smaller `<h3>` sub-section, for sections nested inside another one.
 */
@Component({
  selector: 'tm-section',
  imports: [NgTemplateOutlet, TmIcon],
  templateUrl: './section.html',
  host: { class: 'block', role: 'region', '[attr.aria-labelledby]': 'headingId' },
})
export class TmSection {
  readonly heading = input.required<string>();
  readonly icon = input<IconName>();
  readonly meta = input<string>();
  readonly collapsible = input(false, { transform: booleanAttribute });
  readonly muted = input(false, { transform: booleanAttribute });
  /** Heading level: 2 for a top-level section (default), 3 for one nested in another section. */
  readonly level = input<2 | 3>(2);
  /** Only used when collapsible. */
  readonly open = model(false);
  protected readonly headingId = `tm-section-${nextId++}`;
  protected readonly bodyId = `${this.headingId}-body`;
  protected readonly headingClass = computed(() =>
    this.level() === 3 ? 'text-base font-semibold' : 'text-xl font-semibold tracking-tight',
  );
}
