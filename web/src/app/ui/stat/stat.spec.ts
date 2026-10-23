import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { TmStat } from './stat';

describe('TmStat', () => {
  @Component({
    imports: [TmStat],
    template: `<tm-stat label="Updates applied" [value]="value()" [unit]="unit()" [hint]="hint()" [mono]="mono()" />`,
  })
  class Host {
    value = signal<string | number>(1284);
    unit = signal<string | undefined>(undefined);
    hint = signal<string | undefined>('since 12 Oct');
    mono = signal(false);
  }

  async function render() {
    const fixture = TestBed.createComponent(Host);
    await fixture.whenStable();
    return { fixture, host: fixture.componentInstance, el: fixture.nativeElement as HTMLElement };
  }

  it('shows label, grouped number and hint', async () => {
    const { el } = await render();
    expect(el.textContent).toContain('Updates applied');
    expect(el.textContent).toContain('1,284');
    expect(el.textContent).toContain('since 12 Oct');
  });

  it('shows strings as given, with a unit, in mono when asked', async () => {
    const { fixture, host, el } = await render();
    host.value.set('3');
    host.unit.set('days');
    host.hint.set(undefined);
    host.mono.set(true);
    await fixture.whenStable();
    const value = el.querySelector('.font-mono')!;
    expect(value.textContent).toBe('3');
    expect(value.nextElementSibling!.textContent).toBe('days');
    expect(el.textContent).not.toContain('since');
  });
});
