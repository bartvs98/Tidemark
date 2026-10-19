import { Component, computed, effect, inject, input } from '@angular/core';
import { DatePipe } from '@angular/common';
import { HttpErrorResponse, httpResource } from '@angular/common/http';
import { Title } from '@angular/platform-browser';
import { RouterLink } from '@angular/router';
import { TM_TABS, TmAppIcon, TmBadge, TmButton, TmCallout, TmCard, TmClamp, TmEmptyState, TmIcon, TmLevel, TmSkeleton, TmTideGauge, TmTooltip } from '../ui';
import { TmMarkdown } from '../ui/markdown/markdown';
import { hasResult, hasUpdate, imageName, isIgnored, ruleOrigin } from '../dashboard/containers';
import { ContainerUpdate } from '../dashboard/container-update/container-update';
import { ContainerVersion } from '../dashboard/container-version/container-version';
import { ContainerRuleProblems } from '../dashboard/rule-problems/rule-problems';
import { errorKindHint, heldText } from '../dashboard/format';
import { ContainerDetailData } from './detail-types';
import { describeRules } from './rules';

const HOW: Record<string, string> = {
  label: 'from the tidemark.repo label',
  config: 'set in the config file',
  image: "from the image's source label",
  registry: 'from the GitHub Container Registry path',
  known: 'official image',
  guess: 'guessed from the image name',
};

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

  // value() throws in the error state, so always go through hasValue().
  protected readonly data = computed(() => (this.detail.hasValue() ? this.detail.value() : undefined));
  protected readonly changelog = computed(() => this.data()?.changelog);
  protected readonly update = computed(() => this.data()?.container.update);
  /** The last successful result; kept when a later refresh failed, absent if no check ever succeeded. */
  protected readonly result = computed(() => {
    const u = this.update();
    return hasResult(u) ? u : undefined;
  });
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

  constructor() {
    const title = inject(Title);
    effect(() => title.setTitle(`${this.name()} · Tidemark`));
  }

  protected labelEntries(labels: Record<string, string>): [string, string][] {
    return Object.entries(labels).sort(([a], [b]) => a.localeCompare(b));
  }
}
