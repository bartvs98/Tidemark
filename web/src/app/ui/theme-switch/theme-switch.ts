import { Component, inject } from '@angular/core';
import { SegmentOption, TmSegmented } from '../segmented/segmented';
import { Theme, ThemePreference } from './theme';

const OPTIONS: SegmentOption<ThemePreference>[] = [
  { value: 'system', icon: 'monitor', label: 'System theme' },
  { value: 'light', icon: 'sun', label: 'Light theme' },
  { value: 'dark', icon: 'moon', label: 'Dark theme' },
];

@Component({
  selector: 'tm-theme-switch',
  imports: [TmSegmented],
  templateUrl: './theme-switch.html',
})
export class TmThemeSwitch {
  protected readonly theme = inject(Theme);
  protected readonly options = OPTIONS;
}
