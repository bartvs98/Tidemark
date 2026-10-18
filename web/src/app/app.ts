import { httpResource } from '@angular/common/http';
import { Component, signal } from '@angular/core';
import { RouterLink, RouterOutlet } from '@angular/router';
import { SettingsDialog } from './settings/settings-dialog';
import { TmButton, TmThemeSwitch, TmTideBackdrop, TmToaster, TmTooltip } from './ui';

@Component({
  selector: 'tm-root',
  imports: [RouterOutlet, RouterLink, SettingsDialog, TmButton, TmThemeSwitch, TmTideBackdrop, TmToaster, TmTooltip],
  templateUrl: './app.html',
})
export class App {
  protected readonly settingsOpen = signal(false);
  /** Tidemark's own version for the footer (GET /api/health). */
  protected readonly health = httpResource<{ status: string; version: string }>(() => '/api/health');
}
