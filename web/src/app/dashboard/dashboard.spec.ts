import { ApplicationRef } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { Dashboard } from './dashboard';
import { ContainerList, ContainerSummary, groupByStack, hasResult, hasUpdate, imageName, refreshFailed, splitIgnored, UpdateInfo } from './containers';

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

describe('Dashboard', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    localStorage.clear();
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

  it('shows app icons, with a letter avatar when there is none', async () => {
    const body: ContainerList = {
      source: { kind: 'docker', location: 'unix:///var/run/docker.sock' },
      containers: [container('jellyfin', 'media', { icon: { src: '/api/icons/jellyfin' } }), container('mkvtoolnix', 'media')],
    };
    const el = await render((req) => req.flush(body));
    const icons = [...el.querySelectorAll('tm-app-icon')];
    expect(icons[0].querySelector('img')?.getAttribute('src')).toBe('/api/icons/jellyfin');
    expect(icons[1].querySelector('img')).toBeNull();
    expect(icons[1].textContent?.trim()).toBe('S');
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
    expect(el.querySelector('[data-testid="summary"]')?.textContent).toContain('1 update');
    expect(el.querySelector('[data-testid="next-run"]')?.textContent).toContain('next');
  });

  it('shows the risk level with its reasons, and filters to updates or breaking ones', async () => {
    const update = (cls: 'none' | 'minor' | 'major', level: 'none' | 'attention' | 'breaking', reasons?: string[]) => ({
      running: '1.0.0', source: 'tag' as const, latest: cls === 'none' ? '1.0.0' : '2.0.0', class: cls,
      floating: false, newImage: false, checkedAt: '2026-10-16T13:00:00Z', level, reasons,
    });
    const body: ContainerList = {
      source: { kind: 'docker', location: 'x' },
      check: { running: false, done: 3, total: 3, finishedAt: '2026-10-16T13:00:00Z' },
      containers: [
        container('jellyfin', 'media', { update: update('major', 'breaking', ['Major version: 1.0.0 → 2.0.0']) }),
        container('sonarr', 'media', { update: update('minor', 'attention', ['2.0.0: settings have been removed']) }),
        container('mkvtoolnix', 'files', { update: update('none', 'none') }),
      ],
    };
    const el = await render((req) => req.flush(body));
    const rowNames = () => [...el.querySelectorAll('tbody tr a')].map((a) => a.textContent?.trim());

    const levels = [...el.querySelectorAll('[data-testid="level"]')];
    expect(levels.map((l) => l.textContent?.trim())).toEqual(['Breaking', 'Attention']); // mkvtoolnix is up to date: no badge
    expect(levels[0].getAttribute('title')).toBe('Major version: 1.0.0 → 2.0.0');

    const buttons = [...el.querySelectorAll<HTMLButtonElement>('[data-testid="filter"] button')];
    expect(buttons.map((b) => b.textContent?.replace(/\s+/g, ' ').trim())).toEqual(['All', 'Updates 2', 'Breaking 1']);
    expect(rowNames()).toEqual(['mkvtoolnix', 'jellyfin', 'sonarr']);

    buttons[2].click();
    await TestBed.inject(ApplicationRef).whenStable();
    expect(rowNames()).toEqual(['jellyfin']);
    expect(localStorage.getItem('tidemark.filter')).toBe('breaking');

    buttons[1].click();
    await TestBed.inject(ApplicationRef).whenStable();
    expect(rowNames()).toEqual(['jellyfin', 'sonarr']);
  });

  describe('when a check fails', () => {
    const result = (extra: Partial<UpdateInfo> = {}): UpdateInfo => ({
      running: '1.0.0', source: 'tag', latest: '2.0.0', class: 'major', floating: false, newImage: false,
      checkedAt: '2026-10-15T23:00:00Z', level: 'breaking', reasons: ['Major version: 1.0.0 → 2.0.0'], ...extra,
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
      expect(note.getAttribute('title')).toContain('Showing the result from');
      expect(note.getAttribute('title')).toContain(failed.error);

      // No result + error: the dashed badge, the error on hover, and just the tag as version.
      const badge = rows[2].querySelector('[data-testid="check-failed"]')!;
      expect(text(badge)).toBe('check failed');
      expect(badge.getAttribute('title')).toBe(failed.error);
      expect(rows[2].querySelector('[data-testid="refresh-failed"]')).toBeNull();
      expect(text(rows[2].querySelector('tm-container-version'))).toBe('1.0');
    });

    it('still counts and filters a kept result', async () => {
      const el = await render((req) => req.flush(list()));
      const buttons = [...el.querySelectorAll<HTMLButtonElement>('[data-testid="filter"] button')];
      expect(buttons.map(text)).toEqual(['All', 'Updates 2', 'Breaking 1']);
      expect(el.querySelector('[data-testid="summary"]')?.textContent).toContain('2 updates');
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
      const marks = [...ignored.querySelectorAll('[data-testid="ignored"]')];
      expect(marks.map(text)).toEqual(['Ignored', 'Ignored']);
      expect(marks[0].getAttribute('title')).toBe('Not checked for updates: ignored in the config file.');
      expect(marks[1].getAttribute('title')).toBe('Not checked for updates: ignored by the tidemark.ignore label.');
      expect(ignored.querySelector('tm-update-class')).toBeNull();
    });

    it('shows ignored containers as tiles too', async () => {
      localStorage.setItem('tidemark.view', 'tiles');
      const el = await render((req) => req.flush(body()));
      const ignored = el.querySelector('[data-testid="ignored-section"]')!;
      ignored.querySelector<HTMLButtonElement>('h2 button')!.click();
      await TestBed.inject(ApplicationRef).whenStable();
      expect(ignored.querySelectorAll('tm-container-tile').length).toBe(2);
    });

    it('leaves ignored containers out of the Updates and Breaking filters', async () => {
      const list = body();
      // Even if an update were present, an ignored container doesn't count.
      list.containers[1].update = breaking;
      const el = await render((req) => req.flush(list));
      const buttons = [...el.querySelectorAll<HTMLButtonElement>('[data-testid="filter"] button')];
      expect(buttons.map(text)).toEqual(['All', 'Updates 1', 'Breaking 1']);
      expect(el.querySelector('[data-testid="summary"]')?.textContent).toContain('1 update');

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
      expect(marks[0].getAttribute('title')).toContain('"maybe" is not true or false');
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

  it('shows the API error message', async () => {
    const el = await render((req) =>
      req.flush({ error: "can't reach Docker at unix:///var/run/docker.sock" }, { status: 502, statusText: 'Bad Gateway' }),
    );
    expect(el.textContent).toContain("Can't list containers");
    expect(el.textContent).toContain("can't reach Docker");
  });
});
