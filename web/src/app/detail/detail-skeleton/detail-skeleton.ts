import { Component } from '@angular/core';
import { TmSkeleton } from '../../ui';

/**
 * The detail page while it loads: the header card (icon, name, image; versions, badges and gauge at
 * the right), the links row, the tab row and two release cards, with the same boxes as the loaded
 * page. The 'Containers' back link stays in the page itself.
 */
@Component({
  selector: 'tm-detail-skeleton',
  imports: [TmSkeleton],
  templateUrl: './detail-skeleton.html',
  host: { role: 'status', 'aria-label': 'Loading container', class: 'block' },
})
export class DetailSkeleton {
  protected readonly tabs = ['5rem', '4rem', '4.5rem'];
}
