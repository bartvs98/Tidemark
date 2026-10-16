import { Component, inject, Injectable, signal } from '@angular/core';
import { TmButton } from './button';
import { IconName, TmIcon } from './icon';

export type ToastTone = 'info' | 'ok' | 'attention' | 'breaking';

export interface Toast {
  id: number;
  tone: ToastTone;
  message: string;
}

/**
 * Show short, transient messages. Put one <tm-toaster /> in the app shell.
 *
 *   inject(Toasts).show('Check finished · 3 updates', 'ok');
 */
@Injectable({ providedIn: 'root' })
export class Toasts {
  private nextId = 1;
  readonly items = signal<Toast[]>([]);

  show(message: string, tone: ToastTone = 'info', durationMs = 5000): number {
    const id = this.nextId++;
    this.items.update((list) => [...list, { id, tone, message }]);
    if (durationMs > 0) {
      setTimeout(() => this.dismiss(id), durationMs);
    }
    return id;
  }

  dismiss(id: number): void {
    this.items.update((list) => list.filter((t) => t.id !== id));
  }
}

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
  template: `
    @for (toast of toasts.items(); track toast.id) {
      <div
        class="pointer-events-auto flex w-80 items-start gap-2.5 rounded-lg border border-line bg-raised p-3 shadow-overlay"
        [attr.role]="toast.tone === 'breaking' ? 'alert' : 'status'"
      >
        <tm-icon [name]="icon[toast.tone]" [class]="'mt-0.5 ' + toneClass[toast.tone]" />
        <p class="flex-1">{{ toast.message }}</p>
        <button tmButton variant="ghost" size="sm" square icon="x" aria-label="Dismiss" (click)="toasts.dismiss(toast.id)"></button>
      </div>
    }
  `,
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
