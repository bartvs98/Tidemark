import { Component, computed, effect, inject, linkedSignal, LOCALE_ID, signal } from '@angular/core';
import { HttpErrorResponse, httpResource } from '@angular/common/http';
import { RouterLink } from '@angular/router';
import {
  SegmentOption,
  TmButton,
  TmCallout,
  TmCard,
  TmIcon,
  TmSection,
  TmSegmented,
  TmTooltip,
} from '../ui';
import { TmBarChart, TmBarList, TmStat, TmTimeChart } from '../ui/charts';
import { ErrorKind } from '../dashboard/containers';
import { errorKindLabel } from '../dashboard/format';
import { formatDuration, formatTokens } from '../detail/ai-format';
import { shortDate, timeAgo } from '../detail/dates';
import { behindFor, capitalize, count, dayLabel, longDuration, monthLabel, percent } from './stats-format';
import { StatsSkeleton } from './stats-skeleton/stats-skeleton';
import { AppliedUpdate, Stats as StatsData, StatsRange } from './stats-types';

const RANGE_KEY = 'tidemark.statsRange';
const RANGES: SegmentOption<StatsRange>[] = [
  { value: '30d', label: '30 days' },
  { value: '90d', label: '90 days' },
  { value: '1y', label: '1 year' },
  { value: 'all', label: 'All' },
];
/** Rows of the "Behind" table shown before "Show all". */
const TOP_BEHIND = 5;

/** The stats page: what Tidemark has seen over time (GET /api/stats). */
@Component({
  selector: 'tm-stats',
  imports: [
    RouterLink,
    StatsSkeleton,
    TmBarChart,
    TmBarList,
    TmButton,
    TmCallout,
    TmCard,
    TmIcon,
    TmSection,
    TmSegmented,
    TmStat,
    TmTimeChart,
    TmTooltip,
  ],
  templateUrl: './stats.html',
  host: { class: 'block' },
})
export class Stats {
  private readonly locale = inject(LOCALE_ID);

  protected readonly ranges = RANGES;
  protected readonly range = signal<StatsRange>(readRange());
  protected readonly resource = httpResource<StatsData>(() => `/api/stats?range=${this.range()}`);

  // value() throws in the error state, so go through hasValue(). The last good response stays
  // while a new range loads or a refresh fails.
  protected readonly data = linkedSignal<StatsData | undefined, StatsData | undefined>({
    source: () => (this.resource.hasValue() ? this.resource.value() : undefined),
    computation: (stats, previous) => stats ?? previous?.value,
  });
  /** The range the shown data belongs to; the control moves first, so a spinner-free fade tells it's loading. */
  protected readonly loading = computed(() => this.resource.isLoading());

  protected readonly errorMessage = computed(() => {
    const err = this.resource.error();
    if (err instanceof HttpErrorResponse) {
      return (err.error as { error?: string } | null)?.error ?? `The API answered ${err.status} ${err.statusText}.`;
    }
    return err ? String(err) : '';
  });

  private readonly now = computed(() => {
    const generated = this.data()?.generatedAt;
    return generated ? new Date(generated).getTime() : Date.now();
  });


