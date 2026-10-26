/** GET /api/stats (internal/api/stats.go). */
export type StatsRange = '30d' | '90d' | '1y' | 'all';

export type UpdateKind = 'patch' | 'minor' | 'major' | 'digest';

export interface Stats {
  generatedAt: string;
  range: StatsRange;
  since?: string; // absent for 'all'
  partial?: true; // Docker couldn't be listed
  behind: BehindStats;
  upstream: UpstreamStats;
  health: HealthStats;
  ai?: AiStats; // absent when there are no ai_calls at all (in any range)
  lifetime: LifetimeStats;
}

export interface BehindStats {
  /** Current containers with an update (any review state, not ignored), longest behind first; those without behindSince last. */
  containers: {
    name: string;
    stack?: string;
    icon?: { src: string; light?: string; dark?: string };
    running: string;
    latest: string;
    class: UpdateKind;
    level: 'none' | 'attention' | 'breaking';
    review?: 'reviewed' | 'snoozed';
    releasesBehind: number; // releases in range running→latest from the stored notes (0 without notes)
    releasesPartial?: boolean; // the stored notes stop before the running version: releasesBehind and behindSince are lower bounds
    behindSince?: string; // published_at of the oldest release newer than running (needs notes)
  }[];
  /** history rows with a previous image (prev_image_id != '') detected in range. */
  applied: {
    total: number;
    perStack: { stack: string; removed?: boolean; count: number }[]; // stack '' for containers without a Compose project or no longer on the host (removed); most first
    perWeek: { week: string; count: number }[]; // ISO date of the week's Monday (UTC), every week in range incl. zeros
  };
  /** Per applied update in range whose new version has a published release in the stored notes: detected_at - published_at. */
  timeToUpdate: {
    count: number;
    medianMs?: number;
    /** Always patch, minor, major in this order; medianMs absent when count is 0. */
    byClass: KindMedian[];
    /** Always none, attention, breaking in this order (the level the notes give the update). */
    byLevel: KindMedian[];
  };
  /** From the last Telegram message about an update to when that version was seen running; count 0 when nothing matches. */
  fromMessage: { count: number; medianMs?: number };
  /** The newest run_stats row overall, whatever the range; absent without one. */
  now?: TidePoint;
  /** One point per run_stats row in range; if more than 180, the last row per UTC day. */
  /** One point per run; `releasesBehind` is the update debt: releases between running and latest over all updates. */
  tide: TidePoint[];
}
export interface TidePoint {
  at: string;
  checked: number;
  upToDate: number;
  updates: number;
  attention: number;
  breaking: number;
  releasesBehind: number;
}
export interface KindMedian {
  kind: string;
  count: number;
  medianMs?: number;
}
export interface AppliedUpdate {
  name: string;
  from: string;
  to: string;
  at: string;
  ms: number;
}

export interface UpstreamStats {
  /** Source repos of current containers that have stored notes, most releases in range first. */
  repos: {
    repo: string; // display name owner/name
    url: string;
    containers: string[];
    releases: number; // published in range (pre-releases included)
    breaking: number;
    attention: number; // of those, by analysis level (calm = releases - breaking - attention)
    lastRelease?: string; // newest published_at overall
    medianGapDays?: number; // median days between consecutive releases in range (needs 2+); one decimal
    partial?: boolean; // the range reaches back past the stored notes: `releases` is a lower bound
    quiet?: boolean; // lastRelease is more than 9 months before generatedAt
  }[];
  /** All those repos together, per calendar month (UTC, 'YYYY-MM'), every month in range incl. zeros. */
  /** `partial`: some repos' stored notes don't reach back to this month, so they're left out of it. */
  perMonth: { month: string; releases: number; breaking: number; attention: number; partial?: boolean }[];
  /** Applied updates in range with the most releases between from and to, top 5 (`ms`: time to update, 0 when unknown). */
  biggestJumps: (AppliedUpdate & { releases: number; class: UpdateKind; partial?: boolean })[];
}

export type FailureKind = 'rate-limit' | 'not-found' | 'timeout' | 'network' | 'registry';

export interface HealthStats {
  runs: {
    total: number;
    failed: number; // check_runs in range; failed = error != ''
    byTrigger: { startup: number; schedule: number; manual: number };
    medianMs?: number;
    slowestMs?: number;
  };
  /** One point per finished check run in range (thinned to 180 by day). */
  durations: { at: string; ms: number; images: number; failed: boolean }[];
  /** Containers whose latest attempt failed, right now. */
  failing: { name: string; kind?: FailureKind; error: string; since: string }[];
  /** Container-checks that failed over the range, per kind, most first. */
  failuresByKind: { kind: string; count: number }[];
  /** Requests per UTC day over the range (zeros included). */
  /** Per day, or per week (`day` is then the Monday) when the span is over 92 days: see `requestsPer`. */
  requests: ({ day: string } & RequestCounts)[];
  requestsPer: 'day' | 'week';
  requestTotals: RequestCounts;
  github: { token: boolean; limit?: number; remaining?: number; resetAt?: string }; // last-known, no request made
}

/** Requests, and those refused for a rate limit split like the chart (release notes = GitHub + GitLab). */
export interface RequestCounts {
  registry: number;
  github: number;
  gitlab: number;
  limitedRegistry: number;
  limitedNotes: number;
}

export interface AiStats {
  /** Calendar months in range (all: from the first call), zeros included. */
  months: { month: string; calls: number; failed: number; inputTokens: number; outputTokens: number }[];
  byModel: { provider: string; model: string; calls: number; failed: number; inputTokens: number; outputTokens: number }[];
  calls: number;
  failed: number;
  manual: number;
  auto: number;
  extended: number; // in range
  medianMs?: number; // successful calls in range
}

export interface LifetimeStats {
  /** Ignores the range. */
  since?: string; // first check run's start
  startedAt: string; // this process's start (uptime)
  containersTracked: number;
  releasesStored: number; // "release notes read"
  breakingFlagged: number;
  fastestUpdate?: AppliedUpdate;
  longestBehind?: AppliedUpdate;
  longestStreak?: { runs: number; from: string; to: string }; // consecutive runs with no open updates
}
