import { Component, inject } from '@angular/core';
import { TmButton } from '../button/button';
import { IconName, TmIcon } from '../icon/icon';
import { Toasts, ToastTone } from './toasts';

const TONE_ICON: Record<ToastTone, IconName> = {
  info: 'info',
  ok: 'circle-check',
  attention: 'triangle-alert',
  breaking: 'octagon-alert',
};

const TONE_CLASS: Record<ToastTone, string> = {
  info: 'text-accent-text',
  ok: 'text-ok',
  attention: 'text-attention',
  breaking: 'text-breaking',
};

@Component({
  selector: 'tm-toaster',
  imports: [TmIcon, TmButton],
  templateUrl: './toaster.html',
  host: {
    class: 'pointer-events-none fixed right-4 bottom-4 z-50 flex flex-col items-end gap-2',
    'aria-live': 'polite',
  },
})
export class TmToaster {
  protected readonly toasts = inject(Toasts);
  protected readonly icon = TONE_ICON;
  protected readonly toneClass = TONE_CLASS;
}
