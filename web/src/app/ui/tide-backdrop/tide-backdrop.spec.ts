import { TestBed } from '@angular/core/testing';
import { TmTideBackdrop } from './tide-backdrop';

describe('TmTideBackdrop', () => {
  it('is decorative', async () => {
    const fixture = TestBed.createComponent(TmTideBackdrop);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.getAttribute('aria-hidden')).toBe('true');
    expect(el.querySelectorAll('path').length).toBe(3);
    expect(el.textContent?.trim()).toBe('');
  });
});
