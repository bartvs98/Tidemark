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

    expect(el.querySelector('[data-testid="summary"]')?.textContent).toContain('3 containers · 1 stacks · 1 not running');
    const groups = [...el.querySelectorAll('tr.tm-group')].map((r) => r.textContent?.replace(/\s+/g, ' ').trim());
    expect(groups).toEqual(['media 2', 'Ungrouped 1']);
    expect(el.textContent).toContain('exited');
    expect(el.textContent).toContain('Snapshot');
  });

  it('shows the API error message', async () => {
    const el = await render((req) =>
      req.flush({ error: "can't reach Docker at unix:///var/run/docker.sock" }, { status: 502, statusText: 'Bad Gateway' }),
    );
    expect(el.textContent).toContain("Can't list containers");
    expect(el.textContent).toContain("can't reach Docker");
  });
});
