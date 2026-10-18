import { AppIconSources, Level } from '../ui';

/** Shapes of GET /api/containers (internal/api/containers.go). */
export interface ContainerList {
  source: {
    kind: 'docker' | 'snapshot';
    location: string;
    capturedAt?: string;
  };
  check?: CheckStatus;
  containers: ContainerSummary[];
}

export interface CheckStatus {
  running: boolean;
  trigger?: 'startup' | 'schedule' | 'manual';
  /** Next scheduled run; absent when the schedule is off. */
  nextRunAt?: string;
  startedAt?: string;
  finishedAt?: string;
  done: number;
  total: number;
  error?: string;
  /** The config file couldn't be loaded; its rules aren't applied (labels still are). */
  rulesError?: string;
}

/** How the running version was determined (internal/resolve). */
export type UpdateSource = 'tag' | 'label' | 'digest' | 'env' | 'unknown';

/**
 * The result fields describe the last *successful* check. A failed attempt doesn't throw that
 * result away: it only sets `error` + `failedAt`. So there are three states:
 * result (checkedAt), result + failed refresh (checkedAt + error), no result yet (error only).
 */
export interface UpdateInfo {
  running?: string;
  source: UpdateSource;
  latest?: string;
  latestInMajor?: string;
  class: 'none' | 'patch' | 'minor' | 'major' | 'digest';
  floating: boolean;
  newImage: boolean;
  /** When the last successful check was; absent if there never was one. */
  checkedAt?: string;
  /** The most recent check attempt failed with this. */
  error?: string;
  /** When that failed attempt was; set together with `error`. */
  failedAt?: string;
  /** Breaking-change verdict from the version jump and the stored release notes. */
  level: Exclude<Level, 'unknown'>;
  /** Why, most severe first: "Major version: 2 → 3", "v3.0.0: Redis 8.0 or newer is now required". */
  reasons?: string[];
  /** Newest version a rule keeps back (outside the pin or the running major). */
  held?: string;
  heldBy?: 'pin' | 'allow-major';
}

/** Rule names as used in labels (`tidemark.<key>`) and the config file. */
export type RuleKey = 'ignore' | 'allow-major' | 'pin' | 'prerelease' | 'tag-pattern' | 'repo';

/** Per-container rules (Docker labels or the config file). */
export interface ContainerRules {
  /** Not checked: such a container has no `update`. */
  ignore?: true;
  /** Present only when false: major updates are held back. */
  allowMajor?: false;
  /** Version prefix, e.g. "1.4" or "16". */
  pin?: string;
  prerelease?: true;
  /** Regex source. */
  tagPattern?: string;
  /** e.g. "github.com/owner/repo". */
  repo?: string;
  /** Where each set rule came from. */
  from: Partial<Record<RuleKey, 'label' | 'config'>>;
  /** Rules skipped because their value was invalid. */
  problems?: string[];
}

export interface ContainerSummary {
  id: string;
  name: string;
  image: {
    ref: string;
    display: string;
    registry: string;
    repository: string;
    tag: string;
    id: string;
    created?: string;
    platform?: string;
    missing?: boolean;
  };
  state: 'running' | 'exited' | 'paused' | 'restarting' | 'created' | 'dead' | string;
  status: string;
  created: string;
  stack?: string;
  service?: string;
  /** App icon (selfh.st), if one matched. */
  icon?: AppIconSources;
  /** Result of the last update check, once there is one. */
  update?: UpdateInfo;
  /** Per-container rules; omitted when no rule is set and there are no problems. */
  rules?: ContainerRules;
}

export interface Stack {
  /** Compose project name, or null for containers not started by Compose. */
  name: string | null;
  containers: ContainerSummary[];
}

/** Groups by Compose project: stacks A→Z, containers A→Z, ungrouped containers last. */
export function groupByStack(containers: ContainerSummary[]): Stack[] {
  const byName = new Map<string | null, ContainerSummary[]>();
  for (const c of containers) {
    const key = c.stack || null;
    byName.set(key, [...(byName.get(key) ?? []), c]);
  }
  return [...byName.entries()]
    .map(([name, list]) => ({ name, containers: list.sort((a, b) => a.name.localeCompare(b.name)) }))
    .sort((a, b) => (a.name === null ? 1 : b.name === null ? -1 : a.name.localeCompare(b.name)));
}

/** The container has `ignore` set: it isn't checked and sits in the "Ignored" section. */
export function isIgnored(c: ContainerSummary): boolean {
  return !!c.rules?.ignore;
}

/** Splits off the ignored containers (sorted by name); the rest go to groupByStack. */
export function splitIgnored(containers: ContainerSummary[]): { active: ContainerSummary[]; ignored: ContainerSummary[] } {
  const active: ContainerSummary[] = [];
  const ignored: ContainerSummary[] = [];
  for (const c of containers) (isIgnored(c) ? ignored : active).push(c);
  ignored.sort((a, b) => a.name.localeCompare(b.name));
  return { active, ignored };
}

/** Where a rule came from, in words: "label tidemark.pin" or "config file". */
export function ruleOrigin(rules: ContainerRules, key: RuleKey): { kind: 'label' | 'config'; text: string; label?: string } | undefined {
  const from = rules.from[key];
  if (!from) return undefined;
  return from === 'label' ? { kind: 'label', text: 'label', label: `tidemark.${key}` } : { kind: 'config', text: 'config file' };
}

/** Image name without the tag: "redis", "louislam/uptime-kuma", "ghcr.io/sergi0g/cup". */
export function imageName(c: ContainerSummary): string {
  const { registry, repository, ref } = c.image;
  if (!registry) return ref;
  return registry === 'docker.io' ? repository.replace(/^library\//, '') : `${registry}/${repository}`;
}

/** There is a result from a successful check (possibly with a later failed refresh). */
export function hasResult(u: UpdateInfo | undefined): u is UpdateInfo & { checkedAt: string } {
  return !!u?.checkedAt;
}

/** A result is shown, but the most recent attempt to refresh it failed. */
export function refreshFailed(u: UpdateInfo | undefined): boolean {
  return hasResult(u) && !!u.error;
}

/** The last successful check found a newer version or image (a kept result counts too). */
export function hasUpdate(c: ContainerSummary): boolean {
  return !isIgnored(c) && hasResult(c.update) && c.update.class !== 'none';
}

/** The most recent attempt failed, whether or not an earlier result is still shown. */
export function checkFailed(c: ContainerSummary): boolean {
  return !isIgnored(c) && !!c.update?.error;
}

/** Containers with an update of any kind. */
export function countUpdates(containers: ContainerSummary[]): number {
  return containers.filter(hasUpdate).length;
}

/** Dashboard filter: everything, only containers with an update, or only breaking updates. */
export type Filter = 'all' | 'updates' | 'breaking';

export function matchesFilter(c: ContainerSummary, filter: Filter): boolean {
  if (filter === 'updates') return hasUpdate(c);
  if (filter === 'breaking') return hasUpdate(c) && c.update!.level === 'breaking';
  return true;
}
