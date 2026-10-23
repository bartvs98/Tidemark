import { Component, computed, ElementRef, inject, input, LOCALE_ID, signal, viewChild } from '@angular/core';
import { formatDate } from '@angular/common';
import { ChartTone, closeTipOn, focusVisible, labelWidth, observeWidth, stepIndex, TONE_COLOR, yScale } from '../chart-tip/chart';
import { TmChartTip } from '../chart-tip/chart-tip';

export interface TimePoint {
  /** ISO-8601 time. */
  at: string;
  value: number;
  /** Extra line in the tooltip and the table. */
  note?: string;
}

const DAY = 86_400_000;
let nextId = 0;
/** Space for the x labels under the plot, and above it for the top gridline label. */
const AXIS_H = 22;
const TOP = 8;
const RIGHT = 8;
/** Width the chart lays out at before it's measured (and in tests). */
const FALLBACK_WIDTH = 600;

/**
 * One series over time: a 2 px line over a soft area, with a crosshair and tooltip that snap
 * to the nearest point. Fills its container's width; `height` is the whole chart, x labels
 * included.
 *
 *   <tm-time-chart label="Up to date" [points]="tide" [max]="1" [format]="percent" />
 *
 * Keyboard: Tab to the chart, arrow keys (Home/End) move between points. Screen readers get
 * the `label` as the image name and an `sr-only` table of every point. Fewer than 2 points
 * shows `empty` instead.
 */
