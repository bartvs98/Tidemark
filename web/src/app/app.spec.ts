import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { App } from './app';

describe('App', () => {
  it('loads the settings dialog on the first click of the gear and opens it', async () => {
    // jsdom has no modal dialogs.
    HTMLDialogElement.prototype.showModal ??= function (this: HTMLDialogElement) {
      this.setAttribute('open', '');
    };
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])] });
    const http = TestBed.inject(HttpTestingController);
    const fixture = TestBed.createComponent(App);
    fixture.detectChanges();
    http.expectOne('/api/health').flush({ status: 'ok', version: 'dev' });
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('tm-settings-dialog')).toBeNull();

    el.querySelector<HTMLButtonElement>('[data-testid="settings-button"]')!.click();
    fixture.detectChanges();
    // The dialog's chunk loads; its settings request is the sign it rendered with open already true.
    await vi.waitFor(() => expect(http.match('/api/settings').length).toBeGreaterThan(0), { timeout: 3000 });
    await vi.waitFor(() => expect(el.querySelector('tm-settings-dialog')).not.toBeNull());
    expect(el.querySelector('dialog')?.open).toBe(true);
  });
});
