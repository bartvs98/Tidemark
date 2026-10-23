import { Component, computed, ElementRef, inject, input, signal, viewChild } from '@angular/core';
import { ChartTone, closeTipOn, columnPath, focusVisible, labelWidth, observeWidth, stepIndex, TONE_COLOR, yScale } from '../chart-tip/chart';
import { ChartTipRow, TmChartTip } from '../chart-tip/chart-tip';

export interface BarSegment {
  value: number;
  /** Default `accent`. */
  tone?: ChartTone;
  /** Shown in the legend, tooltip and table ("Breaking", "Calm"). */
  name: string;
}

export interface ChartBar {
  /** Category on the x axis ("12 Oct", "Oct"), also the tooltip heading. */
  label: string;
  /** Stacked bottom → top in this order. */
  segments: BarSegment[];
  /** Extra line in the tooltip and the table ("3 rate-limited"). */
  note?: string;
}

const AXIS_H = 22;
const TOP = 8;
const RIGHT = 8;
const MAX_BAR = 24;
/** Surface gap between adjacent bars and between stacked segments. */
const GAP = 2;
const FALLBACK_WIDTH = 600;
let nextId = 0;

/**
 * Vertical bars over categories (weeks, months, days), optionally stacked: each segment
 * has a name and a tone (accent unless the page says it's attention / breaking / muted).
 * Bars are at most 24 px wide with a rounded top and square baseline; 2 px surface gaps
 * separate segments and neighbouring bars. A legend shows above when there's more than one
 * segment name. Fills its container's width; `height` is the whole SVG, x labels included.
 *
 *   <tm-bar-chart label="Releases per month" [bars]="months" />
 *
 * Hover or tap a bar's column (the full slot, not just the mark) for a tooltip with every
 * segment; Tab to the chart and use the arrow keys (Home/End) to move between bars. Screen
 * readers get `label` as the image name and an `sr-only` table. All zeros shows `empty`.
 */
