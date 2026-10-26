import { Component, computed, effect, inject, linkedSignal, signal, untracked, viewChild } from '@angular/core';
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
  TmSearch,
  TmSection,
  TmSegmented,
  TmSkeleton,
  TmTideGauge,
  TmTooltip,
  Toasts,
} from '../ui';
import { ContainerColumnBar } from './column-bar/column-bar';
import { DashboardSkeleton } from './dashboard-skeleton/dashboard-skeleton';
import { ContainerTable } from './container-table/container-table';
import { ContainerTile } from './container-tile/container-tile';
import {
  checkFailed,
  ContainerList,
  ContainerSummary,
  countOpenUpdates,
  Filter,
  Grouping,
  groupBySeverity,
  groupByStack,
  hasResult,
  hasUpdate,
  isCurrent,
  isOpenUpdate,
  isReviewed,
  matchesFilter,
  orderStacks,
  REVIEW_ICONS,
  splitIgnored,
  splitSnoozed,
} from './containers';
import { DashboardState } from './dashboard-state';

export type ViewMode = 'list' | 'tiles';

const VIEW_KEY = 'tidemark.view';
const FILTER_KEY = 'tidemark.filter';
const GROUP_KEY = 'tidemark.group';
/** Session only: survives a trip to a detail page, but a reload later starts unfiltered. */
const QUERY_KEY = 'tidemark.query';
const POLL_MS = 1500;
/** Without a running check: slow, so scheduled and startup checks show up without a reload. */
const IDLE_POLL_MS = 60_000;

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
    DashboardSkeleton,
    TmBadge,
    TmButton,
    TmCallout,
    TmCard,
    TmEmptyState,
    TmIcon,
    TmSearch,
    TmSection,
    TmSegmented,
    TmSkeleton,
    TmTideGauge,
    TmTooltip,
  ],
  templateUrl: './dashboard.html',
  host: { '(document:keydown)': 'onKeydown($event)' },
})
export class Dashboard {
  private readonly http = inject(HttpClient);
  private readonly toasts = inject(Toasts);
  private readonly state = inject(DashboardState);

  protected readonly list = httpResource<ContainerList>(() => '/api/containers');

  protected readonly views = VIEWS;
  protected readonly view = signal<ViewMode>(readStored(VIEW_KEY, ['list', 'tiles'], 'list'));
  protected readonly filter = signal<Filter>(readStored(FILTER_KEY, ['all', 'updates', 'breaking'], 'all'));
  protected readonly groups = GROUPS;
  protected readonly group = signal<Grouping>(readStored(GROUP_KEY, ['stack', 'severity'], 'stack'));
  protected readonly query = signal(readSession(QUERY_KEY));
  private readonly search = viewChild(TmSearch);
  protected readonly starting = signal(false);

  // value() throws while the resource is in an error state, so always go through hasValue().
  // A linkedSignal can see its previous value: a failed refresh keeps the last good list
  // (shown with a callout) instead of emptying the page. Before the first response it's the list
  // from the last visit (DashboardState), so coming back from a detail page shows no skeleton.
  protected readonly data = linkedSignal<ContainerList | undefined, ContainerList | undefined>({
    source: () => (this.list.hasValue() ? this.list.value() : undefined),
    computation: (list, previous) => list ?? previous?.value ?? untracked(this.state.list),
  });

