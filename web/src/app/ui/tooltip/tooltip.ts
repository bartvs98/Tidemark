import { DestroyRef, Directive, effect, ElementRef, inject, input } from '@angular/core';
import { DOCUMENT } from '@angular/common';

const SHOW_DELAY_MS = 400;
const HIDE_DELAY_MS = 120;
const MARGIN = 8;
const GAP = 6;
let nextId = 0;
/** The open tooltip, so opening another closes it. */
let current: TmTooltip | null = null;

const INTERACTIVE =
  'a[href], button, input, select, textarea, summary, label, [role="button"], [role="link"], [role="menuitem"], [role="tab"], [role="option"], [role="checkbox"], [role="switch"]';

/**
 * A styled tooltip that replaces the native `title`.
 *
 *   <span [tmTooltip]="'Pinned to 1.4'">…</span>
 *
 * Empty text means no tooltip. The text is reflected as `data-tooltip` on the host.
 * Mouse: shows after a short hover (not on click); hides on leave or press. The pointer may move
 * onto the tooltip without closing it (WCAG 1.4.13). Keyboard: shows on focus, hides on blur.
 * Touch and pen: a tap toggles it; a tap elsewhere closes it. A tap on a non-interactive host has
 * its default prevented; on a link or button (or inside one) the click goes through as usual.
 * Escape and scroll always close it, and only one tooltip is open at a time.
 * The tooltip lives in the top layer (popover API) in `document.body`, so no sticky header or
 * stretched link covers it, and is linked with `aria-describedby` while it exists.
 */
@Directive({
  selector: '[tmTooltip]',
  host: { '[attr.data-tooltip]': 'tmTooltip() || null' },
})
export class TmTooltip {
  readonly tmTooltip = input<string | null | undefined>();

