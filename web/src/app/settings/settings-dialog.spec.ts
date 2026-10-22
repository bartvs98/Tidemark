import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { Toasts } from '../ui';
import { SettingField, SettingKey, Settings, SettingsDialog } from './settings-dialog';

const SETTINGS: Settings = {
  version: 'sha-1382579',
  source: { kind: 'docker', location: 'tcp://socket-proxy:2375' },
  github: { token: false, rateLimit: { limit: 60, remaining: 0, resetAt: '2026-10-18T19:24:25Z' } },
  checks: { schedule: '0 */6 * * *', nextRunAt: '2026-10-19T00:00:00Z' },
  rules: { path: '/data/config.yaml', error: 'line 3: unknown rule "pinn"' },
  icons: true,
  editable: false,
};

describe('SettingsDialog', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    // jsdom has no modal dialogs; showModal/close only need to toggle `open` and fire "close".
    HTMLDialogElement.prototype.showModal ??= function (this: HTMLDialogElement) {
      this.setAttribute('open', '');
    };
    HTMLDialogElement.prototype.close ??= function (this: HTMLDialogElement) {
      this.removeAttribute('open');
      this.dispatchEvent(new Event('close'));
    };
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])] });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  async function render() {
    const fixture = TestBed.createComponent(SettingsDialog);
    await fixture.whenStable();
    return fixture;
  }

  it('asks for settings only while open', async () => {
    const fixture = await render();
    http.expectNone('/api/settings');
    expect(fixture.nativeElement.querySelector('dialog').open).toBe(false);

    fixture.componentRef.setInput('open', true);
    fixture.detectChanges();
    http.expectOne('/api/settings').flush(SETTINGS);
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('dialog').open).toBe(true);
  });

  it('shows the configuration without any secret, the quota and the version', async () => {
    const fixture = await render();
    fixture.componentRef.setInput('open', true);
    fixture.detectChanges();
    http.expectOne('/api/settings').flush(SETTINGS);
    await fixture.whenStable();

    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('[data-testid="settings-token"]')?.textContent).toContain('Not set');
    expect(el.querySelector('[data-testid="settings-quota"]')?.textContent).toContain('0 / 60, limit reached');
    expect(el.textContent).toContain('TIDEMARK_GITHUB_TOKEN');
    expect(el.textContent).toContain('0 */6 * * *');
    expect(el.textContent).toContain("Didn't load");
    expect(el.textContent).toContain('tcp://socket-proxy:2375');
    expect(el.querySelector('[data-testid="settings-version"]')?.textContent).toContain('sha-1382579');
    expect(el.querySelector('a[href="/styleguide"]')).not.toBeNull();
  });

  it('splits the settings into pages behind tabs, starting on General and keeping the page on reopen', async () => {
    const fixture = await render();
    fixture.componentRef.setInput('open', true);
    fixture.detectChanges();
    http.expectOne('/api/settings').flush(SETTINGS);
    await fixture.whenStable();

    const el = fixture.nativeElement as HTMLElement;
    const tabs = [...el.querySelectorAll<HTMLElement>('[role="tab"]')];
    expect(tabs.map((t) => t.textContent?.trim())).toEqual(['General', 'Checks', 'Notifications', 'AI', 'GitHub']);
    const visible = () => el.querySelector('[role="tabpanel"]:not([inert])') as HTMLElement;
    expect(visible().querySelector('[data-testid="settings-version"]')).not.toBeNull();

    tabs[2].click();
    await fixture.whenStable();
    expect(tabs[2].getAttribute('aria-selected')).toBe('true');
    expect(visible().textContent).toContain('Telegram');
    expect(visible().querySelector('[data-testid="settings-version"]')).toBeNull();

    fixture.componentRef.setInput('open', false);
    fixture.componentRef.setInput('open', true);
    fixture.detectChanges();
    http.expectOne('/api/settings').flush(SETTINGS);
    await fixture.whenStable();
    expect(visible().textContent).toContain('Telegram');
  });

  it('closes from the close button and on the dialog closing itself (Escape)', async () => {
    const fixture = await render();
    fixture.componentRef.setInput('open', true);
    fixture.detectChanges();
    http.expectOne('/api/settings').flush(SETTINGS);
    await fixture.whenStable();

    const dialog = fixture.nativeElement.querySelector('dialog') as HTMLDialogElement;
    (dialog.querySelector('button[aria-label="Close"]') as HTMLButtonElement).click();
    await fixture.whenStable();
    expect(dialog.open).toBe(false);
    expect(fixture.componentInstance.open()).toBe(false);

    // In the app [(open)] writes false back to the parent; setInput needs a change to apply.
    fixture.componentRef.setInput('open', false);
    fixture.componentRef.setInput('open', true);
    fixture.detectChanges();
    http.expectOne('/api/settings').flush({
      ...SETTINGS,
      github: { token: true, rateLimitError: 'GitHub answered 503' },
      rules: { path: '/data/config.yaml', missing: true },
    });
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('[data-testid="settings-token"]')?.textContent).toContain('Set');
    expect(fixture.nativeElement.textContent).toContain('Not found, labels only');

    dialog.close(); // what Escape does
    await fixture.whenStable();
    expect(fixture.componentInstance.open()).toBe(false);
  });

  describe('notifications', () => {
    type Notifications = NonNullable<Settings['notifications']>;

    async function open(notifications?: Notifications) {
      const fixture = await render();
      fixture.componentRef.setInput('open', true);
      fixture.detectChanges();
      http.expectOne('/api/settings').flush({ ...SETTINGS, notifications });
      await fixture.whenStable();
      return fixture.nativeElement as HTMLElement;
    }
    const on = (n: Partial<Notifications> = {}): Notifications => ({ notifier: 'telegram', level: 'all', ...n });

    it('says what to set when off, with no test button', async () => {
      const el = await open();
      expect(el.querySelector('[data-testid="settings-notify-off"]')?.textContent).toContain('Off');
      expect(el.textContent).toContain('TIDEMARK_TELEGRAM_BOT_TOKEN');
      expect(el.textContent).toContain('TIDEMARK_TELEGRAM_CHAT_ID');
      expect(el.querySelector('[data-testid="settings-notify-test"]')).toBeNull();
    });

    it('shows the level wording', async () => {
      for (const [level, text] of [
        ['all', 'All updates'],
        ['attention', 'Attention and breaking'],
        ['breaking', 'Breaking only'],
      ] as const) {
        TestBed.resetTestingModule();
        TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])] });
        http = TestBed.inject(HttpTestingController);
        const el = await open(on({ level }));
        expect(el.querySelector('[data-testid="settings-notifications"]')?.textContent).toContain(text);
      }
    });

    it('shows after each check without links and nothing sent yet', async () => {
      const el = await open(on());
      const text = el.querySelector('[data-testid="settings-notifications"]')?.textContent ?? '';
      expect(text).toContain('After each check');
      expect(text).toContain('Not set, messages have no links');
      expect(text).toContain('Nothing sent yet');
      expect(el.querySelector('[data-testid="settings-notify-test"]')).not.toBeNull();
    });

    it('shows the digest, the public URL and the last send', async () => {
      const el = await open(
        on({ digest: '0 9 * * *', publicUrl: 'https://tidemark.example.lan', last: { at: '2026-10-19T09:00:00Z', kind: 'updates', items: 3 } }),
      );
      const text = el.querySelector('[data-testid="settings-notifications"]')?.textContent ?? '';
      expect(text).toContain('Digest');
      expect(text).toContain('0 9 * * *');
      expect(text).toContain('https://tidemark.example.lan');
      expect(text).toContain('3 updates');
      expect(text).not.toContain('Failed');
    });

    it('shows when the next message can go out', async () => {
      const next = async (n: Partial<Notifications>) => {
        TestBed.resetTestingModule();
        TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])] });
        http = TestBed.inject(HttpTestingController);
        const el = await open(on(n));
        return el.querySelector('[data-testid="settings-notify-next"]')?.textContent ?? '';
      };
      const digest = await next({ digest: '0 9 * * *', nextAt: '2026-10-20T09:00:00Z' });
      expect(digest).toContain('20 Oct');
      expect(digest).not.toContain('After the check');
      expect(await next({ nextAt: '2026-10-20T06:00:00Z' })).toContain('After the check at 20 Oct');
      expect(await next({})).toContain('none is scheduled');
    });

    it('shows a failed last attempt', async () => {
      const el = await open(on({ last: { at: '2026-10-19T09:00:00Z', kind: 'test', items: 0, error: 'telegram: chat not found' } }));
      const last = el.querySelector('[data-testid="settings-notify-last"]')?.textContent ?? '';
      expect(last).toContain('Failed');
      expect(last).toContain('test message');
    });

    it('sends a test message, toasts and reloads the settings', async () => {
      const el = await open(on());
      const toasts = TestBed.inject(Toasts);
      const button = el.querySelector('[data-testid="settings-notify-test"]') as HTMLButtonElement;
      button.click();
      const req = http.expectOne('/api/notifications/test');
      expect(req.request.method).toBe('POST');
      req.flush(null, { status: 204, statusText: 'No Content' });
      await new Promise((r) => setTimeout(r));
      http.expectOne('/api/settings').flush({ ...SETTINGS, notifications: on() });
      expect(toasts.items().map((t) => [t.message, t.tone])).toEqual([['Test message sent', 'ok']]);
    });

    it('toasts the server error when the test fails and reloads', async () => {
      const el = await open(on());
      const toasts = TestBed.inject(Toasts);
      (el.querySelector('[data-testid="settings-notify-test"]') as HTMLButtonElement).click();
      http.expectOne('/api/notifications/test').flush({ error: 'telegram: Bad Request: chat not found' }, { status: 502, statusText: 'Bad Gateway' });
      await new Promise((r) => setTimeout(r));
      http.expectOne('/api/settings').flush({ ...SETTINGS, notifications: on() });
      expect(toasts.items().map((t) => [t.message, t.tone])).toEqual([['telegram: Bad Request: chat not found', 'attention']]);
    });
  });

  it('says how to turn editing on when read-only, with no form', async () => {
    for (const settings of [SETTINGS, { ...SETTINGS, editable: true }]) {
      TestBed.resetTestingModule();
      TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])] });
      http = TestBed.inject(HttpTestingController);
      const fixture = await render();
      fixture.componentRef.setInput('open', true);
      fixture.detectChanges();
      http.expectOne('/api/settings').flush(settings);
      await fixture.whenStable();
      const el = fixture.nativeElement as HTMLElement;
      expect(el.querySelector('[data-testid="settings-readonly"]')?.textContent).toContain('TIDEMARK_SETTINGS_EDITABLE=true');
      expect(el.querySelector('input, tm-switch, tm-select, tm-secret-setting')).toBeNull();
    }
  });

  describe('editable', () => {
    const FIELDS: Record<SettingKey, SettingField> = {
      schedule: { value: '0 */6 * * *', source: 'default', env: 'TIDEMARK_SCHEDULE' },
      githubToken: { secret: true, set: true, source: 'stored', env: 'TIDEMARK_GITHUB_TOKEN' },
      icons: { value: 'on', source: 'default', env: 'TIDEMARK_ICONS' },
      iconsUrl: { value: 'https://icons.example.lan', source: 'env', env: 'TIDEMARK_ICONS_URL' },
      telegramBotToken: { secret: true, set: false, source: 'default', env: 'TIDEMARK_TELEGRAM_BOT_TOKEN' },
      telegramChatId: { value: '', source: 'default', env: 'TIDEMARK_TELEGRAM_CHAT_ID' },
      notifyLevel: { value: 'all', source: 'default', env: 'TIDEMARK_NOTIFY_LEVEL' },
      notifyDigest: { value: '', source: 'default', env: 'TIDEMARK_NOTIFY_DIGEST' },
      publicUrl: { value: 'https://tidemark.example.lan', source: 'stored', env: 'TIDEMARK_PUBLIC_URL' },
      aiProvider: { value: 'off', source: 'default', env: 'TIDEMARK_AI_PROVIDER' },
      aiApiKey: { secret: true, set: false, source: 'default', env: 'TIDEMARK_AI_API_KEY' },
      aiModel: { value: '', source: 'default', env: 'TIDEMARK_AI_MODEL' },
      aiBaseUrl: { value: '', source: 'default', env: 'TIDEMARK_AI_BASE_URL' },
      aiAutoMinReleases: { value: '0', source: 'default', env: 'TIDEMARK_AI_AUTO_MIN_RELEASES' },
    };
    const EDITABLE: Settings = { ...SETTINGS, editable: true, fields: FIELDS };
    const PAGES = ['general', 'checks', 'notifications', 'ai', 'github'];

    let fixture: Awaited<ReturnType<typeof render>>;
    let el: HTMLElement;

    async function open(settings: Settings = EDITABLE) {
      fixture = await render();
      fixture.componentRef.setInput('open', true);
      fixture.detectChanges();
      http.expectOne('/api/settings').flush(settings);
      await settle();
      el = fixture.nativeElement as HTMLElement;
    }
    const panel = () => el.querySelector('[role="tabpanel"]:not([inert])') as HTMLElement;
    async function goTo(page: string) {
      el.querySelectorAll<HTMLElement>('[role="tab"]')[PAGES.indexOf(page)].click();
      await fixture.whenStable();
    }
    const input = (key: string) => panel().querySelector(`[data-testid="setting-${key}"]`) as HTMLInputElement;
    const button = (text: string) =>
      [...panel().querySelectorAll<HTMLButtonElement>('button')].find((b) => b.textContent?.trim() === text) as HTMLButtonElement;
    const saveBar = () => panel().querySelector('[data-testid="settings-save-bar"]');
    async function type(field: HTMLInputElement, value: string) {
      field.value = value;
      field.dispatchEvent(new Event('input'));
      await fixture.whenStable();
    }
    async function settle() {
      await new Promise((r) => setTimeout(r));
      await fixture.whenStable();
    }
    async function click(b: HTMLButtonElement) {
      b.click();
      await fixture.whenStable();
    }

    it('shows each page as a form with the values, locks fields set by env and never prefills secrets', async () => {
      await open();
      expect(el.querySelector('[data-testid="settings-readonly"]')).toBeNull();
      const icons = panel().querySelector('[role="switch"]') as HTMLButtonElement;
      expect(icons.getAttribute('aria-checked')).toBe('true');
      expect(input('iconsUrl').value).toBe('https://icons.example.lan');
      expect(input('iconsUrl').disabled).toBe(true);
      expect(panel().textContent).toMatch(/Set by\s*TIDEMARK_ICONS_URL/);
      expect(saveBar()).toBeNull();

      await goTo('checks');
      expect(input('schedule').value).toBe('0 */6 * * *');
      expect(input('schedule').disabled).toBe(false);

      await goTo('notifications');
      expect(panel().querySelector('[data-testid="setting-telegramBotToken"]')?.textContent).toContain('Not set');
      expect(input('publicUrl').value).toBe('https://tidemark.example.lan');
      expect(panel().textContent).toContain('All updates');

      await goTo('github');
      expect(panel().querySelector('[data-testid="setting-githubToken"]')?.textContent).toContain('Set');
      expect(panel().querySelector('input')).toBeNull();
      await click(button('Replace'));
      const secret = panel().querySelector('tm-password-input input') as HTMLInputElement;
      expect(secret.value).toBe('');
      expect(document.activeElement).toBe(secret);
      expect(saveBar()).toBeNull(); // an open but empty input changes nothing
    });

    it('saves only the changed keys of the page, shows the answer and keeps other pages edits', async () => {
      await open();
      await goTo('checks');
      await type(input('schedule'), '@daily');
      expect(saveBar()).not.toBeNull();

      await goTo('notifications');
      await click(button('Set'));
      await type(panel().querySelector('tm-password-input input') as HTMLInputElement, '123:abc');
      await type(input('telegramChatId'), '42');
      await type(input('publicUrl'), '');
      await click(button('Save'));
      const req = http.expectOne('/api/settings');
      expect(req.request.method).toBe('PATCH');
      expect(req.request.body).toEqual({ telegramBotToken: '123:abc', telegramChatId: '42', publicUrl: null });
      expect(button('Save').getAttribute('aria-busy')).toBe('true');
      req.flush({
        ...EDITABLE,
        notifications: { notifier: 'telegram', level: 'all' },
        fields: {
          ...FIELDS,
          telegramBotToken: { ...FIELDS.telegramBotToken, set: true, source: 'stored' },
          telegramChatId: { ...FIELDS.telegramChatId, value: '42', source: 'stored' },
          publicUrl: { ...FIELDS.publicUrl, value: '', source: 'default' },
        },
      });
      await settle();
      expect(TestBed.inject(Toasts).items().map((t) => t.message)).toEqual(['Settings saved']);
      expect(saveBar()).toBeNull();
      expect(panel().querySelector('[data-testid="setting-telegramBotToken"]')?.textContent).toContain('Set');
      expect(panel().querySelector('tm-password-input')).toBeNull();
      expect(input('telegramChatId').value).toBe('42');

      await goTo('checks');
      expect(input('schedule').value).toBe('@daily');
      expect(saveBar()).not.toBeNull();
    });

    describe('AI page', () => {
      const ON: Settings['ai'] = { provider: 'openai', model: 'llama3.1:8b', baseUrl: 'http://ollama:11434/v1', autoMinReleases: 5, usage: { since: '2026-10-01T00:00:00+02:00', calls: 14, failed: 2, inputTokens: 182000, outputTokens: 9100 } };
      const withAi = (fields: Partial<Record<SettingKey, SettingField>>, ai?: Settings['ai']): Settings => ({
        ...EDITABLE,
        ai,
        fields: { ...FIELDS, ...fields },
      });

      it('is off by default: only the provider is asked for, then the rest follows the provider', async () => {
        await open();
        await goTo('ai');
        expect(panel().querySelector('[data-testid="settings-ai-status"]')?.textContent).toContain('Off');
        expect(panel().querySelector('tm-select')).not.toBeNull();
        expect(input('aiModel')).toBeNull();
        expect(panel().querySelector('[data-testid="setting-aiApiKey"]')).toBeNull();
        expect(panel().textContent).toContain('only for the summaries you ask for.');

        const edit = (key: string, v: string) => (fixture.componentInstance as unknown as { edit(k: string, v: string): void }).edit(key, v);
        edit('aiProvider', 'anthropic');
        await fixture.whenStable();
        expect(input('aiModel').placeholder).toBe('claude-haiku-5-5');
        expect(panel().querySelector('[data-testid="setting-aiApiKey"]')?.textContent).toContain('Not set');
        expect(input('aiBaseUrl')).toBeNull(); // only for the OpenAI-compatible provider
        expect(panel().textContent).toContain('Off, needs an API key');

        edit('aiProvider', 'gemini');
        await fixture.whenStable();
        expect(input('aiModel').placeholder).toBe('gemini-flash-latest');

        edit('aiProvider', 'openai');
        await fixture.whenStable();
        expect(input('aiModel').placeholder).toBe('required');
        expect(input('aiBaseUrl')).not.toBeNull();
        expect(panel().textContent).toContain('http://ollama:11434/v1');
      });

      it('shows the values, locks env fields and mentions the automatic summaries', async () => {
        await open(
          withAi(
            {
              aiProvider: { value: 'openai', source: 'stored', env: 'TIDEMARK_AI_PROVIDER' },
              aiModel: { value: 'llama3.1:8b', source: 'env', env: 'TIDEMARK_AI_MODEL' },
              aiBaseUrl: { value: 'http://ollama:11434/v1', source: 'stored', env: 'TIDEMARK_AI_BASE_URL' },
              aiAutoMinReleases: { value: '5', source: 'stored', env: 'TIDEMARK_AI_AUTO_MIN_RELEASES' },
            },
            ON,
          ),
        );
        await goTo('ai');
        expect(panel().querySelector('[data-testid="settings-ai-status"]')?.textContent).toContain('On');
        expect(input('aiModel').value).toBe('llama3.1:8b');
        expect(input('aiModel').disabled).toBe(true);
        expect(panel().textContent).toMatch(/Set by\s*TIDEMARK_AI_MODEL/);
        expect(input('aiBaseUrl').value).toBe('http://ollama:11434/v1');
        expect(input('aiAutoMinReleases').value).toBe('5');
        expect(panel().querySelector('[data-testid="settings-ai-note"]')?.textContent).toContain('automatically for updates with 5+ releases');
      });

      it('never prefills the key and saves only the changed keys', async () => {
        await open(withAi({ aiProvider: { value: 'anthropic', source: 'stored', env: 'TIDEMARK_AI_PROVIDER' }, aiApiKey: { secret: true, set: true, source: 'stored', env: 'TIDEMARK_AI_API_KEY' } }, { provider: 'anthropic', model: 'claude-haiku-5-5', autoMinReleases: 0 }));
        await goTo('ai');
        expect(panel().querySelector('[data-testid="setting-aiApiKey"]')?.textContent).toContain('Set');
        expect(panel().querySelector('input[type="password"]')).toBeNull();
        await type(input('aiAutoMinReleases'), '3');
        await click(button('Replace'));
        await type(panel().querySelector('tm-password-input input') as HTMLInputElement, 'sk-new');
        await click(button('Save'));
        const req = http.expectOne('/api/settings');
        expect(req.request.method).toBe('PATCH');
        expect(req.request.body).toEqual({ aiApiKey: 'sk-new', aiAutoMinReleases: '3' });
        req.flush(EDITABLE);
        await settle();
      });

      it('is read-only without the editable flag: the state, and how to turn it on', async () => {
        await open({ ...SETTINGS, ai: ON });
        await goTo('ai');
        expect(panel().querySelector('input, tm-select, tm-secret-setting')).toBeNull();
        const text = panel().querySelector('[data-testid="settings-ai"]')!.textContent!;
        expect(text).toContain('OpenAI-compatible');
        expect(text).toContain('llama3.1:8b');
        expect(text).toContain('http://ollama:11434/v1');
        expect(text).toContain('From 5 releases behind');
        expect(panel().querySelector('[data-testid="settings-ai-usage"]')?.textContent).toBe('14 summaries · 182k tokens in · 9.1k out · 2 failed');
        expect(panel().querySelector('[data-testid="settings-ai-note"]')?.textContent).toContain('5+ releases');

        TestBed.resetTestingModule();
        TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])] });
        http = TestBed.inject(HttpTestingController);
        await open(SETTINGS);
        await goTo('ai');
        expect(panel().querySelector('[data-testid="settings-ai-off"]')).not.toBeNull();
        expect(panel().textContent).toContain('TIDEMARK_AI_PROVIDER');
      });
    });

    it('clears a secret on Save, with an undo', async () => {
      await open();
      await goTo('github');
      await click(button('Clear'));
      expect(panel().textContent).toContain('Cleared on Save');
      await click(button('Undo'));
      expect(saveBar()).toBeNull();
      await click(button('Clear'));
      await click(button('Save'));
      const req = http.expectOne('/api/settings');
      expect(req.request.body).toEqual({ githubToken: null });
      req.flush({ ...EDITABLE, fields: { ...FIELDS, githubToken: { ...FIELDS.githubToken, set: false, source: 'default' } } });
      await settle();
      expect(panel().querySelector('[data-testid="setting-githubToken"]')?.textContent).toContain('Not set');
    });

    it('puts field errors under their field and keeps the edit', async () => {
      await open();
      await goTo('checks');
      await type(input('schedule'), 'nope');
      await click(button('Save'));
      http
        .expectOne('/api/settings')
        .flush(
          { error: "Some settings aren't valid", fields: { schedule: 'invalid cron spec: expected exactly 5 fields, found 1: [nope]' } },
          { status: 422, statusText: 'Unprocessable Entity' },
        );
      await settle();
      expect(panel().textContent).toContain('invalid cron spec: expected exactly 5 fields');
      expect(input('schedule').getAttribute('aria-invalid')).toBe('true');
      expect(input('schedule').value).toBe('nope');
      expect(saveBar()).not.toBeNull();
      expect(TestBed.inject(Toasts).items()).toEqual([]);

      await type(input('schedule'), 'nope2'); // editing clears the error
      expect(input('schedule').getAttribute('aria-invalid')).toBeNull();
    });

    it('toasts other errors', async () => {
      await open();
      await goTo('checks');
      await type(input('schedule'), '@daily');
      await click(button('Save'));
      http.expectOne('/api/settings').flush({ error: 'applying failed' }, { status: 500, statusText: 'Internal Server Error' });
      await settle();
      expect(TestBed.inject(Toasts).items().map((t) => [t.message, t.tone])).toEqual([['applying failed', 'attention']]);
    });

    it('discards the page edits, and closing the dialog drops them all', async () => {
      await open();
      const icons = panel().querySelector('[role="switch"]') as HTMLButtonElement;
      await click(icons);
      expect(icons.getAttribute('aria-checked')).toBe('false');
      await click(button('Discard'));
      expect(icons.getAttribute('aria-checked')).toBe('true');
      expect(saveBar()).toBeNull();

      await goTo('checks');
      await type(input('schedule'), '@daily');
      fixture.componentRef.setInput('open', false);
      fixture.detectChanges();
      fixture.componentRef.setInput('open', true);
      fixture.detectChanges();
      http.expectOne('/api/settings').flush(EDITABLE);
      await settle();
      expect(input('schedule').value).toBe('0 */6 * * *');
      expect(saveBar()).toBeNull();
    });

    it('waits with the test message while the notifications page has changes', async () => {
      await open({ ...EDITABLE, notifications: { notifier: 'telegram', level: 'all' } });
      await goTo('notifications');
      const test = panel().querySelector('[data-testid="settings-notify-test"]') as HTMLButtonElement;
      expect(test.disabled).toBe(false);
      await type(input('notifyDigest'), '0 9 * * *');
      expect(test.disabled).toBe(true);
      expect(test.parentElement?.getAttribute('data-tooltip')).toContain('Save or discard');
    });

    it('shows warnings on the General page', async () => {
      await open({ ...EDITABLE, warnings: ['stored schedule: invalid cron spec: x'] });
      expect(panel().querySelector('[data-testid="settings-warnings"]')?.textContent).toContain('stored schedule: invalid cron spec: x');
    });
  });
});
