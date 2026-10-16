import { Component, computed, input } from '@angular/core';
import { TmBadge, TmSkeleton, TmUpdateClass, TmVersionDiff, UpdateClass } from '../ui';
import { ContainerSummary, UpdateSource } from './containers';

const SOURCE_HINT: Record<UpdateSource, string> = {
  tag: '',
  label: 'detected from image label',
  digest: 'detected by image digest',
  env: 'detected from image metadata',
  unknown: 'running version unknown',
};

/** Running → latest version, how the running version was found, and the in-major option. */
@Component({
  selector: 'tm-container-version',
  imports: [TmVersionDiff, TmSkeleton],
  template: `
    @if (update(); as u) {
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

  protected readonly update = computed(() => this.container().update);
  protected readonly showLatest = computed(() => {
    const u = this.update();
    return !!u?.latest && u.latest !== u.running && u.class !== 'none';
  });
  protected readonly hint = computed(() => {
    const u = this.update();
    return u && !u.error ? SOURCE_HINT[u.source] : '';
  });
}

/** The update class badge, "check failed", or a placeholder while checking. */
@Component({
  selector: 'tm-container-update',
  imports: [TmBadge, TmSkeleton, TmUpdateClass],
  template: `
    @if (update(); as u) {
      @if (u.error) {
        <!-- relative z-10: stays hoverable above the row's stretched link, so the reason shows -->
        <tm-badge tone="unknown" icon="circle-help" [attr.title]="u.error" class="relative z-10">check failed</tm-badge>
      } @else {
        <tm-update-class [kind]="kind()" />
      }
    } @else if (checking()) {
      <tm-skeleton width="4.5rem" height="1.25rem" />
    }
  `,
  host: { class: 'inline-flex' },
})
export class ContainerUpdate {
  readonly container = input.required<ContainerSummary>();
  readonly checking = input(false);

  protected readonly update = computed(() => this.container().update);
  protected readonly kind = computed(() => (this.update()?.class ?? 'none') as UpdateClass);
}
