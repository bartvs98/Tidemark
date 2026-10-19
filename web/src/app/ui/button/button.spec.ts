import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { TmButton } from './button';

@Component({
  imports: [TmButton],
  template: `<button tmButton [loading]="loading()">Check now</button>`,
})
class Host {
  loading = signal(false);
}

describe('TmButton', () => {
  it('marks a loading button busy', async () => {
    const fixture = TestBed.createComponent(Host);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    const host = fixture.componentInstance;
    const button = el.querySelector('button')!;
    expect(button.getAttribute('aria-busy')).toBeNull();

    host.loading.set(true);
    await fixture.whenStable();
    expect(button.getAttribute('aria-busy')).toBe('true');
    expect(button.getAttribute('aria-disabled')).toBe('true');
  });
});
