import { booleanAttribute, Component, computed, input } from '@angular/core';
import { IconName, TmIcon } from '../icon/icon';

export type BadgeTone = 'neutral' | 'accent' | 'ok' | 'attention' | 'breaking' | 'unknown';

const TONES: Record<BadgeTone, string> = {
  neutral: 'border-line text-fg-muted',
  accent: 'border-transparent bg-accent-soft text-accent-text',
  ok: 'border-transparent text-ok',
  attention: 'border-attention-line bg-attention-soft text-attention',
  breaking: 'border-breaking-line bg-breaking-soft text-breaking',
  unknown: 'border-dashed border-line-strong text-fg-muted',
};

/** Small label. Prefer the status components (tm-level, tm-update-class) for update info. */
@Component({
  selector: 'tm-badge',
  imports: [TmIcon],
  templateUrl: './badge.html',
  host: { '[class]': 'classes()' },
})
export class TmBadge {
  readonly tone = input<BadgeTone>('neutral');
  readonly icon = input<IconName>();
  readonly mono = input(false, { transform: booleanAttribute });

  protected readonly classes = computed(
    () =>
      'inline-flex h-5 items-center gap-1 rounded-sm border px-1.5 text-xs font-medium whitespace-nowrap ' +
      TONES[this.tone()] +
      (this.mono() ? ' font-mono' : ''),
  );
}
