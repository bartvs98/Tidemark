import { Component, computed, DestroyRef, DOCUMENT, effect, inject, Injectable, signal } from '@angular/core';
import { SegmentOption, TmSegmented } from './segmented';

export type ThemePreference = 'dark' | 'light' | 'system';

const STORAGE_KEY = 'tidemark.theme';

/**
 * Holds the theme preference and reflects it as <html data-theme="…">.
 * tokens.css maps that attribute to `color-scheme`, which flips every light-dark() token.
 * index.html sets the attribute before Angular boots, so there is no flash on load.
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

const OPTIONS: SegmentOption<ThemePreference>[] = [
  { value: 'system', icon: 'monitor', label: 'System theme' },
  { value: 'light', icon: 'sun', label: 'Light theme' },
  { value: 'dark', icon: 'moon', label: 'Dark theme' },
];

@Component({
  selector: 'tm-theme-switch',
  imports: [TmSegmented],
  template: `<tm-segmented label="Theme" iconOnly [options]="options" [(value)]="theme.preference" />`,
})
export class TmThemeSwitch {
  protected readonly theme = inject(Theme);
  protected readonly options = OPTIONS;
}
