import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { Toasts } from '../ui';
import { Settings, SettingsDialog } from './settings-dialog';

const SETTINGS: Settings = {
  version: 'sha-1382579',
  source: { kind: 'docker', location: 'tcp://socket-proxy:2375' },
  github: { token: false, rateLimit: { limit: 60, remaining: 0, resetAt: '2026-10-18T19:24:25Z' } },
  checks: { schedule: '0 */6 * * *', nextRunAt: '2026-10-19T00:00:00Z' },
  rules: { path: '/data/config.yaml', error: 'line 3: unknown rule "pinn"' },
  icons: true,
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
});
