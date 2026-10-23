import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ChartTone } from '../chart-tip/chart';
import { TimePoint, TmTimeChart } from './time-chart';

describe('TmTimeChart', () => {
  const percent = (v: number) => `${Math.round(v * 100)}%`;

  @Component({
    imports: [TmTimeChart],
    template: `<tm-time-chart label="Up to date" [points]="points()" [max]="1" [format]="percent" [tone]="tone()" />`,
  })
  class Host {
    points = signal<TimePoint[]>([
      { at: '2026-10-01T06:00:00Z', value: 0.5 },
      { at: '2026-10-08T06:00:00Z', value: 0.75, note: '3 updates' },
      { at: '2026-10-15T06:00:00Z', value: 1 },
    ]);
    tone = signal<ChartTone>('accent');
    percent = percent;
  }

  async function render() {
    const fixture = TestBed.createComponent(Host);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    return { fixture, host: fixture.componentInstance, el, svg: () => el.querySelector('svg')! };
  }
  const tip = () => document.body.querySelector<HTMLElement>('tm-chart-tip');

  it('is a named, focusable image with a data table', async () => {
    const { svg, el } = await render();
    expect(svg().getAttribute('role')).toBe('img');
    expect(svg().getAttribute('aria-label')).toBe('Up to date');
    expect(svg().getAttribute('tabindex')).toBe('0');
    const table = el.querySelector('table.sr-only')!;
    expect(table.querySelector('caption')!.textContent).toBe('Up to date');
    const rows = [...table.querySelectorAll('tbody tr')].map((r) => r.textContent);
    expect(rows).toEqual(['1 Oct50%', '8 Oct75%3 updates', '15 Oct100%']);
  });

  it('draws one 2px line and an area, gridlines from the y domain', async () => {
    const { el } = await render();
    expect(el.querySelectorAll('.tm-chart-line').length).toBe(1);
    expect(el.querySelectorAll('.tm-chart-area').length).toBe(1);
    const yLabels = [...el.querySelectorAll('text[data-axis="y"]')].map((t) => t.textContent);
    expect(yLabels).toEqual(['0%', '50%', '100%']);
  });

  it('colours the marks by tone', async () => {
    const { fixture, host, el } = await render();
    expect((el.querySelector('.tm-chart-line') as SVGElement).style.stroke).toBe('var(--color-accent)');
    host.tone.set('breaking');
    await fixture.whenStable();
    expect((el.querySelector('.tm-chart-line') as SVGElement).style.stroke).toBe('var(--color-breaking)');
  });

  it('moves a crosshair and tooltip with the arrow keys', async () => {
    const { fixture, svg, el } = await render();
    const key = (k: string) => svg().dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true }));
    key('ArrowLeft'); // starts at the latest point
    await fixture.whenStable();
    expect(el.querySelector('.tm-chart-crosshair')).not.toBeNull();
    expect(tip()!.textContent).toContain('15 Oct');
    expect(tip()!.textContent).toContain('100%');
    expect(svg().getAttribute('aria-describedby')).toBe(tip()!.id);

    key('ArrowLeft');
    await fixture.whenStable();
    expect(tip()!.textContent).toContain('75%');
    expect(tip()!.textContent).toContain('3 updates');

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await fixture.whenStable();
    expect(tip()).toBeNull();
    expect(el.querySelector('.tm-chart-crosshair')).toBeNull();
  });

  it('shows times when points are less than 2 days apart', async () => {
    const { fixture, host, el } = await render();
    host.points.set([
      { at: '2026-10-01T06:00:00Z', value: 0.5 },
      { at: '2026-10-01T12:00:00Z', value: 0.6 },
    ]);
    await fixture.whenStable();
    const first = el.querySelector('tbody tr td')!.textContent!;
    expect(first).toMatch(/^1 Oct, \d\d:\d\d$/);
  });

  it('shows the empty text below 2 points', async () => {
    const { fixture, host, el } = await render();
    host.points.set([{ at: '2026-10-01T06:00:00Z', value: 1 }]);
    await fixture.whenStable();
    expect(el.querySelector('svg')).toBeNull();
    expect(el.textContent).toContain('Fills in after a few check runs');
  });
});
