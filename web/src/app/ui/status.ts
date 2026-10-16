import { Component, input } from '@angular/core';
import { TmBadge } from './badge';
import { TmIcon } from './icon';

/*
 * Tidemark's status language (docs/DESIGN-SYSTEM.md → Status):
 *   - Update class (how big the jump is) is always NEUTRAL.
 *   - Level (how risky it is) is the ONLY thing that gets colour.
 *   - Every status has an icon and a word, never colour alone.
 */

export type UpdateClass = 'none' | 'patch' | 'minor' | 'major' | 'digest';
export type Level = 'none' | 'attention' | 'breaking' | 'unknown';

/** "Up to date" check, or a neutral badge with a tide dot: patch / minor / major / digest. */
@Component({
  selector: 'tm-update-class',
  imports: [TmBadge, TmIcon],
  template: `
    @if (kind() === 'none') {
      <span class="inline-flex items-center gap-1 text-sm text-fg-muted">
        <tm-icon name="check" [size]="14" class="text-ok" />Up to date
      </span>
    } @else {
      <tm-badge>
        <span class="size-1.5 rounded-full bg-accent" aria-hidden="true"></span>
        {{ kind() === 'digest' ? 'new image' : kind() }}
      </tm-badge>
    }
  `,
  host: { class: 'inline-flex' },
})
export class TmUpdateClass {
  readonly kind = input.required<UpdateClass>();
}

/** Risk badge. Renders nothing for level "none", so calm rows stay calm. */
@Component({
  selector: 'tm-level',
  imports: [TmBadge],
  template: `
    @switch (level()) {
      @case ('attention') {
        <tm-badge tone="attention" icon="triangle-alert">Attention</tm-badge>
      }
      @case ('breaking') {
        <tm-badge tone="breaking" icon="octagon-alert">Breaking</tm-badge>
      }
      @case ('unknown') {
        <tm-badge tone="unknown" icon="circle-help">Unknown</tm-badge>
      }
    }
  `,
  host: { class: 'inline-flex empty:hidden' },
})
export class TmLevel {
  readonly level = input.required<Level>();
}

/** `1.4.2 → 1.6.0` in mono. Without `to` it shows just the running version. */
@Component({
  selector: 'tm-version-diff',
  template: `
    <span class="text-fg-muted">{{ from() }}</span>
    @if (to(); as to) {
      <span class="text-fg-subtle" aria-label="to">→</span>
      <span class="font-medium text-fg">{{ to }}</span>
    }
  `,
  host: { class: 'inline-flex items-baseline gap-1.5 font-mono text-sm tabular-nums' },
})
export class TmVersionDiff {
  readonly from = input.required<string>();
  readonly to = input<string | null>();
}
