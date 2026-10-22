import { DatePipe, DecimalPipe, NgTemplateOutlet } from '@angular/common';
import { HttpClient, HttpErrorResponse, httpResource } from '@angular/common/http';
import { afterNextRender, Component, computed, DestroyRef, DOCUMENT, effect, ElementRef, inject, Injector, model, signal, untracked } from '@angular/core';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import {
  SelectOption,
  TM_TABS,
  TmButton,
  TmCallout,
  TmDialog,
  TmField,
  TmIcon,
  TmInput,
  TmSelect,
  TmSkeleton,
  TmSwitch,
  TmTooltip,
  Toasts,
} from '../ui';
import { SecretSetting } from './secret-setting/secret-setting';

/** The settings that can be edited here (with TIDEMARK_SETTINGS_EDITABLE=true), by API name. */
export type SettingKey =
  | 'schedule'
  | 'githubToken'
  | 'icons'
  | 'iconsUrl'
  | 'telegramBotToken'
  | 'telegramChatId'
  | 'notifyLevel'
  | 'notifyDigest'
  | 'publicUrl'
  | 'aiProvider'
  | 'aiApiKey'
  | 'aiModel'
  | 'aiBaseUrl'
  | 'aiAutoMinReleases';

/** One editable setting: its effective value (never for secrets: only `set`), where it comes from and its env var. */
export interface SettingField {
  value?: string;
  secret?: true;
  set?: boolean;
  source: 'env' | 'stored' | 'default';
  env: string;
}

/** Shape of GET /api/settings (internal/api/settings.go), also the answer to PATCH. */
export interface Settings {
  version: string;
  source: { kind: 'docker' | 'snapshot'; location: string; capturedAt?: string };
  github: {
    token: boolean;
    /** GitHub's current quota; absent when it couldn't be looked up (rateLimitError says why). */
    rateLimit?: { limit: number; remaining: number; resetAt: string };
    rateLimitError?: string;
  };
  /** schedule is a cron spec, absent when scheduled checks are off. */
  checks: { schedule?: string; nextRunAt?: string };
  /** path is absent without a rules file; missing when nothing is there; error when it didn't load. */
  rules: { path?: string; missing?: boolean; error?: string };
  icons: boolean;
  /** Absent when notifications are off. */
  notifications?: {
    notifier: 'telegram';
    level: 'all' | 'attention' | 'breaking';
    /** Cron spec of the digest; absent = after each check. */
    digest?: string;
    publicUrl?: string;
    /** The newest send attempt; error is set when it failed. */
    last?: { at: string; kind: 'updates' | 'first' | 'test'; items: number; error?: string };
    /** When Tidemark next looks for something to send: the next digest, or the next scheduled check. */
    nextAt?: string;
  };
  /** TIDEMARK_SETTINGS_EDITABLE: the fields can be changed here. */
  editable: boolean;
  /** Absent when AI summaries are off. `model` is the effective one, `baseUrl` is set for the openai provider only. */
  ai?: { provider: 'anthropic' | 'openai' | 'gemini'; model: string; baseUrl?: string; autoMinReleases: number };
  /** Absent when the server has no settings manager (then nothing is editable). */
  fields?: Record<SettingKey, SettingField>;
  /** Stored values that were ignored, a half Telegram pair… */
  warnings?: string[];
}

export type SettingsPage = 'general' | 'checks' | 'notifications' | 'ai' | 'github';

export const NOTIFY_LEVELS = {
  all: 'All updates',
  attention: 'Attention and breaking',
  breaking: 'Breaking only',
} as const;

export const AI_PROVIDERS = {
  off: 'Off',
  anthropic: 'Anthropic',
  openai: 'OpenAI-compatible',
  gemini: 'Gemini',
} as const;

/** What the model field shows when empty: the provider's default (the server's `ai.DefaultAnthropicModel`, …). */
const AI_DEFAULT_MODEL: Record<string, string> = { anthropic: 'claude-haiku-5-5', gemini: 'gemini-flash-latest', openai: 'required' };

/** The editable settings on each page; each page saves only its own. */
const PAGE_KEYS: Record<SettingsPage, readonly SettingKey[]> = {
  general: ['icons', 'iconsUrl'],
  checks: ['schedule'],
  notifications: ['telegramBotToken', 'telegramChatId', 'notifyLevel', 'notifyDigest', 'publicUrl'],
  ai: ['aiProvider', 'aiApiKey', 'aiModel', 'aiBaseUrl', 'aiAutoMinReleases'],
  github: ['githubToken'],
};
const PAGES = Object.keys(PAGE_KEYS) as SettingsPage[];

