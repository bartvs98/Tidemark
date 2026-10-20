import { afterNextRender, Component, computed, effect, ElementRef, inject, Injector, input, linkedSignal, signal, viewChildren } from '@angular/core';
import { DatePipe } from '@angular/common';
import { HttpErrorResponse, httpResource } from '@angular/common/http';
import { Title } from '@angular/platform-browser';
import { RouterLink } from '@angular/router';
import { TM_TABS, TmAppIcon, TmBadge, TmButton, TmCallout, TmCard, TmClamp, TmEmptyState, TmIcon, TmLevel, TmSkeleton, TmTideGauge, TmTooltip } from '../ui';
import { TmMarkdown } from '../ui/markdown/markdown';
import { hasResult, hasUpdate, imageName, isIgnored, Reason, ruleOrigin } from '../dashboard/containers';
import { ContainerUpdate } from '../dashboard/container-update/container-update';
import { ContainerVersion } from '../dashboard/container-version/container-version';
import { ContainerRuleProblems } from '../dashboard/rule-problems/rule-problems';
import { errorKindHint, heldText } from '../dashboard/format';
import { ContainerDetailData } from './detail-types';
import { describeRules } from './rules';
import { History } from './history/history';
import { ReviewActions } from './review-actions/review-actions';

const HOW: Record<string, string> = {
  label: 'from the tidemark.repo label',
  config: 'set in the config file',
  image: "from the image's source label",
  registry: 'from the GitHub Container Registry path',
  known: 'official image',
  guess: 'guessed from the image name',
};

/** Where a quoted reason's line sits in the release notes: the release and the index into its hits. */
interface FlagTarget {
  tag: string;
  flag: number;
}

/** How long a line jumped to stays highlighted. */
const JUMP_HIGHLIGHT_MS = 2000;

const SOURCE: Record<string, string> = {
  tag: 'image tag',
  label: 'image label',
  digest: 'image digest matched against the registry',
  env: 'image metadata (*_VERSION)',
  unknown: 'unknown',
};

@Component({
  selector: 'tm-container-detail',
  imports: [
    DatePipe,
    RouterLink,
    ContainerRuleProblems,
    ContainerUpdate,
    ContainerVersion,
    History,
    ReviewActions,
    TmAppIcon,
    TmBadge,
    TmButton,
    TmCallout,
    TmCard,
    TmClamp,
    TmEmptyState,
    TmIcon,
    TmLevel,
    TmMarkdown,
    TmSkeleton,
    TmTideGauge,
    TmTooltip,
    ...TM_TABS,
  ],
  templateUrl: './detail.html',
})
export class ContainerDetail {
  /** From the route (containers/:name) via withComponentInputBinding(). */
  readonly name = input.required<string>();

  protected readonly detail = httpResource<ContainerDetailData>(() => `/api/containers/${encodeURIComponent(this.name())}`);

