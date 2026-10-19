import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Level, TmLevel } from './level';

@Component({
  imports: [TmLevel],
  template: `<tm-level [level]="level()" />`,
})
class Host {
  level = signal<Level>('none');
}

describe('TmLevel', () => {
  it('renders no risk badge for level none, and icon + word otherwise', async () => {
    const fixture = TestBed.createComponent(Host);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    const host = fixture.componentInstance;
    expect(el.querySelector('tm-level')!.textContent!.trim()).toBe('');

    host.level.set('breaking');
    await fixture.whenStable();
    const level = el.querySelector('tm-level')!;
    expect(level.textContent).toContain('Breaking');
    expect(level.querySelector('svg')).not.toBeNull();
  });
});
