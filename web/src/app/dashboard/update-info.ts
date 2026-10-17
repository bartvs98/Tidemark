import { Component, computed, input } from '@angular/core';
import { TmBadge, TmIcon, TmLevel, TmSkeleton, TmUpdateClass, TmVersionDiff, UpdateClass } from '../ui';
import { ContainerSummary, isIgnored, ruleOrigin, UpdateInfo, UpdateSource } from './containers';

const SOURCE_HINT: Record<UpdateSource, string> = {
  tag: '',
  label: 'detected from image label',
  digest: 'detected by image digest',
  env: 'detected from image metadata',
  unknown: 'running version unknown',
};

/**
 * The quiet line under the versions when a rule keeps a newer version back:
 * "held back by pin 1.4" / "held back, majors off". The version itself goes in front, in mono.
 */
export function heldText(u: UpdateInfo, c: ContainerSummary): { text: string; value?: string; title: string } {
  if (u.heldBy === 'pin') {
    const pin = c.rules?.pin;
    return {
      text: 'held back by pin',
      value: pin,
      title: pin ? `Pinned to ${pin}: only versions starting with ${pin} count.` : 'Pinned: only versions in the pin count.',
    };
  }
  if (u.heldBy === 'allow-major') {
    return { text: 'held back, majors off', title: 'Major updates are off for this container: only versions in the running major count.' };
  }
  return { text: 'held back by a rule', title: 'A rule for this container keeps this version back.' };
}

/** Running → latest version, how the running version was found, and the in-major option. */
@Component({
  selector: 'tm-container-version',
  imports: [TmVersionDiff, TmSkeleton],
  template: `
    @if (ignored()) {
      <!-- Ignored: not checked, so there's no version change to show. -->
      <span class="font-mono text-sm text-fg-muted">{{ container().image.tag || '—' }}</span>
    } @else if (update(); as u) {
      @if (u.running) {
        <tm-version-diff [from]="u.running" [to]="showLatest() ? u.latest : null" />
      } @else if (u.latest) {
        <span class="font-mono text-sm text-fg-muted">? <span class="text-fg-subtle">→</span> {{ u.latest }}</span>
      } @else {
        <span class="font-mono text-sm text-fg-muted">{{ container().image.tag || '—' }}</span>
      }
      @if (hint()) {
        <div class="truncate text-xs text-fg-subtle">{{ hint() }}</div>
      }
      @if (u.latestInMajor) {
        <div class="truncate text-xs text-fg-subtle">
          or <span class="font-mono">{{ u.latestInMajor }}</span> in your major
        </div>
      }
      @if (held(); as h) {
        <div class="truncate text-xs text-fg-subtle" [attr.title]="h.title" data-testid="held">
          <span class="font-mono">{{ u.held }}</span> {{ h.text }}
          @if (h.value) {
            <span class="font-mono">{{ h.value }}</span>
          }
        </div>
      }
    } @else if (checking()) {
      <tm-skeleton width="7rem" height="0.875rem" />
    } @else {
      <span class="font-mono text-sm text-fg-muted">{{ container().image.tag || '—' }}</span>
    }
  `,
  host: { class: 'block min-w-0' },
})
export class ContainerVersion {
  readonly container = input.required<ContainerSummary>();
  readonly checking = input(false);

  protected readonly ignored = computed(() => isIgnored(this.container()));
  protected readonly update = computed(() => this.container().update);
  protected readonly held = computed(() => {
    const u = this.update();
    return u?.held ? heldText(u, this.container()) : undefined;
  });
  protected readonly showLatest = computed(() => {
    const u = this.update();
    return !!u?.latest && u.latest !== u.running && u.class !== 'none';
  });
  protected readonly hint = computed(() => {
    const u = this.update();
    return u && !u.error ? SOURCE_HINT[u.source] : '';
  });
}

/** The update class and risk level badges, "check failed", or a placeholder while checking. */
@Component({
  selector: 'tm-container-update',
  imports: [TmBadge, TmIcon, TmLevel, TmSkeleton, TmUpdateClass],
  template: `
    @if (ignored()) {
      <!-- relative z-10: hoverable above a stretched link, so the origin shows -->
      <span class="relative z-10 inline-flex items-center gap-1 text-sm text-fg-subtle" [attr.title]="ignoredTitle()" data-testid="ignored">
        <tm-icon name="eye-off" [size]="14" />Ignored
      </span>
    } @else if (update(); as u) {
      @if (u.error) {
        <!-- relative z-10: stays hoverable above the row's stretched link, so the reason shows -->
        <tm-badge tone="unknown" icon="circle-help" [attr.title]="u.error" class="relative z-10">check failed</tm-badge>
      } @else {
        <tm-update-class [kind]="kind()" />
        @if (kind() !== 'none') {
          <!-- relative z-10: hoverable above a stretched link, so the reasons tooltip shows -->
          <tm-level [level]="u.level" [attr.title]="reasons()" class="relative z-10" data-testid="level" />
        }
      }
    } @else if (checking()) {
      <tm-skeleton width="4.5rem" height="1.25rem" />
    }
  `,
  host: { class: 'inline-flex flex-wrap items-center gap-1.5' },
})
export class ContainerUpdate {
  readonly container = input.required<ContainerSummary>();
  readonly checking = input(false);

  protected readonly ignored = computed(() => isIgnored(this.container()));
  protected readonly ignoredTitle = computed(() => {
    const rules = this.container().rules;
    const origin = rules && ruleOrigin(rules, 'ignore');
    const by = origin?.label ? ` by the ${origin.label} label` : origin ? ' in the config file' : '';
    return `Not checked for updates: ignored${by}.`;
  });
  protected readonly update = computed(() => this.container().update);
  protected readonly kind = computed(() => (this.update()?.class ?? 'none') as UpdateClass);
  protected readonly reasons = computed(() => this.update()?.reasons?.join('\n') || null);
}

/**
 * Small neutral marker for rules that were skipped because their value was invalid.
 * Not a risk, so no colour; the problems show on hover and on the detail page.
 */
@Component({
  selector: 'tm-rule-problems',
  imports: [TmBadge],
  template: `
    @if (problems().length; as n) {
      <!-- relative z-10: hoverable above a stretched link, so the problems show -->
      <tm-badge tone="unknown" icon="info" class="relative z-10" [attr.title]="title()" data-testid="rule-problems">
        {{ n === 1 ? 'rule skipped' : n + ' rules skipped' }}
      </tm-badge>
    }
  `,
  host: { class: 'inline-flex empty:hidden' },
})
export class ContainerRuleProblems {
  readonly container = input.required<ContainerSummary>();
  protected readonly problems = computed(() => this.container().rules?.problems ?? []);
  protected readonly title = computed(() => this.problems().join('\n'));
}
