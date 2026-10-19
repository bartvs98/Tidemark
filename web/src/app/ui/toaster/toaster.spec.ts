import { TestBed } from '@angular/core/testing';
import { TmToaster } from './toaster';
import { Toasts } from './toasts';

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
