import { ApplicationRef } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Theme, THEME_COLORS } from './theme';

describe('Theme', () => {
  const meta = () => document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
  let osDark = true;
  let listeners: ((e: MediaQueryListEvent) => void)[] = [];
  const original = window.matchMedia;

  beforeEach(() => {
    localStorage.clear();
    meta()?.remove();
    osDark = true;
    listeners = [];
    // jsdom has no matchMedia: fake the OS setting, and let the test flip it.
    window.matchMedia = ((query: string) => ({
      matches: query.includes('dark') && osDark,
      media: query,
      addEventListener: (_: string, fn: (e: MediaQueryListEvent) => void) => listeners.push(fn),
      removeEventListener: () => {},
    })) as unknown as typeof window.matchMedia;
  });
  afterEach(() => {
    window.matchMedia = original;
    meta()?.remove();
  });

  const flipOs = (dark: boolean) => {
    osDark = dark;
    listeners.forEach((fn) => fn({ matches: dark } as MediaQueryListEvent));
  };

  it('points theme-color at the resolved page background', async () => {
    const theme = TestBed.inject(Theme);
    await TestBed.inject(ApplicationRef).whenStable();
    expect(document.documentElement.dataset['theme']).toBe('dark');
    expect(meta()?.content).toBe(THEME_COLORS.dark);

    theme.preference.set('light');
    await TestBed.inject(ApplicationRef).whenStable();
    expect(meta()?.content).toBe(THEME_COLORS.light);
    expect(document.querySelectorAll('meta[name="theme-color"]').length).toBe(1);
    expect(localStorage.getItem('tidemark.theme')).toBe('light');
  });

  it('follows the OS when set to system, also when the OS flips', async () => {
    localStorage.setItem('tidemark.theme', 'system');
    const theme = TestBed.inject(Theme);
    await TestBed.inject(ApplicationRef).whenStable();
    expect(theme.isDark()).toBe(true);
    expect(meta()?.content).toBe(THEME_COLORS.dark);

    flipOs(false);
    await TestBed.inject(ApplicationRef).whenStable();
    expect(theme.isDark()).toBe(false);
    expect(meta()?.content).toBe(THEME_COLORS.light);
  });
});