  // value() throws in the error state, so always go through hasValue(). A failed reload
  // (after a review, say) keeps the last good data for the same container, with a callout.
  protected readonly data = linkedSignal<{ name: string; data?: ContainerDetailData }, ContainerDetailData | undefined>({
    source: () => ({ name: this.name(), data: this.detail.hasValue() ? this.detail.value() : undefined }),
    computation: (next, previous) => next.data ?? (previous?.source.name === next.name ? previous.value : undefined),
  });
  protected readonly changelog = computed(() => this.data()?.changelog);
  protected readonly update = computed(() => this.data()?.container.update);
  /** The last successful result; kept when a later refresh failed, absent if no check ever succeeded. */
  protected readonly result = computed(() => {
    const u = this.update();
    return hasResult(u) ? u : undefined;
  });
  /** Why it's flagged; quotes whose line is in the loaded notes get a target to jump to. */
  protected readonly reasons = computed<(Reason & { target?: FlagTarget })[]>(() => {
    const releases = this.changelog()?.releases ?? [];
    return (this.result()?.reasons ?? []).map((r) => {
      const release = r.tag && r.line ? releases.find((x) => x.tag === r.tag) : undefined;
      const flag = release?.body.trim() ? release.hits.findIndex((h) => h.line === r.line && h.level !== 'none') : -1;
      return flag >= 0 ? { ...r, target: { tag: r.tag!, flag } } : r;
    });
  });
  /** The selected tab; a jump switches back to Changes. */
  protected readonly tab = signal('changes');
  /** Advice for the failed check, by kind; empty when unclassified. */
  protected readonly failureHint = computed(() => errorKindHint(this.update()?.errorKind));
  protected readonly rules = computed(() => this.data()?.container.rules);
  protected readonly ruleLines = computed(() => describeRules(this.rules()));
  protected readonly ignored = computed(() => {
    const c = this.data()?.container;
    if (!c || !isIgnored(c)) return undefined;
    return { origin: ruleOrigin(c.rules!, 'ignore') };
  });
  protected readonly held = computed(() => {
    const c = this.data()?.container;
    return c?.update?.held ? heldText(c.update, c) : undefined;
  });
  /** Releases between running and latest, for the header's tide gauge. Absent when there's nothing to count. */
  protected readonly behind = computed(() => {
    const c = this.data()?.container;
    const cl = this.changelog();
    const n = cl?.releases?.length ?? 0;
    if (!c || !hasUpdate(c) || !c.update?.running || !n) return undefined;
    const partial = !!cl?.partial;
    const text = `${n}${partial ? '+' : ''} ${n === 1 && !partial ? 'release' : 'releases'} behind`;
    return { text, level: n === 0 ? 1 : Math.max(0.15, 1 - n / 12) };
  });
  /** The header gets the animated severity frame only for these levels. */
  protected readonly frameLevel = computed(() => {
    const level = this.ignored() ? undefined : this.result()?.level;
    return level === 'breaking' || level === 'attention' ? level : null;
  });
  /** Review actions only make sense for an update that was found. */
  protected readonly canReview = computed(() => {
    const c = this.data()?.container;
    return !!c && hasUpdate(c);
  });
  /** Reviewed or snoozed: the frame keeps its colour but stops circling (it's an alert signal). */
  protected readonly frameStill = computed(() => this.canReview() && !!this.data()?.container.review);
  /** Image changes, newest first. */
  protected readonly history = computed(() => this.data()?.history ?? []);
  protected readonly githubRepo = computed(() => {
    const url = this.changelog()?.repo.url;
    return url?.startsWith('https://github.com/') ? url : undefined;
  });
  protected readonly notFound = computed(() => {
    const err = this.detail.error();
    return err instanceof HttpErrorResponse && err.status === 404;
  });
  protected readonly errorMessage = computed(() => {
    const err = this.detail.error();
    if (err instanceof HttpErrorResponse) return (err.error as { error?: string } | null)?.error ?? err.message;
    return err ? String(err) : '';
  });

  protected readonly how = HOW;
  protected readonly sourceLabel = SOURCE;
  protected readonly imageName = imageName;

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);
  private readonly clamps = viewChildren(TmClamp);

  constructor() {
    const title = inject(Title);
    effect(() => title.setTitle(`${this.name()} · Tidemark`));
  }

  /** Shows a quoted line in its release notes: open the release and its clamp, scroll there, focus and highlight it. */
  protected jumpTo(t: FlagTarget): void {
    this.tab.set('changes');
    // The Changes panel may take a render or two to appear; the opened clamp lands after the next one.
    const find = (tries: number) =>
      afterNextRender(
        () => {
          const details = [...this.host.nativeElement.querySelectorAll<HTMLDetailsElement>('details[data-release]')].find(
            (d) => d.dataset['release'] === t.tag,
          );
          const line = details?.querySelector<HTMLElement>(`[data-flag~="${t.flag}"]`);
          if (!details || !line) {
            if (tries > 1) find(tries - 1);
            return;
          }
          details.open = true;
          for (const c of this.clamps()) c.reveal(line);
          afterNextRender(() => this.land(line), { injector: this.injector });
        },
        { injector: this.injector },
      );
    find(3);
  }

  private land(line: HTMLElement): void {
    const still = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
    line.scrollIntoView({ block: 'center', behavior: still ? 'auto' : 'smooth' });
    line.tabIndex = -1;
    line.focus({ preventScroll: true });
    line.classList.add('tm-flag-jumped');
    setTimeout(() => line.classList.remove('tm-flag-jumped'), JUMP_HIGHLIGHT_MS);
  }

  protected labelEntries(labels: Record<string, string>): [string, string][] {
    return Object.entries(labels).sort(([a], [b]) => a.localeCompare(b));
  }
}
