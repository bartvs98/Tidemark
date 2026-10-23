import { afterRenderEffect, Component, ElementRef, inject, input } from '@angular/core';
import { ChartTone, TONE_COLOR } from './chart';

export interface ChartTipRow {
  /** Formatted value: the strong part. */
  value: string;
  /** Series or segment name, after the value. */
  name?: string;
  /** Draws a short line key in the tone's colour before the row. */
  tone?: ChartTone;
}

const MARGIN = 8;
const GAP = 8;
let nextId = 0;

/**
 * The hover/focus readout of the chart pieces: `.tm-tooltip` styling in the top layer
 * (`popover="manual"`), positioned above `anchor` (viewport px: the mark's centre and top),
 * below it when there's no room, kept inside the viewport. Values lead, names follow; rows
 * are keyed with a short line in the series colour. Internal to `ui/`: the charts render it
 * inside `@if` while a point or bar is active, so it opens (and fades in) once and moves along.
 */
@Component({
  selector: 'tm-chart-tip',
  templateUrl: './chart-tip.html',
  host: {
    class: 'tm-tooltip pointer-events-none',
    popover: 'manual',
    role: 'tooltip',
    '[id]': 'id()',
  },
})
export class TmChartTip {
  readonly heading = input.required<string>();
  readonly rows = input<ChartTipRow[]>([]);
  readonly note = input<string>();
  readonly anchor = input.required<{ x: number; y: number }>();

  /** Element id, for the chart's `aria-describedby`. */
  readonly id = input(`tm-chart-tip-${nextId++}`);
  protected readonly tone = TONE_COLOR;
  private readonly el = inject<ElementRef<HTMLElement>>(ElementRef).nativeElement;

  constructor() {
    afterRenderEffect(() => {
      const { x, y } = this.anchor();
      this.heading();
      this.rows();
      this.note();
      const tip = this.el;
      try {
        if (!tip.matches(':popover-open')) tip.showPopover?.();
      } catch {
        // no popover support (tests): it just stays in place
      }
      const doc = tip.ownerDocument;
      const vw = doc.documentElement.clientWidth || doc.defaultView?.innerWidth || 0;
      const vh = doc.documentElement.clientHeight || doc.defaultView?.innerHeight || 0;
      const r = tip.getBoundingClientRect();
      let top = y - r.height - GAP;
      if (top < MARGIN) top = y + GAP + 16;
      top = Math.max(MARGIN, Math.min(top, vh - r.height - MARGIN));
      const left = Math.max(MARGIN, Math.min(x - r.width / 2, vw - r.width - MARGIN));
      tip.style.left = `${left}px`;
      tip.style.top = `${top}px`;
    });
  }
}
