import { Component, input } from '@angular/core';
import { IconName, TmIcon } from '../icon/icon';

/** Shown when a list has nothing in it. Project buttons as content for the next step. */
@Component({
  selector: 'tm-empty-state',
  imports: [TmIcon],
  templateUrl: './empty-state.html',
  host: { class: 'flex flex-col items-center px-6 py-12 text-center' },
})
export class TmEmptyState {
  readonly heading = input.required<string>();
  readonly description = input<string>();
  readonly icon = input<IconName>('box');
}
