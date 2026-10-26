import { afterRenderEffect, Component, computed, ElementRef, inject, input, signal, viewChild } from '@angular/core';
import { closeTipOn, focusVisible, observeWidth } from '../chart-tip/chart';
import { ChartTipRow, TmChartTip } from '../chart-tip/chart-tip';

export interface CalendarDay {
  /** YYYY-MM-DD (UTC calendar day). */
  day: string;
  /** 0..1; absent = no data that day (empty square). */
  value?: number;
  /** Extra tooltip / table line ("7 of 9 up to date"). */
  note?: string;
}

/** Squares grow with the width they get, between these sizes (px); wider gaps on big squares. */
const MIN_CELL = 11;
const MAX_CELL = 20;
const gapFor = (cell: number) => (cell >= 16 ? 3 : 2);
/** Room left of the grid for the Mon / Wed / Fri labels, and above it for the month labels. */
const LEFT = 30;
const TOP = 18;
/** A month label needs this many week columns to itself. */
const LABEL_COLS = 3;
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
const WEEKDAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
const ROW_LABELS = [
  { row: 0, text: 'Mon' },
  { row: 2, text: 'Wed' },
  { row: 4, text: 'Fri' },
];
let nextId = 0;

/** 'YYYY-MM-DD' as UTC parts, without ever going through local time. */
function parts(day: string): { m: number; d: number; weekday: number } {
  const [y, m, d] = day.split('-').map(Number);
  // getUTCDay: Sunday 0 -> Monday-first row 0..6.
  const weekday = (new Date(Date.UTC(y, m - 1, d)).getUTCDay() + 6) % 7;
  return { m: m - 1, d, weekday };
}

/** Colour step 1 (lowest) to 4 (a full 100 %) of a value. */
export function calendarStep(value: number): 1 | 2 | 3 | 4 {
  return value < 0.5 ? 1 : value < 0.75 ? 2 : value < 1 ? 3 : 4;
}

/**
 * A GitHub-style stretch of days as squares: a column per week, Monday-first rows, month
 * labels above and a small legend below. Value 0..1 maps to four steps of the accent
 * (under 50 %, under 75 %, under 100 %, 100 % strongest); a day without a value is an empty
 * square. Squares grow to fill the width (11 to 20 px). Still wider than its container at
 * 11 px (a phone): scrolls horizontally, starting at the latest week.
 *
 *   <tm-calendar-chart label="Up to date per day" [days]="days" />
 *
 * Hover or tap a square for the date, the value and `note`; Tab to the chart and use the
 * arrow keys (left/right a week, up/down a day, Home/End). Screen readers get `label` as the
 * image name and an `sr-only` table.
 */
