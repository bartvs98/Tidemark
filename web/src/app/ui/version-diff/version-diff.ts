import { Component, input } from '@angular/core';

/** `1.4.2 → 1.6.0` in mono. Without `to` it shows just the running version. */
@Component({
  selector: 'tm-version-diff',
  templateUrl: './version-diff.html',
  host: { class: 'inline-flex flex-wrap items-baseline gap-x-1.5 font-mono text-sm tabular-nums' },
})
export class TmVersionDiff {
  readonly from = input.required<string>();
  readonly to = input<string | null>();
}