  // ── Behind & updates ──
  protected readonly behindStats = computed(() => {
    const d = this.data();
    if (!d) return undefined;
    const last = d.behind.tide.at(-1);
    const behindTotal = d.behind.containers.reduce((sum, c) => sum + c.releasesBehind, 0);
    const median = d.behind.timeToUpdate.medianMs;
    return {
      upToDate: last && last.checked ? percent(last.upToDate / last.checked) : '—',
      upToDateHint: last && last.checked ? `${last.upToDate} of ${count(last.checked, 'container')}` : 'No check yet',
      applied: d.behind.applied.total,
      median: median === undefined ? '—' : longDuration(median),
      medianHint:
        d.behind.timeToUpdate.count > 0
          ? `from release to update, ${count(d.behind.timeToUpdate.count, 'update')}`
          : 'Needs release dates',
      releasesBehind: behindTotal,
      releasesHint: `across ${count(d.behind.containers.length, 'container')}`,
    };
  });
  protected readonly tidePoints = computed(
    () =>
      // A run that checked nothing (every check failed) has no ratio to plot.
      this.data()?.behind.tide.filter((p) => p.checked > 0).map((p) => ({
        at: p.at,
        value: p.upToDate / p.checked,
        note: `${p.upToDate} of ${p.checked} up to date`,
      })) ?? [],
  );
  /** Update debt per run: releases between running and latest over all updates (stored notes). */
  protected readonly debtPoints = computed(
    () =>
      this.data()?.behind.tide.filter((p) => p.checked > 0).map((p) => ({
        at: p.at,
        value: p.releasesBehind,
        note: `${count(p.updates, 'open update')}`,
      })) ?? [],
  );
  protected readonly weekBars = computed(
    () =>
      this.data()?.behind.applied.perWeek.map((w) => ({
        label: dayLabel(w.week, this.locale),
        segments: [{ value: w.count, name: 'Updates' }],
      })) ?? [],
  );
  protected readonly stackItems = computed(
    () =>
      this.data()?.behind.applied.perStack.map((s) => ({ label: s.stack || 'Ungrouped', value: s.count })) ?? [],
  );
  /** Current updates by how long ago the first newer release came out (needs release notes); top 5. */
  protected readonly behindItems = computed(() =>
    (this.data()?.behind.containers ?? [])
      .filter((c) => c.behindSince)
      .slice(0, TOP_BEHIND)
      .map((c) => ({
        label: c.name,
        value: this.now() - new Date(c.behindSince!).getTime(),
        display: behindFor(c.behindSince, this.now()),
        hint: count(c.releasesBehind, 'release'),
        href: `/containers/${encodeURIComponent(c.name)}`,
      })),
  );

  // ── Upstream ──
  protected readonly jumpItems = computed(
    () =>
      this.data()?.upstream.biggestJumps.map((j) => ({
        label: `${j.from} → ${j.to}`,
        mono: true,
        value: j.releases,
        display: count(j.releases, 'release'),
        hint: j.name,
        href: `/containers/${encodeURIComponent(j.name)}`,
      })) ?? [],
  );
  protected readonly monthBars = computed(() => {
    const months = this.data()?.upstream.perMonth ?? [];
    const withYear = months.length > 12;
    return months.map((m) => ({
      label: monthLabel(m.month, this.locale, withYear),
      segments: [
        { value: Math.max(0, m.releases - m.breaking - m.attention), tone: 'accent' as const, name: 'Calm' },
        { value: m.attention, tone: 'attention' as const, name: 'Attention' },
        { value: m.breaking, tone: 'breaking' as const, name: 'Breaking' },
      ],
      faded: m.partial,
      note: m.partial ? 'Leaves out repos whose notes start later' : undefined,
    }));
  });
  protected readonly partialMonths = computed(() => this.data()?.upstream.perMonth.some((m) => m.partial) ?? false);

  // ── Check health ──
  protected readonly healthStats = computed(() => {
    const h = this.data()?.health;
    if (!h) return undefined;
    const { runs, github, requestTotals } = h;
    let quota: string;
    let quotaHint: string;
    if (github.remaining !== undefined && github.limit !== undefined) {
      quota = `${github.remaining.toLocaleString('en-US')} of ${github.limit.toLocaleString('en-US')}`;
      quotaHint = github.resetAt ? `resets ${timeAgoOrIn(github.resetAt, this.now())}` : github.token ? 'with a token' : 'without a token';
    } else if (github.token) {
      quota = 'With a token';
      quotaHint = '5,000 requests an hour';
    } else {
      quota = '60/h';
      quotaHint = 'per IP, without a token';
    }
    return {
      runs: runs.total,
      runsHint: `${runs.byTrigger.schedule} scheduled · ${runs.byTrigger.manual} manual · ${runs.byTrigger.startup} at startup`,
      failed: runs.failed,
      median: runs.medianMs === undefined ? '—' : formatDuration(runs.medianMs),
      slowest: runs.slowestMs === undefined ? undefined : `slowest ${formatDuration(runs.slowestMs)}`,
      quota,
      quotaHint,
      limited: requestTotals.limited,
      requests: requestTotals.registry + requestTotals.github + requestTotals.gitlab,
    };
  });
  protected readonly durationPoints = computed(
    () =>
      this.data()?.health.durations.map((p) => ({
        at: p.at,
        value: p.ms,
        note: `${count(p.images, 'image')}${p.failed ? ' · failed' : ''}`,
      })) ?? [],
  );
  protected readonly requestBars = computed(() => {
    const h = this.data()?.health;
    if (!h) return [];
    return h.requests.map((r) => ({
      label: dayLabel(r.day, this.locale),
      segments: [
        { value: r.registry, tone: 'accent' as const, name: 'Registry' },
        // One segment for both forges: two greys couldn't be told apart.
        { value: r.github + r.gitlab, tone: 'muted' as const, name: 'Release notes' },
      ],
      note: r.limited ? `${r.limited} rate limited` : undefined,
    }));
  });
  protected readonly requestsTitle = computed(() =>
    this.data()?.health.requestsPer === 'week' ? 'Requests per week' : 'Requests per day',
  );
  protected readonly failureLabel = failureLabel;
  protected readonly failureItems = computed(
    () =>
      this.data()?.health.failuresByKind.map((f) => ({
        label: failureLabel(f.kind),
        value: f.count,
      })) ?? [],
  );

