import { Component, inject, signal } from '@angular/core';
import {
  ICON_NAMES,
  TM_TABS,
  TmAiMarker,
  TmAppIcon,
  TmBadge,
  TmButton,
  TmCallout,
  TmCard,
  TmClamp,
  TmDialog,
  TmEmptyState,
  TmIcon,
  TmLevel,
  TmMenu,
  TmSection,
  TmField,
  TmInput,
  TmPasswordInput,
  TmSearch,
  TmSelect,
  TmSwitch,
  SelectOption,
  TmSegmented,
  TmSkeleton,
  TmThemeSwitch,
  TmTideGauge,
  TmTooltip,
  SegmentOption,
  MenuOption,
  TmUpdateClass,
  TmVersionDiff,
  Toasts,
} from '../ui';
import { BarListItem, CalendarDay, ChartBar, TimePoint, TmBarChart, TmBarList, TmCalendarChart, TmStat, TmTimeChart } from '../ui/charts';
import { MarkdownFlag, TmMarkdown } from '../ui/markdown/markdown';
import { ContainerColumnBar } from '../dashboard/column-bar/column-bar';
import { DashboardSkeleton } from '../dashboard/dashboard-skeleton/dashboard-skeleton';
import { DetailSkeleton } from '../detail/detail-skeleton/detail-skeleton';
import { StatsSkeleton } from '../stats/stats-skeleton/stats-skeleton';
import { ContainerLink, ContainerTable } from '../dashboard/container-table/container-table';
import { ContainerTile } from '../dashboard/container-tile/container-tile';
import { SecretSetting } from '../settings/secret-setting/secret-setting';
import type { SettingField } from '../settings/settings-dialog';
import { ContainerSummary, groupByStack, UpdateInfo } from '../dashboard/containers';
import { ContainerUpdate } from '../dashboard/container-update/container-update';
import { ContainerVersion } from '../dashboard/container-version/container-version';
import { ReviewActions } from '../detail/review-actions/review-actions';
import { AiSummary } from '../detail/ai-summary/ai-summary';
import { History } from '../detail/history/history';
import { AiState, HistoryEntry } from '../detail/detail-types';
import { ContainerRuleProblems } from '../dashboard/rule-problems/rule-problems';

