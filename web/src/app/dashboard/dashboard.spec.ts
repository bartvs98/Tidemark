import { ApplicationRef } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { Dashboard } from './dashboard';
import { reviewedTitle, snoozedText } from './format';
import {
  ContainerList,
  ContainerSummary,
  groupBySeverity,
  groupByStack,
  hasResult,
  hasUpdate,
  imageName,
  isCurrent,
  isOpenUpdate,
  matchesFilter,
  orderStacks,
  refreshFailed,
  ReviewState,
  isBuildKey,
  splitIgnored,
  splitSnoozed,
  UpdateInfo,
} from './containers';

function container(name: string, stack?: string, extra: Partial<ContainerSummary> = {}): ContainerSummary {
  return {
    id: name,
    name,
    image: {
      ref: `${name}:1.0`,
      display: `${name}:1.0`,
      registry: 'docker.io',
      repository: `library/${name}`,
      tag: '1.0',
      id: 'sha256:x',
    },
    state: 'running',
    status: 'Up 2 days',
    created: '2026-10-01T00:00:00Z',
    stack,
    service: stack ? name : undefined,
    ...extra,
  };
}

describe('groupByStack', () => {
  it('sorts stacks and containers by name, ungrouped last', () => {
    const stacks = groupByStack([
      container('zed'),
      container('sonarr', 'media'),
      container('adguard', 'networking'),
      container('jellyfin', 'media'),
      container('alpha'),
    ]);
    expect(stacks.map((s) => s.name)).toEqual(['media', 'networking', null]);
    expect(stacks[0].containers.map((c) => c.name)).toEqual(['jellyfin', 'sonarr']);
    expect(stacks[2].containers.map((c) => c.name)).toEqual(['alpha', 'zed']);
  });
});

describe('splitIgnored', () => {
  it('takes ignored containers out of their stacks, sorted by name', () => {
    const { active, ignored } = splitIgnored([
      container('zed', 'media', { rules: { ignore: true, from: { ignore: 'label' } } }),
      container('sonarr', 'media'),
      container('alpha', undefined, { rules: { ignore: true, from: { ignore: 'config' } } }),
      container('pinned', 'media', { rules: { pin: '1.4', from: { pin: 'label' } } }),
    ]);
    expect(active.map((c) => c.name)).toEqual(['sonarr', 'pinned']);
    expect(ignored.map((c) => c.name)).toEqual(['alpha', 'zed']);
  });

  it('never counts an ignored container as an update', () => {
    const u: UpdateInfo = { running: '1', source: 'tag', latest: '2', class: 'major', floating: false, newImage: false, checkedAt: '2026-10-16T13:00:00Z', level: 'breaking' };
    expect(hasUpdate(container('a', undefined, { update: u }))).toBe(true);
    expect(hasUpdate(container('a', undefined, { update: u, rules: { ignore: true, from: {} } }))).toBe(false);
  });
});

describe('imageName', () => {
  it('drops library/ for Docker Hub and keeps other registries', () => {
    expect(imageName(container('redis'))).toBe('redis');
    const ghcr = container('cup', undefined, {
      image: { ...container('cup').image, registry: 'ghcr.io', repository: 'sergi0g/cup' },
    });
    expect(imageName(ghcr)).toBe('ghcr.io/sergi0g/cup');
  });
});

/** A result from a successful check: up to date (class none) or an update at the given level. */
function checked(cls: UpdateInfo['class'], level: UpdateInfo['level'] = 'none', extra: Partial<UpdateInfo> = {}): UpdateInfo {
  return {
    running: '1.0.0', source: 'tag', latest: cls === 'none' ? '1.0.0' : '2.0.0', class: cls, floating: false, newImage: false,
    checkedAt: '2026-10-16T13:00:00Z', level, ...extra,
  };
}
const neverChecked: UpdateInfo = {
  source: 'unknown', class: 'none', floating: false, newImage: false, level: 'none', error: 'no such host', failedAt: '2026-10-16T13:00:00Z',
};

describe('groupBySeverity', () => {
  it('orders sections by severity, leaves out empty ones and sorts containers by name', () => {
    const groups = groupBySeverity([
      container('zz-current', 'a', { update: checked('none') }),
      container('patchy', 'a', { update: checked('patch') }),
      container('jelly', 'b', { update: checked('major', 'breaking') }),
      container('abc', 'b', { update: checked('major', 'breaking') }),
      container('never', 'b', { update: neverChecked }),
      container('pending', 'b'),
      container('kept', 'c', { update: checked('none', 'none', { error: 'timeout', failedAt: '2026-10-16T14:00:00Z' }) }),
      container('ignored', 'c', { rules: { ignore: true, from: { ignore: 'label' } } }),
    ]);
    expect(groups.map((g) => [g.key, g.heading, g.icon])).toEqual([
      ['breaking', 'Breaking', 'octagon-alert'],
      ['updates', 'Updates', 'git-compare'],
      ['failed', 'Check failed', 'circle-help'],
      ['unchecked', 'Not checked yet', 'loader'],
      ['current', 'Up to date', 'circle-check'],
    ]);
    expect(groups[0].containers.map((c) => c.name)).toEqual(['abc', 'jelly']);
    // A kept result that's up to date stays up to date; only "never checked" is "Check failed".
    expect(groups[4].containers.map((c) => c.name)).toEqual(['kept', 'zz-current']);
  });

  it('puts attention between breaking and plain updates', () => {
    const groups = groupBySeverity([
      container('a', undefined, { update: checked('minor', 'attention') }),
      container('b', undefined, { update: checked('minor') }),
    ]);
    expect(groups.map((g) => g.key)).toEqual(['attention', 'updates']);
  });
});

describe('orderStacks', () => {
  it('puts stacks that need a look first and fully up-to-date stacks after, each A→Z', () => {
    const stacks = orderStacks([
      container('a1', 'alpha', { update: checked('none') }),
      container('z1', 'zulu', { update: checked('patch') }),
      container('z2', 'zulu', { update: checked('none') }),
      container('m1', 'mike', { update: neverChecked }),
      container('b1', 'bravo', { update: checked('none') }),
      container('u1', undefined, { update: checked('none') }),
      container('p1', 'papa'), // not checked yet: no result, so not "up to date"
    ]);
    expect(stacks.map((s) => [s.name, s.current])).toEqual([
      ['mike', false],
      ['papa', false],
      ['zulu', false],
      ['alpha', true],
      ['bravo', true],
      [null, true],
    ]);
  });

  it('only calls a checked container without an update current', () => {
    expect(isCurrent(container('a', undefined, { update: checked('none') }))).toBe(true);
    expect(isCurrent(container('a', undefined, { update: checked('patch') }))).toBe(false);
    expect(isCurrent(container('a', undefined, { update: neverChecked }))).toBe(false);
    expect(isCurrent(container('a'))).toBe(false);
    expect(isCurrent(container('a', undefined, { update: checked('none'), rules: { ignore: true, from: {} } }))).toBe(false);
  });
});

const reviewed = (version = '2.0.0'): ReviewState => ({ kind: 'reviewed', version, at: '2026-10-20T09:00:00Z' });
const snoozedUntil = (until = '2026-11-03T09:00:00Z'): ReviewState => ({ kind: 'snoozed', until, at: '2026-10-20T09:00:00Z' });
const snoozedVersion = (version = '2.0.0'): ReviewState => ({ kind: 'snoozed', version, at: '2026-10-20T09:00:00Z' });

