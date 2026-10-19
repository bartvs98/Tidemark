import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { TideGaugeSize, tideWavePath, TmTideGauge } from './tide-gauge';

describe('TmTideGauge', () => {
  @Component({
    imports: [TmTideGauge],
    template: `<tm-tide-gauge [level]="level()" [size]="size()" [label]="label()" />`,
  })
  class GaugeHost {
    level = signal(0.5);
    size = signal<TideGaugeSize>('compact');
    label = signal('18 of 25 containers up to date');
  }

  async function gauge() {
    const fixture = TestBed.createComponent(GaugeHost);
    await fixture.whenStable();
    const el = (fixture.nativeElement as HTMLElement).querySelector('tm-tide-gauge') as HTMLElement;
    return { fixture, host: fixture.componentInstance, el, level: () => el.style.getPropertyValue('--tm-level') };
  }

  it('is an image named by its label, with no text inside', async () => {
    const { fixture, host, el } = await gauge();
    expect(el.getAttribute('role')).toBe('img');
    expect(el.getAttribute('aria-label')).toBe('18 of 25 containers up to date');
    expect(el.textContent?.trim()).toBe('');

    host.label.set('3 releases behind');
    await fixture.whenStable();
    expect(el.getAttribute('aria-label')).toBe('3 releases behind');
  });

  it('clamps the level to 0 … 1', async () => {
    const { fixture, host, level } = await gauge();
    expect(level()).toBe('0.5');
    for (const [input, used] of [[1.7, '1'], [-0.3, '0'], [NaN, '0'], [Infinity, '1'], [0.25, '0.25']] as const) {
      host.level.set(input);
      await fixture.whenStable();
      expect(level()).toBe(used);
    }
  });

  it('has a hero size that fills its parent and a compact one', async () => {
    const { fixture, host, el } = await gauge();
    expect(el.classList).toContain('tm-gauge-compact');
    expect(el.querySelectorAll('.tm-gauge-wave').length).toBe(2);

    host.size.set('hero');
    await fixture.whenStable();
    expect(el.classList).toContain('tm-gauge-hero');
    expect(el.classList).not.toContain('tm-gauge-compact');
    expect(el.querySelectorAll('.tm-gauge-wave').length).toBe(3);
    expect(el.querySelectorAll('.tm-gauge-crest').length).toBe(1); // only the front wave
  });

  it('draws seamless waves: whole periods, closed below the crest', () => {
    const { crest, fill } = tideWavePath(100, 5, 300);
    expect(crest.startsWith('M0 5 Q25 -5 50 5')).toBe(true);
    expect(crest.endsWith('T400 5')).toBe(true); // one period past the width, for the loop
    expect(fill).toBe(`${crest} V2000 H0Z`);
  });

  it('can leave room above the peaks for a stroked crest', () => {
    const { crest } = tideWavePath(100, 5, 300, 2000, 2);
    // Peaks at y=2 instead of y=0, so a 2 px stroke isn't clipped flat by the SVG's top edge.
    expect(crest.startsWith('M0 7 Q25 -3 50 7')).toBe(true);
  });
});