  private readonly doc = inject(DOCUMENT);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef).nativeElement;
  private readonly abort = new AbortController();
  private readonly id = `tm-tooltip-${nextId++}`;
  private tip: HTMLElement | null = null;
  private showTimer: ReturnType<typeof setTimeout> | undefined;
  private hideTimer: ReturnType<typeof setTimeout> | undefined;
  private open = false;
  private openListeners: AbortController | null = null;
  private baseDescribedBy: string | null = null;
  /** Pointer type of the latest press on the host, to tell a touch tap's click from a mouse click. */
  private pressType = '';

  constructor() {
    const { signal } = this.abort;
    const host = this.host;
    host.addEventListener('pointerenter', (e) => {
      if ((e as PointerEvent).pointerType === 'mouse') this.scheduleShow();
    }, { signal });
    host.addEventListener('pointerleave', (e) => {
      if (!isTouch(e as PointerEvent)) this.scheduleHide();
    }, { signal });
    host.addEventListener('pointerdown', (e) => {
      this.pressType = (e as PointerEvent).pointerType ?? '';
      if (!isTouch(e as PointerEvent)) this.hide();
    }, { signal });
    host.addEventListener('click', (e) => {
      const tap = this.pressType === 'touch' || this.pressType === 'pen';
      this.pressType = '';
      if (!tap || !this.tmTooltip()) return;
      if (!host.closest(INTERACTIVE)) e.preventDefault();
      if (this.open) this.hide();
      else this.show();
    }, { signal });
    host.addEventListener('focusin', (e) => {
      if (focusVisible(e.target as Element)) this.show();
    }, { signal });
    host.addEventListener('focusout', () => this.hide(), { signal });

    effect(() => {
      const text = this.tmTooltip();
      if (!text) {
        this.hide();
        this.destroyTip();
      } else if (this.tip) {
        this.tip.textContent = text;
        if (this.open) this.position();
      }
    });

    inject(DestroyRef).onDestroy(() => {
      this.abort.abort();
      clearTimeout(this.showTimer);
      clearTimeout(this.hideTimer);
      this.destroyTip();
    });
  }

  private scheduleShow(): void {
    clearTimeout(this.hideTimer);
    if (this.open) return;
    clearTimeout(this.showTimer);
    this.showTimer = setTimeout(() => this.show(), SHOW_DELAY_MS);
  }

  private scheduleHide(): void {
    clearTimeout(this.showTimer);
    clearTimeout(this.hideTimer);
    this.hideTimer = setTimeout(() => this.hide(), HIDE_DELAY_MS);
  }

  private ensureTip(): HTMLElement | null {
    const text = this.tmTooltip();
    if (!text) return null;
    if (!this.tip) {
      const tip = this.doc.createElement('div');
      tip.id = this.id;
      tip.setAttribute('role', 'tooltip');
      tip.setAttribute('popover', 'manual');
      tip.className = 'tm-tooltip';
      tip.addEventListener('pointerenter', () => clearTimeout(this.hideTimer), { signal: this.abort.signal });
      tip.addEventListener('pointerleave', () => this.scheduleHide(), { signal: this.abort.signal });
      this.doc.body.appendChild(tip);
      this.tip = tip;
      this.baseDescribedBy = this.host.getAttribute('aria-describedby');
      this.host.setAttribute('aria-describedby', [this.baseDescribedBy, this.id].filter(Boolean).join(' '));
    }
    this.tip.textContent = text;
    return this.tip;
  }

  private show(): void {
    clearTimeout(this.showTimer);
    clearTimeout(this.hideTimer);
    const tip = this.ensureTip();
    if (!tip || this.open) return;
    if (current && current !== this) current.hide();
    current = this;
    this.open = true;
    tip.showPopover?.();
    tip.dataset['open'] = '';
    this.position();
    this.openListeners = new AbortController();
    const signal = AbortSignal.any([this.abort.signal, this.openListeners.signal]);
    const onKey = (e: Event) => {
      if ((e as KeyboardEvent).key === 'Escape') this.hide();
    };
    this.doc.addEventListener('keydown', onKey, { signal, capture: true });
    this.doc.addEventListener('scroll', () => this.hide(), { signal, capture: true, passive: true });
    this.doc.addEventListener('pointerdown', (e) => {
      const target = e.target as Node | null;
      if (target && (this.host.contains(target) || tip.contains(target))) return;
      this.hide();
    }, { signal, capture: true });
  }

  private hide(): void {
    clearTimeout(this.showTimer);
    clearTimeout(this.hideTimer);
    if (!this.open) return;
    this.open = false;
    if (current === this) current = null;
    if (this.tip) {
      delete this.tip.dataset['open'];
      try {
        this.tip.hidePopover?.();
      } catch {
        // already hidden
      }
    }
    this.openListeners?.abort();
    this.openListeners = null;
  }

  private position(): void {
    const tip = this.tip;
    if (!tip) return;
    const view = this.doc.defaultView;
    const vw = this.doc.documentElement.clientWidth || view?.innerWidth || 0;
    const vh = this.doc.documentElement.clientHeight || view?.innerHeight || 0;
    const h = this.host.getBoundingClientRect();
    const t = tip.getBoundingClientRect();
    let top = h.top - t.height - GAP;
    if (top < MARGIN) top = h.bottom + GAP;
    top = Math.max(MARGIN, Math.min(top, vh - t.height - MARGIN));
    let left = h.left + h.width / 2 - t.width / 2;
    left = Math.max(MARGIN, Math.min(left, vw - t.width - MARGIN));
    tip.style.left = `${left}px`;
    tip.style.top = `${top}px`;
  }

  private destroyTip(): void {
    if (!this.tip) return;
    this.tip.remove();
    this.tip = null;
    this.open = false;
    if (current === this) current = null;
    if (this.baseDescribedBy) this.host.setAttribute('aria-describedby', this.baseDescribedBy);
    else this.host.removeAttribute('aria-describedby');
  }
}

function isTouch(e: PointerEvent): boolean {
  return e.pointerType === 'touch' || e.pointerType === 'pen';
}

function focusVisible(el: Element): boolean {
  try {
    return el.matches(':focus-visible');
  } catch {
    return true;
  }
}
