import { afterNextRender, Component, DestroyRef, ElementRef, inject, input, signal, viewChild } from '@angular/core';
import { TmButton } from './button';

/**
 * Caps tall content at `max` with a fade and a "Show all" button. Content only slightly
 * taller than the cap (up to a third more) is shown whole: a button that reveals two
 * more lines is just a hurdle.
 *
 *   <tm-clamp max="40rem"><tm-markdown … /></tm-clamp>
 */
@Component({
  selector: 'tm-clamp',
  imports: [TmButton],
  template: `
    <div
      class="relative"
      [class.overflow-hidden]="clamped()"
      [style.max-height]="clamped() ? max() : null"
      [attr.data-clamped]="clamped() || null"
    >
      <div #content><ng-content /></div>
      @if (clamped()) {
        <div class="pointer-events-none absolute inset-x-0 bottom-0 h-16 bg-linear-to-t from-surface" aria-hidden="true"></div>
      }
    </div>
    @if (tall()) {
      <button tmButton variant="ghost" size="sm" [icon]="open() ? 'chevron-up' : 'chevron-down'" class="mt-2" [attr.aria-expanded]="open()" (click)="toggle()">
        {{ open() ? 'Show less' : 'Show all' }}
      </button>
    }
  `,
  host: { class: 'block' },
})
export class TmClamp {
  /** Cap as a CSS length, e.g. "40rem". */
  readonly max = input('40rem');

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly content = viewChild.required<ElementRef<HTMLElement>>('content');

  /** Content is clearly taller than the cap. */
  protected readonly tall = signal(false);
  protected readonly open = signal(false);
  protected clamped(): boolean {
    return this.tall() && !this.open();
  }

  constructor() {
    const destroyRef = inject(DestroyRef);
    // Measure the unclamped content box; it grows as lazy images load, so keep observing.
    afterNextRender(() => {
      const el = this.content().nativeElement;
      const measure = () => this.tall.set(el.offsetHeight > this.capPx() * 4 / 3);
      measure();
      if (typeof ResizeObserver === 'undefined') return;
      const ro = new ResizeObserver(measure);
      ro.observe(el);
      destroyRef.onDestroy(() => ro.disconnect());
    });
  }

  protected toggle(): void {
    const wasOpen = this.open();
    this.open.set(!wasOpen);
    // Collapsing a long text from its end would leave you far below it: go back to its top.
    if (wasOpen) {
      const top = this.host.nativeElement.getBoundingClientRect().top;
      if (top < 0) this.host.nativeElement.scrollIntoView({ block: 'start' });
    }
  }

  private capPx(): number {
    const probe = document.createElement('div');
    probe.style.cssText = `position:absolute;visibility:hidden;height:${this.max()}`;
    this.host.nativeElement.append(probe);
    const px = probe.offsetHeight;
    probe.remove();
    return px;
  }
}