describe('review helpers', () => {
  it('words a review against a build (a digest) without showing the digest', () => {
    const build = 'sha256:3f9a1c0d2b7e8a41';
    expect(isBuildKey(build)).toBe(true);
    expect(isBuildKey('2.0.0')).toBe(false);
    expect(reviewedTitle(reviewed(build), 'en-US')).toBe('Reviewed this build on 20 Oct. It shows as new again when a new build comes out.');
    const z = snoozedText(snoozedVersion(build), 'en-US');
    expect(z.text).toBe('until a new build');
    expect(z.value).toBeUndefined();
    expect(z.title).not.toContain('sha256');
  });

  it('keeps reviewed updates in the filters and stack order as settled, and splits snoozed ones off', () => {
    const rev = container('rev', 'a', { update: checked('major', 'breaking'), review: reviewed() });
    const snz = container('snz', 'b', { update: checked('major', 'breaking'), review: snoozedUntil() });
    const open = container('open', 'c', { update: checked('patch') });
    expect([rev, snz, open].map(isOpenUpdate)).toEqual([false, false, true]);
    expect(matchesFilter(rev, 'breaking')).toBe(true);
    expect(matchesFilter(snz, 'breaking')).toBe(false);
    expect(matchesFilter(snz, 'updates')).toBe(false);
    expect(matchesFilter(snz, 'all')).toBe(true);

    const { active, snoozed } = splitSnoozed([snz, rev, open]);
    expect(active.map((c) => c.name)).toEqual(['rev', 'open']);
    expect(snoozed.map((c) => c.name)).toEqual(['snz']);

    // A stack whose only update is reviewed counts as nothing to look at.
    const stacks = orderStacks([rev, container('cur', 'a', { update: checked('none') }), open]);
    expect(stacks.map((s) => [s.name, s.current])).toEqual([
      ['c', false],
      ['a', true],
    ]);
    // Reviewed stays in its level's section; snoozed is left out.
    expect(groupBySeverity([rev, snz, open]).map((g) => [g.key, g.containers.map((c) => c.name)])).toEqual([
      ['breaking', ['rev']],
      ['updates', ['open']],
    ]);
  });
});

