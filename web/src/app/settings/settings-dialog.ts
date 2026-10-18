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
  template: `
    <tm-dialog heading="Settings" [(open)]="open">
      @if (settings.hasValue()) {
        @let s = settings.value();
        <div class="flex flex-col gap-6" data-testid="settings">
          <section aria-labelledby="settings-github">
            <h3 id="settings-github" class="mb-1 text-xs font-medium tracking-wide text-fg-subtle uppercase">GitHub</h3>
            <dl>
              <div class="tm-setting">
                <dt>Token</dt>
                <dd data-testid="settings-token">
                  @if (s.github.token) {
                    <span class="inline-flex items-center gap-1.5"><tm-icon name="check" />Set</span>
                  } @else {
                    <span class="inline-flex items-center gap-1.5 text-fg-muted"><tm-icon name="circle-help" />Not set</span>
                  }
                </dd>
              </div>
              <div class="tm-setting">
                <dt>Requests left</dt>
                <dd data-testid="settings-quota">
                  @if (s.github.rateLimit; as rl) {
                    <span class="font-mono">{{ rl.remaining | number }} / {{ rl.limit | number }}</span>
                    <span class="text-fg-muted">{{ rl.remaining === 0 ? ', limit reached' : ' this hour' }}, resets {{ rl.resetAt | date: 'HH:mm' }}</span>
                  } @else {
                    <span class="text-fg-muted" [tmTooltip]="s.github.rateLimitError">Unknown</span>
                  }
                </dd>
              </div>
            </dl>
            @if (!s.github.token) {
              <p class="mt-2 text-sm text-fg-muted">
                Without a token GitHub allows 60 requests an hour. Set
                <span class="font-mono text-xs">TIDEMARK_GITHUB_TOKEN</span> (read-only, public repositories) for 5,000.
              </p>
            }
          </section>

          <section aria-labelledby="settings-checks">
            <h3 id="settings-checks" class="mb-1 text-xs font-medium tracking-wide text-fg-subtle uppercase">Checks</h3>
            <dl>
              <div class="tm-setting">
                <dt>Schedule</dt>
                <dd>
                  @if (s.checks.schedule) {
                    <span class="font-mono">{{ s.checks.schedule }}</span>
                  } @else {
                    <span class="text-fg-muted">Off, only Check now</span>
                  }
                </dd>
              </div>
              @if (s.checks.nextRunAt) {
                <div class="tm-setting">
                  <dt>Next check</dt>
                  <dd>{{ s.checks.nextRunAt | date: 'd MMM, HH:mm' }}</dd>
                </div>
              }
              <div class="tm-setting">
                <dt>Rules file</dt>
                <dd>
                  @if (s.rules.path) {
                    <span class="font-mono break-all" [class.text-fg-muted]="s.rules.missing">{{ s.rules.path }}</span>
                    @if (s.rules.missing) {
                      <span class="mt-0.5 block text-fg-muted">Not found, labels only</span>
                    } @else if (s.rules.error) {
                      <span class="mt-0.5 flex items-center gap-1.5 text-attention" [tmTooltip]="s.rules.error">
                        <tm-icon name="triangle-alert" />Didn't load
                      </span>
                    }
                  } @else {
                    <span class="text-fg-muted">None, labels only</span>
                  }
                </dd>
              </div>
            </dl>
          </section>

          <section aria-labelledby="settings-containers">
            <h3 id="settings-containers" class="mb-1 text-xs font-medium tracking-wide text-fg-subtle uppercase">Containers</h3>
            <dl>
              <div class="tm-setting">
                <dt>{{ s.source.kind === 'snapshot' ? 'Snapshot' : 'Docker' }}</dt>
                <dd class="font-mono break-all">{{ s.source.location }}</dd>
              </div>
              <div class="tm-setting">
                <dt>App icons</dt>
                <dd>{{ s.icons ? 'On' : 'Off' }}</dd>
              </div>
            </dl>
          </section>

          <p class="text-sm text-fg-muted">
            Settings come from environment variables, so they're read-only here. The README lists them all.
          </p>

          <footer class="flex flex-wrap items-center justify-between gap-3 border-t border-line pt-4">
            <a tmButton variant="ghost" size="sm" icon="palette" routerLink="/styleguide" (click)="open.set(false)">Styleguide</a>
            <span class="text-sm text-fg-muted" data-testid="settings-version">
              Tidemark <span class="font-mono text-fg">{{ s.version }}</span>
            </span>
          </footer>
        </div>
      } @else if (settings.error()) {
        <tm-callout tone="attention" heading="Couldn't load settings">
          <p>Tidemark didn't answer. Close this and try again.</p>
        </tm-callout>
      } @else {
        <div class="flex flex-col gap-4" aria-busy="true">
          @for (i of [1, 2, 3, 4, 5]; track i) {
            <tm-skeleton height="1.25rem" />
          }
        </div>
      }
    </tm-dialog>
  `,
})
export class SettingsDialog {
  readonly open = model(false);

  /** Requested only while open; reopening asks again (the server caches GitHub's answer for a minute). */
  protected readonly settings = httpResource<Settings>(() => (this.open() ? '/api/settings' : undefined));
}
