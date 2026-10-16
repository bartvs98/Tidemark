import { booleanAttribute, Component, computed, input } from '@angular/core';
import { IconName, TmIcon } from './icon';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
export type ButtonSize = 'sm' | 'md';

const BASE =
  'inline-flex items-center justify-center gap-1.5 rounded-md font-medium whitespace-nowrap select-none ' +
  'no-underline hover:no-underline transition-colors cursor-pointer ' +
  'disabled:pointer-events-none disabled:opacity-50 aria-disabled:pointer-events-none aria-disabled:opacity-50';

const VARIANTS: Record<ButtonVariant, string> = {
  primary: 'bg-accent text-on-accent hover:bg-accent-hover',
  secondary: 'border border-line-strong bg-surface text-fg hover:bg-hover',
  ghost: 'text-fg-muted hover:bg-hover hover:text-fg',
  danger: 'bg-breaking text-on-accent hover:opacity-90',
};

const SIZES: Record<ButtonSize, { normal: string; square: string }> = {
  sm: { normal: 'h-7 px-2.5 text-sm', square: 'size-7 text-sm' },
  md: { normal: 'h-9 px-3.5 text-base', square: 'size-9 text-base' },
};

/**
 * Styles a native <button> or <a>, so all native behaviour (type, disabled, routerLink, href) keeps working.
 *
 *   <button tmButton variant="primary" icon="refresh-cw" (click)="check()">Check now</button>
 *   <a tmButton variant="ghost" href="…" icon="external-link">Release notes</a>
 *   <button tmButton square variant="ghost" icon="x" aria-label="Dismiss"></button>
 */
@Component({
  selector: 'button[tmButton], a[tmButton]',
  imports: [TmIcon],
  template: `
    @if (loading()) {
      <tm-icon name="loader" class="animate-spin motion-reduce:animate-none" />
    } @else if (icon(); as icon) {
      <tm-icon [name]="icon" />
    }
    <ng-content />
  `,
  host: {
    '[class]': 'classes()',
    '[attr.aria-busy]': 'loading() || null',
    '[attr.aria-disabled]': 'loading() || null',
  },
})
export class TmButton {
  readonly variant = input<ButtonVariant>('secondary');
  readonly size = input<ButtonSize>('md');
  readonly icon = input<IconName>();
  readonly square = input(false, { transform: booleanAttribute });
  readonly loading = input(false, { transform: booleanAttribute });

  protected readonly classes = computed(() => {
    const size = SIZES[this.size()];
    return [BASE, VARIANTS[this.variant()], this.square() ? size.square : size.normal].join(' ');
  });
}
