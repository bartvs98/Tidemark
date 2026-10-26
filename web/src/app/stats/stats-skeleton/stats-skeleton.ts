import { Component } from '@angular/core';
import { TmCard, TmSkeleton } from '../../ui';

/** The stats page while the first response is on its way: a stat row and two chart cards. */
@Component({
  selector: 'tm-stats-skeleton',
  imports: [TmCard, TmSkeleton],
  templateUrl: './stats-skeleton.html',
  host: { role: 'status', 'aria-label': 'Loading stats', class: 'block' },
})
export class StatsSkeleton {
  protected readonly items = [1, 2, 3, 4, 5, 6];
}
