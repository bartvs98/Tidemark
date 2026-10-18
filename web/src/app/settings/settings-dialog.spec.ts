import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
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
});
