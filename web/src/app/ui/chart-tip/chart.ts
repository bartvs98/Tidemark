import { DestroyRef, inject, signal, Signal } from '@angular/core';
import { DOCUMENT } from '@angular/common';

/**
 * Mark colour of a chart series or bar. Colour means risk: `accent` is the default,
 * `attention` / `breaking` only when the data is about that level, `muted` for "other".
 * The name next to a mark (legend, tooltip, table) always says what it is.
 */
export type ChartTone = 'accent' | 'attention' | 'breaking' | 'muted';

/** The token behind each tone, for `[style.fill]` / `[style.stroke]` (SVG attributes don't take var()). */
export const TONE_COLOR: Record<ChartTone, string> = {
  accent: 'var(--color-accent)',
  attention: 'var(--color-attention)',
  breaking: 'var(--color-breaking)',
  muted: 'var(--color-fg-subtle)',
};

/** A round step at or above `x`: 1, 2 or 5 × 10ⁿ. */
export function niceStep(x: number): number {
  if (!(x > 0) || !Number.isFinite(x)) return 1;
  const pow = 10 ** Math.floor(Math.log10(x));
  const f = x / pow;
  return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10) * pow;
}

/**
 * The y domain and gridline values: `intervals` (3) round steps from `min`. `max` given:
 * used as is, ticks at round steps inside it (always including `max`). Otherwise the data
 * max rounded up to a step. Gives 3–4 gridlines, as the dataviz spec asks. `integer`: steps
 * of at least 1 (counts never get a 0.5 gridline).
 */
export function yScale(
  dataMax: number,
  min = 0,
  max?: number,
  integer = false,
  intervals = 3,
): { min: number; max: number; ticks: number[] } {
  const stepFor = (span: number) => (integer ? Math.max(1, niceStep(span / intervals)) : niceStep(span / intervals));
  if (max !== undefined && max > min) {
    const step = stepFor(max - min);
    const ticks: number[] = [];
    for (let v = min; v < max - step / 2; v += step) ticks.push(round(v));
    ticks.push(max);
    return { min, max, ticks };
  }
  const span = Math.max(dataMax - min, 0);
  const step = span > 0 ? stepFor(span) : 1;
  const top = min + Math.max(1, Math.ceil(span / step)) * step;
  const ticks: number[] = [];
  for (let v = min; v <= top + step / 2; v += step) ticks.push(round(v));
  return { min, max: round(top), ticks };
}

/** Floating-point noise off tick values (0.1 + 0.2). */
function round(v: number): number {
  return Math.round(v * 1e9) / 1e9;
}

/** Rough width of a text-xs label, for spacing axis labels before they're measured. */
export function labelWidth(text: string): number {
  return text.length * 6.5;
}

/**
 * A column with a rounded data end (radius `r`) and a square baseline, from (x, y) at the top
 * to the baseline at y + h. `r` 0 gives a plain rectangle (stacked segments below the top one).
 */
export function columnPath(x: number, y: number, w: number, h: number, r = 4): string {
  if (w <= 0 || h <= 0) return '';
  const rr = Math.min(r, w / 2, h);
  const b = y + h;
  if (rr <= 0) return `M${x} ${b}V${y}H${x + w}V${b}Z`;
  return `M${x} ${b}V${y + rr}A${rr} ${rr} 0 0 1 ${x + rr} ${y}H${x + w - rr}A${rr} ${rr} 0 0 1 ${x + w} ${y + rr}V${b}Z`;
}

/** Width of `el` as a signal, kept up to date by a ResizeObserver (0 until laid out, or in tests). */
export function observeWidth(el: HTMLElement): Signal<number> {
  const width = signal(el.clientWidth);
  if (typeof ResizeObserver !== 'undefined') {
    const ro = new ResizeObserver((entries) => width.set(Math.floor(entries[0].contentRect.width)));
    ro.observe(el);
    inject(DestroyRef).onDestroy(() => ro.disconnect());
  }
  return width.asReadonly();
}

/**
 * Closes a chart's tooltip the way `[tmTooltip]` closes: Escape, scrolling, or a press
 * outside the chart. Call from a constructor.
 */
export function closeTipOn(host: HTMLElement, close: () => void): void {
  const doc = inject(DOCUMENT);
  const abort = new AbortController();
  const { signal } = abort;
  doc.addEventListener('keydown', (e) => e.key === 'Escape' && close(), { signal, capture: true });
  doc.addEventListener('scroll', close, { signal, capture: true, passive: true });
  doc.addEventListener('pointerdown', (e) => host.contains(e.target as Node) || close(), { signal, capture: true });
  inject(DestroyRef).onDestroy(() => abort.abort());
}

/** Clamp an index into 0 … n-1 after an arrow-key step; `null` for an unknown key. */
export function stepIndex(key: string, current: number | null, n: number): number | null {
  const at = current ?? n - 1;
  switch (key) {
    case 'ArrowLeft':
    case 'ArrowDown':
      return current === null ? at : Math.max(0, at - 1);
    case 'ArrowRight':
    case 'ArrowUp':
      return current === null ? at : Math.min(n - 1, at + 1);
    case 'Home':
      return 0;
    case 'End':
      return n - 1;
    default:
      return null;
  }
}

/** Keyboard focus (not a click): only then does focusing a chart open its readout. */
export function focusVisible(el: Element): boolean {
  try {
    return el.matches(':focus-visible');
  } catch {
    return true;
  }
}