describe('Dashboard', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    });
    http = TestBed.inject(HttpTestingController);
  });

  async function render(respond: (req: ReturnType<HttpTestingController['expectOne']>) => void) {
    const fixture = TestBed.createComponent(Dashboard);
    fixture.detectChanges();
    respond(http.expectOne('/api/containers'));
    await fixture.whenStable();
    return fixture.nativeElement as HTMLElement;
  }

  it('lists containers grouped by stack with a summary', async () => {
    const body: ContainerList = {
      source: { kind: 'snapshot', location: 'snapshots/home.json', capturedAt: '2026-10-16T12:38:39Z' },
      containers: [
        container('jellyfin', 'media'),
        container('sonarr', 'media', { state: 'exited', status: 'Exited (0) 2 days ago' }),
        container('pihole'),
      ],
    };
    const el = await render((req) => req.flush(body));

    expect(el.querySelector('[data-testid="summary"]')?.textContent).toContain('3 containers · 1 stack · 1 not running');
    const sections = [...el.querySelectorAll('tm-section')];
    expect(sections.map((s) => s.querySelector('h2')?.textContent?.trim())).toEqual(['media', 'Ungrouped']);
    expect(sections[0].textContent).toContain('2 containers');
    expect(sections[1].textContent).toContain('1 container');
    // One pinned column bar for all stacks; each table keeps headers for screen readers.
    expect(el.querySelectorAll('[data-testid="column-bar"]').length).toBe(1);
    const tableHeads = [...el.querySelectorAll('tm-section thead')];
    expect(tableHeads.length).toBe(2);
    expect(tableHeads.every((h) => h.classList.contains('sm:sr-only'))).toBe(true);
    expect(el.textContent).toContain('exited');
    expect(el.querySelector('a[href="/containers/jellyfin"]')).not.toBeNull();
    expect(el.textContent).toContain('Snapshot');
  });

  it('comes back from a detail page with the last list and open sections, while it reloads', async () => {
    const body: ContainerList = { source: { kind: 'docker', location: 'x' }, containers: [container('jellyfin', 'media'), container('pihole')] };
    const first = TestBed.createComponent(Dashboard);
    first.detectChanges();
    http.expectOne('/api/containers').flush(body);
    await first.whenStable();
    first.componentInstance['ignoredOpen'].set(true);
    first.destroy();

    // The next visit renders the stored list at once (no skeleton), so the router can restore the scroll position.
    const again = TestBed.createComponent(Dashboard);
    again.detectChanges();
    TestBed.tick(); // whenStable() would wait for the pending reload
    const el = again.nativeElement as HTMLElement;
    expect(el.querySelector('tm-skeleton')).toBeNull();
    expect(el.querySelector('a[href="/containers/jellyfin"]')).not.toBeNull();
    expect(again.componentInstance['ignoredOpen']()).toBe(true);

    http.expectOne('/api/containers').flush({ ...body, containers: [container('jellyfin', 'media')] });
    await again.whenStable();
    expect(el.querySelector('a[href="/containers/pihole"]')).toBeNull();
  });

  it('shows app icons, with a letter avatar when there is none', async () => {
    const body: ContainerList = {
      source: { kind: 'docker', location: 'unix:///var/run/docker.sock' },
      containers: [container('jellyfin', 'media', { icon: { src: '/api/icons/jellyfin' } }), container('mkvtoolnix', 'media')],
    };
    const el = await render((req) => req.flush(body));
    const icons = [...el.querySelectorAll('tm-app-icon')];
    expect(icons[0].querySelector('img')?.getAttribute('src')).toBe('/api/icons/jellyfin');
    expect(icons[1].querySelector('img')).toBeNull();
    expect(icons[1].textContent?.trim()).toBe('M');
  });

  it('switches to tiles and remembers the choice', async () => {
    const body: ContainerList = {
      source: { kind: 'docker', location: 'unix:///var/run/docker.sock' },
      containers: [container('jellyfin', 'media'), container('sonarr', 'media'), container('pihole')],
    };
    const el = await render((req) => req.flush(body));
    expect(el.querySelectorAll('tm-container-tile').length).toBe(0);

    el.querySelector<HTMLButtonElement>('button[aria-label="Tiles"]')!.click();
    await TestBed.inject(ApplicationRef).whenStable();

    expect(el.querySelectorAll('table').length).toBe(0);
    expect(el.querySelectorAll('tm-container-tile').length).toBe(3);
    expect(localStorage.getItem('tidemark.view')).toBe('tiles');
  });

  it('shows versions, update classes and failures from the last check', async () => {
    const body: ContainerList = {
      source: { kind: 'docker', location: 'unix:///var/run/docker.sock' },
      check: { running: false, done: 3, total: 3, finishedAt: '2026-10-16T13:00:00Z', nextRunAt: '2026-10-16T18:00:00Z' },
      containers: [
        container('jellyfin', 'media', {
          update: { running: '10.9.11', source: 'label', latest: '10.10.7', class: 'minor', floating: true, newImage: true, checkedAt: '2026-10-16T13:00:00Z', level: 'none' },
        }),
        container('sonarr', 'media', {
          update: { running: '4.0.14', source: 'tag', latest: '4.0.14', class: 'none', floating: false, newImage: false, checkedAt: '2026-10-16T13:00:00Z', level: 'none' },
        }),
        container('private', 'media', {
          update: { source: 'unknown', class: 'none', floating: false, newImage: false, error: 'not found, or private', failedAt: '2026-10-16T13:00:00Z', level: 'none' },
        }),
      ],
    };
    const el = await render((req) => req.flush(body));
    const rows = [...el.querySelectorAll('tbody tr')].map((r) => r.textContent?.replace(/s+/g, ' ').trim() ?? '');
    expect(rows[0].replace(/ /g, '')).toContain('10.9.11→10.10.7');
    expect(rows[0]).toContain('detected from image label');
    expect(rows[0]).toContain('minor');
    expect(rows[1]).toContain('check failed'); // rows sort by name: jellyfin, private, sonarr
    expect(rows[2]).toContain('Up to date');
    expect(el.querySelector('[data-testid="headline"]')?.textContent?.trim()).toBe('1 update');
    expect(el.querySelector('[data-testid="next-run"]')?.textContent).toContain('next');
  });

  it('shows the risk level with its reasons, and filters to updates or breaking ones', async () => {
    const update = (cls: 'none' | 'minor' | 'major', level: 'none' | 'attention' | 'breaking', reasons?: UpdateInfo['reasons']) => ({
      running: '1.0.0', source: 'tag' as const, latest: cls === 'none' ? '1.0.0' : '2.0.0', class: cls,
      floating: false, newImage: false, checkedAt: '2026-10-16T13:00:00Z', level, reasons,
    });
    const body: ContainerList = {
      source: { kind: 'docker', location: 'x' },
      check: { running: false, done: 3, total: 3, finishedAt: '2026-10-16T13:00:00Z' },
      containers: [
        container('jellyfin', 'media', { update: update('major', 'breaking', [{ text: 'Major version: 1.0.0 → 2.0.0' }]) }),
        container('sonarr', 'media', {
          update: update('minor', 'attention', [
            { text: 'settings have been removed', tag: '2.0.0', line: '- settings have been removed' },
            { text: 'x'.repeat(200), tag: '1.9.0', line: 'x' },
            { text: 'third', tag: '1.9.0', line: '- third' },
            { text: 'fourth', tag: '1.9.0', line: '- fourth' },
            { text: 'fifth', tag: '1.8.0', line: '- fifth' },
          ]),
        }),
        container('mkvtoolnix', 'files', { update: update('none', 'none') }),
      ],
    };
    const el = await render((req) => req.flush(body));
    const rowNames = () => [...el.querySelectorAll('tbody tr a')].map((a) => a.textContent?.trim());

    const levels = [...el.querySelectorAll('[data-testid="level"]')];
    expect(levels.map((l) => l.textContent?.trim())).toEqual(['Breaking', 'Attention']); // mkvtoolnix is up to date: no badge
    expect(levels[0].getAttribute('data-tooltip')).toBe('Major version: 1.0.0 → 2.0.0');
    // Quotes carry their release; long lines are cut short for the small tooltip.
    // The detail page lists every quote; the tooltip the first three, then a count.
    const [quote, long, third, more] = levels[1].getAttribute('data-tooltip')!.split('\n');
    expect(third).toBe('1.9.0: third');
    expect(more).toBe('and 2 more flagged lines');
    expect(quote).toBe('2.0.0: settings have been removed');
    expect(long.length).toBe(140);
    expect(long.startsWith('1.9.0: xxx')).toBe(true);
    expect(long.endsWith('…')).toBe(true);

    const buttons = [...el.querySelectorAll<HTMLButtonElement>('[data-testid="filter"] button')];
    expect(buttons.map((b) => b.textContent?.replace(/\s+/g, ' ').trim())).toEqual(['All', 'Updates 2', 'Breaking 1']);
    expect(rowNames()).toEqual(['jellyfin', 'sonarr']); // the up-to-date stack waits in the collapsed "Up to date" section

    buttons[2].click();
    await TestBed.inject(ApplicationRef).whenStable();
    expect(rowNames()).toEqual(['jellyfin']);
    expect(localStorage.getItem('tidemark.filter')).toBe('breaking');

    buttons[1].click();
    await TestBed.inject(ApplicationRef).whenStable();
    expect(rowNames()).toEqual(['jellyfin', 'sonarr']);
  });

  describe('search', () => {
    function body(): ContainerList {
      return {
        source: { kind: 'docker', location: 'x' },
        containers: [
          container('jellyfin', 'media', { update: checked('major', 'breaking') }),
          container('sonarr', 'media', { update: checked('minor', 'attention') }),
          container('adguard', 'networking', { image: { ...container('x').image, ref: 'adguard/adguardhome:1', display: 'adguard/adguardhome:1' } }),
          container('filebrowser', 'tools', { rules: { ignore: true, from: { ignore: 'label' } } }),
        ],
      };
    }
    const settle = () => TestBed.inject(ApplicationRef).whenStable();
    const names = (el: HTMLElement) => [...el.querySelectorAll('tbody tr a')].map((a) => a.textContent?.trim());
    async function type(el: HTMLElement, text: string) {
      const input = el.querySelector<HTMLInputElement>('[data-testid="search"] input')!;
      input.value = text;
      input.dispatchEvent(new Event('input'));
      await settle();
    }

    it('filters by name, image and stack, ignoring case and surrounding whitespace', async () => {
      const el = await render((req) => req.flush(body()));
      await type(el, '  JELLY ');
      expect(names(el)).toEqual(['jellyfin']);
      await type(el, 'adguardhome');
      expect(names(el)).toEqual(['adguard']);
      await type(el, 'media');
      expect(names(el)).toEqual(['jellyfin', 'sonarr']);
      await type(el, '');
      expect(names(el).length).toBe(3);
    });

    it('combines with the Show filter and works in severity grouping and tiles', async () => {
      const el = await render((req) => req.flush(body()));
      await type(el, 'media');
      el.querySelector<HTMLButtonElement>('[data-testid="filter"] button:nth-child(3)')!.click(); // Breaking
      await settle();
      expect(names(el)).toEqual(['jellyfin']);

      el.querySelector<HTMLButtonElement>('[data-testid="filter"] button:nth-child(1)')!.click();
      el.querySelector<HTMLButtonElement>('[data-testid="group"] button:nth-child(2)')!.click();
      el.querySelector<HTMLButtonElement>('button[aria-label="Tiles"]')!.click();
      await settle();
      expect([...el.querySelectorAll('tm-container-tile')].length).toBe(2);
      expect(el.querySelector('[data-testid="severity-breaking"]')).not.toBeNull();
    });

    it('searches ignored containers too and leaves the hero figures alone', async () => {
      const el = await render((req) => req.flush(body()));
      const headline = el.querySelector('[data-testid="headline"]')!.textContent;
      await type(el, 'filebrowser');
      expect(el.querySelector('[data-testid="ignored-section"]')).not.toBeNull();
      expect(el.querySelector('[data-testid="no-match"]')).toBeNull();
      await type(el, 'jelly');
      expect(el.querySelector('[data-testid="ignored-section"]')).toBeNull();
      expect(el.querySelector('[data-testid="headline"]')!.textContent).toBe(headline);
      expect(el.querySelector('[data-testid="summary"]')!.textContent).toContain('4 containers');
    });

    it('shows an empty state with a way to clear the search', async () => {
      const el = await render((req) => req.flush(body()));
      await type(el, ' nope ');
      const empty = el.querySelector('[data-testid="no-match"]')!;
      expect(empty.textContent).toContain('No containers match "nope"');
      empty.querySelector<HTMLButtonElement>('button')!.click();
      await settle();
      expect(el.querySelector('[data-testid="no-match"]')).toBeNull();
      expect(names(el).length).toBe(3);
    });

    it('keeps the query for the session only', async () => {
      const el = await render((req) => req.flush(body()));
      await type(el, 'sonarr');
      expect(sessionStorage.getItem('tidemark.query')).toBe('sonarr');
      expect(localStorage.getItem('tidemark.query')).toBeNull();
    });

    it('restores the query from the session', async () => {
      sessionStorage.setItem('tidemark.query', 'sonarr');
      const el = await render((req) => req.flush(body()));
      expect(names(el)).toEqual(['sonarr']);
    });

    it('"/" focuses the field, except while typing in an input', async () => {
      const el = await render((req) => req.flush(body()));
      document.body.appendChild(el);
      const input = el.querySelector<HTMLInputElement>('[data-testid="search"] input')!;

      const slash = new KeyboardEvent('keydown', { key: '/', bubbles: true, cancelable: true });
      document.body.dispatchEvent(slash);
      expect(document.activeElement).toBe(input);
      expect(slash.defaultPrevented).toBe(true);

      const typing = new KeyboardEvent('keydown', { key: '/', bubbles: true, cancelable: true });
      input.dispatchEvent(typing);
      expect(typing.defaultPrevented).toBe(false);
      el.remove();
    });
  });

  describe('when a check fails', () => {
    const result = (extra: Partial<UpdateInfo> = {}): UpdateInfo => ({
      running: '1.0.0', source: 'tag', latest: '2.0.0', class: 'major', floating: false, newImage: false,
      checkedAt: '2026-10-15T23:00:00Z', level: 'breaking', reasons: [{ text: 'Major version: 1.0.0 → 2.0.0' }], ...extra,
    });
    const failed = { error: 'dial tcp: lookup registry-1.docker.io: no such host', failedAt: '2026-10-16T07:02:00Z' };
    const text = (e: Element | null | undefined) => e?.textContent?.replace(/\s+/g, ' ').trim();

    function list(): ContainerList {
      return {
        source: { kind: 'docker', location: 'x' },
        check: { running: false, done: 3, total: 3, finishedAt: '2026-10-16T07:02:00Z' },
        containers: [
          container('fresh', 'media', { update: result({ class: 'minor', level: 'none', reasons: undefined }) }),
          container('kept', 'media', { update: result(failed) }),
          container('never', 'media', { update: { source: 'unknown', class: 'none', floating: false, newImage: false, level: 'none', ...failed } }),
        ],
      };
    }

    it('keeps the last result with a quiet note, and shows "check failed" only without one', async () => {
      const el = await render((req) => req.flush(list()));
      const rows = [...el.querySelectorAll('tbody tr')]; // fresh, kept, never

      // Result: no failure note.
      expect(rows[0].querySelector('[data-testid="refresh-failed"]')).toBeNull();
      expect(text(rows[0])).toContain('minor');

      // Result + error: the result stays (versions, class, level), plus the note.
      expect(text(rows[1])?.replace(/ /g, '')).toContain('1.0.0→2.0.0');
      expect(text(rows[1].querySelector('[data-testid="level"]'))).toBe('Breaking');
      expect(rows[1].querySelector('[data-testid="check-failed"]')).toBeNull();
      const note = rows[1].querySelector('[data-testid="refresh-failed"]')!;
      expect(text(note)).toBe('refresh failed');
      expect(note.querySelector('tm-icon')).not.toBeNull();
      expect(note.closest('tm-badge')).toBeNull(); // not a badge: it isn't a risk
      expect(note.getAttribute('data-tooltip')).toContain('Showing the result from');
      expect(note.getAttribute('data-tooltip')).toContain(failed.error);

      // No result + error: the dashed badge, the error on hover, and just the tag as version.
      const badge = rows[2].querySelector('[data-testid="check-failed"]')!;
      expect(text(badge)).toBe('check failed');
      expect(badge.getAttribute('data-tooltip')).toBe(failed.error);
      expect(rows[2].querySelector('[data-testid="refresh-failed"]')).toBeNull();
      expect(text(rows[2].querySelector('tm-container-version'))).toBe('1.0');
    });

    it.each([
      ['rate-limit', 'rate limited'],
      ['not-found', 'not found or private'],
      ['timeout', 'timed out'],
      ['network', 'network error'],
      ['registry', 'registry error'],
    ] as const)('shows the short reason for %s', async (kind, label) => {
      const l = list();
      l.containers[1].update = result({ ...failed, errorKind: kind });
      l.containers[2].update = { ...l.containers[2].update!, errorKind: kind };
      const el = await render((req) => req.flush(l));
      const rows = [...el.querySelectorAll('tbody tr')];
      const note = rows[1].querySelector<HTMLElement>('[data-testid="refresh-failed"]')!;
      expect(text(note)).toMatch(new RegExp(`^refresh failed\\s*· ${label}$`));
      expect(note.getAttribute('tabindex')).toBe('0');
      expect(note.getAttribute('data-tooltip')).toContain(failed.error);
      expect(text(rows[2].querySelector('[data-testid="failure-reason"]'))).toBe(`· ${label}`);
      expect(rows[2].querySelector('[data-testid="check-failed"]')!.getAttribute('tabindex')).toBe('0');
    });

    it('shows no reason without a kind', async () => {
      const el = await render((req) => req.flush(list()));
      const rows = [...el.querySelectorAll('tbody tr')];
      expect(rows[1].querySelector('[data-testid="failure-reason"]')).toBeNull();
      expect(rows[2].querySelector('[data-testid="failure-reason"]')).toBeNull();
    });

    it('still counts and filters a kept result', async () => {
      const el = await render((req) => req.flush(list()));
      const buttons = [...el.querySelectorAll<HTMLButtonElement>('[data-testid="filter"] button')];
      expect(buttons.map(text)).toEqual(['All', 'Updates 2', 'Breaking 1']);
      expect(el.querySelector('[data-testid="headline"]')?.textContent?.trim()).toBe('2 updates');
      expect(text(el.querySelector('[data-testid="failed-count"]'))).toBe('· 2 checks failed');

      buttons[2].click();
      await TestBed.inject(ApplicationRef).whenStable();
      expect([...el.querySelectorAll('tbody tr a')].map(text)).toEqual(['kept']);
    });

    it('shows the note on tiles too', async () => {
      localStorage.setItem('tidemark.view', 'tiles');
      const el = await render((req) => req.flush(list()));
      const tiles = [...el.querySelectorAll('tm-container-tile')];
      expect(tiles.map((t) => !!t.querySelector('[data-testid="refresh-failed"]'))).toEqual([false, true, false]);
    });

    it('has helpers for the three states', () => {
      const kept = container('a', undefined, { update: result(failed) });
      const never = container('b', undefined, { update: { ...result(failed), checkedAt: undefined } });
      expect([hasResult(kept.update), refreshFailed(kept.update), hasUpdate(kept)]).toEqual([true, true, true]);
      expect([hasResult(never.update), refreshFailed(never.update), hasUpdate(never)]).toEqual([false, false, false]);
      expect(hasResult(undefined)).toBe(false);
    });
  });

  it('says so when the filter leaves nothing to show', async () => {
    localStorage.setItem('tidemark.filter', 'breaking');
    const body: ContainerList = { source: { kind: 'docker', location: 'x' }, containers: [container('mkvtoolnix', 'files')] };
    const el = await render((req) => req.flush(body));
    expect(el.textContent).toContain('Nothing breaking');
    el.querySelector<HTMLButtonElement>('tm-empty-state button')!.click();
    await TestBed.inject(ApplicationRef).whenStable();
    expect(el.querySelectorAll('tbody tr').length).toBe(1);
  });

  it('says Summarising once every image is checked and AI summaries are queued', async () => {
    const body: ContainerList = {
      source: { kind: 'docker', location: 'x' },
      check: { running: true, done: 24, total: 24, followUps: { done: 1, total: 3 } },
      containers: [container('jellyfin', 'media')],
    };
    const el = await render((req) => req.flush(body));
    expect(el.querySelector('[data-testid="check-now"]')!.textContent).toContain('Summarising 1/3');
    expect(el.querySelector('[data-testid="headline"]')!.textContent).toContain('Summarising 1/3');
  });

  it('shows progress while a check runs, and Check now starts one', async () => {
    const body: ContainerList = {
      source: { kind: 'docker', location: 'unix:///var/run/docker.sock' },
      check: { running: true, done: 3, total: 24 },
      containers: [container('jellyfin', 'media')],
    };
    const el = await render((req) => req.flush(body));
    const button = el.querySelector<HTMLButtonElement>('[data-testid="check-now"]')!;
    expect(button.textContent).toContain('Checking 3/24');
    expect(el.querySelectorAll('tm-container-version tm-skeleton').length).toBe(1);

    button.click();
    http.expectOne({ method: 'POST', url: '/api/checks' }).flush({ running: true, done: 0, total: 24 });
  });

  it('keeps polling while the check runs, then stops', async () => {
    // Angular's scheduler runs on the faked timers too, so settle by advancing the clock.
    vi.useFakeTimers();
    const settle = () => vi.advanceTimersByTimeAsync(20);
    try {
      const running = (done: number): ContainerList => ({
        source: { kind: 'docker', location: 'x' },
        check: { running: true, done, total: 3 },
        containers: [container('jellyfin', 'media')],
      });
      const fixture = TestBed.createComponent(Dashboard);
      fixture.detectChanges();
      http.expectOne('/api/containers').flush(running(0));
      await settle();

      for (const done of [1, 2]) {
        await vi.advanceTimersByTimeAsync(1500);
        http.expectOne('/api/containers').flush(running(done));
        await settle();
      }
      await vi.advanceTimersByTimeAsync(1500);
      http.expectOne('/api/containers').flush({ ...running(3), check: { running: false, done: 3, total: 3, finishedAt: '2026-10-16T13:00:00Z' } });
      await settle();

      await vi.advanceTimersByTimeAsync(5000);
      http.expectNone('/api/containers');

      // Then slowly, so scheduled checks show up without a reload.
      await vi.advanceTimersByTimeAsync(60_000);
      http.expectOne('/api/containers').flush({ ...running(3), check: { running: false, done: 3, total: 3 } });
      await settle();
    } finally {
      vi.useRealTimers();
    }
  });

  it("doesn't poll in a hidden tab, and reloads once when it's shown again", async () => {
    vi.useFakeTimers();
    const settle = () => vi.advanceTimersByTimeAsync(20);
    const setHidden = (hidden: boolean) => {
      Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => (hidden ? 'hidden' : 'visible') });
      document.dispatchEvent(new Event('visibilitychange'));
    };
    try {
      const body: ContainerList = { source: { kind: 'docker', location: 'x' }, check: { running: true, done: 0, total: 3 }, containers: [container('jellyfin', 'media')] };
      const fixture = TestBed.createComponent(Dashboard);
      fixture.detectChanges();
      http.expectOne('/api/containers').flush(body);
      await settle();

      setHidden(true);
      await vi.advanceTimersByTimeAsync(120_000);
      http.expectNone('/api/containers');

      setHidden(false);
      await settle();
      http.expectOne('/api/containers').flush(body);
      await settle();
      await vi.advanceTimersByTimeAsync(1500);
      http.expectOne('/api/containers').flush(body); // and polling again
    } finally {
      delete (document as { visibilityState?: unknown }).visibilityState;
      vi.useRealTimers();
    }
  });

  it('keeps the list and keeps polling when one poll fails', async () => {
    vi.useFakeTimers();
    const settle = () => vi.advanceTimersByTimeAsync(20);
    try {
      const running = (done: number): ContainerList => ({
        source: { kind: 'docker', location: 'x' },
        check: { running: true, done, total: 3 },
        containers: [container('jellyfin', 'media')],
      });
      const fixture = TestBed.createComponent(Dashboard);
      const el: HTMLElement = fixture.nativeElement;
      fixture.detectChanges();
      http.expectOne('/api/containers').flush(running(1));
      await settle();

      await vi.advanceTimersByTimeAsync(1500);
      http.expectOne('/api/containers').flush({ error: 'restarting' }, { status: 502, statusText: 'Bad Gateway' });
      await settle();
      fixture.detectChanges();
      expect(el.querySelector('[data-testid="refresh-error"]')?.textContent).toContain('restarting');
      expect(el.textContent).toContain('jellyfin');
      expect(el.textContent).not.toContain("Can't list containers");

      await vi.advanceTimersByTimeAsync(1500);
      http.expectOne('/api/containers').flush(running(2));
      await settle();
      fixture.detectChanges();
      expect(el.querySelector('[data-testid="refresh-error"]')).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  describe('rules', () => {
    const update = (extra: Partial<UpdateInfo> = {}): UpdateInfo => ({
      running: '1.4.2', source: 'tag', latest: '1.4.7', class: 'patch', floating: false, newImage: false, checkedAt: '2026-10-16T13:00:00Z', level: 'none', ...extra,
    });
    const breaking = update({ latest: '2.0.0', class: 'major', level: 'breaking' });
    const text = (e: Element | null | undefined) => e?.textContent?.replace(/\s+/g, ' ').trim();

    function body(): ContainerList {
      return {
        source: { kind: 'docker', location: 'x' },
        check: { running: false, done: 4, total: 4, finishedAt: '2026-10-16T13:00:00Z' },
        containers: [
          container('jellyfin', 'media', { update: breaking }),
          container('watchtower', 'media', { rules: { ignore: true, from: { ignore: 'label' } } }),
          container('filebrowser', undefined, { rules: { ignore: true, from: { ignore: 'config' } } }),
          container('mkvtoolnix'),
        ],
      };
    }

    it('puts ignored containers in a collapsed section after every stack', async () => {
      const el = await render((req) => req.flush(body()));
      const sections = [...el.querySelectorAll('tm-section')];
      expect(sections.map((s) => text(s.querySelector('h2')))).toEqual(['media', 'Ungrouped', 'Ignored']);
      expect(sections[0].textContent).toContain('1 container'); // watchtower left its stack

      const ignored = el.querySelector('[data-testid="ignored-section"]')!;
      expect(ignored.textContent).toContain('2 containers');
      const toggle = ignored.querySelector<HTMLButtonElement>('h2 button')!;
      expect(toggle.getAttribute('aria-expanded')).toBe('false');
      expect(ignored.querySelectorAll('tbody tr').length).toBe(0);

      toggle.click();
      await TestBed.inject(ApplicationRef).whenStable();
      expect(toggle.getAttribute('aria-expanded')).toBe('true');
      const rows = [...ignored.querySelectorAll('tbody tr')];
      expect(rows.map((r) => text(r.querySelector('a')))).toEqual(['filebrowser', 'watchtower']);
      expect(text(rows[1])).toContain('media · Up 2 days'); // its stack, since it's not in that section
      expect(rows.every((r) => r.hasAttribute('data-dimmed'))).toBe(true); // quiet: faded, full again on hover
      const marks = [...ignored.querySelectorAll('[data-testid="ignored"]')];
      expect(marks.map(text)).toEqual(['Ignored', 'Ignored']);
      expect(marks[0].getAttribute('data-tooltip')).toBe('Not checked for updates: ignored in the config file.');
      expect(marks[1].getAttribute('data-tooltip')).toBe('Not checked for updates: ignored by the tidemark.ignore label.');
      expect(ignored.querySelector('tm-update-class')).toBeNull();
    });

    it('shows ignored containers as tiles too', async () => {
      localStorage.setItem('tidemark.view', 'tiles');
      const el = await render((req) => req.flush(body()));
      const ignored = el.querySelector('[data-testid="ignored-section"]')!;
      ignored.querySelector<HTMLButtonElement>('h2 button')!.click();
      await TestBed.inject(ApplicationRef).whenStable();
      const tiles = [...ignored.querySelectorAll('tm-container-tile')];
      expect(tiles.length).toBe(2);
      // Faded like up-to-date tiles, and the colour comes back on hover (the group- classes).
      expect(tiles.every((t) => t.hasAttribute('data-dimmed') && t.className.includes('hover:opacity-100'))).toBe(true);
      expect(tiles[0].querySelector('tm-app-icon')!.className).toContain('group-hover:grayscale-0');
    });

    it('leaves ignored containers out of the Updates and Breaking filters', async () => {
      const list = body();
      // Even if an update were present, an ignored container doesn't count.
      list.containers[1].update = breaking;
      const el = await render((req) => req.flush(list));
      const buttons = [...el.querySelectorAll<HTMLButtonElement>('[data-testid="filter"] button')];
      expect(buttons.map(text)).toEqual(['All', 'Updates 1', 'Breaking 1']);
      expect(el.querySelector('[data-testid="headline"]')?.textContent?.trim()).toBe('1 update');

      buttons[1].click();
      await TestBed.inject(ApplicationRef).whenStable();
      expect(el.querySelector('[data-testid="ignored-section"]')).toBeNull();
      expect(el.querySelectorAll('tm-section').length).toBe(1);
    });

    it('shows only the Ignored section when every container is ignored', async () => {
      const list = body();
      list.containers = list.containers.filter((c) => c.rules?.ignore);
      const el = await render((req) => req.flush(list));
      expect(el.querySelector('tm-empty-state')).toBeNull();
      expect(el.querySelector('[data-testid="ignored-section"]')).not.toBeNull();
    });

    it('says quietly which version a rule holds back', async () => {
      const list = body();
      list.containers = [
        container('immich', undefined, { rules: { pin: '1.4', from: { pin: 'label' } }, update: update({ held: '2.1.0', heldBy: 'pin' }) }),
        container('postgres', undefined, {
          rules: { allowMajor: false, from: { 'allow-major': 'config' } },
          update: update({ running: '16.4', latest: '16.10', class: 'minor', held: '18.0', heldBy: 'allow-major' }),
        }),
      ];
      const el = await render((req) => req.flush(list));
      const held = [...el.querySelectorAll('[data-testid="held"]')];
      expect(held.map(text)).toEqual(['2.1.0 held back by pin 1.4', '18.0 held back, majors off']);
      expect(held[0].querySelectorAll('.font-mono').length).toBe(2);
      // The held-in version is still the update shown.
      expect(el.querySelector('tbody tr')?.textContent?.replace(/\s/g, '')).toContain('1.4.2→1.4.7');
    });

    it('marks containers whose rules have problems', async () => {
      const list = body();
      list.containers.push(
        container('redis', undefined, { rules: { from: {}, problems: ['tidemark.allow-major: "maybe" is not true or false'] } }),
      );
      const el = await render((req) => req.flush(list));
      const marks = [...el.querySelectorAll('[data-testid="rule-problems"]')];
      expect(marks.length).toBe(1);
      expect(text(marks[0])).toBe('rule skipped');
      expect(marks[0].getAttribute('data-tooltip')).toContain('"maybe" is not true or false');
    });

    it('says when the config file could not be read', async () => {
      const list = body();
      list.check!.rulesError = 'open /config/tidemark.yml: permission denied';
      const el = await render((req) => req.flush(list));
      const notice = el.querySelector('[data-testid="rules-error"]')!;
      expect(notice.textContent).toContain("Couldn't read the config file");
      expect(notice.querySelector('.font-mono')?.textContent).toBe('open /config/tidemark.yml: permission denied');
    });

    it('shows no config notice when the file loaded', async () => {
      const el = await render((req) => req.flush(body()));
      expect(el.querySelector('[data-testid="rules-error"]')).toBeNull();
    });
  });

  describe('hero and grouping', () => {
    const text = (e: Element | null | undefined) => e?.textContent?.replace(/\s+/g, ' ').trim();
    function body(): ContainerList {
      return {
        source: { kind: 'docker', location: 'x' },
        check: { running: false, done: 6, total: 6, finishedAt: '2026-10-16T13:00:00Z' },
        containers: [
          container('jellyfin', 'media', { update: checked('major', 'breaking') }),
          container('sonarr', 'media', { update: checked('none') }),
          container('vault', 'secrets', { update: checked('minor', 'attention') }),
          container('adguard', 'networking', { update: checked('none') }),
          container('stirling-pdf', 'office', { update: checked('patch') }),
          container('private', 'office', { update: neverChecked }),
          container('filebrowser', 'tools', { rules: { ignore: true, from: { ignore: 'label' } } }),
        ],
      };
    }
    /** Top-level sections only; settled stacks are nested inside "Up to date". */
    const sectionNames = (el: HTMLElement) => [...el.querySelectorAll('[data-view] > tm-section')].map((s) => text(s.querySelector('h2')));
    const settle = () => TestBed.inject(ApplicationRef).whenStable();

    it('shows the headline, the figures and the gauge', async () => {
      const el = await render((req) => req.flush(body()));
      expect(text(el.querySelector('[data-testid="headline"]'))).toBe('3 updates');
      expect(text(el.querySelector('[data-testid="stat-breaking"]'))).toBe('Breaking 1');
      expect(text(el.querySelector('[data-testid="stat-attention"]'))).toBe('Attention 1');
      expect(text(el.querySelector('[data-testid="stat-updates"]'))).toBe('Updates 3');
      expect(text(el.querySelector('[data-testid="stat-current"]'))).toBe('Up to date 2');
      // 5 checked (ignored and never-checked containers don't count), 2 of them up to date.
      expect(el.querySelector('[data-testid="gauge"]')!.getAttribute('aria-label')).toBe('2 of 5 checked containers up to date');
      // The summary explains why the gauge total (5) is lower than the container count (7).
      expect(text(el.querySelector('[data-testid="summary"]'))).toContain('7 containers · 5 stacks · 1 ignored ·');
      expect(text(el.querySelector('[data-testid="gauge-count"]'))).toBe('2/5 up to date');
    });

    it('shows a channel build (nightly) without comparing it to the newest stable release', async () => {
      const list = body();
      list.containers = [
        container('mealie', 'food', {
          update: checked('digest', 'none', { running: undefined, source: 'unknown', channel: 'nightly', latest: 'v3.1.0', floating: true, newImage: true }),
        }),
      ];
      const el = await render((req) => req.flush(list));
      const row = text(el.querySelector('tm-container-version'));
      expect(row).toContain('nightly');
      expect(row).toContain('new build available');
      expect(row).toContain('newest stable v3.1.0');
      expect(row).not.toContain('?');
      expect(row).not.toContain('running version unknown');
    });

    it('says "All up to date" when nothing is newer', async () => {
      const list = body();
      list.containers = list.containers.filter((c) => hasResult(c.update) && c.update.class === 'none');
      const el = await render((req) => req.flush(list));
      expect(text(el.querySelector('[data-testid="headline"]'))).toBe('All up to date');
      expect(el.querySelector('[data-testid="gauge"]')!.getAttribute('aria-label')).toBe('2 of 2 checked containers up to date');
    });

    it('shows progress in the headline while checking, and does not tone down containers without a result', async () => {
      const el = await render((req) =>
        req.flush({
          source: { kind: 'docker', location: 'x' },
          check: { running: true, done: 0, total: 2 },
          containers: [container('a', 's'), container('b', 's')],
        }),
      );
      expect(text(el.querySelector('[data-testid="headline"]'))).toBe('Checking 0/2');
      expect(el.querySelector('[data-testid="gauge"]')!.getAttribute('aria-label')).toBe('No containers checked yet');
      expect(el.querySelectorAll('tr[data-current]').length).toBe(0);
    });

    it('in stack mode, keeps stacks whole, dims current rows and tucks settled stacks into "Up to date"', async () => {
      const el = await render((req) => req.flush(body()));
      expect(sectionNames(el)).toEqual(['media', 'office', 'secrets', 'Up to date', 'Ignored']);
      const rowNames = (rows: Element[]) => rows.map((r) => text(r.querySelector('a')));
      // sonarr is up to date in media, which has an update: toned down and dimmed as a whole.
      expect(rowNames([...el.querySelectorAll('tbody tr[data-current]')])).toEqual(['sonarr']);
      const dimmed = [...el.querySelectorAll('tbody tr[data-dimmed]')];
      expect(rowNames(dimmed)).toEqual(['sonarr']);
      expect(dimmed[0].className).toContain('opacity-60');
      expect(dimmed[0].className).toContain('hover:opacity-100');
      expect(dimmed[0].className).toContain('focus-within:opacity-100');
      expect(el.querySelector('[data-testid="row-stack"]')).toBeNull(); // the heading already says it

      // networking only has up-to-date containers: inside the collapsed, muted "Up to date" section.
      const settled = el.querySelector('[data-testid="stacks-current"]')!;
      expect(text(settled.querySelector('h2'))).toBe('Up to date');
      expect(settled.textContent).toContain('1 stack');
      const toggle = settled.querySelector<HTMLButtonElement>('h2 button')!;
      expect(toggle.getAttribute('aria-expanded')).toBe('false');
      expect(settled.querySelectorAll('tbody tr').length).toBe(0);
      toggle.click();
      await settle();
      const sub = [...settled.querySelectorAll('tm-section')];
      expect(sub.map((s) => text(s.querySelector('h3')))).toEqual(['networking']);
      expect(sub[0].textContent).toContain('1 container');
      expect(rowNames([...sub[0].querySelectorAll('tbody tr')])).toEqual(['adguard']);
      // Toned down and dimmed, as everywhere: hover brings the colour back.
      expect(sub[0].querySelector('tr[data-current]')).not.toBeNull();
      expect(sub[0].querySelector('tr[data-dimmed]')).not.toBeNull();
    });

    it('puts settled ungrouped containers in "Up to date" too, and keeps the open state across groupings', async () => {
      const list = body();
      list.containers.push(container('pihole', undefined, { update: checked('none') }));
      const el = await render((req) => req.flush(list));
      const settled = el.querySelector('[data-testid="stacks-current"]')!;
      expect(settled.textContent).toContain('2 stacks');
      settled.querySelector<HTMLButtonElement>('h2 button')!.click();
      await settle();
      expect([...settled.querySelectorAll('tm-section h3')].map(text)).toEqual(['networking', 'Ungrouped']);
      expect(sectionNames(el)).not.toContain('Ungrouped');

      el.querySelector<HTMLButtonElement>('[data-testid="group"] button:nth-child(2)')!.click(); // Severity
      await settle();
      const current = el.querySelector('[data-testid="severity-current"]')!;
      expect(current.querySelector('h2 button')!.getAttribute('aria-expanded')).toBe('true');
      expect(current.querySelectorAll('tr[data-dimmed]').length).toBe(3); // dimmed in the severity grouping too
    });

    it('opens "Up to date" when a search only matches settled stacks', async () => {
      const el = await render((req) => req.flush(body()));
      const input = el.querySelector<HTMLInputElement>('[data-testid="search"] input')!;
      input.value = 'adguard';
      input.dispatchEvent(new Event('input'));
      await settle();
      expect(sectionNames(el)).toEqual(['Up to date']);
      const settled = el.querySelector('[data-testid="stacks-current"]')!;
      expect(settled.querySelector('h2 button')!.getAttribute('aria-expanded')).toBe('true');
      expect([...settled.querySelectorAll('tbody tr a')].map(text)).toEqual(['adguard']);
    });

    it('opens "Up to date" when everything is up to date, in both groupings, and not again after closing it', async () => {
      const list = body();
      list.containers = list.containers.map((c) => (c.rules?.ignore ? c : { ...c, update: checked('none') }));
      const first = TestBed.createComponent(Dashboard);
      first.detectChanges();
      http.expectOne('/api/containers').flush(list);
      await first.whenStable();
      const el = first.nativeElement as HTMLElement;
      expect(sectionNames(el)).toEqual(['Up to date', 'Ignored']);
      const toggle = (root: HTMLElement) => root.querySelector<HTMLButtonElement>('[data-view] > tm-section h2 button')!;
      expect(toggle(el).getAttribute('aria-expanded')).toBe('true');

      el.querySelector<HTMLButtonElement>('[data-testid="group"] button:nth-child(2)')!.click(); // Severity
      await settle();
      expect(toggle(el).getAttribute('aria-expanded')).toBe('true');
      toggle(el).click();
      await settle();
      expect(toggle(el).getAttribute('aria-expanded')).toBe('false');
      first.destroy();

      // Back from a detail page: the component is new, the choice to close it stays.
      const again = TestBed.createComponent(Dashboard);
      again.detectChanges();
      TestBed.tick();
      expect(toggle(again.nativeElement).getAttribute('aria-expanded')).toBe('false');
    });

    it("groups by severity, remembers it, and shows each container's stack", async () => {
      const el = await render((req) => req.flush(body()));
      const group = [...el.querySelectorAll<HTMLButtonElement>('[data-testid="group"] button')];
      expect(group.map(text)).toEqual(['Stack', 'Severity']);
      group[1].click();
      await settle();
      expect(localStorage.getItem('tidemark.group')).toBe('severity');

      expect(sectionNames(el)).toEqual(['Breaking', 'Attention', 'Updates', 'Check failed', 'Up to date', 'Ignored']);
      expect(text(el.querySelector('[data-testid="severity-breaking"] [data-testid="row-stack"]'))).toBe('media');

      // Up to date is collapsed by default, like Ignored.
      const current = el.querySelector('[data-testid="severity-current"]')!;
      const toggle = current.querySelector<HTMLButtonElement>('h2 button')!;
      expect(toggle.getAttribute('aria-expanded')).toBe('false');
      expect(current.querySelectorAll('tbody tr').length).toBe(0);
      toggle.click();
      await settle();
      expect([...current.querySelectorAll('tbody tr a')].map(text)).toEqual(['adguard', 'sonarr']);
      expect([...current.querySelectorAll('tbody tr')].every((r) => r.hasAttribute('data-current'))).toBe(true);
    });

    it('applies the filter on top of the severity groups', async () => {
      localStorage.setItem('tidemark.group', 'severity');
      localStorage.setItem('tidemark.filter', 'updates');
      const el = await render((req) => req.flush(body()));
      expect(sectionNames(el)).toEqual(['Breaking', 'Attention', 'Updates']);
    });

    it('shows the stack on tiles in severity mode', async () => {
      localStorage.setItem('tidemark.group', 'severity');
      localStorage.setItem('tidemark.view', 'tiles');
      const el = await render((req) => req.flush(body()));
      const tile = el.querySelector('[data-testid="severity-attention"] tm-container-tile')!;
      expect(text(tile.querySelector('[data-testid="tile-stack"]'))).toBe('secrets');
      expect(tile.hasAttribute('data-current')).toBe(false);
    });

    it('tones down up-to-date tiles in stack mode', async () => {
      localStorage.setItem('tidemark.view', 'tiles');
      const el = await render((req) => req.flush(body()));
      const tiles = [...el.querySelectorAll('tm-container-tile')];
      expect(tiles.filter((t) => t.hasAttribute('data-current')).map((t) => text(t.querySelector('h3 a')))).toEqual(['sonarr']);
      const dimmed = tiles.filter((t) => t.hasAttribute('data-dimmed'));
      expect(dimmed.map((t) => text(t.querySelector('h3 a')))).toEqual(['sonarr']);
      expect(dimmed[0].className).toContain('opacity-60');
      expect(el.querySelector('[data-testid="tile-stack"]')).toBeNull();
    });
  });

  describe('reviewed and snoozed', () => {
    const text = (e: Element | null | undefined) => e?.textContent?.replace(/\s+/g, ' ').trim();
    const settle = () => TestBed.inject(ApplicationRef).whenStable();
    function body(): ContainerList {
      return {
        source: { kind: 'docker', location: 'x' },
        check: { running: false, done: 6, total: 6, finishedAt: '2026-10-16T13:00:00Z' },
        containers: [
          container('jellyfin', 'media', { update: checked('major', 'breaking'), review: reviewed() }),
          container('sonarr', 'media', { update: checked('none') }),
          container('vault', 'secrets', { update: checked('minor', 'attention') }),
          container('stirling-pdf', 'office', { update: checked('patch'), review: snoozedUntil() }),
          container('immich', 'photos', { update: checked('major', 'breaking'), review: snoozedVersion() }),
          container('adguard', 'networking', { update: checked('none') }),
        ],
      };
    }
    const sectionNames = (el: HTMLElement) => [...el.querySelectorAll('[data-view] > tm-section')].map((s) => text(s.querySelector('h2')));

    it('keeps a reviewed update in place, faded, with a marker, and leaves it out of the hero', async () => {
      const el = await render((req) => req.flush(body()));
      expect(text(el.querySelector('[data-testid="headline"]'))).toBe('1 update');
      expect(text(el.querySelector('[data-testid="stat-breaking"]'))).toBe('Breaking 0');
      expect(text(el.querySelector('[data-testid="stat-attention"]'))).toBe('Attention 1');
      expect(text(el.querySelector('[data-testid="stat-updates"]'))).toBe('Updates 1');
      expect(text(el.querySelector('[data-testid="summary"]'))).toContain('6 containers · 5 stacks · 1 reviewed · 2 snoozed ·');
      // The gauge still measures truly up to date: 2 of the 6 checked.
      expect(el.querySelector('[data-testid="gauge"]')!.getAttribute('aria-label')).toBe('2 of 6 checked containers up to date');

      // media has only a reviewed update (and an up-to-date one): it joins the settled stacks in "Up to date".
      expect(sectionNames(el)).toEqual(['secrets', 'Up to date', 'Snoozed']);
      el.querySelector<HTMLButtonElement>('[data-testid="stacks-current"] h2 button')!.click();
      await settle();
      expect([...el.querySelectorAll('[data-testid="stacks-current"] tm-section h3')].map(text)).toEqual(['media', 'networking']);
      const row = [...el.querySelectorAll('tbody tr')].find((r) => text(r.querySelector('a')) === 'jellyfin')!;
      expect(row.hasAttribute('data-dimmed')).toBe(false); // reviewed rows aren't dimmed (their badges fade)
      const dimmed = [...el.querySelectorAll('[data-testid="stacks-current"] tr[data-dimmed] a')].map(text);
      expect(dimmed).toEqual(['sonarr', 'adguard']);
      // Snoozed rows are quiet too: dimmed.
      el.querySelector<HTMLButtonElement>('[data-testid="snoozed-section"] h2 button')!.click();
      await settle();
      const snoozed = [...el.querySelectorAll('[data-testid="snoozed-section"] tbody tr')];
      expect(snoozed.length).toBe(2);
      expect(snoozed.every((r) => r.hasAttribute('data-dimmed'))).toBe(true);
      expect(row.querySelector('tm-update-class')!.classList).toContain('opacity-50');
      expect(row.querySelector('[data-testid="level"]')!.classList).toContain('opacity-50');
      const marker = row.querySelector('[data-testid="reviewed"]')!;
      expect(text(marker)).toBe('Reviewed');
      expect(marker.querySelector('tm-icon')).not.toBeNull();
      expect(marker.getAttribute('data-tooltip')).toContain('Reviewed up to 2.0.0 on 20 Oct.');
      expect(marker.hasAttribute('title')).toBe(false);
      // Not bold: it has been looked at.
      expect(row.querySelector('a')!.className).not.toContain('font-semibold');

      // An open update is neither faded nor marked.
      const vault = [...el.querySelectorAll('tbody tr')].find((r) => text(r.querySelector('a')) === 'vault')!;
      expect(vault.querySelector('[data-testid="level"]')!.classList).not.toContain('opacity-50');
      expect(vault.querySelector('[data-testid="reviewed"]')).toBeNull();
    });

    it('keeps reviewed updates in the Updates and Breaking filters, but not snoozed ones', async () => {
      const el = await render((req) => req.flush(body()));
      const filter = el.querySelector('[data-testid="filter"]')!;
      expect(text(filter)).toContain('Updates 2');
      expect(text(filter)).toContain('Breaking 1');
      [...filter.querySelectorAll('button')].find((b) => b.textContent?.includes('Breaking'))!.click();
      await settle();
      expect([...el.querySelectorAll('tbody tr a')].map(text)).toEqual(['jellyfin']);
      expect(el.querySelector('[data-testid="snoozed-section"]')).toBeNull();
    });

    it('moves snoozed updates to a collapsed Snoozed section above Ignored, with until when', async () => {
      const list = body();
      list.containers.push(container('filebrowser', 'tools', { rules: { ignore: true, from: { ignore: 'label' } } }));
      const el = await render((req) => req.flush(list));
      expect(sectionNames(el).slice(-2)).toEqual(['Snoozed', 'Ignored']);
      const snoozed = el.querySelector('[data-testid="snoozed-section"]')!;
      expect(snoozed.textContent).toContain('2 containers');
      const toggle = snoozed.querySelector<HTMLButtonElement>('h2 button')!;
      expect(toggle.getAttribute('aria-expanded')).toBe('false');
      toggle.click();
      await settle();
      const rows = [...snoozed.querySelectorAll('tbody tr')];
      expect(rows.map((r) => text(r.querySelector('a')))).toEqual(['immich', 'stirling-pdf']);
      expect(text(rows[0])).toContain('photos · Up 2 days'); // its stack, since it left that section
      expect(rows.every((r) => r.querySelector('a')!.className.includes('text-fg-muted'))).toBe(true);
      const lines = rows.map((r) => r.querySelector('[data-testid="snoozed"]')!);
      expect(text(lines[0])).toBe('until newer than 2.0.0');
      expect(lines[0].querySelector('.font-mono')!.textContent).toBe('2.0.0');
      expect(text(lines[1])).toBe('until 3 Nov');
      expect(lines[1].getAttribute('data-tooltip')).toContain('Snoozed on 20 Oct');
      expect(rows[0].querySelector('[data-testid="level"]')!.classList).toContain('opacity-50');
      // Not in their old stacks.
      expect(sectionNames(el)).not.toContain('office');
      expect(sectionNames(el)).not.toContain('photos');
    });

    it('shows snoozed tiles too, and leaves them out of the severity groups', async () => {
      localStorage.setItem('tidemark.view', 'tiles');
      localStorage.setItem('tidemark.group', 'severity');
      const el = await render((req) => req.flush(body()));
      expect(sectionNames(el)).toEqual(['Breaking', 'Attention', 'Up to date', 'Snoozed']);
      const snoozed = el.querySelector('[data-testid="snoozed-section"]')!;
      snoozed.querySelector<HTMLButtonElement>('h2 button')!.click();
      await settle();
      const tiles = [...snoozed.querySelectorAll('tm-container-tile')];
      expect(tiles.length).toBe(2);
      expect(tiles[1].className).toContain('bg-transparent'); // toned down
      expect(text(tiles[1].querySelector('[data-testid="snoozed"]'))).toBe('until 3 Nov');
      // The reviewed tile keeps its place under Breaking, with the marker.
      expect(el.querySelector('[data-testid="severity-breaking"] [data-testid="reviewed"]')).not.toBeNull();
    });

    it('searches inside the Snoozed section', async () => {
      const el = await render((req) => req.flush(body()));
      const input = el.querySelector<HTMLInputElement>('[data-testid="search"] input')!;
      input.value = 'stir';
      input.dispatchEvent(new Event('input'));
      await settle();
      const snoozed = el.querySelector('[data-testid="snoozed-section"]')!;
      expect(snoozed.textContent).toContain('1 container');
      input.value = 'vault';
      input.dispatchEvent(new Event('input'));
      await settle();
      expect(el.querySelector('[data-testid="snoozed-section"]')).toBeNull();
    });

    it('says "No open updates" when only reviewed or snoozed updates are left', async () => {
      const list = body();
      list.containers = list.containers.filter((c) => c.name !== 'vault');
      const el = await render((req) => req.flush(list));
      expect(text(el.querySelector('[data-testid="headline"]'))).toBe('No open updates');
      expect(text(el.querySelector('[data-testid="stat-updates"]'))).toBe('Updates 0');
    });

    it('says snoozed updates show under All when the Updates filter is empty', async () => {
      localStorage.setItem('tidemark.filter', 'updates');
      const list = body();
      list.containers = list.containers.filter((c) => c.review?.kind !== 'reviewed' && c.name !== 'vault');
      const el = await render((req) => req.flush(list));
      expect(el.textContent).toContain('Snoozed updates show under All.');
    });
  });

  it('shows the API error message', async () => {
    const el = await render((req) =>
      req.flush({ error: "can't reach Docker at unix:///var/run/docker.sock" }, { status: 502, statusText: 'Bad Gateway' }),
    );
    expect(el.textContent).toContain("Can't list containers");
    expect(el.textContent).toContain("can't reach Docker");
  });
});
