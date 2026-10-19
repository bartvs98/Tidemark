import { DatePipe, DecimalPipe } from '@angular/common';
import { httpResource } from '@angular/common/http';
import { Component, model } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TmButton, TmCallout, TmDialog, TmIcon, TmSkeleton, TmTooltip } from '../ui';

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
}

/**
 * The settings dialog behind the header's gear button. It shows how Tidemark is configured
 * (env vars, so nothing is editable here), the GitHub quota, a link to the styleguide and
 * Tidemark's version. Settings are fetched each time it opens.
 *
 *   <tm-settings-dialog [(open)]="settingsOpen" />
 */
@Component({
  selector: 'tm-settings-dialog',
  imports: [DatePipe, DecimalPipe, RouterLink, TmButton, TmCallout, TmDialog, TmIcon, TmSkeleton, TmTooltip],
  templateUrl: './settings-dialog.html',
})
export class SettingsDialog {
  readonly open = model(false);

  /** Requested only while open; reopening asks again (the server keeps GitHub's quota current from its own requests and asks again after a minute). */
  protected readonly settings = httpResource<Settings>(() => (this.open() ? '/api/settings' : undefined));
}
