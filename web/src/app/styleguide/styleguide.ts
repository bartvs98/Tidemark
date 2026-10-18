import { Component, inject, signal } from '@angular/core';
import {
  ICON_NAMES,
  TM_TABS,
  TmAppIcon,
  TmBadge,
  TmButton,
  TmCallout,
  TmCard,
  TmClamp,
  TmEmptyState,
  TmIcon,
  TmLevel,
  TmSection,
  TmSegmented,
  TmSkeleton,
  TmThemeSwitch,
  SegmentOption,
  TmUpdateClass,
  TmVersionDiff,
  Toasts,
} from '../ui';
import { MarkdownFlag, TmMarkdown } from '../ui/markdown';
import { ContainerColumnBar, ContainerLink, ContainerTable } from '../dashboard/container-table';
import { ContainerTile } from '../dashboard/container-tile';
import { ContainerSummary, groupByStack, UpdateInfo } from '../dashboard/containers';
import { ContainerRuleProblems, ContainerUpdate, ContainerVersion } from '../dashboard/update-info';

/** A sample container from an image ref ("jellyfin/jellyfin:10.9.11", "ghcr.io/owner/app:v1"). */
function sample(name: string, ref: string, extra: Partial<ContainerSummary> = {}): ContainerSummary {
  const [path, tag = 'latest'] = ref.split(':');
  const first = path.split('/')[0];
  const registry = path.includes('/') && first.includes('.') ? first : 'docker.io';
  const repository = registry !== 'docker.io' ? path.slice(first.length + 1) : path.includes('/') ? path : `library/${path}`;
  return {
    id: name,
    name,
    image: { ref, display: ref, registry, repository, tag, id: 'sha256:x' },
    state: 'running',
    status: 'Up 2 days',
    created: '',
    ...extra,
  };
}

const CHECKED_AT = '2026-10-17T23:00:00Z';
const UPDATE = { source: 'tag' as const, floating: false, newImage: false, checkedAt: CHECKED_AT, level: 'none' as const };
/** The latest attempt failed; spread over a result, that result is kept from CHECKED_AT. */
const REFRESH_FAILED = {
  error: 'registry-1.docker.io: dial tcp: lookup registry-1.docker.io: no such host',
  failedAt: '2026-10-18T07:02:00Z',
};
/** No check ever succeeded: only the error. */
const NO_RESULT: UpdateInfo = {
  source: 'unknown',
  class: 'none',
  floating: false,
  newImage: false,
  level: 'none',
  error: 'ghcr.io/me/private-app: not found, or private',
  failedAt: REFRESH_FAILED.failedAt,
};
const icon = (ref: string) => ({ src: `/api/icons/${ref}` });

/** Living reference for the design system: every token and component in both themes. */
@Component({
  selector: 'tm-styleguide',
  imports: [
    ContainerRuleProblems,
    ContainerUpdate,
    ContainerVersion,
    ContainerColumnBar,
    ContainerTable,
    ContainerTile,
    TmAppIcon,
    TmBadge,
    TmButton,
    TmCallout,
    TmCard,
    TmClamp,
    TmMarkdown,
    TmEmptyState,
    TmIcon,
    TmLevel,
    TmSection,
    TmSegmented,
    TmSkeleton,
    TmThemeSwitch,
    TmUpdateClass,
    TmVersionDiff,
    ...TM_TABS,
  ],
  templateUrl: './styleguide.html',
})
export class Styleguide {
  protected readonly toasts = inject(Toasts);
  protected readonly loading = signal(false);
  protected readonly icons = ICON_NAMES;
  protected readonly view = signal<'list' | 'tiles'>('list');
  protected readonly views: SegmentOption<'list' | 'tiles'>[] = [
    { value: 'list', label: 'List', icon: 'list' },
    { value: 'tiles', label: 'Tiles', icon: 'layout-grid' },
  ];

  protected readonly filter = signal<'all' | 'updates' | 'breaking'>('all');
  protected readonly filters: SegmentOption<'all' | 'updates' | 'breaking'>[] = [
    { value: 'all', label: 'All' },
    { value: 'updates', label: 'Updates', count: 11 },
    { value: 'breaking', label: 'Breaking', count: 4 },
  ];

