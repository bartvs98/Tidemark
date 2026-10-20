import type { MarkdownFlag } from '../ui/markdown/markdown';
import { ContainerSummary } from '../dashboard/containers';

/** Shape of GET /api/containers/{name} (internal/api/detail.go). */
export interface ContainerDetailData {
  container: ContainerSummary;
  image: {
    digests: string[];
    labels: Record<string, string>;
  };
  /** Absent when no source repository could be found. */
  changelog?: Changelog;
  /** Image changes detected per check, newest first; the last entry (no `prevImageId`) is when tracking started. */
  history: HistoryEntry[];
}

/** One detected change of the container's image (internal/store history). */
export interface HistoryEntry {
  at: string;
  /** Version after the change; absent when unknown. */
  running?: string;
  imageId: string;
  prevRunning?: string;
  /** Absent on the first entry: tracking started then. */
  prevImageId?: string;
}

export interface Changelog {
  repo: { url: string; name: string; how: 'label' | 'config' | 'image' | 'registry' | 'known' | 'guess' };
  kind: 'releases' | 'changelog' | 'none' | '';
  releases: Release[];
  /** inList: GitHub release list, on the page with the running version, scrolled to it. */
  links: { compare?: string; releases?: string; running?: string; inList?: string };
  partial: boolean;
  error?: string;
  fetchedAt?: string;
}

export interface Release {
  tag: string;
  name: string;
  /** Markdown from the repository: untrusted, rendered through tm-markdown (sanitised). */
  body: string;
  url: string;
  publishedAt?: string;
  /** Breaking-change verdict for this release's notes, and the lines that caused it. */
  level: 'none' | 'attention' | 'breaking';
  hits: MarkdownFlag[];
}