/** A sample container from an image ref ("jellyfin/jellyfin:10.9.11", "ghcr.io/owner/app:v1"). */
function sample(
  name: string,
  ref: string,
  extra: Partial<ContainerSummary> = {},
): ContainerSummary {
  const [path, tag = 'latest'] = ref.split(':');
  const first = path.split('/')[0];
  const registry = path.includes('/') && first.includes('.') ? first : 'docker.io';
  const repository =
    registry !== 'docker.io'
      ? path.slice(first.length + 1)
      : path.includes('/')
        ? path
        : `library/${path}`;
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
const UPDATE = {
  source: 'tag' as const,
  floating: false,
  newImage: false,
  checkedAt: CHECKED_AT,
  level: 'none' as const,
};
/** The latest attempt failed; spread over a result, that result is kept from CHECKED_AT. */
const REFRESH_FAILED = {
  error: 'ghcr.io/immich-app/immich-server: registry rate limit reached, try again later',
  errorKind: 'rate-limit' as const,
  failedAt: '2026-10-18T07:02:00Z',
};
/** No check ever succeeded: only the error (this one has no kind, so no short reason). */
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
    SecretSetting,
    ContainerUpdate,
    ContainerVersion,
    ContainerColumnBar,
    DashboardSkeleton,
    DetailSkeleton,
    StatsSkeleton,
    ContainerTable,
    ContainerTile,
    TmAppIcon,
    TmBadge,
    TmButton,
    TmCallout,
    TmCard,
    TmClamp,
    TmDialog,
    TmMarkdown,
    TmEmptyState,
    TmIcon,
    TmLevel,
    TmMenu,
    ReviewActions,
    AiSummary,
    TmAiMarker,
    History,
    TmSection,
    TmField,
    TmInput,
    TmPasswordInput,
    TmSearch,
    TmSelect,
    TmSwitch,
    TmSegmented,
    TmSkeleton,
    TmThemeSwitch,
    TmTideGauge,
    TmTooltip,
    TmBarChart,
    TmBarList,
    TmCalendarChart,
    TmStat,
    TmTimeChart,
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

  protected readonly searchEmpty = signal('');
  protected readonly searchFilled = signal('jelly');

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
    {
      group: 'Accent',
      tokens: ['accent', 'accent-hover', 'accent-text', 'accent-soft', 'on-accent', 'focus'],
    },
    {
      group: 'Status',
      tokens: [
        'ok',
        'attention',
        'attention-soft',
        'attention-line',
        'breaking',
        'breaking-soft',
        'breaking-line',
      ],
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
    '3. Images become links: ![new settings page](https://example.com/settings.png)',
    '',
    '> Upgrade notes: back up first.',
    '',
    '```yaml',
    'database_url: postgres://…',
    '```',
  ].join('\n');

  protected readonly sampleFlags: MarkdownFlag[] = [
    { line: '## Breaking changes', level: 'breaking', match: 'Breaking changes' },
    {
      line: '- The `legacy_api` option was removed (#12345), thanks @frenck.',
      level: 'breaking',
      section: 'Breaking changes',
    },
    {
      line: '- Config key `DB_URL` is now `DATABASE_URL`.',
      level: 'breaking',
      section: 'Breaking changes',
    },
    { line: '> Upgrade notes: back up first.', level: 'attention', match: 'Upgrade notes' },
  ];

  protected readonly longNotes = Array.from(
    { length: 24 },
    (_, i) => `- Fix number ${i + 1} in a long release`,
  ).join('\n');
  /** One flag in view, two hidden by the clamp. */
  protected readonly longFlags: MarkdownFlag[] = [
    { line: '- Fix number 2 in a long release', level: 'attention' },
    { line: '- Fix number 17 in a long release', level: 'breaking' },
    { line: '- Fix number 21 in a long release', level: 'attention' },
  ];

  protected readonly ignoredOpen = signal(false);
  protected readonly snoozedOpen = signal(true);
  protected readonly settledOpen = signal(true);

  protected readonly menuItems: MenuOption[] = [
    { value: 'day', label: '1 day' },
    { value: 'week', label: '1 week', icon: 'clock' },
    {
      value: 'newer',
      label: 'Until a newer version',
      disabled: true,
      hint: 'The latest version is unknown, so there is nothing to mark it against.',
    },
  ];
  protected readonly menuChoice = signal('');

  protected readonly formSchedule = signal('0 */6 * * *');
  protected readonly formUrl = signal('');
  protected readonly formBad = signal('every six hours');
  protected readonly formToken = signal('');
  protected readonly formTokenFilled = signal('123456789:AAE-example-token');
  protected readonly formLevel = signal<'all' | 'attention' | 'breaking'>('all');
  protected readonly formEmptyLevel = signal<'' | 'all' | 'attention' | 'breaking'>('');
  protected readonly formLevels: SelectOption<'all' | 'attention' | 'breaking'>[] = [
    { value: 'all', label: 'Every update' },
    { value: 'attention', label: 'Attention and breaking', hint: 'Skips updates without risk' },
    { value: 'breaking', label: 'Breaking only' },
  ];
  /** The same options, typed to allow '' (nothing chosen yet). */
  protected readonly formLevelsOrNone: SelectOption<'' | 'all' | 'attention' | 'breaking'>[] =
    this.formLevels;
  protected readonly formIcons = signal(true);
  protected readonly formDim = signal(false);
  protected readonly secretEdit = signal<string | null | undefined>(undefined);
  protected readonly secretSet: SettingField = {
    secret: true,
    set: true,
    source: 'stored',
    env: 'TIDEMARK_TELEGRAM_BOT_TOKEN',
  };
  protected readonly secretUnset: SettingField = {
    secret: true,
    set: false,
    source: 'default',
    env: 'TIDEMARK_GITHUB_TOKEN',
  };
  protected readonly secretLocked: SettingField = {
    secret: true,
    set: true,
    source: 'env',
    env: 'TIDEMARK_GITHUB_TOKEN',
  };
  protected readonly dialogOpen = signal(false);
  protected readonly pagesDialogOpen = signal(false);

  /** Per-container rules as the dashboard shows them (real components, sample data). */
  protected readonly ruleSamples: { label: string; container: ContainerSummary }[] = [
    {
      label: 'Pinned to 1.4, newer major held back',
      container: sample('immich', 'ghcr.io/immich-app/immich-server:v1.4', {
        rules: { pin: '1.4', from: { pin: 'label' } },
        update: {
          ...UPDATE,
          running: '1.4.2',
          latest: '1.4.7',
          class: 'patch',
          held: '2.1.0',
          heldBy: 'pin',
        },
      }),
    },
    {
      label: 'Major updates off',
      container: sample('postgres', 'postgres:16', {
        rules: { allowMajor: false, from: { 'allow-major': 'config' } },
        update: {
          ...UPDATE,
          running: '16.4',
          latest: '16.10',
          class: 'minor',
          held: '18.0',
          heldBy: 'allow-major',
        },
      }),
    },
    {
      label: 'Ignored',
      container: sample('watchtower', 'containrrr/watchtower:latest', {
        rules: { ignore: true, from: { ignore: 'label' } },
      }),
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

  /** Updates the user has dealt with: reviewed (faded, "Reviewed" marker) and snoozed (until a date / a newer version). */
  protected readonly reviewed = sample('sonarr-4k', 'lscr.io/linuxserver/sonarr:latest', {
    stack: 'media',
    update: {
      ...UPDATE,
      source: 'label',
      running: '4.0.12',
      latest: '4.0.14',
      class: 'patch',
      level: 'attention',
      reasons: [{ text: 'config path moved', tag: '4.0.13' }],
    },
    review: { kind: 'reviewed', version: '4.0.14', at: '2026-10-18T09:00:00Z' },
  });
  protected readonly snoozedUntil = sample('jellyfin', 'jellyfin/jellyfin:10.9.11', {
    stack: 'media',
    update: {
      ...UPDATE,
      source: 'label',
      running: '10.9.11',
      latest: '10.10.7',
      class: 'minor',
      level: 'attention',
      reasons: [{ text: 'clients older than 10.9 can no longer connect', tag: '10.10.0' }],
    },
    review: { kind: 'snoozed', until: '2026-11-03T09:00:00Z', at: '2026-10-18T09:00:00Z' },
  });
  protected readonly snoozedNewer = sample(
    'immich-server',
    'ghcr.io/immich-app/immich-server:v1.135.3',
    {
      stack: 'photos',
      update: {
        ...UPDATE,
        running: 'v1.135.3',
        latest: 'v2.0.1',
        class: 'major',
        level: 'breaking',
        reasons: [{ text: 'Major version: v1.135.3 → v2.0.1' }],
      },
      review: { kind: 'snoozed', version: 'v2.0.1', at: '2026-10-18T09:00:00Z' },
    },
  );
  /** A channel tag (or a floating tag without a version): reviews and snoozes are tied to the build, not a version. */
  private readonly buildUpdate = {
    ...UPDATE,
    running: 'nightly',
    class: 'digest' as const,
    channel: 'nightly',
    floating: true,
    newImage: true,
    tagDigest: 'sha256:3f9a1c0d2b7e8a41',
    reviewKey: 'sha256:3f9a1c0d2b7e8a41',
  };
  protected readonly reviewedBuild = sample(
    'sonarr-nightly',
    'lscr.io/linuxserver/sonarr:nightly',
    {
      stack: 'media',
      update: this.buildUpdate,
      review: { kind: 'reviewed', version: 'sha256:3f9a1c0d2b7e8a41', at: '2026-10-18T09:00:00Z' },
    },
  );
  protected readonly snoozedBuild = sample('radarr-nightly', 'lscr.io/linuxserver/radarr:nightly', {
    stack: 'media',
    update: this.buildUpdate,
    review: { kind: 'snoozed', version: 'sha256:3f9a1c0d2b7e8a41', at: '2026-10-18T09:00:00Z' },
  });
  protected readonly reviewSamples = [
    this.reviewed,
    this.snoozedUntil,
    this.snoozedNewer,
    this.reviewedBuild,
    this.snoozedBuild,
  ];
  /** What the detail header's review actions show, per state. */
  protected readonly reviewActionStates: { label: string; container: ContainerSummary }[] = [
    { label: 'No review yet', container: { ...this.snoozedUntil, review: undefined } },
    { label: 'Reviewed', container: this.reviewed },
    { label: 'Snoozed until a date', container: this.snoozedUntil },
    { label: 'Snoozed until a newer version', container: this.snoozedNewer },
    {
      label: 'Channel tag, no review yet: against the build',
      container: { ...this.reviewedBuild, review: undefined },
    },
    { label: 'Reviewed this build', container: this.reviewedBuild },
    { label: 'Snoozed until a new build', container: this.snoozedBuild },
    {
      label: 'No version or build known: both disabled',
      container: {
        ...this.snoozedUntil,
        review: undefined,
        update: { ...UPDATE, running: 'dev', class: 'digest', newImage: true },
      },
    },
  ];
  /** The AI summary card in each state (the buttons call the API, so here a click only shows an error). */
  private readonly aiBase: AiState = {
    provider: 'anthropic',
    model: 'claude-haiku-5-5',
    autoMinReleases: 5,
    releases: 4,
    generating: false,
  };
  private readonly aiSummary: NonNullable<AiState['summary']> = {
    changes: [
      'Adds a built-in backup job that runs nightly and keeps the last seven copies.',
      'The web UI now remembers the selected library between visits.',
      'Fixes a crash when a playlist contains an unreadable file.',
    ],
    actions: [
      'Rename the config key DB_URL to DATABASE_URL before starting v2.0.0.',
      'Mount /backups if you want to keep the nightly copies.',
    ],
    verdict: 'breaking',
    reasons: [
      { text: 'The config key DB_URL was renamed to DATABASE_URL.', tag: 'v2.0.0' },
      { text: 'The database is migrated on first start and cannot be rolled back.' },
    ],
    provider: 'anthropic',
    model: 'claude-haiku-5-5',
    releases: 4,
    omitted: 0,
    createdAt: new Date(Date.now() - 3 * 3600_000).toISOString(),
    stale: false,
    inputTokens: 12340,
    outputTokens: 640,
    durationMs: 4200,
  };
  protected readonly aiStates: { label: string; ai: AiState }[] = [
    { label: 'Summary, with a breaking verdict', ai: { ...this.aiBase, summary: this.aiSummary } },
    {
      label: 'Summary, nothing to flag, some releases left out',
      ai: {
        ...this.aiBase,
        summary: {
          ...this.aiSummary,
          verdict: 'none',
          reasons: [],
          actions: [],
          omitted: 3,
          releases: 20,
        },
      },
    },
    {
      label: 'Extended: only the releases after 3.1.0 were sent',
      ai: {
        ...this.aiBase,
        summary: { ...this.aiSummary, extendedFrom: '3.1.0', inputTokens: 2800, durationMs: 1900 },
      },
    },
    {
      label: 'Summary, call stats unknown (an older one)',
      ai: {
        ...this.aiBase,
        summary: { ...this.aiSummary, inputTokens: 0, outputTokens: 0, durationMs: 0 },
      },
    },
    {
      label: 'Stale: the notes changed since',
      ai: { ...this.aiBase, summary: { ...this.aiSummary, verdict: 'attention', stale: true } },
    },
    { label: 'No summary yet (automatic from 5 releases)', ai: this.aiBase },
    {
      label: 'No summary yet, OpenAI-compatible, auto off',
      ai: {
        ...this.aiBase,
        provider: 'openai',
        model: 'llama3.1:8b',
        autoMinReleases: 0,
        releases: 1,
      },
    },
    { label: 'Generating (an automatic run)', ai: { ...this.aiBase, generating: true } },
    {
      label: 'Failed, no summary',
      ai: { ...this.aiBase, error: 'the API key was rejected', failedAt: '2026-10-22T08:00:00Z' },
    },
    {
      label: 'Failed, an earlier summary is kept',
      ai: {
        ...this.aiBase,
        summary: { ...this.aiSummary, verdict: 'none', reasons: [] },
        error: 'rate limited by Anthropic',
        failedAt: '2026-10-22T08:00:00Z',
      },
    },
  ];

  protected readonly historyEntries: HistoryEntry[] = [
    {
      at: '2026-10-12T07:00:00Z',
      running: '1.6.0',
      imageId: 'sha256:3f9a1c0d2b7e8a41',
      prevRunning: '1.4.2',
      prevImageId: 'sha256:9b21c4e07a55d013',
    },
    {
      at: '2026-10-01T07:00:00Z',
      imageId: 'sha256:9b21c4e07a55d013',
      prevImageId: 'sha256:51d0aa7c92e3b6f8',
    },
    { at: '2026-09-20T07:00:00Z', running: '1.4.2', imageId: 'sha256:51d0aa7c92e3b6f8' },
  ];

  /** Every state tm-container-version + tm-container-update can be in. */
  protected readonly updateStates: {
    label: string;
    container: ContainerSummary;
    checking?: boolean;
  }[] = [
    {
      label: 'Up to date',
      container: sample('sonarr', 'lscr.io/linuxserver/sonarr:latest', {
        update: { ...UPDATE, source: 'label', running: '4.0.14', latest: '4.0.14', class: 'none' },
      }),
    },
    {
      label: 'Update, with a risk level',
      container: sample('jellyfin', 'jellyfin/jellyfin:10.9.11', {
        update: {
          ...UPDATE,
          source: 'label',
          running: '10.9.11',
          latest: '10.10.7',
          class: 'minor',
          level: 'attention',
          reasons: [
            {
              text: 'clients older than 10.9 can no longer connect',
              tag: '10.10.0',
              line: '- clients older than 10.9 can no longer connect',
            },
          ],
        },
      }),
    },
    {
      label: 'Channel build (nightly), new build available',
      container: sample('mealie', 'ghcr.io/mealie-recipes/mealie:nightly', {
        update: {
          ...UPDATE,
          source: 'unknown',
          channel: 'nightly',
          latest: 'v3.1.0',
          class: 'digest',
          newImage: true,
        },
      }),
    },
    {
      label: 'Refresh failed, earlier result kept',
      container: sample('immich-server', 'ghcr.io/immich-app/immich-server:v1.135.3', {
        update: {
          ...UPDATE,
          running: 'v1.135.3',
          latest: 'v2.0.1',
          class: 'major',
          level: 'breaking',
          reasons: [{ text: 'Major version: v1.135.3 → v2.0.1' }],
          ...REFRESH_FAILED,
        },
      }),
    },
    {
      label: 'Check failed, no result yet',
      container: sample('private-app', 'ghcr.io/me/private-app:2.1', { update: NO_RESULT }),
    },
    {
      label: 'Check failed, with a reason',
      container: sample('internal-tool', 'ghcr.io/me/internal-tool:3.0', {
        update: { ...NO_RESULT, errorKind: 'not-found' },
      }),
    },
    {
      label: 'Checking, no result yet',
      container: sample('radarr', 'lscr.io/linuxserver/radarr:latest'),
      checking: true,
    },
    {
      label: 'Ignored',
      container: sample('watchtower', 'containrrr/watchtower:latest', {
        rules: { ignore: true, from: { ignore: 'label' } },
      }),
    },
  ];

  /** A small home server, laid out exactly like the dashboard's list view. */
  protected readonly stacks = groupByStack([
    sample('jellyfin', 'jellyfin/jellyfin:10.9.11', {
      stack: 'media',
      icon: icon('jellyfin'),
      update: {
        ...UPDATE,
        source: 'label',
        running: '10.9.11',
        latest: '10.10.7',
        class: 'minor',
        level: 'attention',
        reasons: [
          {
            text: 'clients older than 10.9 can no longer connect',
            tag: '10.10.0',
            line: '- clients older than 10.9 can no longer connect',
          },
        ],
      },
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
      update: {
        ...UPDATE,
        running: 'v1.135.3',
        latest: 'v2.0.1',
        latestInMajor: 'v1.135.4',
        class: 'major',
        level: 'breaking',
        reasons: [{ text: 'Major version: v1.135.3 → v2.0.1' }],
      },
    }),
    sample('redis', 'redis:7.4-alpine', {
      stack: 'photos',
      icon: icon('redis'),
      update: {
        ...UPDATE,
        source: 'env',
        running: '7.4.2',
        latest: '7.4.6',
        class: 'patch',
        ...REFRESH_FAILED,
      },
    }),
    sample('pihole', 'pihole/pihole:latest', {
      update: {
        ...UPDATE,
        source: 'label',
        running: '2025.08.0',
        latest: '2025.08.0',
        class: 'digest',
        newImage: true,
      },
    }),
    sample('private-app', 'ghcr.io/me/private-app:2.1', { update: NO_RESULT }),
  ]);

  /** Tiles: the same containers, one of each state. */
  protected readonly tiles = this.stacks
    .flatMap((s) => s.containers)
    .filter((c) =>
      ['jellyfin', 'sonarr', 'immich-server', 'redis', 'radarr', 'private-app'].includes(c.name),
    );

  /** Settled stacks (everything up to date), as they sit inside the dashboard's "Up to date" section. */
  protected readonly settledStacks = groupByStack([
    sample('adguard', 'adguard/adguardhome:v0.107.62', {
      stack: 'networking',
      update: { ...UPDATE, running: 'v0.107.62', latest: 'v0.107.62', class: 'none' },
    }),
    sample('caddy', 'caddy:2.10', {
      stack: 'networking',
      update: { ...UPDATE, running: '2.10.0', latest: '2.10.0', class: 'none' },
    }),
    sample('filebrowser', 'filebrowser/filebrowser:v2.32', {
      update: { ...UPDATE, running: 'v2.32.0', latest: 'v2.32.0', class: 'none' },
    }),
  ]);

  /** An updatable and an up-to-date container, with the stack name shown (the "group by severity" layout). */
  protected readonly stackSamples = this.stacks
    .flatMap((s) => s.containers)
    .filter((c) => ['jellyfin', 'sonarr'].includes(c.name));

  protected readonly multiLineTip = 'Breaking: config key renamed\nCheck the release notes first';
  protected readonly heroLevels = [1, 0.55, 0.15];
  protected readonly compactLevels = [
    { level: 1, text: 'Up to date' },
    { level: 0.55, text: '3 releases behind' },
    { level: 0.15, text: '12 releases behind' },
  ];
  protected readonly frameLevels = ['attention', 'breaking'] as const;

  // ── Charts (stats page) ──
  protected readonly percent = (v: number) => `${Math.round(v * 100)}%`;
  /** Up to date ÷ checked, one check run every 6 hours for 30 days: dips when updates land, recovers as they're applied. */
  protected readonly tidePoints: TimePoint[] = Array.from({ length: 120 }, (_, i) => {
    const at = new Date(Date.UTC(2026, 8, 23, 6) + i * 6 * 3_600_000).toISOString();
    const behind = Math.round(2 + 2 * Math.sin(i / 9) + (i % 17 < 4 ? 3 : 0));
    return { at, value: (25 - Math.max(0, behind)) / 25, note: `${25 - Math.max(0, behind)} of 25 checked` };
  });
  protected readonly tideOnePoint: TimePoint[] = [this.tidePoints[0]];
  /** Applied updates per week, 13 weeks. */
  protected readonly appliedWeeks: ChartBar[] = [2, 0, 3, 5, 1, 0, 4, 2, 6, 1, 3, 0, 2].map((n, i) => ({
    label: new Date(Date.UTC(2026, 6, 27) + i * 7 * 86_400_000).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', timeZone: 'UTC' }),
    segments: [{ name: 'Updates applied', value: n }],
  }));
  /** Releases per month across the watched repos, calm / attention / breaking. */
  protected readonly releaseMonths: ChartBar[] = (
    [['May', 14, 2, 0], ['Jun', 9, 1, 1], ['Jul', 17, 3, 0], ['Aug', 11, 0, 2], ['Sep', 21, 4, 1], ['Oct', 8, 1, 0]] as const
  ).map(([label, calm, attention, breaking]) => ({
    label,
    segments: [
      { name: 'Calm', value: calm },
      { name: 'Attention', value: attention, tone: 'attention' as const },
      { name: 'Breaking', value: breaking, tone: 'breaking' as const },
    ],
  }));
  /** A made-up year of daily checks (UTC days up to 25 Oct 2026): a gap early on, a run of 100 % days, a dip. */
  protected readonly tideYear: CalendarDay[] = this.tideDays(365);
  protected readonly tideMonth: CalendarDay[] = this.tideDays(30);
  protected readonly emptyBars: ChartBar[] = [{ label: 'Oct', segments: [{ name: 'Updates applied', value: 0 }] }];
  protected readonly stackItems: BarListItem[] = [
    { label: 'media', value: 9, href: '/styleguide', hint: '5 containers' },
    { label: 'immich', value: 6, href: '/styleguide', hint: '4 containers' },
    { label: 'monitoring', value: 4, hint: '3 containers' },
    { label: 'proxy', value: 3, hint: '2 containers' },
    { label: 'paperless', value: 3, hint: '3 containers' },
    { label: 'homeassistant', value: 1 },
    { label: 'vaultwarden', value: 1 },
    { label: 'No stack', value: 2, tone: 'muted' },
  ];
  protected readonly jumpItems: BarListItem[] = [
    { label: '1.118.2 → 1.132.0', value: 14, display: '14 releases', hint: 'immich-server', mono: true },
    { label: '10.9.11 → 10.11.0', value: 6, display: '6 releases', hint: 'jellyfin', mono: true },
    { label: '4.0.8 → 4.0.14', value: 3, display: '3 releases', hint: 'sonarr', mono: true },
  ];

  /** A mono hint with a plain note, as in "Oldest software running". */
  protected readonly ageItems: BarListItem[] = [
    { label: 'uptime-kuma', value: 22, display: '22 months', hint: '1.23.16', hintMono: true },
    { label: 'mosquitto', value: 10, display: '10 months', hint: '2.0.22', hintMono: true, hintNote: 'latest' },
    { label: 'vaultwarden', value: 2, display: '2 months', hint: '1.37.2', hintMono: true },
  ];

  /** Two lists on one scale (`max`), as in "Time to update by kind". */
  protected readonly kindClassItems: BarListItem[] = [
    { label: 'Patch', value: 2, display: '2 days', hint: '14 updates' },
    { label: 'Minor', value: 9, display: '9 days', hint: '6 updates' },
    { label: 'Major', value: 0, display: '—', hint: '0 updates' },
  ];
  protected readonly kindLevelItems: BarListItem[] = [
    { label: 'Calm', value: 3, display: '3 days', hint: '18 updates' },
    { label: 'Attention', value: 12, display: '12 days', hint: '4 updates', tone: 'attention' },
    { label: 'Breaking', value: 30, display: '30 days', hint: '1 update', tone: 'breaking' },
  ];
  protected readonly kindMax = 30;

  /** Sample rows and tiles link back here instead of to a detail page that doesn't exist. */
  protected readonly sampleLink: ContainerLink = () => ['/styleguide'];

  protected fakeLoad(): void {
    this.loading.set(true);
    setTimeout(() => {
      this.loading.set(false);
      this.toasts.show('Check finished · 5 updates, 1 breaking', 'ok');
    }, 1500);
  }

  private tideDays(n: number): CalendarDay[] {
    const end = Date.UTC(2026, 9, 25);
    return Array.from({ length: n }, (_, i) => {
      const k = n - 1 - i;
      const day = new Date(end - k * 86_400_000).toISOString().slice(0, 10);
      const total = 9;
      const gap = i % 41 > 36 || (i > 60 && i < 66);
      if (gap) return { day };
      const behind = i % 29 < 8 ? 0 : Math.round(2 + 2 * Math.sin(i / 6) + (i % 17 < 3 ? 3 : 0));
      const upToDate = total - Math.max(0, Math.min(behind, total));
      return { day, value: upToDate / total, note: `${upToDate} of ${total} up to date` };
    });
  }
}
