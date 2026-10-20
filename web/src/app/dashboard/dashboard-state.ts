import { Injectable, signal } from '@angular/core';
import { ContainerList } from './containers';

/**
 * What the dashboard keeps while you're on a detail page (the component itself is destroyed):
 * the last list and which collapsed sections were open. Coming back renders the page at its
 * full height straight away (while it reloads), so the router can restore the scroll position.
 */
@Injectable({ providedIn: 'root' })
export class DashboardState {
  readonly list = signal<ContainerList | undefined>(undefined);
  readonly currentOpen = signal(false);
  readonly snoozedOpen = signal(false);
  readonly ignoredOpen = signal(false);
}