  private readonly containers = computed(() => this.data()?.containers ?? []);
  protected readonly containerCount = computed(() => this.containers().length);
  /**
   * Ignored and snoozed containers leave their stack for the collapsed "Ignored" and "Snoozed"
   * sections at the bottom; `active` is the rest.
   */
  private readonly split = computed(() => {
    const { active, ignored } = splitIgnored(this.containers());
    return { ...splitSnoozed(active), ignored };
  });
  /** All stacks (for the summary). */
  protected readonly stacks = computed(() => groupByStack(this.containers()));
  /** The search text, trimmed and lower-cased; empty means no search. */
  private readonly needle = computed(() => this.query().trim().toLowerCase());
  protected readonly searchText = computed(() => this.query().trim());
  /** Containers outside Ignored and Snoozed, under the current filter and search. */
  private readonly shown = computed(() => {
    const filter = this.filter();
    const query = this.needle();
    return this.split().active.filter((c) => matchesFilter(c, filter) && matchesQuery(c, query));
  });
  /** Stack mode: stacks that need a look first, fully up-to-date stacks after. */
  private readonly shownStacks = computed(() => orderStacks(this.shown()));
  /**
   * Stack mode: settled stacks (every container up to date or reviewed) go together into one
   * collapsed "Up to date" section, under "All" only: the Updates and Breaking filters show
   * reviewed updates on purpose, so there every stack keeps its own section (settled ones last).
   */
  private readonly collectSettled = computed(() => this.filter() === 'all');
  /** Stack mode: the stacks shown as their own sections. */
  protected readonly workStacks = computed(() => this.shownStacks().filter((s) => !this.collectSettled() || !s.current));
  /** Stack mode: the stacks inside "Up to date". */
  protected readonly settledStacks = computed(() => (this.collectSettled() ? this.shownStacks().filter((s) => s.current) : []));
  /** Severity mode: Breaking, Attention, Updates, Check failed, Not checked yet, Up to date. */
  protected readonly shownSeverity = computed(() => groupBySeverity(this.shown()));
  /**
   * "Up to date" is all the main area shows (everything is up to date, or a search only matches
   * up-to-date containers): the collapsed section opens (see the effect below), so the page
   * isn't empty and the matches don't hide behind it.
   */
  private readonly onlyUpToDate = computed(() =>
    this.group() === 'stack'
      ? this.workStacks().length === 0 && this.settledStacks().length > 0
      : this.shownSeverity().length > 0 && this.shownSeverity().every((g) => g.key === 'current'),
  );
  /** Ignored containers have no update, so they only show under "All". */
  protected readonly shownIgnored = computed(() => {
    const query = this.needle();
    return this.filter() === 'all' ? this.split().ignored.filter((c) => matchesQuery(c, query)) : [];
  });
  /** Snoozed updates are put off, so like ignored ones they only show under "All". */
  protected readonly shownSnoozed = computed(() => {
    const query = this.needle();
    return this.filter() === 'all' ? this.split().snoozed.filter((c) => matchesQuery(c, query)) : [];
  });
  protected readonly nothingShown = computed(
    () => this.shown().length === 0 && this.shownSnoozed().length === 0 && this.shownIgnored().length === 0,
  );
  /** The "Up to date" section (severity mode's group, stack mode's settled stacks): collapsed by default, like "Ignored", unless it's all there is. One signal, so it stays open or closed across both groupings. */
  protected readonly currentOpen = this.state.currentOpen;
  protected readonly snoozedOpen = this.state.snoozedOpen;
  protected readonly ignoredOpen = this.state.ignoredOpen;
  protected readonly reviewIcons = REVIEW_ICONS;
  /** Counts include reviewed updates (they stay in place), not snoozed or ignored ones. */
  protected readonly filters = computed<SegmentOption<Filter>[]>(() => {
    const updates = this.split().active.filter(hasUpdate);
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
    const reviewed = this.split().active.filter(isReviewed).length;
    if (reviewed) parts.push(`${reviewed} reviewed`);
    const snoozed = this.split().snoozed.length;
    if (snoozed) parts.push(`${snoozed} snoozed`);
    const ignored = this.split().ignored.length;
    if (ignored) parts.push(`${ignored} ignored`);
    return parts.join(' · ');
  });

  /**
   * Hero figures. Ignored containers aren't checked, so they count nowhere. Breaking, Attention
   * and Updates count open updates only (not reviewed or snoozed); the gauge's current/checked
   * cover every checked container, snoozed ones included.
   */
  protected readonly stats = computed(() => {
    const { active, snoozed } = this.split();
    const checkedOnes = [...active, ...snoozed];
    const open = active.filter(isOpenUpdate);
    return {
      breaking: open.filter((c) => c.update!.level === 'breaking').length,
      attention: open.filter((c) => c.update!.level === 'attention').length,
      updates: open.length,
      /** Updates that are reviewed or snoozed: not open, but not up to date either. */
      settled: active.filter(isReviewed).length + snoozed.length,
      current: checkedOnes.filter(isCurrent).length,
      /** Containers with a result from a successful check. */
      checked: checkedOnes.filter((c) => hasResult(c.update)).length,
    };
  });

  /**
   * The hero's big line: progress while checking, otherwise the number of open updates.
   * "No open updates" when the only ones left are reviewed or snoozed, so it never claims
   * everything is up to date when it isn't.
   */
  protected readonly headline = computed(() => {
    const check = this.check();
    if (check?.running) return `Checking ${check.done}/${check.total}`;
    const { updates, settled, checked } = this.stats();
    if (updates) return plural(updates, 'update');
    if (settled) return 'No open updates';
    if (checked) return 'All up to date';
    return plural(this.containers().length, 'container');
  });

