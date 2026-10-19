import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { TmUpdateClass, UpdateClass } from './update-class';

@Component({
  imports: [TmUpdateClass],
  template: `<tm-update-class [kind]="kind()" />`,
})
class Host {
  kind = signal<UpdateClass>('none');
}

describe('TmUpdateClass', () => {
  it('shows "Up to date" for no update and a neutral badge otherwise', async () => {
    const fixture = TestBed.createComponent(Host);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    const host = fixture.componentInstance;
    expect(el.querySelector('tm-update-class')!.textContent).toContain('Up to date');

    host.kind.set('digest');
    await fixture.whenStable();
    expect(el.querySelector('tm-update-class')!.textContent).toContain('new image');
  });
});
