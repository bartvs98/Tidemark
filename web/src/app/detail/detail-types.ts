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
  /** The AI summary state; absent when AI is off or there is nothing to summarise (internal/api/detail.go). */
  ai?: AiState;
}

export type AiProvider = 'anthropic' | 'openai' | 'gemini';

/** The AI summary of this update and what a new one would cover; also the answer to POST /api/containers/{name}/summary. */
export interface AiState {
  provider: AiProvider;
  model: string;
  /** 0 = automatic summaries are off. */
  autoMinReleases: number;
  /** Releases in range now: what a summary would cover. */
  releases: number;
  generating: boolean;
  summary?: AiSummaryData;
  /** The last attempt failed (a kept summary may still be there). */
  error?: string;
  failedAt?: string;
}

export interface AiSummaryData {
  /** All model-written text is plain text: never render it as HTML or markdown. */
  changes: string[];
  actions: string[];
  verdict: 'none' | 'attention' | 'breaking';
  reasons: { text: string; tag?: string }[];
  provider: string;
  model: string;
  releases: number;
  omitted: number;
  createdAt: string;
  /** The release notes in range changed since this summary was made. */
  stale: boolean;
  /** Made by extending the summary up to this version with only the newer releases; absent for a full one. */
  extendedFrom?: string;
  /** Tokens and time of the call that made it; 0 when unknown. */
  inputTokens: number;
  outputTokens: number;
  durationMs: number;
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