  // ── AI usage ──
  protected readonly aiStats = computed(() => {
    const ai = this.data()?.ai;
    if (!ai) return undefined;
    const inTokens = ai.byModel.reduce((sum, m) => sum + m.inputTokens, 0);
    const outTokens = ai.byModel.reduce((sum, m) => sum + m.outputTokens, 0);
    return {
      calls: ai.calls,
      callsHint: `${ai.auto} automatic · ${ai.manual} manual${ai.extended ? ` · ${ai.extended} extended` : ''}`,
      tokensIn: formatTokens(inTokens),
      tokensOut: formatTokens(outTokens),
      failed: ai.failed,
      median: ai.medianMs === undefined ? '—' : formatDuration(ai.medianMs),
    };
  });
  protected readonly tokenBars = computed(() => {
    const months = this.data()?.ai?.months ?? [];
    const withYear = months.length > 12;
    return months.map((m) => ({
      label: monthLabel(m.month, this.locale, withYear),
      segments: [
        { value: m.inputTokens, tone: 'accent' as const, name: 'In' },
        { value: m.outputTokens, tone: 'muted' as const, name: 'Out' },
      ],
    }));
  });

  // ── Lifetime ──
  protected readonly lifetime = computed(() => {
    const d = this.data();
    if (!d) return undefined;
    const l = d.lifetime;
    return {
      since: l.since ? shortDate(l.since, this.locale) : '—',
      sinceHint: l.since ? `${longDuration(Math.max(0, this.now() - new Date(l.since).getTime()))} ago` : undefined,
      fastest: l.fastestUpdate,
      longestBehind: l.longestBehind,
      streak: l.longestStreak,
      streakHint: l.longestStreak
        ? `${shortDate(l.longestStreak.from, this.locale)} to ${shortDate(l.longestStreak.to, this.locale)}`
        : undefined,
      uptime: longDuration(Math.max(0, this.now() - new Date(l.startedAt).getTime())),
    };
  });

  constructor() {
    effect(() => storeRange(this.range()));
  }

  protected readonly count = count;
  protected readonly formatDuration = formatDuration;
  protected readonly formatTokens = formatTokens;
  protected readonly longDuration = longDuration;
  protected readonly percentFormat = (v: number): string => percent(v);

  protected date(iso: string): string {
    return shortDate(iso, this.locale);
  }
  protected ago(iso: string): string {
    return timeAgo(iso, this.locale, this.now());
  }
  protected span(u: AppliedUpdate): string {
    return longDuration(u.ms);
  }
  protected gapText(days: number | undefined): string {
    if (days === undefined) return '—';
    if (days < 1) return 'more than daily';
    return days === 1 ? 'daily' : `every ${days} days`;
  }
}

/** A failure kind as a label: "Rate limited", "Not found"; unknown kinds ("other") capitalised as they are. */
function failureLabel(kind: string): string {
  return capitalize(errorKindLabel(kind as ErrorKind) || kind.replace('-', ' '));
}

/** "in 12 min", "in 3 h" for a time ahead of `now`; "now" once it has passed. */
function timeAgoOrIn(iso: string, now: number): string {
  const ms = new Date(iso).getTime() - now;
  return ms <= 0 ? 'now' : `in ${longDuration(ms)}`;
}

function readRange(): StatsRange {
  try {
    const stored = localStorage.getItem(RANGE_KEY);
    if (stored && RANGES.some((r) => r.value === stored)) return stored as StatsRange;
  } catch {
    // storage blocked: fall back to the default
  }
  return '90d';
}

function storeRange(range: StatsRange): void {
  try {
    localStorage.setItem(RANGE_KEY, range);
  } catch {
    // storage blocked: the choice just won't persist
  }
}
