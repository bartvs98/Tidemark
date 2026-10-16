/** Shapes of GET /api/containers (internal/api/containers.go). */
export interface ContainerList {
  source: {
    kind: 'docker' | 'snapshot';
    location: string;
    capturedAt?: string;
  };
  containers: ContainerSummary[];
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
