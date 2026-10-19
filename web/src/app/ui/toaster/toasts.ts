import { Injectable, signal } from '@angular/core';

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
