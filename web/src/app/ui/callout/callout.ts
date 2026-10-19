import { Component, computed, input } from '@angular/core';
import { IconName, TmIcon } from '../icon/icon';

export type CalloutTone = 'info' | 'attention' | 'breaking';

const TONES: Record<CalloutTone, { icon: IconName; box: string; iconClass: string }> = {
  info: { icon: 'info', box: 'border-line bg-surface', iconClass: 'text-accent-text' },
  attention: { icon: 'triangle-alert', box: 'border-attention-line bg-attention-soft', iconClass: 'text-attention' },
  breaking: { icon: 'octagon-alert', box: 'border-breaking-line bg-breaking-soft', iconClass: 'text-breaking' },
};

/**
 * A short message in context: why something is missing, what to do next.
 *
 *   <tm-callout tone="attention" heading="GitHub rate limit reached">Add a token…</tm-callout>
 */
@Component({
  selector: 'tm-callout',
  imports: [TmIcon],
  templateUrl: './callout.html',
  host: {
    '[class]': "'flex items-start gap-2.5 rounded-md border px-3 py-2.5 ' + tone().box",
    '[attr.role]': "kind() === 'breaking' ? 'alert' : null",
  },
})
export class TmCallout {
  readonly kind = input<CalloutTone>('info', { alias: 'tone' });
  readonly heading = input<string>();
  protected readonly tone = computed(() => TONES[this.kind()]);
}