@Component({
  selector: 'tm-bar-chart',
  imports: [TmChartTip],
  templateUrl: './bar-chart.html',
  styleUrl: './bar-chart.css',
  host: { class: 'block min-w-0' },
})
export class TmBarChart {
  readonly label = input.required<string>();
  readonly bars = input.required<ChartBar[]>();
  /** Y labels, tooltip and table values. */
  readonly format = input<(v: number) => string>((v) => String(v));
  /** Whole chart height in px, x labels included (the legend is extra). */
  readonly height = input(160);
  /** Label every n-th bar; default: as many as fit without colliding (the latest always). */
  readonly tickEvery = input<number>();
  readonly legend = input(true);
  readonly empty = input('Nothing in this range');

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef).nativeElement;
  private readonly measured = observeWidth(this.host);
  private readonly svg = viewChild<ElementRef<SVGSVGElement>>('svg');
  protected readonly active = signal<number | null>(null);
  protected readonly tipId = `tm-bar-chart-tip-${nextId++}`;
  protected readonly tone = TONE_COLOR;

  constructor() {
    closeTipOn(this.host, () => this.active.set(null));
  }

  protected readonly width = computed(() => this.measured() || FALLBACK_WIDTH);
  protected readonly totals = computed(() => this.bars().map((b) => b.segments.reduce((s, x) => s + Math.max(0, x.value), 0)));
  protected readonly isEmpty = computed(() => !this.totals().some((t) => t > 0));

  /** Distinct segment names in order of first appearance, with the tone they first had. */
  protected readonly series = computed(() => {
    const seen = new Map<string, ChartTone>();
    for (const b of this.bars()) for (const s of b.segments) if (!seen.has(s.name)) seen.set(s.name, s.tone ?? 'accent');
    return [...seen].map(([name, tone]) => ({ name, tone }));
  });
  protected readonly showLegend = computed(() => this.legend() && this.series().length > 1);
  protected readonly hasNotes = computed(() => this.bars().some((b) => b.note));

  protected readonly chart = computed(() => {
    const bars = this.bars();
    if (this.isEmpty()) return null;
    const totals = this.totals();
    const integer = bars.every((b) => b.segments.every((s) => Number.isInteger(s.value)));
    const y = yScale(Math.max(...totals), 0, undefined, integer);
    const format = this.format();
    const yLabels = y.ticks.map((v) => format(v));
    const left = Math.ceil(Math.max(...yLabels.map(labelWidth))) + 10;
    const w = this.width();
    const plotW = Math.max(1, w - left - RIGHT);
    const plotH = Math.max(1, this.height() - TOP - AXIS_H);
    const base = TOP + plotH;
    const band = plotW / bars.length;
    const barW = Math.max(1, Math.min(MAX_BAR, band - GAP));
    const scale = plotH / (y.max || 1);
    const yOf = (v: number) => base - v * scale;

    const columns = bars.map((b, i) => {
      const x = left + i * band + (band - barW) / 2;
      let top = -1;
      b.segments.forEach((s, j) => s.value > 0 && (top = j));
      let cum = 0;
      const segments = b.segments.flatMap((s, j) => {
        if (!(s.value > 0)) return [];
        const h = s.value * scale;
        const bottom = base - cum;
        cum += h;
        // Lower segments give up their top 2 px to the gap; the top one keeps its full height.
        const drawn = j === top ? h : Math.max(1, h - GAP);
        const d = columnPath(r(x), r(bottom - drawn), r(barW), r(drawn), j === top ? 4 : 0);
        return [{ d, color: TONE_COLOR[s.tone ?? 'accent'] }];
      });
      return { x: left + i * band, center: left + i * band + band / 2, top: yOf(totals[i]), segments };
    });

    // X labels: every n-th bar, counted back from the latest so it's always labelled.
    const widest = Math.max(...bars.map((b) => labelWidth(b.label)));
    const every = Math.max(1, Math.round(this.tickEvery() ?? Math.ceil((widest + 12) / band)));
    const last = bars.length - 1;
    const placed = bars.flatMap((b, i) => {
      if ((last - i) % every !== 0) return [];
      const w = labelWidth(b.label);
      const fits = band >= w + 4;
      const c = left + i * band + band / 2;
      if (fits || (i !== 0 && i !== last)) return [{ x: c, text: b.label, anchor: 'middle', x0: c - w / 2, x1: c + w / 2 }];
      return i === last
        ? [{ x: left + (i + 1) * band, text: b.label, anchor: 'end', x0: left + (i + 1) * band - w, x1: left + (i + 1) * band }]
        : [{ x: left, text: b.label, anchor: 'start', x0: left, x1: left + w }];
    });
    // An edge-anchored label sticks out further, so drop a neighbour it would run into (latest wins).
    const xLabels: { x: number; text: string; anchor: string }[] = [];
    let limit = Infinity;
    for (const l of placed.reverse()) {
      if (l.x1 + 6 > limit) continue;
      xLabels.unshift({ x: l.x, text: l.text, anchor: l.anchor });
      limit = l.x0;
    }

    const grid = y.ticks.map((v, i) => ({ y: Math.round(yOf(v)) + 0.5, label: yLabels[i] }));
    return { left, plotW, plotH, base, band, columns, grid, xLabels, top: TOP };
  });

  protected readonly tip = computed(() => {
    const i = this.active();
    const c = this.chart();
    const bar = i === null ? undefined : this.bars()[i];
    if (i === null || !c || !bar) return null;
    const format = this.format();
    const rows: ChartTipRow[] = [...bar.segments].reverse().map((s) => ({
      value: format(s.value),
      name: s.name,
      tone: s.tone ?? 'accent',
    }));
    if (bar.segments.length > 1) rows.push({ value: format(this.totals()[i]), name: 'Total' });
    const box = this.svg()?.nativeElement.getBoundingClientRect();
    const col = c.columns[i];
    return {
      heading: bar.label,
      rows,
      note: bar.note,
      anchor: { x: (box?.left ?? 0) + col.center, y: (box?.top ?? 0) + col.top - 4 },
    };
  });

  /** Table cell: the bar's value for a series name. */
  protected valueOf(bar: ChartBar, name: string): string {
    const total = bar.segments.filter((s) => s.name === name).reduce((sum, s) => sum + s.value, 0);
    return this.format()(total);
  }

  protected onPointer(e: PointerEvent, svg: Element): void {
    const c = this.chart();
    if (!c) return;
    const x = e.clientX - svg.getBoundingClientRect().left - c.left;
    const i = Math.floor(x / c.band);
    this.active.set(i >= 0 && i < c.columns.length ? i : null);
  }

  protected onLeave(e: PointerEvent): void {
    if (e.pointerType !== 'touch' && e.pointerType !== 'pen') this.active.set(null);
  }

  protected onKey(e: KeyboardEvent): void {
    const next = stepIndex(e.key, this.active(), this.bars().length);
    if (next === null) return;
    e.preventDefault();
    this.active.set(next);
  }

  protected onFocus(svg: Element): void {
    if (this.active() === null && focusVisible(svg)) this.active.set(this.bars().length - 1);
  }
}

function r(v: number): number {
  return Math.round(v * 10) / 10;
}
