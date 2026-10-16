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
}

/** How the running version was determined (internal/resolve). */
export type UpdateSource = 'tag' | 'label' | 'digest' | 'env' | 'unknown';

export interface UpdateInfo {
  running?: string;
  source: UpdateSource;
  latest?: string;
  latestInMajor?: string;
  class: 'none' | 'patch' | 'minor' | 'major' | 'digest';
  floating: boolean;
  newImage: boolean;
  error?: string;
  checkedAt: string;
  /** Breaking-change verdict from the version jump and the stored release notes. */
  level: Exclude<Level, 'unknown'>;
  /** Why, most severe first: "Major version: 2 → 3", "v3.0.0: Redis 8.0 or newer is now required". */
  reasons?: string[];
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

/** Image name without the tag: "redis", "louislam/uptime-kuma", "ghcr.io/sergi0g/cup". */
export function imageName(c: ContainerSummary): string {
  const { registry, repository, ref } = c.image;
  if (!registry) return ref;
  return registry === 'docker.io' ? repository.replace(/^library\//, '') : `${registry}/${repository}`;
}

/** A successful check found a newer version or image. */
export function hasUpdate(c: ContainerSummary): boolean {
  return !!c.update && !c.update.error && c.update.class !== 'none';
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
