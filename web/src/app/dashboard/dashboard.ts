import { Component, computed, effect, inject, signal } from '@angular/core';
import { DatePipe, NgTemplateOutlet } from '@angular/common';
import { HttpClient, HttpErrorResponse, httpResource } from '@angular/common/http';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import {
  SegmentOption,
  TmAppIcon,
  TmBadge,
  TmButton,
  TmCallout,
  TmCard,
  TmEmptyState,
  TmSection,
  TmSegmented,
  TmSkeleton,
  Toasts,
} from '../ui';
import { ContainerTile } from './container-tile';
import { ContainerList, countUpdates, Filter, groupByStack, hasUpdate, imageName, matchesFilter, splitIgnored } from './containers';
import { ContainerRuleProblems, ContainerUpdate, ContainerVersion } from './update-info';

export type ViewMode = 'list' | 'tiles';

const VIEW_KEY = 'tidemark.view';
const FILTER_KEY = 'tidemark.filter';
const POLL_MS = 1500;

/** List columns. The pinned header bar and every stack's table share these widths. */
const COLUMNS = [
  { label: 'Container', width: 'w-[25%]' },
  { label: 'Image', width: 'w-[28%]' },
  { label: 'Version', width: 'w-[26%]' },
  { label: 'Update', width: 'w-[21%]' },
];

const VIEWS: SegmentOption<ViewMode>[] = [
  { value: 'list', label: 'List', icon: 'list' },
  { value: 'tiles', label: 'Tiles', icon: 'layout-grid' },
];

@Component({
  selector: 'tm-dashboard',
  imports: [
    DatePipe,
    NgTemplateOutlet,
    RouterLink,
    ContainerRuleProblems,
    ContainerTile,
    ContainerUpdate,
    ContainerVersion,
    TmAppIcon,
    TmBadge,
    TmButton,
    TmCallout,
    TmCard,
    TmEmptyState,
    TmSection,
    TmSegmented,
    TmSkeleton,
  ],
  templateUrl: './dashboard.html',
})
export class Dashboard {
  private readonly http = inject(HttpClient);
  private readonly toasts = inject(Toasts);

  protected readonly list = httpResource<ContainerList>(() => '/api/containers');

  protected readonly views = VIEWS;
  protected readonly columns = COLUMNS;
  protected readonly view = signal<ViewMode>(readStored(VIEW_KEY, ['list', 'tiles'], 'list'));
  protected readonly filter = signal<Filter>(readStored(FILTER_KEY, ['all', 'updates', 'breaking'], 'all'));
  protected readonly starting = signal(false);

  // value() throws while the resource is in an error state, so always go through hasValue().
  private readonly data = computed(() => (this.list.hasValue() ? this.list.value() : undefined));

  private readonly containers = computed(() => this.data()?.containers ?? []);
  /** Ignored containers leave their stack for the collapsed "Ignored" section at the bottom. */
  private readonly split = computed(() => splitIgnored(this.containers()));
  /** All stacks (for the summary), and the ones shown under the current filter. */
  protected readonly stacks = computed(() => groupByStack(this.containers()));
  protected readonly shownStacks = computed(() => {
    const filter = this.filter();
    return groupByStack(this.split().active.filter((c) => matchesFilter(c, filter)));
  });
  /** Ignored containers have no update, so they only show under "All". */
  protected readonly shownIgnored = computed(() => (this.filter() === 'all' ? this.split().ignored : []));
  protected readonly ignoredOpen = signal(false);
  protected readonly filters = computed<SegmentOption<Filter>[]>(() => {
    const updates = this.containers().filter(hasUpdate);
    return [
      { value: 'all', label: 'All' },
      { value: 'updates', label: 'Updates', count: updates.length },
      { value: 'breaking', label: 'Breaking', count: updates.filter((c) => c.update!.level === 'breaking').length },
    ];
  });
  protected readonly source = computed(() => this.data()?.source);
  protected readonly check = computed(() => this.data()?.check);
  protected readonly checking = computed(() => this.check()?.running ?? false);
  protected readonly rulesError = computed(() => this.check()?.rulesError);

  protected readonly summary = computed(() => {
    const containers = this.containers();
    const running = containers.filter((c) => c.state === 'running').length;
    const stacks = this.stacks().filter((s) => s.name !== null).length;
    const parts = [plural(containers.length, 'container'), plural(stacks, 'stack')];
    if (running !== containers.length) parts.push(`${containers.length - running} not running`);
    if (this.check()?.finishedAt && !this.checking()) {
      const updates = countUpdates(containers);
      parts.push(updates === 0 ? 'all up to date' : plural(updates, 'update'));
    }
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

  /** Times today show as "15:21"; other days add the date. */
  protected sameDay(iso: string): boolean {
    return new Date(iso).toDateString() === new Date().toDateString();
  }
  protected readonly plural = plural;

  constructor() {
    effect(() => store(VIEW_KEY, this.view()));
    effect(() => store(FILTER_KEY, this.filter()));

    // While a check runs, poll so results appear as they come in. Depend on the status
    // object (new on every response), not the boolean, which stays `true` between polls.
    effect((onCleanup) => {
      if (!this.check()?.running) return;
      const timer = setTimeout(() => this.list.reload(), POLL_MS);
      onCleanup(() => clearTimeout(timer));
    });

    // Tell the user when a run they were watching finishes.
    let wasChecking = false;
    effect(() => {
      const check = this.check();
      if (!check) return;
      if (wasChecking && !check.running) {
        const updates = countUpdates(this.data()?.containers ?? []);
        if (check.error) this.toasts.show(`Check failed: ${check.error}`, 'breaking');
        else this.toasts.show(updates ? `Check finished · ${plural(updates, 'update')}` : 'Check finished · all up to date', 'ok');
      }
      wasChecking = check.running;
    });
  }

  protected async checkNow(): Promise<void> {
    this.starting.set(true);
    try {
      await firstValueFrom(this.http.post('/api/checks', null));
      this.list.reload();
    } catch {
      this.toasts.show("Couldn't start a check. Is the server running?", 'attention');
    } finally {
      this.starting.set(false);
    }
  }
}

function readStored<T extends string>(key: string, allowed: T[], fallback: T): T {
  try {
    const stored = localStorage.getItem(key) as T | null;
    if (stored && allowed.includes(stored)) return stored;
  } catch {
    // ignore
  }
  return fallback;
}

function store(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    // storage blocked: the choice just won't persist
  }
}

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? '' : 's'}`;
}