@Component({
  selector: 'tm-calendar-chart',
  imports: [TmChartTip],
  templateUrl: './calendar-chart.html',
  styleUrl: './calendar-chart.css',
  host: { class: 'block min-w-0' },
})
export class TmCalendarChart {
  readonly label = input.required<string>();
  /** Contiguous days, oldest first. */
  readonly days = input.required<CalendarDay[]>();
  /** Tooltip, legend and table value. */
  readonly format = input<(v: number) => string>((v) => `${Math.round(v * 100)}%`);
  readonly empty = input('Nothing in this range');

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef).nativeElement;
  private readonly measured = observeWidth(this.host);
  private readonly svg = viewChild<ElementRef<SVGSVGElement>>('svg');
  private readonly scroller = viewChild<ElementRef<HTMLElement>>('scroller');
  protected readonly active = signal<number | null>(null);
  protected readonly tipId = `tm-calendar-chart-tip-${nextId++}`;
  protected readonly rowLabels = ROW_LABELS;
  protected readonly top = TOP;
  protected readonly steps = [1, 2, 3, 4] as const;
  private scrolled = false;

  constructor() {
    closeTipOn(this.host, () => this.active.set(null));
    // First render: show the latest week.
    afterRenderEffect(() => {
      const el = this.scroller()?.nativeElement;
      this.chart();
      if (!el || this.scrolled) return;
      this.scrolled = true;
      el.scrollLeft = el.scrollWidth;
    });
  }

  protected readonly hasData = computed(() => this.days().some((d) => d.value !== undefined && d.value !== null));

  protected readonly chart = computed(() => {
    const days = this.days();
    if (!days.length || !this.hasData()) return null;
    const offset = parts(days[0].day).weekday;
    const cols = Math.floor((days.length - 1 + offset) / 7) + 1;
    // The largest square whose column fits the width (unmeasured: the smallest).
    const fit = Math.floor((this.measured() - LEFT) / cols);
    let cell = MAX_CELL;
    while (cell > MIN_CELL && cell + gapFor(cell) > fit) cell--;
    const gap = gapFor(cell);
    const pitch = cell + gap;
    const cells = days.map((d, i) => {
      const col = Math.floor((i + offset) / 7);
      const row = (i + offset) % 7;
      const has = d.value !== undefined && d.value !== null;
      return { col, row, x: LEFT + col * pitch, y: TOP + row * pitch, step: has ? calendarStep(d.value!) : 0 };
    });

    // Month labels at the first week column showing a month's 1st; the first column names its
    // month unless the next label is too close.
    const labels: { x: number; text: string; col: number }[] = [{ x: LEFT, text: MONTHS[parts(days[0].day).m], col: 0 }];
    days.forEach((d, i) => {
      const p = parts(d.day);
      if (p.d !== 1 || i === 0) return;
      const col = cells[i].col;
      const prev = labels[labels.length - 1];
      if (col - prev.col >= LABEL_COLS) labels.push({ x: LEFT + col * pitch, text: MONTHS[p.m], col });
      else if (labels.length === 1) labels[0] = { x: LEFT + col * pitch, text: MONTHS[p.m], col };
    });
    return { cells, cols, cell, pitch, width: LEFT + cols * pitch - gap, height: TOP + 7 * pitch - gap, labels };
  });

  /** "Mon 12 Oct" */
  protected title(day: string): string {
    const p = parts(day);
    return `${WEEKDAYS[p.weekday]} ${p.d} ${MONTHS[p.m]}`;
  }

  protected valueText(d: CalendarDay): string {
    return d.value === undefined || d.value === null ? 'No check' : this.format()(d.value);
  }

  protected readonly hasNotes = computed(() => this.days().some((d) => d.note));

  protected readonly tip = computed(() => {
    const i = this.active();
    const c = this.chart();
    const d = i === null ? undefined : this.days()[i];
    if (i === null || !c || !d) return null;
    const hasValue = d.value !== undefined && d.value !== null;
    const rows: ChartTipRow[] = [{ value: this.valueText(d) }];
    const box = this.svg()?.nativeElement.getBoundingClientRect();
    const cell = c.cells[i];
    return {
      heading: this.title(d.day),
      rows,
      note: hasValue ? d.note : undefined,
      anchor: { x: (box?.left ?? 0) + cell.x + c.cell / 2, y: (box?.top ?? 0) + cell.y - 4 },
    };
  });

  protected onPointer(e: PointerEvent, svg: Element): void {
    const c = this.chart();
    if (!c) return;
    const box = svg.getBoundingClientRect();
    const col = Math.floor((e.clientX - box.left - LEFT) / c.pitch);
    const row = Math.floor((e.clientY - box.top - TOP) / c.pitch);
    const i = col < 0 || row < 0 || row > 6 ? -1 : c.cells.findIndex((x) => x.col === col && x.row === row);
    this.active.set(i >= 0 ? i : null);
  }

  protected onLeave(e: PointerEvent): void {
    if (e.pointerType !== 'touch' && e.pointerType !== 'pen') this.active.set(null);
  }

  protected onKey(e: KeyboardEvent): void {
    const n = this.days().length;
    const at = this.active();
    const jump: Record<string, number> = { ArrowLeft: -7, ArrowRight: 7, ArrowUp: -1, ArrowDown: 1 };
    let next: number;
    if (e.key === 'Home') next = 0;
    else if (e.key === 'End') next = n - 1;
    else if (e.key in jump) next = at === null ? n - 1 : Math.min(n - 1, Math.max(0, at + jump[e.key]));
    else return;
    e.preventDefault();
    this.active.set(next);
  }

  protected onFocus(svg: Element): void {
    if (this.active() === null && focusVisible(svg)) this.active.set(this.days().length - 1);
  }
}
