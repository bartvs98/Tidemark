import { Component } from '@angular/core';
import { COLUMNS } from '../container-table/container-table';

/**
 * The one column header bar above all stacks in list view: sticky just under the app header,
 * hidden below `sm` (rows stack there). Decorative (`aria-hidden`): each table keeps its own
 * `<thead>` for screen readers.
 */
@Component({
  selector: 'tm-column-bar',
  templateUrl: './column-bar.html',
  host: {
    class: 'sticky top-16 z-30 -mx-4 mb-1 hidden bg-canvas/95 px-4 backdrop-blur sm:-mx-8 sm:block sm:px-8',
    'aria-hidden': 'true',
    'data-testid': 'column-bar',
  },
})
export class ContainerColumnBar {
  protected readonly columns = COLUMNS;
}
