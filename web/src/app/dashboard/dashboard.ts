import { Component, computed, effect, inject, signal } from '@angular/core';
import { DatePipe, NgTemplateOutlet } from '@angular/common';
import { HttpClient, HttpErrorResponse, httpResource } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';
import {
  SegmentOption,
  TmBadge,
  TmButton,
  TmCallout,
  TmCard,
  TmEmptyState,
  TmIcon,
  TmSection,
  TmSegmented,
  TmSkeleton,
  TmTideGauge,
  TmTooltip,
  Toasts,
} from '../ui';
import { ContainerColumnBar, ContainerTable } from './container-table';
import { ContainerTile } from './container-tile';
import {
  checkFailed,
  ContainerList,
  countUpdates,
  Filter,
  Grouping,
  groupBySeverity,
  groupByStack,
  hasResult,
  hasUpdate,
  isCurrent,
  matchesFilter,
  orderStacks,
  splitIgnored,
} from './containers';

export type ViewMode = 'list' | 'tiles';

const VIEW_KEY = 'tidemark.view';
const FILTER_KEY = 'tidemark.filter';
const GROUP_KEY = 'tidemark.group';
const POLL_MS = 1500;

const VIEWS: SegmentOption<ViewMode>[] = [
  { value: 'list', label: 'List', icon: 'list' },
  { value: 'tiles', label: 'Tiles', icon: 'layout-grid' },
];

const GROUPS: SegmentOption<Grouping>[] = [
  { value: 'stack', label: 'Stack', icon: 'layers' },
  { value: 'severity', label: 'Severity', icon: 'arrow-down-wide-narrow' },
];

@Component({
  selector: 'tm-dashboard',
  imports: [
    DatePipe,
    NgTemplateOutlet,
    ContainerColumnBar,
    ContainerTable,
    ContainerTile,
    TmBadge,
    TmButton,
    TmCallout,
    TmCard,
    TmEmptyState,
    TmIcon,
    TmSection,
    TmSegmented,
    TmSkeleton,
    TmTideGauge,
    TmTooltip,
  ],
  templateUrl: './dashboard.html',
})
export class Dashboard {
  private readonly http = inject(HttpClient);
  private readonly toasts = inject(Toasts);

  protected readonly list = httpResource<ContainerList>(() => '/api/containers');

  protected readonly views = VIEWS;
  protected readonly view = signal<ViewMode>(readStored(VIEW_KEY, ['list', 'tiles'], 'list'));
  protected readonly filter = signal<Filter>(readStored(FILTER_KEY, ['all', 'updates', 'breaking'], 'all'));
  protected readonly groups = GROUPS;
  protected readonly group = signal<Grouping>(readStored(GROUP_KEY, ['stack', 'severity'], 'stack'));
  protected readonly starting = signal(false);

  // value() throws while the resource is in an error state, so always go through hasValue().
  private readonly data = computed(() => (this.list.hasValue() ? this.list.value() : undefined));

  private readonly containers = computed(() => this.data()?.containers ?? []);
  protected readonly containerCount = computed(() => this.containers().length);
  /** Ignored containers leave their stack for the collapsed "Ignored" section at the bottom. */
  private readonly split = computed(() => splitIgnored(this.containers()));
  /** All stacks (for the summary). */
  protected readonly stacks = computed(() => groupByStack(this.containers()));
  /** Non-ignored containers under the current filter. */
  private readonly shown = computed(() => {
    const filter = this.filter();
    return this.split().active.filter((c) => matchesFilter(c, filter));
  });
  /** Stack mode: stacks that need a look first, fully up-to-date stacks after. */
  protected readonly shownStacks = computed(() => orderStacks(this.shown()));
  /** Severity mode: Breaking, Attention, Updates, Check failed, Not checked yet, Up to date. */
  protected readonly shownSeverity = computed(() => groupBySeverity(this.shown()));
  /** Ignored containers have no update, so they only show under "All". */
  protected readonly shownIgnored = computed(() => (this.filter() === 'all' ? this.split().ignored : []));
  protected readonly nothingShown = computed(() => this.shown().length === 0 && this.shownIgnored().length === 0);
  /** Severity mode's "Up to date" section: collapsed by default, like "Ignored". */
  protected readonly currentOpen = signal(false);
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
  /** Containers whose latest check attempt failed (an earlier result may still be shown). */
  protected readonly failedCount = computed(() => this.containers().filter(checkFailed).length);

  protected readonly summary = computed(() => {
    const containers = this.containers();
    const running = containers.filter((c) => c.state === 'running').length;
    const stacks = this.stacks().filter((s) => s.name !== null).length;
    const parts = [plural(containers.length, 'container'), plural(stacks, 'stack')];
    if (running !== containers.length) parts.push(`${containers.length - running} not running`);
    return parts.join(' · ');
  });

  /** Hero figures. Ignored containers aren't checked, so they count nowhere. */
  protected readonly stats = computed(() => {
    const active = this.split().active;
    const updates = active.filter(hasUpdate);
    return {
      breaking: updates.filter((c) => c.update!.level === 'breaking').length,
      attention: updates.filter((c) => c.update!.level === 'attention').length,
      updates: updates.length,
      current: active.filter(isCurrent).length,
      /** Containers with a result from a successful check. */
      checked: active.filter((c) => hasResult(c.update)).length,
    };
  });

  /** The hero's big line: progress while checking, otherwise the number of updates. */
  protected readonly headline = computed(() => {
    const check = this.check();
    if (check?.running) return `Checking ${check.done}/${check.total}`;
    const { updates, checked } = this.stats();
    if (updates) return plural(updates, 'update');
    if (checked) return 'All up to date';
    return plural(this.containers().length, 'container');
  });

  /** Gauge: up to date ÷ checked (1 while nothing is checked, so it doesn't read as "far behind"). */
  protected readonly gauge = computed(() => {
    const { current, checked } = this.stats();
    return {
      level: checked ? current / checked : 1,
      label: checked ? `${current} of ${plural(checked, 'checked container')} up to date` : 'No containers checked yet',
    };
  });

  protected readonly errorMessage = computed(() => {
    const err = this.list.error();
    if (err instanceof HttpErrorResponse) {
      return (err.error as { error?: string } | null)?.error ?? `The API answered ${err.status} ${err.statusText}.`;
    }
    return err ? String(err) : '';
  });

  /** Times today show as "15:21"; other days add the date. */
  protected sameDay(iso: string): boolean {
    return new Date(iso).toDateString() === new Date().toDateString();
  }
  protected readonly plural = plural;

  constructor() {
    effect(() => store(VIEW_KEY, this.view()));
    effect(() => store(FILTER_KEY, this.filter()));
    effect(() => store(GROUP_KEY, this.group()));

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
