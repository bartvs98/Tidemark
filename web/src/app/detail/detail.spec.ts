import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { ContainerDetail } from './detail';
import { ContainerDetailData } from './detail-types';

function detail(overrides: Partial<ContainerDetailData> = {}): ContainerDetailData {
  return {
    container: {
      id: 'x',
      name: 'homeassistant',
      image: {
        ref: 'homeassistant/home-assistant:latest',
        display: 'homeassistant/home-assistant:latest',
        registry: 'docker.io',
        repository: 'homeassistant/home-assistant',
        tag: 'latest',
        id: 'sha256:abc',
      },
      state: 'running',
      status: 'Up 3 weeks',
      created: '2026-10-01T00:00:00Z',
      update: { running: '2026.9.0', source: 'label', latest: '2026.10.1', class: 'minor', floating: true, newImage: true, checkedAt: '2026-10-16T12:00:00Z' },
    },
    image: { digests: ['homeassistant/home-assistant@sha256:1'], labels: { 'org.opencontainers.image.version': '2026.9.0' } },
    changelog: {
      repo: { url: 'https://github.com/home-assistant/core', name: 'home-assistant/core', how: 'image' },
      kind: 'releases',
      releases: [
        { tag: '2026.10.1', name: '2026.10.1', body: 'Fixes (#42)', url: 'https://github.com/home-assistant/core/releases/tag/2026.10.1', publishedAt: '2026-10-10T00:00:00Z' },
        { tag: '2026.10.0', name: 'October release', body: '## Breaking\n- removed X', url: 'u' },
      ],
      links: {
        compare: 'https://github.com/home-assistant/core/compare/2026.9.0...2026.10.1',
        releases: 'https://github.com/home-assistant/core/releases',
        running: 'https://github.com/home-assistant/core/tree/2026.9.0',
      },
      partial: false,
    },
    ...overrides,
  };
}

describe('ContainerDetail', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])] });
    http = TestBed.inject(HttpTestingController);
  });

  async function render(name: string, respond: (req: ReturnType<HttpTestingController['expectOne']>) => void) {
    const fixture = TestBed.createComponent(ContainerDetail);
    fixture.componentRef.setInput('name', name);
    fixture.detectChanges();
    respond(http.expectOne(`/api/containers/${encodeURIComponent(name)}`));
    await fixture.whenStable();
    return fixture.nativeElement as HTMLElement;
  }

  it('shows release notes newest first, with the first one open and links to the forge', async () => {
    const el = await render('homeassistant', (req) => req.flush(detail()));
    const releases = [...el.querySelectorAll('[data-testid="changes"] details')] as HTMLDetailsElement[];
    expect(releases.map((d) => d.querySelector('summary .font-mono')?.textContent)).toEqual(['2026.10.1', '2026.10.0']);
    expect(releases.map((d) => d.open)).toEqual([true, false]);
    expect(releases[0].querySelector('a[href="https://github.com/home-assistant/core/issues/42"]')).not.toBeNull();
    expect(releases[1].querySelector('h2')?.textContent).toBe('Breaking');

    const links = el.querySelector('[data-testid="links"]')!;
    expect(links.textContent).toContain('Compare 2026.9.0 → 2026.10.1');
    expect(links.querySelector('a[href$="/compare/2026.9.0...2026.10.1"]')).not.toBeNull();
  });

  it('explains a missing repository, missing notes and fetch errors', async () => {
    let el = await render('a', (req) => req.flush(detail({ changelog: undefined })));
    expect(el.textContent).toContain('No source repository found');

    const none = detail();
    none.changelog = { ...none.changelog!, kind: 'none', releases: [], error: 'GitHub rate limit reached' };
    el = await render('b', (req) => req.flush(none));
    expect(el.textContent).toContain('No release notes found');
    expect(el.textContent).toContain("Couldn't refresh the release notes");
  });

  it('shows a not-found state for unknown containers', async () => {
    const el = await render('ghost', (req) => req.flush({ error: 'no container named ghost' }, { status: 404, statusText: 'Not Found' }));
    expect(el.textContent).toContain('No container named ghost');
  });
});