@Component({
  selector: 'tm-time-chart',
  imports: [TmChartTip],
  templateUrl: './time-chart.html',
  styleUrl: './time-chart.css',
  host: { class: 'block min-w-0' },
})
export class TmTimeChart {
  /** Accessible name of the chart and caption of its table. */
  readonly label = input.required<string>();
  /** Oldest first. */
  readonly points = input.required<TimePoint[]>();
  readonly min = input<number>(0);
  /** Top of the y axis; defaults to the data max rounded up. */
  readonly max = input<number>();
  /** Y labels and tooltip value. */
  readonly format = input<(v: number) => string>((v) => String(v));
  readonly tone = input<ChartTone>('accent');
  /** Whole chart height in px, x labels included. */
  readonly height = input(160);
  readonly empty = input('Fills in after a few check runs');

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef).nativeElement;
  private readonly locale = inject(LOCALE_ID);
  private readonly measured = observeWidth(this.host);
  /** Index of the point under the crosshair. */
  protected readonly active = signal<number | null>(null);
  protected readonly tipId = `tm-time-chart-tip-${nextId++}`;
  private readonly svg = viewChild<ElementRef<SVGSVGElement>>('svg');

  constructor() {
    closeTipOn(this.host, () => this.active.set(null));
  }

  protected readonly hasNotes = computed(() => this.points().some((p) => p.note));
  protected readonly color = computed(() => TONE_COLOR[this.tone()]);
  protected readonly width = computed(() => this.measured() || FALLBACK_WIDTH);

  /** Time stamps, and whether to show times (points closer than 2 days). */
  private readonly times = computed(() => this.points().map((p) => Date.parse(p.at)));
  private readonly withTime = computed(() => {
    const t = this.times();
    for (let i = 1; i < t.length; i++) if (t[i] - t[i - 1] < 2 * DAY) return true;
    return false;
  });
  private readonly spansYears = computed(() => {
    const t = this.times();
    return t.length > 1 && new Date(t[0]).getFullYear() !== new Date(t[t.length - 1]).getFullYear();
  });

  /** Tooltip / table date. */
  protected dateText(at: string): string {
    const fmt = (this.spansYears() ? 'd MMM yyyy' : 'd MMM') + (this.withTime() ? ', HH:mm' : '');
    return formatDate(at, fmt, this.locale);
  }

  protected readonly chart = computed(() => {
    const pts = this.points();
    if (pts.length < 2) return null;
    const values = pts.map((p) => p.value);
    const y = yScale(Math.max(...values), this.min(), this.max(), values.every(Number.isInteger));
    const format = this.format();
    const yLabels = y.ticks.map((v) => format(v));
    const left = Math.ceil(Math.max(...yLabels.map(labelWidth))) + 10;
    const w = this.width();
    const h = this.height();
    const plotW = Math.max(1, w - left - RIGHT);
    const plotH = Math.max(1, h - TOP - AXIS_H);
    const times = this.times();
    const t0 = times[0];
    const span = times[times.length - 1] - t0;
    const xs = times.map((t, i) => left + (span > 0 ? ((t - t0) / span) * plotW : (i / (pts.length - 1)) * plotW));
    const range = y.max - y.min || 1;
    const yOf = (v: number) => TOP + plotH - ((Math.min(Math.max(v, y.min), y.max) - y.min) / range) * plotH;
    const ys = values.map(yOf);
    const base = TOP + plotH;

    let line = '';
    xs.forEach((x, i) => (line += `${i ? 'L' : 'M'}${r(x)} ${r(ys[i])}`));
    const area = `${line}L${r(xs[xs.length - 1])} ${base}L${r(xs[0])} ${base}Z`;

    const grid = y.ticks.map((v, i) => ({ y: Math.round(yOf(v)) + 0.5, label: yLabels[i] }));

    // Evenly spaced date labels, as many as fit (2 … 5): first starts, last ends at the edge.
    const sample = formatDate(pts[0].at, this.axisFormat(span), this.locale);
    const fit = Math.floor(plotW / (labelWidth(sample) + 24));
    const n = Math.max(2, Math.min(5, fit));
    const xLabels = Array.from({ length: n }, (_, i) => {
      const t = t0 + (span * i) / (n - 1);
      return {
        x: left + (plotW * i) / (n - 1),
        text: formatDate(new Date(t), this.axisFormat(span), this.locale),
        anchor: i === 0 ? 'start' : i === n - 1 ? 'end' : 'middle',
      };
    });

    return { left, plotW, plotH, base, xs, ys, line, area, grid, xLabels, top: TOP };
  });

  private axisFormat(span: number): string {
    if (span > 300 * DAY || this.spansYears()) return 'MMM yyyy';
    if (span < 2 * DAY) return 'HH:mm';
    return 'd MMM';
  }

  protected readonly tip = computed(() => {
    const i = this.active();
    const c = this.chart();
    if (i === null || !c) return null;
    const p = this.points()[i];
    if (!p) return null;
    const box = this.svg()?.nativeElement.getBoundingClientRect();
    return {
      x: c.xs[i],
      y: c.ys[i],
      anchor: { x: (box?.left ?? 0) + c.xs[i], y: (box?.top ?? 0) + c.ys[i] - 4 },
      heading: this.dateText(p.at),
      rows: [{ value: this.format()(p.value) }],
      note: p.note,
    };
  });

  protected onPointer(e: PointerEvent, svg: Element): void {
    const c = this.chart();
    if (!c) return;
    const x = e.clientX - svg.getBoundingClientRect().left;
    let best = 0;
    for (let i = 1; i < c.xs.length; i++) if (Math.abs(c.xs[i] - x) < Math.abs(c.xs[best] - x)) best = i;
    this.active.set(best);
  }

  protected onLeave(e: PointerEvent): void {
    // A finger lifting also "leaves": keep the readout until a tap elsewhere.
    if (e.pointerType !== 'touch' && e.pointerType !== 'pen') this.active.set(null);
  }

  protected onKey(e: KeyboardEvent): void {
    const next = stepIndex(e.key, this.active(), this.points().length);
    if (next === null) return;
    e.preventDefault();
    this.active.set(next);
  }

  protected onFocus(svg: Element): void {
    if (this.active() === null && focusVisible(svg)) this.active.set(this.points().length - 1);
  }
}

function r(v: number): number {
  return Math.round(v * 10) / 10;
}
