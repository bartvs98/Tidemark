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
}

export interface Changelog {
  repo: { url: string; name: string; how: 'label' | 'image' | 'registry' | 'known' | 'guess' };
  kind: 'releases' | 'changelog' | 'none' | '';
  releases: Release[];
  links: { compare?: string; releases?: string; running?: string };
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
}
