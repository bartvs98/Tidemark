import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ChartBar, TmBarChart } from './bar-chart';

describe('TmBarChart', () => {
  const month = (label: string, calm: number, attention: number, breaking: number, note?: string): ChartBar => ({
    label,
    note,
    segments: [
      { name: 'Calm', value: calm },
      { name: 'Attention', value: attention, tone: 'attention' },
      { name: 'Breaking', value: breaking, tone: 'breaking' },
    ],
  });

  @Component({
    imports: [TmBarChart],
    template: `<tm-bar-chart label="Releases per month" [bars]="bars()" [legend]="legend()" />`,
  })
  class Host {
    bars = signal<ChartBar[]>([month('Aug', 4, 1, 0), month('Sep', 6, 0, 2, '1 pre-release'), month('Oct', 3, 2, 1)]);
    legend = signal(true);
  }

  async function render() {
    const fixture = TestBed.createComponent(Host);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    return { fixture, host: fixture.componentInstance, el, svg: () => el.querySelector('svg')! };
  }
  const tip = () => document.body.querySelector<HTMLElement>('tm-chart-tip');

  it('stacks non-zero segments, rounding only the top one', async () => {
    const { el } = await render();
    const paths = [...el.querySelectorAll('svg g path')] as SVGPathElement[];
    expect(paths.length).toBe(2 + 2 + 3);
    expect(paths.map((p) => p.style.fill)).toEqual([
      'var(--color-accent)', 'var(--color-attention)',
      'var(--color-accent)', 'var(--color-breaking)',
      'var(--color-accent)', 'var(--color-attention)', 'var(--color-breaking)',
    ]);
    // Arcs only on each bar's top segment.
    expect(paths.map((p) => p.getAttribute('d')!.includes('A'))).toEqual([false, true, false, true, false, false, true]);
  });

  it('shows a legend of the segment names when there is more than one', async () => {
    const { fixture, host, el } = await render();
    expect([...el.querySelectorAll('ul li')].map((l) => l.textContent!.trim())).toEqual(['Calm', 'Attention', 'Breaking']);
    host.legend.set(false);
    await fixture.whenStable();
    expect(el.querySelector('ul')).toBeNull();
    host.legend.set(true);
    host.bars.set([{ label: 'W1', segments: [{ name: 'Updates', value: 3 }] }]);
    await fixture.whenStable();
    expect(el.querySelector('ul')).toBeNull();
  });

  it('has a data table with every series, the total and notes', async () => {
    const { el, svg } = await render();
    expect(svg().getAttribute('aria-label')).toBe('Releases per month');
    const head = [...el.querySelectorAll('table thead th')].map((t) => t.textContent!.trim());
    expect(head).toEqual(['Category', 'Calm', 'Attention', 'Breaking', 'Total', 'Note']);
    const sep = [...el.querySelectorAll('table tbody tr')[1].children].map((c) => c.textContent);
    expect(sep).toEqual(['Sep', '6', '0', '2', '8', '1 pre-release']);
  });

  it('opens a tooltip per bar with the arrow keys, top segment first', async () => {
    const { fixture, svg, el } = await render();
    svg().dispatchEvent(new KeyboardEvent('keydown', { key: 'Home', bubbles: true }));
    await fixture.whenStable();
    expect(el.querySelector('.tm-chart-band')).not.toBeNull();
    const rows = [...tip()!.querySelectorAll('div.flex')].map((r) => r.textContent);
    expect(tip()!.textContent).toContain('Aug');
    expect(rows).toEqual(['0Breaking', '1Attention', '4Calm', '5Total']);

    svg().dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }));
    await fixture.whenStable();
    expect(tip()!.textContent).toContain('1 pre-release');

    svg().dispatchEvent(new FocusEvent('blur'));
    await fixture.whenStable();
    expect(tip()).toBeNull();
  });

  it('labels the latest bar always, and every n-th before it', async () => {
    const { fixture, host, el } = await render();
    host.bars.set(Array.from({ length: 52 }, (_, i) => ({ label: `W${i + 1}`, segments: [{ name: 'Applied', value: i % 3 }] })));
    await fixture.whenStable();
    const labels = [...el.querySelectorAll('text[data-axis="x"]')].map((t) => t.textContent);
    expect(labels.at(-1)).toBe('W52');
    expect(labels.length).toBeGreaterThan(2);
    expect(labels.length).toBeLessThan(52);
  });

  it('shows the empty text when every value is 0', async () => {
    const { fixture, host, el } = await render();
    host.bars.set([month('Aug', 0, 0, 0)]);
    await fixture.whenStable();
    expect(el.querySelector('svg')).toBeNull();
    expect(el.textContent).toContain('Nothing in this range');
  });
});