  protected readonly colors = [
    { group: 'Surfaces', tokens: ['canvas', 'surface', 'raised', 'hover'] },
    { group: 'Lines', tokens: ['line', 'line-strong'] },
    { group: 'Text', tokens: ['fg', 'fg-muted', 'fg-subtle'] },
    { group: 'Accent', tokens: ['accent', 'accent-hover', 'accent-text', 'accent-soft', 'on-accent', 'focus'] },
    {
      group: 'Status',
      tokens: ['ok', 'attention', 'attention-soft', 'attention-line', 'breaking', 'breaking-soft', 'breaking-line'],
    },
  ];

  protected readonly typeScale = [
    { cls: 'text-3xl font-semibold tracking-tight', label: '3xl · 32', sample: 'Containers' },
    { cls: 'text-2xl font-semibold tracking-tight', label: '2xl · 24', sample: 'Page title' },
    { cls: 'text-xl font-semibold', label: 'xl · 20', sample: 'Section heading' },
    { cls: 'text-lg font-semibold', label: 'lg · 16', sample: 'Card heading, emphasis' },
    { cls: 'text-base', label: 'base · 14', sample: 'Body text: what changed, then what to do.' },
    { cls: 'text-sm text-fg-muted', label: 'sm · 13', sample: 'Secondary text, table meta' },
    { cls: 'text-xs text-fg-muted', label: 'xs · 12', sample: 'Badges, column headers' },
    { cls: 'font-mono text-sm', label: 'mono · 13', sample: '1.4.2 → 1.6.0 · sha256:3f9a1c…' },
  ];

  protected readonly appIcons = [
    { name: 'redis', icon: { src: '/api/icons/redis', light: '/api/icons/redis?variant=light' } },
    { name: 'home-assistant', icon: { src: '/api/icons/home-assistant' } },
    { name: 'mkvtoolnix', icon: undefined },
    { name: 'broken', icon: { src: '/api/icons/does-not-exist' } },
  ];

  protected readonly sampleNotes = [
    '## Breaking changes',
    '',
    '- The `legacy_api` option was removed (#12345), thanks @frenck.',
    '- Config key `DB_URL` is now `DATABASE_URL`.',
    '',
    '### Fixes',
    '',
    '1. Faster startup',
    '2. See [the docs](https://example.com) for details.',
    '',
    '> Upgrade notes: back up first.',
    '',
    '```yaml',
    'database_url: postgres://…',
    '```',
  ].join('\n');

  protected readonly sampleFlags: MarkdownFlag[] = [
    { line: '## Breaking changes', level: 'breaking' },
    { line: '- The `legacy_api` option was removed (#12345), thanks @frenck.', level: 'breaking' },
    { line: '- Config key `DB_URL` is now `DATABASE_URL`.', level: 'breaking' },
    { line: '> Upgrade notes: back up first.', level: 'attention' },
  ];

  protected readonly longNotes = Array.from({ length: 24 }, (_, i) => `- Fix number ${i + 1} in a long release`).join('\n');

  protected readonly ignoredOpen = signal(false);

  /** Per-container rules as the dashboard shows them (real components, sample data). */
  protected readonly ruleSamples: { label: string; container: ContainerSummary }[] = [
    {
      label: 'Pinned to 1.4, newer major held back',
      container: sample('immich', 'ghcr.io/immich-app/immich-server:v1.4', {
        rules: { pin: '1.4', from: { pin: 'label' } },
        update: { ...UPDATE, running: '1.4.2', latest: '1.4.7', class: 'patch', held: '2.1.0', heldBy: 'pin' },
      }),
    },
    {
      label: 'Major updates off',
      container: sample('postgres', 'postgres:16', {
        rules: { allowMajor: false, from: { 'allow-major': 'config' } },
        update: { ...UPDATE, running: '16.4', latest: '16.10', class: 'minor', held: '18.0', heldBy: 'allow-major' },
      }),
    },
    {
      label: 'Ignored',
      container: sample('watchtower', 'containrrr/watchtower:latest', { rules: { ignore: true, from: { ignore: 'label' } } }),
    },
    {
      label: 'A rule was skipped',
      container: sample('redis', 'redis:7.4-alpine', {
        rules: { from: {}, problems: ['tidemark.allow-major: "maybe" is not true or false'] },
        update: { ...UPDATE, running: '7.4.2', latest: '7.4.6', class: 'patch' },
      }),
    },
  ];

  protected readonly radii = ['rounded-sm', 'rounded-md', 'rounded-lg', 'rounded-xl'];