/** Pending edits by key: a string (typed) or null (a secret to clear). Absent = untouched. */
type Edits = Partial<Record<SettingKey, string | null>>;
/** A PATCH /api/settings body: a string sets, null clears (back to the default). */
type Patch = Partial<Record<SettingKey, string | null>>;

/**
 * The settings dialog behind the header's gear button. It shows how Tidemark is configured on
 * pages behind side tabs (a tab row on phones): General (version, container source, icons,
 * styleguide link), Checks, Notifications (a block per provider, with its test button), AI
 * (provider, key, model, auto threshold) and GitHub (token, quota). Settings are fetched each time it opens; the page stays where it was.
 *
 * Read-only unless the server says `editable` (TIDEMARK_SETTINGS_EDITABLE=true): then each page
 * shows a form for its settings and a Save / Discard bar once it has changes. Save sends only
 * that page's changed keys; field errors from the server show under their field. A setting set
 * by an env var is locked. Edits survive switching pages; closing the dialog drops them.
 *
 *   <tm-settings-dialog [(open)]="settingsOpen" />
 */
@Component({
  selector: 'tm-settings-dialog',
  imports: [
    DatePipe,
    DecimalPipe,
    NgTemplateOutlet,
    RouterLink,
    ...TM_TABS,
    SecretSetting,
    TmButton,
    TmCallout,
    TmDialog,
    TmField,
    TmIcon,
    TmInput,
    TmSelect,
    TmSkeleton,
    TmSwitch,
    TmTooltip,
  ],
  templateUrl: './settings-dialog.html',
})
export class SettingsDialog {
  readonly open = model(false);

  protected readonly page = signal<SettingsPage>('general');
  /** The tabs are a column from 640px (.tm-tabs-vertical), so the arrow keys follow the layout. */
  protected readonly orientation = signal<'vertical' | 'horizontal'>('vertical');

  /** Requested only while open; reopening asks again (the server keeps GitHub's quota current from its own requests and asks again after a minute). */
  protected readonly settings = httpResource<Settings>(() => (this.open() ? '/api/settings' : undefined));

  private readonly http = inject(HttpClient);
  private readonly toasts = inject(Toasts);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  protected readonly levels = NOTIFY_LEVELS;
  protected readonly levelOptions: SelectOption[] = Object.entries(NOTIFY_LEVELS).map(([value, label]) => ({ value, label }));
  protected readonly aiProviders = AI_PROVIDERS;
  protected readonly aiProviderOptions: SelectOption[] = Object.entries(AI_PROVIDERS).map(([value, label]) => ({ value, label }));
  protected readonly testing = signal(false);

  /** The editable fields, or null when the settings are read-only. */
  protected readonly fields = computed(() => {
    if (!this.settings.hasValue()) return null;
    const s = this.settings.value();
    return s.editable && s.fields ? s.fields : null;
  });
  protected readonly edits = signal<Edits>({});
  /** The server's message per field, from the last Save that failed validation. */
  protected readonly errors = signal<Partial<Record<SettingKey, string>>>({});
  /** The page being saved. */
  protected readonly saving = signal<SettingsPage | null>(null);

  /** What Save would send, per page. */
  protected readonly patches = computed(() => {
    const out = {} as Record<SettingsPage, Patch>;
    for (const page of PAGES) out[page] = this.patchFor(page);
    return out;
  });

  constructor() {
    // Closing the dialog discards unsaved edits.
    effect(() => {
      if (this.open()) return;
      untracked(() => {
        this.edits.set({});
        this.errors.set({});
      });
    });

    const media = inject(DOCUMENT).defaultView?.matchMedia?.('(width >= 40rem)');
    if (!media) return;
    const update = () => this.orientation.set(media.matches ? 'vertical' : 'horizontal');
    update();
    media.addEventListener('change', update);
    inject(DestroyRef).onDestroy(() => media.removeEventListener('change', update));
  }

  /** The value a (non-secret) field shows: the edit, or what's in effect. */
  protected value(key: SettingKey): string {
    const edit = this.edits()[key];
    return typeof edit === 'string' ? edit : (this.fields()?.[key]?.value ?? '');
  }

  /** Records an edit; `undefined` drops it. Editing a field clears its error. */
  protected edit(key: SettingKey, value: string | null | undefined): void {
    this.edits.update((e) => {
      const next = { ...e };
      if (value === undefined) delete next[key];
      else next[key] = value;
      return next;
    });
    if (this.errors()[key]) this.errors.update((e) => omit(e, [key]));
  }

  /** "Set by TIDEMARK_X" when an env var sets the field (it can't be changed here), else null. */
  protected locked(key: SettingKey): string | null {
    const f = this.fields()?.[key];
    return f?.source === 'env' ? `Set by ${f.env}` : null;
  }

