import { Component, computed, inject, input, LOCALE_ID } from '@angular/core';
import { formatDate } from '@angular/common';
import { TmBadge, TmIcon, TmLevel, TmSkeleton, TmTooltip, TmUpdateClass, TmVersionDiff, UpdateClass } from '../ui';
import { ContainerSummary, hasResult, isIgnored, refreshFailed, ruleOrigin, UpdateInfo, UpdateSource } from './containers';

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
  imports: [TmVersionDiff, TmSkeleton, TmTooltip],
  template: `
    @if (ignored()) {
      <!-- Ignored: not checked, so there's no version change to show. -->
      <span class="font-mono text-sm text-fg-muted">{{ container().image.tag || '—' }}</span>
    } @else if (update(); as u) {
      @if (u.channel) {
        <!-- A channel build is usually ahead of the newest stable release, so no arrow between them. -->
        <span class="font-mono text-sm" data-testid="channel">{{ u.channel }}</span>
        <div class="truncate text-xs text-fg-subtle">{{ u.newImage ? 'new build available' : 'latest build' }}</div>
        @if (u.latest) {
          <div class="truncate text-xs text-fg-subtle">newest stable <span class="font-mono">{{ u.latest }}</span></div>
        }
      } @else if (u.running) {
        <tm-version-diff [from]="u.running" [to]="showLatest() ? u.latest : null" />
      } @else if (u.latest) {
        <span class="font-mono text-sm text-fg-muted">? <span class="text-fg-subtle">→</span> {{ u.latest }}</span>
      } @else {
        <span class="font-mono text-sm text-fg-muted">{{ container().image.tag || '—' }}</span>
      }
      @if (hint() && !u.channel) {
        <div class="truncate text-xs text-fg-subtle">{{ hint() }}</div>
      }
      @if (u.latestInMajor) {
        <div class="truncate text-xs text-fg-subtle">
          or <span class="font-mono">{{ u.latestInMajor }}</span> in your major
        </div>
      }
      @if (held(); as h) {
        <div class="truncate text-xs text-fg-subtle" [tmTooltip]="h.title" data-testid="held">
          <span class="font-mono">{{ u.held }}</span> {{ h.text }}
          @if (h.value) {
            <span class="font-mono">{{ h.value }}</span>
          }
        </div>
      }
    } @else if (checking() && !failed()) {
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
  /** The last successful result; a failed attempt without one shows just the tag. */
  protected readonly update = computed(() => {
    const u = this.container().update;
    return hasResult(u) ? u : undefined;
  });
  /** The only attempt so far failed: no skeleton, "check failed" next to it says why. */
  protected readonly failed = computed(() => !!this.container().update?.error);
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
    return u ? SOURCE_HINT[u.source] : '';
  });
}

/** When a date-time is today, just the time ("07:02"); otherwise with the date ("17 Oct, 23:00"). */
export function shortTime(iso: string, locale: string): string {
  const today = new Date(iso).toDateString() === new Date().toDateString();
  return formatDate(iso, today ? 'HH:mm' : 'd MMM, HH:mm', locale);
}

/**
 * Quiet note that the result shown is from an earlier check, because the latest refresh failed.
 * Not a risk, so no colour and no badge: icon + "refresh failed", details on hover.
 * Renders nothing unless there's a kept result and an error.
 */
@Component({
  selector: 'tm-refresh-failed',
  imports: [TmIcon, TmTooltip],
  template: `
    @if (title(); as t) {
      <!-- relative z-10: hoverable above a stretched link, so the error shows -->
      <span class="relative z-10 inline-flex items-center gap-1 text-xs whitespace-nowrap text-fg-muted" [tmTooltip]="t" data-testid="refresh-failed">
        <tm-icon name="history" [size]="12" />refresh failed
      </span>
    }
  `,
  host: { class: 'inline-flex empty:hidden' },
})
export class ContainerRefreshFailed {
  readonly container = input.required<ContainerSummary>();
  private readonly locale = inject(LOCALE_ID);

  protected readonly title = computed(() => {
    const u = this.container().update;
    if (!u || !hasResult(u) || !refreshFailed(u)) return null;
    const failed = u.failedAt ? ` at ${shortTime(u.failedAt, this.locale)}` : '';
    return `Showing the result from ${shortTime(u.checkedAt, this.locale)}. The refresh${failed} failed:\n${u.error}`;
  });
}

/** The update class and risk level badges, "check failed", or a placeholder while checking. */
@Component({
  selector: 'tm-container-update',
  imports: [ContainerRefreshFailed, TmBadge, TmIcon, TmLevel, TmSkeleton, TmTooltip, TmUpdateClass],
  template: `
    @if (ignored()) {
      <!-- relative z-10: hoverable above a stretched link, so the origin shows -->
      <span class="relative z-10 inline-flex items-center gap-1 text-sm text-fg-subtle" [tmTooltip]="ignoredTitle()" data-testid="ignored">
        <tm-icon name="eye-off" [size]="14" />Ignored
      </span>
    } @else if (result(); as u) {
      <tm-update-class [kind]="kind()" />
      @if (kind() !== 'none') {
        <!-- relative z-10: hoverable above a stretched link, so the reasons tooltip shows -->
        <tm-level [level]="u.level" [tmTooltip]="reasons()" class="relative z-10" data-testid="level" />
      }
      <!-- The result above is kept from an earlier check; this says the latest refresh failed. -->
      <tm-refresh-failed [container]="container()" />
    } @else if (error(); as err) {
      <!-- No result at all yet. relative z-10: stays hoverable above the row's stretched link, so the reason shows -->
      <tm-badge tone="unknown" icon="circle-help" [tmTooltip]="err" class="relative z-10" data-testid="check-failed">check failed</tm-badge>
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
  /** The last successful result (kept when a later refresh failed). */
  protected readonly result = computed(() => {
    const u = this.container().update;
    return hasResult(u) ? u : undefined;
  });
  protected readonly error = computed(() => this.container().update?.error);
  protected readonly kind = computed(() => (this.result()?.class ?? 'none') as UpdateClass);
  protected readonly reasons = computed(() => this.result()?.reasons?.join('\n') || null);
}

/**
 * Small neutral marker for rules that were skipped because their value was invalid.
 * Not a risk, so no colour; the problems show on hover and on the detail page.
 */
@Component({
  selector: 'tm-rule-problems',
  imports: [TmBadge, TmTooltip],
  template: `
    @if (problems().length; as n) {
      <!-- relative z-10: hoverable above a stretched link, so the problems show -->
      <tm-badge tone="unknown" icon="info" class="relative z-10" [tmTooltip]="title()" data-testid="rule-problems">
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
