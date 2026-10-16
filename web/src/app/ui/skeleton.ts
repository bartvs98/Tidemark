import { Component, input } from '@angular/core';

/** Loading placeholder. Size it with width/height inputs or utility classes. */
@Component({
  selector: 'tm-skeleton',
  template: '',
  host: {
    class: 'tm-shimmer block rounded-md',
    'aria-hidden': 'true',
    '[style.width]': 'width()',
    '[style.height]': 'height()',
  },
})
export class TmSkeleton {
  readonly width = input('100%');
  readonly height = input('1rem');
}