  protected dirty(page: SettingsPage): boolean {
    return Object.keys(this.patches()[page]).length > 0;
  }

  protected discard(page: SettingsPage): void {
    this.edits.update((e) => omit(e, PAGE_KEYS[page]));
    this.errors.update((e) => omit(e, PAGE_KEYS[page]));
  }

  protected async save(page: SettingsPage): Promise<void> {
    const body = this.patches()[page];
    if (this.saving() || !Object.keys(body).length) return;
    this.saving.set(page);
    this.errors.update((e) => omit(e, PAGE_KEYS[page]));
    try {
      const next = await firstValueFrom(this.http.patch<Settings>('/api/settings', body));
      this.settings.set(next);
      this.edits.update((e) => omit(e, PAGE_KEYS[page]));
      this.toasts.show('Settings saved', 'ok');
    } catch (err) {
      const answer = err instanceof HttpErrorResponse ? (err.error as { error?: string; fields?: Record<string, string> } | null) : null;
      const known = Object.entries(answer?.fields ?? {}).filter(([key]) => PAGES.some((p) => PAGE_KEYS[p].includes(key as SettingKey)));
      if (err instanceof HttpErrorResponse && err.status === 422 && known.length) {
        this.errors.update((e) => ({ ...e, ...Object.fromEntries(known) }));
        // Normally the errors are on this page; if not, go to the first page that has one.
        const keys = known.map(([key]) => key as SettingKey);
        if (!PAGE_KEYS[page].some((k) => keys.includes(k))) {
          this.page.set(PAGES.find((p) => PAGE_KEYS[p].some((k) => keys.includes(k))) ?? page);
        }
        // Focus the first field with an error and bring its message out from under the save bar.
        afterNextRender(
          () => {
            const control = this.host.nativeElement.querySelector<HTMLElement>('[role=tabpanel]:not([inert]) [aria-invalid=true]');
            control?.focus({ preventScroll: true });
            control?.closest('tm-field')?.scrollIntoView({ block: 'center' });
          },
          { injector: this.injector },
        );
      } else {
        this.toasts.show(answer?.error ?? (err instanceof HttpErrorResponse ? err.message : String(err)), 'attention');
      }
    } finally {
      this.saving.set(null);
    }
  }

  private patchFor(page: SettingsPage): Patch {
    const fields = this.fields();
    const edits = this.edits();
    const out: Patch = {};
    if (!fields) return out;
    for (const key of PAGE_KEYS[page]) {
      const edit = edits[key];
      const f = fields[key];
      if (edit === undefined || !f || f.source === 'env') continue;
      if (f.secret) {
        // Sent only when typed or cleared; an open but empty input changes nothing.
        if (edit === null || edit !== '') out[key] = edit;
      } else if (edit !== null && edit !== (f.value ?? '')) {
        out[key] = edit.trim() === '' ? null : edit;
      }
    }
    return out;
  }

  /** The provider being edited (the pending edit, else what's in effect); "off" when none. */
  protected readonly aiProvider = computed(() => this.value('aiProvider') || 'off');
  protected readonly aiModelPlaceholder = computed(() => AI_DEFAULT_MODEL[this.aiProvider()] ?? '');
  /** The auto threshold as edited, 0 when unset or not a number. */
  protected readonly aiAuto = computed(() => {
    if (!this.fields()) return (this.settings.hasValue() && this.settings.value().ai?.autoMinReleases) || 0;
    return Number.parseInt(this.value('aiAutoMinReleases'), 10) || 0;
  });

  /** What the last attempt sent, e.g. "3 updates" or "test message". */
  protected sentWhat(last: NonNullable<NonNullable<Settings['notifications']>['last']>): string {
    if (last.kind === 'test') return 'test message';
    return `${last.items} ${last.items === 1 ? 'update' : 'updates'}`;
  }

  protected async sendTest(): Promise<void> {
    if (this.testing()) return;
    this.testing.set(true);
    try {
      await firstValueFrom(this.http.post('/api/notifications/test', null));
      this.toasts.show('Test message sent', 'ok');
    } catch (err) {
      const reason = err instanceof HttpErrorResponse ? ((err.error as { error?: string } | null)?.error ?? err.message) : String(err);
      this.toasts.show(reason, 'attention');
    } finally {
      this.testing.set(false);
      this.settings.reload(); // the attempt is logged either way, so "Last sent" changes
    }
  }
}

function omit<T extends object>(obj: T, keys: readonly PropertyKey[]): T {
  const next = { ...obj };
  for (const key of keys) delete next[key as keyof T];
  return next;
}
