import { afterNextRender, Component, computed, DestroyRef, ElementRef, inject, input, signal, viewChild } from '@angular/core';
import { TmButton } from '../button/button';

/**
 * Caps tall content at `max` with a fade and a "Show all" button. Content only slightly
 * taller than the cap (up to a third more) is shown whole: a button that reveals two
 * more lines is just a hurdle. With `marks` (a CSS selector), the button also counts the
 * matching elements the cap hides: "Show all · 2 flagged lines".
 *
 *   <tm-clamp max="40rem" marks=".tm-flag-breaking, .tm-flag-attention"><tm-markdown … /></tm-clamp>
 */
@Component({
  selector: 'tm-clamp',
  imports: [TmButton],
  templateUrl: './clamp.html',
  host: { class: 'block' },
})
export class TmClamp {
  /** Cap as a CSS length, e.g. "40rem". */
  readonly max = input('40rem');
  /** Selector for elements worth mentioning when hidden; one starting under the fade counts. */
  readonly marks = input<string>();
  /** What one mark is called; the plural adds an "s". */
  readonly marksName = input('flagged line');

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly content = viewChild.required<ElementRef<HTMLElement>>('content');

  /** Content is clearly taller than the cap. */
  protected readonly tall = signal(false);
  protected readonly open = signal(false);
  /** Marks below the visible part while clamped. */
  protected readonly hiddenMarks = signal(0);
  protected readonly label = computed(() => {
    if (this.open()) return 'Show less';
    const n = this.hiddenMarks();
    return n ? `Show all · ${n} ${this.marksName()}${n === 1 ? '' : 's'}` : 'Show all';
  });
  protected clamped(): boolean {
    return this.tall() && !this.open();
  }

  constructor() {
    const destroyRef = inject(DestroyRef);
    // Measure the unclamped content box; it grows as lazy images load, so keep observing.
    afterNextRender(() => {
      const el = this.content().nativeElement;
      const measure = () => {
        const cap = this.px(this.max());
        this.tall.set(el.offsetHeight > cap * 4 / 3);
        this.hiddenMarks.set(this.countHidden(el, cap));
      };
      measure();
      if (typeof ResizeObserver === 'undefined') return;
      const ro = new ResizeObserver(measure);
      ro.observe(el);
      destroyRef.onDestroy(() => ro.disconnect());
    });
  }

  /** Opens the clamp if `target` is inside it (e.g. before scrolling to it); says whether it was. */
  reveal(target: Element): boolean {
    if (!this.host.nativeElement.contains(target)) return false;
    this.open.set(true);
    return true;
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

  private countHidden(el: HTMLElement, cap: number): number {
    const selector = this.marks();
    if (!selector) return 0;
    // The fade (h-16) makes whatever starts under it unreadable.
    const visible = cap - this.px('4rem');
    const top = el.getBoundingClientRect().top;
    return [...el.querySelectorAll(selector)].filter((m) => m.getBoundingClientRect().top - top > visible).length;
  }

  private px(length: string): number {
    const probe = document.createElement('div');
    probe.style.cssText = `position:absolute;visibility:hidden;height:${length}`;
    this.host.nativeElement.append(probe);
    const px = probe.offsetHeight;
    probe.remove();
    return px;
  }
}
