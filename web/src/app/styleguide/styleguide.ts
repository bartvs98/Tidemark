import { Component, inject, signal } from '@angular/core';
import {
  ICON_NAMES,
  Level,
  TM_TABS,
  TmAppIcon,
  TmBadge,
  TmButton,
  TmCard,
  TmEmptyState,
  TmIcon,
  TmLevel,
  TmSection,
  TmSegmented,
  TmSkeleton,
  SegmentOption,
  TmUpdateClass,
  TmVersionDiff,
  Toasts,
  UpdateClass,
} from '../ui';

interface SampleContainer {
  name: string;
  image: string;
  from: string;
  to: string | null;
  kind: UpdateClass;
  level: Level;
}

/** Living reference for the design system: every token and component in both themes. */
@Component({
  selector: 'tm-styleguide',
  imports: [
    TmAppIcon,
    TmBadge,
    TmButton,
    TmCard,
    TmEmptyState,
    TmIcon,
    TmLevel,
    TmSection,
    TmSegmented,
    TmSkeleton,
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

  protected readonly radii = ['rounded-sm', 'rounded-md', 'rounded-lg', 'rounded-xl'];

  protected readonly stacks: { name: string; containers: SampleContainer[] }[] = [
    {
      name: 'media',
      containers: [
        { name: 'jellyfin', image: 'jellyfin/jellyfin:10.9.11', from: '10.9.11', to: '10.10.7', kind: 'minor', level: 'attention' },
        { name: 'sonarr', image: 'lscr.io/linuxserver/sonarr:latest', from: '4.0.14', to: null, kind: 'none', level: 'none' },
        { name: 'radarr', image: 'lscr.io/linuxserver/radarr:latest', from: '5.21.1', to: '5.22.4', kind: 'minor', level: 'none' },
      ],
    },
    {
      name: 'photos',
      containers: [
        { name: 'immich-server', image: 'ghcr.io/immich-app/immich-server:v1.135.3', from: 'v1.135.3', to: 'v2.0.1', kind: 'major', level: 'breaking' },
        { name: 'redis', image: 'redis:7.4-alpine', from: '7.4.2', to: '7.4.6', kind: 'patch', level: 'none' },
      ],
    },
    {
      name: 'Ungrouped',
      containers: [
        { name: 'pihole', image: 'pihole/pihole:latest', from: '2025.08.0', to: '2025.08.0', kind: 'digest', level: 'none' },
        { name: 'uptime-kuma', image: 'louislam/uptime-kuma:1', from: '1', to: null, kind: 'none', level: 'unknown' },
      ],
    },
  ];

  protected fakeLoad(): void {
    this.loading.set(true);
    setTimeout(() => {
      this.loading.set(false);
      this.toasts.show('Check finished · 5 updates, 1 breaking', 'ok');
    }, 1500);
  }
}
