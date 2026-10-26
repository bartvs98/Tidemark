import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { CalendarDay, calendarStep, TmCalendarChart } from './calendar-chart';

/** `n` contiguous UTC days from `start` (YYYY-MM-DD). */
function run(start: string, n: number, value: (i: number) => number | undefined = () => 1): CalendarDay[] {
  const t0 = Date.parse(`${start}T00:00:00Z`);
  return Array.from({ length: n }, (_, i) => ({
    day: new Date(t0 + i * 86_400_000).toISOString().slice(0, 10),
    value: value(i),
    note: `note ${i}`,
  }));
}

describe('TmCalendarChart', () => {
  @Component({
    imports: [TmCalendarChart],
    template: `<tm-calendar-chart label="Up to date per day" [days]="days()" />`,
  })
  class Host {
    // Wed 21 Oct 2026 .. Tue 3 Nov 2026 (14 days): starts in row 2.
    days = signal<CalendarDay[]>(run('2026-10-21', 14, (i) => [0.2, 0.6, 0.9, 1, undefined][i % 5]));
  }

  async function render() {
    const fixture = TestBed.createComponent(Host);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    return { fixture, host: fixture.componentInstance, el, svg: () => el.querySelector('svg')! };
  }
  const cells = (el: HTMLElement) => [...el.querySelectorAll('svg[role="img"] rect[class*="tm-cal-cell"]')] as SVGRectElement[];
  const tip = () => document.body.querySelector<HTMLElement>('tm-chart-tip');
  const key = (el: Element, k: string) => el.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true }));

  it('maps values to four steps', () => {
    expect([0, 0.49, 0.5, 0.74, 0.75, 0.99, 1].map(calendarStep)).toEqual([1, 1, 2, 2, 3, 3, 4]);
  });

  it('lays out weeks as columns and Monday-first rows, offset by the first weekday', async () => {
    const { el } = await render();
    const rects = cells(el);
    expect(rects.length).toBe(14);
    const pos = rects.map((r) => [Number(r.getAttribute('x')), Number(r.getAttribute('y'))]);
    // Wed first: row 2 of column 0; Sun 25 Oct (index 4) is row 6; Mon 26 Oct (5) starts column 1, row 0.
    expect(pos[0]).toEqual([30, 18 + 2 * 13]);
    expect(pos[4]).toEqual([30, 18 + 6 * 13]);
    expect(pos[5]).toEqual([30 + 13, 18]);
    // 14 days from row 2: 3 week columns.
    expect(pos[13][0]).toBe(30 + 2 * 13);
    expect(el.querySelector('svg')!.getAttribute('width')).toBe(String(30 + 3 * 13 - 2));
  });

  it('colours by step and leaves empty days in the hover tone', async () => {
    const { el } = await render();
    const steps = cells(el).slice(0, 5).map((r) => [...r.classList].filter((c) => c !== 'tm-cal-cell').join());
    expect(steps).toEqual(['s1', 's2', 's3', 's4', 's0']);
  });

  it('labels months at the week their 1st falls in, and the weekday rows', async () => {
    const { fixture, host, el } = await render();
    const months = () => [...el.querySelectorAll('text[data-axis="month"]')].map((t) => t.textContent);
    // 1 Nov is a Sunday of column 1, which is within 3 columns of the first one: Nov replaces Oct.
    expect(months()).toEqual(['Nov']);
    host.days.set(run('2026-10-05', 70));
    await fixture.whenStable();
    expect(months()).toEqual(['Oct', 'Nov', 'Dec']);
    expect([...el.querySelectorAll('text[data-axis="row"]')].map((t) => t.textContent)).toEqual(['Mon', 'Wed', 'Fri']);
  });

  it('shows date, value and note on hover, "No check" for an empty day', async () => {
    const { fixture, svg } = await render();
    const box = svg().getBoundingClientRect();
    const move = (x: number, y: number) =>
      svg().dispatchEvent(new PointerEvent('pointermove', { clientX: box.left + x, clientY: box.top + y, bubbles: true }));
    move(30 + 5, 18 + 2 * 13 + 5);
    await fixture.whenStable();
    expect(tip()!.textContent).toContain('Wed 21 Oct');
    expect(tip()!.textContent).toContain('20%');
    expect(tip()!.textContent).toContain('note 0');
    // index 4 = Sun 25 Oct: no value.
    move(30 + 5, 18 + 6 * 13 + 5);
    await fixture.whenStable();
    expect(tip()!.textContent).toContain('Sun 25 Oct');
    expect(tip()!.textContent).toContain('No check');
    expect(tip()!.textContent).not.toContain('note 4');
    // Outside the grid.
    move(2, 2);
    await fixture.whenStable();
    expect(tip()).toBeNull();
  });

  it('moves with the arrow keys: a week left/right, a day up/down, Home/End, starting at the latest', async () => {
    const { fixture, svg } = await render();
    const heading = () => tip()!.firstElementChild!.textContent;
    key(svg(), 'ArrowLeft');
    await fixture.whenStable();
    expect(heading()).toBe('Tue 3 Nov');
    key(svg(), 'ArrowLeft');
    await fixture.whenStable();
    expect(heading()).toBe('Tue 27 Oct');
    key(svg(), 'ArrowDown');
    await fixture.whenStable();
    expect(heading()).toBe('Wed 28 Oct');
    key(svg(), 'ArrowUp');
    await fixture.whenStable();
    expect(heading()).toBe('Tue 27 Oct');
    key(svg(), 'Home');
    await fixture.whenStable();
    expect(heading()).toBe('Wed 21 Oct');
    key(svg(), 'ArrowLeft');
    await fixture.whenStable();
    expect(heading()).toBe('Wed 21 Oct');
    key(svg(), 'End');
    await fixture.whenStable();
    expect(heading()).toBe('Tue 3 Nov');
    svg().dispatchEvent(new FocusEvent('blur'));
    await fixture.whenStable();
    expect(tip()).toBeNull();
  });

  it('has a data table and a legend from the formatter', async () => {
    const { el, svg } = await render();
    expect(svg().getAttribute('aria-label')).toBe('Up to date per day');
    expect([...el.querySelectorAll('table thead th')].map((t) => t.textContent!.trim())).toEqual(['Date', 'Value', 'Note']);
    const rows = [...el.querySelectorAll('table tbody tr')];
    expect(rows.length).toBe(14);
    expect([...rows[0].children].map((c) => c.textContent)).toEqual(['Wed 21 Oct', '20%', 'note 0']);
    expect(rows[4].children[1].textContent).toBe('No check');
    expect(el.querySelector('ul')!.textContent).toContain('0%');
    expect(el.querySelector('ul')!.textContent).toContain('100%');
  });

  it('reads dates as UTC calendar days', async () => {
    const { fixture, host, svg } = await render();
    host.days.set(run('2026-01-01', 3)); // a Thursday
    await fixture.whenStable();
    key(svg(), 'Home');
    await fixture.whenStable();
    expect(tip()!.firstElementChild!.textContent).toBe('Thu 1 Jan');
  });

  it('shows the empty text with no days or no values', async () => {
    const { fixture, host, el } = await render();
    host.days.set([]);
    await fixture.whenStable();
    expect(el.querySelector('svg')).toBeNull();
    expect(el.textContent).toContain('Nothing in this range');
    host.days.set(run('2026-10-21', 5, () => undefined));
    await fixture.whenStable();
    expect(el.querySelector('svg')).toBeNull();
  });
});
