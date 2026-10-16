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

  it('shows the API error message', async () => {
    const el = await render((req) =>
      req.flush({ error: "can't reach Docker at unix:///var/run/docker.sock" }, { status: 502, statusText: 'Bad Gateway' }),
    );
    expect(el.textContent).toContain("Can't list containers");
    expect(el.textContent).toContain("can't reach Docker");
  });
});
