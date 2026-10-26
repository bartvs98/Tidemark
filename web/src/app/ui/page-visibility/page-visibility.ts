import { DestroyRef, DOCUMENT, inject, Injectable, signal } from '@angular/core';

/**
 * Whether the page is visible (not a background tab or a minimised window), from the
 * Page Visibility API. Polls pause while it's false and reload once when it turns true,
 * so a forgotten tab costs the server nothing.
 */
@Injectable({ providedIn: 'root' })
export class PageVisibility {
  private readonly document = inject(DOCUMENT);
  private readonly state = signal(this.document.visibilityState !== 'hidden');
  readonly visible = this.state.asReadonly();

  constructor() {
    const onChange = () => this.state.set(this.document.visibilityState !== 'hidden');
    this.document.addEventListener('visibilitychange', onChange);
    inject(DestroyRef).onDestroy(() => this.document.removeEventListener('visibilitychange', onChange));
  }
}
