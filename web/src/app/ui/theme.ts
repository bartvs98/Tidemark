import { Component, DOCUMENT, effect, inject, Injectable, signal } from '@angular/core';
import { IconName, TmIcon } from './icon';

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

  constructor() {
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

const OPTIONS: { value: ThemePreference; icon: IconName; label: string }[] = [
  { value: 'system', icon: 'monitor', label: 'System theme' },
  { value: 'light', icon: 'sun', label: 'Light theme' },
  { value: 'dark', icon: 'moon', label: 'Dark theme' },
];

@Component({
  selector: 'tm-theme-switch',
  imports: [TmIcon],
  template: `
    @for (option of options; track option.value) {
      <button
        type="button"
        class="grid size-7 cursor-pointer place-items-center rounded-sm text-fg-muted transition-colors hover:text-fg aria-pressed:bg-hover aria-pressed:text-fg"
        [attr.aria-pressed]="theme.preference() === option.value"
        [attr.aria-label]="option.label"
        [attr.title]="option.label"
        (click)="theme.preference.set(option.value)"
      >
        <tm-icon [name]="option.icon" [size]="14" />
      </button>
    }
  `,
  host: {
    class: 'inline-flex gap-0.5 rounded-md border border-line p-0.5',
    role: 'group',
    'aria-label': 'Theme',
  },
})
export class TmThemeSwitch {
  protected readonly theme = inject(Theme);
  protected readonly options = OPTIONS;
}