  /** Gauge: up to date ÷ checked (1 while nothing is checked, so it doesn't read as "far behind"). */
  protected readonly gauge = computed(() => {
    const { current, checked } = this.stats();
    return {
      level: checked ? current / checked : 1,
      label: checked ? `${current} of ${plural(checked, 'checked container')} up to date` : 'No containers checked yet',
      /** Why the total can be lower than the container count. */
      note: 'Counts checked containers only: ignored ones and ones not checked yet are left out.',
    };
  });

  /** The empty state when the filter leaves nothing; snoozed updates only show under "All". */
  protected readonly emptyFilter = computed(() => {
    const breaking = this.filter() === 'breaking';
    const snoozed = this.split().snoozed.length > 0;
    let description: string;
    if (breaking) description = `None of the available updates look breaking.${snoozed ? ' Snoozed ones show under All.' : ''}`;
    else if (this.checking()) description = 'Still checking. Updates show up here as they are found.';
    else if (snoozed) description = 'Nothing open. Snoozed updates show under All.';
    else description = 'Every container runs the latest version.';
    return { heading: breaking ? 'Nothing breaking' : 'No updates', description };
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
    effect(() => storeSession(QUERY_KEY, this.query()));
    effect(() => {
      const data = this.data();
      if (data) this.state.list.set(data);
    });

    // Only "Up to date" to show: open it. Runs when that turns true (remembered across visits,
    // so coming back from a detail page doesn't reopen it); closing it again is up to the user.
    effect(() => {
      const only = this.onlyUpToDate();
      if (only && !this.state.onlyUpToDate) this.currentOpen.set(true);
      this.state.onlyUpToDate = only;
    });

    // While a check runs, poll fast so results appear as they come in; otherwise slowly.
    // Depend on the status object (new on every response), not the boolean, which stays
    // `true` between polls, and on the resource status, so a failed poll schedules the next.
    effect((onCleanup) => {
      this.list.status();
      const timer = setTimeout(() => this.list.reload(), this.check()?.running ? POLL_MS : IDLE_POLL_MS);
      onCleanup(() => clearTimeout(timer));
    });

    // Tell the user when a run they were watching finishes.
    let wasChecking = false;
    effect(() => {
      const check = this.check();
      if (!check) return;
      if (wasChecking && !check.running) {
        const containers = this.data()?.containers ?? [];
        const updates = countOpenUpdates(containers);
        // Only reviewed or snoozed updates left: don't claim everything is up to date.
        const rest = updates ? plural(updates, 'update') : containers.some(hasUpdate) ? 'no open updates' : 'all up to date';
        if (check.error) this.toasts.show(`Check failed: ${check.error}`, 'breaking');
        else this.toasts.show(`Check finished · ${rest}`, 'ok');
      }
      wasChecking = check.running;
    });
  }

  /** "/" jumps to the search field, unless the user is typing somewhere. */
  protected onKeydown(event: KeyboardEvent): void {
    if (event.key !== '/' || event.ctrlKey || event.metaKey || event.altKey || event.defaultPrevented) return;
    const target = event.target;
    if (target instanceof HTMLElement) {
      if (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) return;
    }
    const search = this.search();
    if (!search) return;
    event.preventDefault();
    search.focus();
  }

  protected async checkNow(): Promise<void> {
    this.starting.set(true);
    try {
      await firstValueFrom(this.http.post('/api/checks', null));
      this.list.reload();
    } catch (err) {
      const reason = err instanceof HttpErrorResponse ? (err.error as { error?: string } | null)?.error : undefined;
      this.toasts.show(reason ? `Couldn't start a check: ${reason}` : "Couldn't start a check. Is the server running?", 'attention');
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

function readSession(key: string): string {
  try {
    return sessionStorage.getItem(key) ?? '';
  } catch {
    return '';
  }
}

function storeSession(key: string, value: string): void {
  try {
    sessionStorage.setItem(key, value);
  } catch {
    // storage blocked: the search just won't survive navigation
  }
}

/** Case-insensitive substring match on name, image (display and ref) and compose stack. `needle` is lower-cased and trimmed. */
function matchesQuery(c: ContainerSummary, needle: string): boolean {
  if (!needle) return true;
  return [c.name, c.image.display, c.image.ref, c.stack ?? ''].some((s) => s.toLowerCase().includes(needle));
}

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? '' : 's'}`;
}
