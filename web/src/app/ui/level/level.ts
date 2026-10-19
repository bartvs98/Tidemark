import { Component, input } from '@angular/core';
import { TmBadge } from '../badge/badge';

export type Level = 'none' | 'attention' | 'breaking' | 'unknown';

/** Risk badge. Renders nothing for level "none", so calm rows stay calm. */
@Component({
  selector: 'tm-level',
  imports: [TmBadge],
  templateUrl: './level.html',
  host: { class: 'inline-flex empty:hidden' },
})
export class TmLevel {
  readonly level = input.required<Level>();
}
