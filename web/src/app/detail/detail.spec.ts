import { ApplicationRef } from '@angular/core';
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
      update: { running: '2026.9.0', source: 'label', latest: '2026.10.1', class: 'minor', floating: true, newImage: true, checkedAt: '2026-10-16T12:00:00Z', level: 'breaking', reasons: ['2026.10.0: removed X'] },
    },
    image: { digests: ['homeassistant/home-assistant@sha256:1'], labels: { 'org.opencontainers.image.version': '2026.9.0' } },
    changelog: {
      repo: { url: 'https://github.com/home-assistant/core', name: 'home-assistant/core', how: 'image' },
      kind: 'releases',
      releases: [
        { tag: '2026.10.1', name: '2026.10.1', body: 'Fixes (#42)', url: 'https://github.com/home-assistant/core/releases/tag/2026.10.1', publishedAt: '2026-10-10T00:00:00Z', level: 'none', hits: [] },
        { tag: '2026.10.0', name: 'October release', body: '## Breaking\n- removed X', url: 'u',
          level: 'breaking', hits: [{ line: '## Breaking', level: 'breaking' }, { line: '- removed X', level: 'breaking' }] },
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
    expect(releases.map((d) => d.open)).toEqual([true, true]); // the newest, and the flagged one
    expect(releases[0].querySelector('a[href="https://github.com/home-assistant/core/issues/42"]')).not.toBeNull();
    expect(releases[1].querySelector('h2')?.textContent).toBe('Breaking');

    const links = el.querySelector('[data-testid="links"]')!;
    expect(links.textContent).toContain('Compare 2026.9.0 → 2026.10.1');
    expect(links.querySelector('a[href$="/compare/2026.9.0...2026.10.1"]')).not.toBeNull();
  });

  it('explains a breaking verdict and highlights the flagged lines', async () => {
    const el = await render('homeassistant', (req) => req.flush(detail()));
    const reasons = el.querySelector('[data-testid="reasons"]')!;
    expect(reasons.textContent).toContain('Read before updating');
    expect(reasons.querySelector('li')?.textContent).toBe('2026.10.0: removed X');

    const [calm, flagged] = [...el.querySelectorAll('[data-testid="changes"] details')];
    expect(calm.querySelector('summary tm-level')?.textContent?.trim()).toBe('');
    expect(flagged.querySelector('summary tm-level')?.textContent).toContain('Breaking');
    expect(flagged.querySelector('h2')?.className).toBe('tm-flag-breaking');
    expect(flagged.querySelector('li')?.className).toBe('tm-flag-breaking');
    // Only notes without flags are capped behind "Show all".
    expect(calm.querySelector('tm-clamp')).not.toBeNull();
    expect(flagged.querySelector('tm-clamp')).toBeNull();
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

  it('lists the rules in plain words with where each came from', async () => {
    const d = detail();
    d.container.rules = {
      pin: '2026.9',
      prerelease: true,
      tagPattern: '^\\d+\\.\\d+\\.\\d+$',
      repo: 'github.com/home-assistant/core',
      from: { pin: 'label', prerelease: 'config', 'tag-pattern': 'label', repo: 'config' },
      problems: ['tidemark.allow-major: "maybe" is not true or false'],
    };
    d.container.update = { ...d.container.update!, held: '2026.10.1', heldBy: 'pin', latest: '2026.9.4', class: 'patch' };
    d.changelog!.repo.how = 'config';
    const el = await render('homeassistant', (req) => req.flush(d));
    (el.querySelector('[ngTab][value="details"]') as HTMLElement | null)?.click();
    await TestBed.inject(ApplicationRef).whenStable();

    const text = (e: Element | null | undefined) => e?.textContent?.replace(/\s+/g, ' ').trim();
    const rules = el.querySelector('[data-testid="rules"]')!;
    const items = [...rules.querySelectorAll('li')].filter((li) => li.querySelector('[data-testid="rule-origin"]'));
    expect(items.map((li) => text(li.querySelector('div')?.firstElementChild))).toEqual([
      'Pinned to',
      'Pre-releases on',
      'Only tags matching',
      'Release notes from',
    ]);
    expect(items.map((li) => text(li.querySelector('[data-testid="rule-origin"]')))).toEqual([
      'label tidemark.pin',
      'config file',
      'label tidemark.tag-pattern',
      'config file',
    ]);
    expect(text(rules.querySelector('[data-testid="rule-problems-list"]'))).toContain('1 rule skipped');
    expect(text(rules.querySelector('[data-testid="rule-problems-list"]'))).toContain('"maybe" is not true or false');
    expect(text(el.querySelector('[data-testid="details"]'))).toContain('2026.10.1 held back by pin 2026.9');
    expect(text(el.querySelector('[data-testid="details"]'))).toContain('set in the config file');
    // Header: held line and the skipped-rule marker.
    expect(text(el.querySelector('header [data-testid="held"]'))).toBe('2026.10.1 held back by pin 2026.9');
    expect(el.querySelector('header [data-testid="rule-problems"]')).not.toBeNull();
  });

  it('says an ignored container is not checked, and why', async () => {
    const d = detail();
    d.container.update = undefined;
    d.container.rules = { ignore: true, from: { ignore: 'label' } };
    d.changelog = undefined; // ignored: no outside calls, so no release notes
    const el = await render('homeassistant', (req) => req.flush(d));
    expect(el.querySelector('[data-testid="ignored-changes"]')?.textContent).toBe("Release notes aren't fetched for ignored containers.");
    expect(el.textContent).not.toContain('No source repository found');
    const text = (e: Element | null | undefined) => e?.textContent?.replace(/\s+/g, ' ').trim();
    expect(text(el.querySelector('[data-testid="ignored-notice"]'))).toContain(
      "Ignored Tidemark doesn't check this container for updates. Set by the label tidemark.ignore.",
    );
    expect(text(el.querySelector('header tm-container-update'))).toBe('Ignored');

    const cfg = detail();
    cfg.container.update = undefined;
    cfg.container.rules = { ignore: true, from: { ignore: 'config' } };
    const el2 = await render('other', (req) => req.flush(cfg));
    expect(text(el2.querySelector('[data-testid="ignored-notice"]'))).toContain('Set in the config file.');
  });

  it('shows a not-found state for unknown containers', async () => {
    const el = await render('ghost', (req) => req.flush({ error: 'no container named ghost' }, { status: 404, statusText: 'Not Found' }));
    expect(el.textContent).toContain('No container named ghost');
  });
});
