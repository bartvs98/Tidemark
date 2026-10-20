import { Component, input } from '@angular/core';
import { TmCard, TmSkeleton } from '../../ui';
import { ContainerColumnBar } from '../column-bar/column-bar';
import { COLUMNS } from '../container-table/container-table';

/**
 * The dashboard while the first response is on its way: the toolbar and the list (or the tile grid)
 * with the same boxes as the loaded page, so nothing jumps when data arrives. Two sections, three
 * and two containers. Follows `view`, which is known before data loads. The hero's headline, summary
 * and figures have their own skeletons in the dashboard itself.
 */
@Component({
  selector: 'tm-dashboard-skeleton',
  imports: [TmCard, TmSkeleton, ContainerColumnBar],
  templateUrl: './dashboard-skeleton.html',
  host: { role: 'status', 'aria-label': 'Loading containers', class: 'block' },
})
export class DashboardSkeleton {
  readonly view = input<'list' | 'tiles'>('list');

  protected readonly columns = COLUMNS;
  /** Containers per section. */
  protected readonly sections = [3, 2];
  protected rows(n: number): number[] {
    return Array.from({ length: n }, (_, i) => i);
  }
}
