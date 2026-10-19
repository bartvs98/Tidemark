import { computed, DestroyRef, DOCUMENT, effect, inject, Injectable, signal } from '@angular/core';

export type ThemePreference = 'dark' | 'light' | 'system';

const STORAGE_KEY = 'tidemark.theme';

/**
 * The page background (`--color-canvas` in styles/tokens.css) per resolved theme, for
 * <meta name="theme-color"> (the browser bar on phones). index.html's pre-paint script has a copy.
 */
export const THEME_COLORS = { light: '#f4f7fa', dark: '#070d17' } as const;

/**
 * Holds the theme preference and reflects it as <html data-theme="…">.
 * tokens.css maps that attribute to `color-scheme`, which flips every light-dark() token.
 * index.html sets the attribute before Angular boots, so there is no flash on load.
 * It also keeps <meta name="theme-color"> on the resolved theme's page background.
 */
@Injectable({ providedIn: 'root' })
export class Theme {
  private readonly document = inject(DOCUMENT);
  readonly preference = signal<ThemePreference>(this.read());

  private readonly systemDark = signal(false);

  /** Whether the dark theme is actually showing, resolving "system" via the OS setting. */
  readonly isDark = computed(() => {
    const pref = this.preference();
    return pref === 'dark' || (pref === 'system' && this.systemDark());
  });

  constructor() {
    const media = this.document.defaultView?.matchMedia?.('(prefers-color-scheme: dark)');
    if (media) {
      this.systemDark.set(media.matches);
      const onChange = (e: MediaQueryListEvent) => this.systemDark.set(e.matches);
      media.addEventListener('change', onChange);
      inject(DestroyRef).onDestroy(() => media.removeEventListener('change', onChange));
    }

    effect(() => {
      const pref = this.preference();
      this.document.documentElement.dataset['theme'] = pref;
      try {
        localStorage.setItem(STORAGE_KEY, pref);
      } catch {
        // storage blocked (private mode): the preference just won't persist
      }
    });

    // isDark() also changes when "system" flips with the OS setting.
    effect(() => {
      const color = this.isDark() ? THEME_COLORS.dark : THEME_COLORS.light;
      let meta = this.document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
      if (!meta) {
        meta = this.document.createElement('meta');
        meta.name = 'theme-color';
        this.document.head.appendChild(meta);
      }
      meta.content = color;
    });
  }

  private read(): ThemePreference {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored === 'light' || stored === 'dark' || stored === 'system') return stored;
    } catch {
      // ignore
    }
    return 'dark';
  }
}
