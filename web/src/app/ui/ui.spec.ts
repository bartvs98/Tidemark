import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Level, TmButton, TmLevel, TmToaster, TmUpdateClass, Toasts, UpdateClass } from './index';

describe('Toasts', () => {
  afterEach(() => vi.useRealTimers());

  it('auto-dismisses after the duration', () => {
    vi.useFakeTimers();
    const toasts = TestBed.inject(Toasts);
    toasts.show('Check finished', 'ok', 1000);
    expect(toasts.items().length).toBe(1);
    vi.advanceTimersByTime(1000);
    expect(toasts.items().length).toBe(0);
  });

  it('dismisses from the toaster button', async () => {
    const fixture = TestBed.createComponent(TmToaster);
    const toasts = TestBed.inject(Toasts);
    toasts.show('First', 'info', 0);
    toasts.show('Rolled back', 'breaking', 0);
    await fixture.whenStable();

    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelectorAll('[role="status"]').length).toBe(1);
    expect(el.querySelector('[role="alert"]')?.textContent).toContain('Rolled back');

    el.querySelector<HTMLButtonElement>('button[aria-label="Dismiss"]')!.click();
    await fixture.whenStable();
    expect(toasts.items().map((t) => t.message)).toEqual(['Rolled back']);
  });
});

@Component({
  imports: [TmLevel, TmUpdateClass, TmButton],
  template: `
    <tm-level [level]="level()" />
    <tm-update-class [kind]="kind()" />
    <button tmButton [loading]="loading()">Check now</button>
  `,
})
class Host {
  level = signal<Level>('none');
  kind = signal<UpdateClass>('none');
  loading = signal(false);
}

describe('status components', () => {
  async function render() {
    const fixture = TestBed.createComponent(Host);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    return { fixture, el, host: fixture.componentInstance };
  }

  it('renders no risk badge for level none, and icon + word otherwise', async () => {
    const { fixture, el, host } = await render();
    expect(el.querySelector('tm-level')!.textContent!.trim()).toBe('');

    host.level.set('breaking');
    await fixture.whenStable();
    const level = el.querySelector('tm-level')!;
    expect(level.textContent).toContain('Breaking');
    expect(level.querySelector('svg')).not.toBeNull();
  });

  it('shows "Up to date" for no update and a neutral badge otherwise', async () => {
    const { fixture, el, host } = await render();
    expect(el.querySelector('tm-update-class')!.textContent).toContain('Up to date');

    host.kind.set('digest');
    await fixture.whenStable();
    expect(el.querySelector('tm-update-class')!.textContent).toContain('new image');
  });

  it('marks a loading button busy', async () => {
    const { fixture, el, host } = await render();
    const button = el.querySelector('button')!;
    expect(button.getAttribute('aria-busy')).toBeNull();

    host.loading.set(true);
    await fixture.whenStable();
    expect(button.getAttribute('aria-busy')).toBe('true');
    expect(button.getAttribute('aria-disabled')).toBe('true');
  });
});
