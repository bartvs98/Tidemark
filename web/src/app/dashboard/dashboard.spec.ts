import { ApplicationRef } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Dashboard } from './dashboard';
import { ContainerList, ContainerSummary, groupByStack, imageName } from './containers';

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
      providers: [provideHttpClient(), provideHttpClientTesting()],
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
      check: { running: false, done: 3, total: 3, finishedAt: '2026-10-16T13:00:00Z' },
      containers: [
        container('jellyfin', 'media', {
          update: { running: '10.9.11', source: 'label', latest: '10.10.7', class: 'minor', floating: true, newImage: true, checkedAt: '' },
        }),
        container('sonarr', 'media', {
          update: { running: '4.0.14', source: 'tag', latest: '4.0.14', class: 'none', floating: false, newImage: false, checkedAt: '' },
        }),
        container('private', 'media', {
          update: { source: 'unknown', class: 'none', floating: false, newImage: false, error: 'not found, or private', checkedAt: '' },
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

  it('shows the API error message', async () => {
    const el = await render((req) =>
      req.flush({ error: "can't reach Docker at unix:///var/run/docker.sock" }, { status: 502, statusText: 'Bad Gateway' }),
    );
    expect(el.textContent).toContain("Can't list containers");
    expect(el.textContent).toContain("can't reach Docker");
  });
});
