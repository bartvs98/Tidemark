import { DatePipe, DecimalPipe, NgTemplateOutlet } from '@angular/common';
import { HttpClient, HttpErrorResponse, httpResource } from '@angular/common/http';
import { Component, DestroyRef, DOCUMENT, inject, model, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { TM_TABS, TmButton, TmCallout, TmDialog, TmIcon, TmSkeleton, TmTooltip, Toasts } from '../ui';

/** Shape of GET /api/settings (internal/api/settings.go). Read-only: it all comes from env vars. */
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
  };
}

export type SettingsPage = 'general' | 'checks' | 'notifications' | 'github';

export const NOTIFY_LEVELS = {
  all: 'All updates',
  attention: 'Attention and breaking',
  breaking: 'Breaking only',
} as const;

/**
 * The settings dialog behind the header's gear button. It shows how Tidemark is configured
 * (env vars, so nothing is editable here) on pages behind side tabs (a tab row on phones):
 * General (version, container source, icons, styleguide link), Checks, Notifications (a block
 * per provider, with its test button) and GitHub (token, quota). Settings are fetched each time
 * it opens; the page stays where it was.
 *
 *   <tm-settings-dialog [(open)]="settingsOpen" />
 */
@Component({
  selector: 'tm-settings-dialog',
  imports: [DatePipe, DecimalPipe, NgTemplateOutlet, RouterLink, ...TM_TABS, TmButton, TmCallout, TmDialog, TmIcon, TmSkeleton, TmTooltip],
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

  protected readonly levels = NOTIFY_LEVELS;
  protected readonly testing = signal(false);

  constructor() {
    const media = inject(DOCUMENT).defaultView?.matchMedia?.('(width >= 40rem)');
    if (!media) return;
    const update = () => this.orientation.set(media.matches ? 'vertical' : 'horizontal');
    update();
    media.addEventListener('change', update);
    inject(DestroyRef).onDestroy(() => media.removeEventListener('change', update));
  }

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