  /** Every state tm-container-version + tm-container-update can be in. */
  protected readonly updateStates: { label: string; container: ContainerSummary; checking?: boolean }[] = [
    {
      label: 'Up to date',
      container: sample('sonarr', 'lscr.io/linuxserver/sonarr:latest', {
        update: { ...UPDATE, source: 'label', running: '4.0.14', latest: '4.0.14', class: 'none' },
      }),
    },
    {
      label: 'Update, with a risk level',
      container: sample('jellyfin', 'jellyfin/jellyfin:10.9.11', {
        update: { ...UPDATE, source: 'label', running: '10.9.11', latest: '10.10.7', class: 'minor', level: 'attention', reasons: ['10.10.0: clients older than 10.9 can no longer connect'] },
      }),
    },
    {
      label: 'Refresh failed, earlier result kept',
      container: sample('immich-server', 'ghcr.io/immich-app/immich-server:v1.135.3', {
        update: { ...UPDATE, running: 'v1.135.3', latest: 'v2.0.1', class: 'major', level: 'breaking', reasons: ['Major version: v1.135.3 → v2.0.1'], ...REFRESH_FAILED },
      }),
    },
    { label: 'Check failed, no result yet', container: sample('private-app', 'ghcr.io/me/private-app:2.1', { update: NO_RESULT }) },
    { label: 'Checking, no result yet', container: sample('radarr', 'lscr.io/linuxserver/radarr:latest'), checking: true },
    { label: 'Ignored', container: sample('watchtower', 'containrrr/watchtower:latest', { rules: { ignore: true, from: { ignore: 'label' } } }) },
  ];

  /** A small home server, laid out exactly like the dashboard's list view. */
  protected readonly stacks = groupByStack([
    sample('jellyfin', 'jellyfin/jellyfin:10.9.11', {
      stack: 'media',
      icon: icon('jellyfin'),
      update: { ...UPDATE, source: 'label', running: '10.9.11', latest: '10.10.7', class: 'minor', level: 'attention', reasons: ['10.10.0: clients older than 10.9 can no longer connect'] },
    }),
    sample('sonarr', 'lscr.io/linuxserver/sonarr:latest', {
      stack: 'media',
      icon: icon('sonarr'),
      update: { ...UPDATE, source: 'label', running: '4.0.14', latest: '4.0.14', class: 'none' },
    }),
    sample('radarr', 'lscr.io/linuxserver/radarr:latest', {
      stack: 'media',
      icon: icon('radarr'),
      state: 'exited',
      status: 'Exited (0) 3 hours ago',
      update: { ...UPDATE, source: 'label', running: '5.21.1', latest: '5.22.4', class: 'minor' },
    }),
    sample('immich-server', 'ghcr.io/immich-app/immich-server:v1.135.3', {
      stack: 'photos',
      icon: icon('immich'),
      update: { ...UPDATE, running: 'v1.135.3', latest: 'v2.0.1', latestInMajor: 'v1.135.4', class: 'major', level: 'breaking', reasons: ['Major version: v1.135.3 → v2.0.1'] },
    }),
    sample('redis', 'redis:7.4-alpine', {
      stack: 'photos',
      icon: icon('redis'),
      update: { ...UPDATE, source: 'env', running: '7.4.2', latest: '7.4.6', class: 'patch', ...REFRESH_FAILED },
    }),
    sample('pihole', 'pihole/pihole:latest', {
      update: { ...UPDATE, source: 'label', running: '2025.08.0', latest: '2025.08.0', class: 'digest', newImage: true },
    }),
    sample('private-app', 'ghcr.io/me/private-app:2.1', { update: NO_RESULT }),
  ]);

  /** Tiles: the same containers, one of each state. */
  protected readonly tiles = this.stacks
    .flatMap((s) => s.containers)
    .filter((c) => ['jellyfin', 'immich-server', 'redis', 'radarr', 'private-app'].includes(c.name));

  /** Sample rows and tiles link back here instead of to a detail page that doesn't exist. */
  protected readonly sampleLink: ContainerLink = () => ['/styleguide'];

  protected fakeLoad(): void {
    this.loading.set(true);
    setTimeout(() => {
      this.loading.set(false);
      this.toasts.show('Check finished · 5 updates, 1 breaking', 'ok');
    }, 1500);
  }
}
