import { Component, computed, effect, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { HttpErrorResponse, httpResource } from '@angular/common/http';
import { SegmentOption, TmBadge, TmButton, TmCard, TmEmptyState, TmSection, TmSegmented, TmSkeleton } from '../ui';
import { ContainerTile } from './container-tile';
import { ContainerList, groupByStack, imageName } from './containers';

export type ViewMode = 'list' | 'tiles';

const VIEW_KEY = 'tidemark.view';

const VIEWS: SegmentOption<ViewMode>[] = [
  { value: 'list', label: 'List', icon: 'list' },
  { value: 'tiles', label: 'Tiles', icon: 'layout-grid' },
];

@Component({
  selector: 'tm-dashboard',
  imports: [DatePipe, ContainerTile, TmBadge, TmButton, TmCard, TmEmptyState, TmSection, TmSegmented, TmSkeleton],
  templateUrl: './dashboard.html',
})
export class Dashboard {
  protected readonly list = httpResource<ContainerList>(() => '/api/containers');

  protected readonly views = VIEWS;
  protected readonly view = signal<ViewMode>(readView());

  // value() throws while the resource is in an error state, so always go through hasValue().
  private readonly data = computed(() => (this.list.hasValue() ? this.list.value() : undefined));

  protected readonly stacks = computed(() => groupByStack(this.data()?.containers ?? []));
  protected readonly source = computed(() => this.data()?.source);

  protected readonly summary = computed(() => {
    const containers = this.data()?.containers ?? [];
    const running = containers.filter((c) => c.state === 'running').length;
    const stacks = this.stacks().filter((s) => s.name !== null).length;
    const parts = [plural(containers.length, 'container'), plural(stacks, 'stack')];
    if (running !== containers.length) parts.push(`${containers.length - running} not running`);
    return parts.join(' · ');
  });

  protected readonly errorMessage = computed(() => {
    const err = this.list.error();
    if (err instanceof HttpErrorResponse) {
      return (err.error as { error?: string } | null)?.error ?? `The API answered ${err.status} ${err.statusText}.`;
    }
    return err ? String(err) : '';
  });

  protected readonly imageName = imageName;
  protected readonly plural = plural;

  constructor() {
    effect(() => {
      const view = this.view();
      try {
        localStorage.setItem(VIEW_KEY, view);
      } catch {
        // storage blocked: the choice just won't persist
      }
    });
  }
}

function readView(): ViewMode {
  try {
    const stored = localStorage.getItem(VIEW_KEY);
    if (stored === 'list' || stored === 'tiles') return stored;
  } catch {
    // ignore
  }
  return 'list';
}

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? '' : 's'}`;
}
