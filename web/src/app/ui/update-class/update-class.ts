import { Component, input } from '@angular/core';
import { TmBadge } from '../badge/badge';
import { TmIcon } from '../icon/icon';

/*
 * Tidemark's status language (docs/BRAND.md → Status):
 *   - Update class (how big the jump is) is always NEUTRAL.
 *   - Level (how risky it is) is the ONLY thing that gets colour.
 *   - Every status has an icon and a word, never colour alone.
 */

export type UpdateClass = 'none' | 'patch' | 'minor' | 'major' | 'digest';

/** "Up to date" check, or a neutral badge with a tide dot: patch / minor / major / digest. */
@Component({
  selector: 'tm-update-class',
  imports: [TmBadge, TmIcon],
  templateUrl: './update-class.html',
  host: { class: 'inline-flex' },
})
export class TmUpdateClass {
  readonly kind = input.required<UpdateClass>();
}
