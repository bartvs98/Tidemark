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

  describe('Checked row', () => {
    const text = (e: Element | null | undefined) => e?.textContent?.replace(/\s+/g, ' ').trim();
    async function details(d: ContainerDetailData) {
      const el = await render('homeassistant', (req) => req.flush(d));
      (el.querySelector('[ngTab][value="details"]') as HTMLElement | null)?.click();
      await TestBed.inject(ApplicationRef).whenStable();
      return el;
    }

    it('shows the last check, and no failure when it worked', async () => {
      const el = await details(detail());
      expect(el.querySelector('[data-testid="checked"]')).not.toBeNull();
      expect(el.querySelector('[data-testid="check-attempt-failed"]')).toBeNull();
      expect(el.querySelector('header [data-testid="refresh-failed"]')).toBeNull();
    });

    it('keeps the last result and adds the failed refresh', async () => {
      const d = detail();
      d.container.update = { ...d.container.update!, error: 'dial tcp: i/o timeout', failedAt: '2026-10-17T07:02:00Z' };
      const el = await details(d);

      // Header: the result stays, with the quiet note.
      expect(text(el.querySelector('header tm-container-version'))?.replace(/ /g, '')).toContain('2026.9.0→2026.10.1');
      expect(text(el.querySelector('header [data-testid="level"]'))).toBe('Breaking');
      expect(text(el.querySelector('header [data-testid="refresh-failed"]'))).toBe('refresh failed');
      expect(el.querySelector('header [data-testid="check-failed"]')).toBeNull();
      expect(el.querySelector('[data-testid="reasons"]')).not.toBeNull();

      const checked = el.querySelector('[data-testid="checked"]')!;
      expect(text(checked)).toMatch(/^Oct 16, 2026/); // the last successful check
      expect(text(checked)).toContain('refresh failed, see above');
      expect(checked.querySelector('[data-testid="check-attempt-failed"]')).toBeNull();
      // The callout carries the details.
      const failed = el.querySelector('[data-testid="check-attempt-failed"]')!;
      expect(text(failed)).toContain('Refresh failed at Oct 17, 2026');
      expect(text(failed)).toContain('Showing the result from Oct 16, 2026');
      expect(failed.querySelector('.font-mono')?.textContent).toBe('dial tcp: i/o timeout');
      expect(el.querySelector('[data-testid="failure-hint"]')).toBeNull(); // no kind, no hint
    });

    it.each([
      ['rate-limit', 'Tidemark tries again at the next check.'],
      ['not-found', ''], // its message already says why
      ['timeout', "Check the server's network or DNS."],
      ['network', "Check the server's network or DNS."],
      ['registry', 'The registry had a problem; Tidemark tries again at the next check.'],
    ] as const)('shows the %s hint and short reason', async (kind, hint) => {
      const d = detail();
      d.container.update = { ...d.container.update!, error: 'x: failed', errorKind: kind, failedAt: '2026-10-17T07:02:00Z' };
      const el = await details(d);
      const shown = el.querySelector('[data-testid="failure-hint"]');
      if (hint) expect(text(shown)).toBe(hint);
      else expect(shown).toBeNull();
      expect(el.querySelector('header [data-testid="refresh-failed"] [data-testid="failure-reason"]')).not.toBeNull();
    });

    it('notes a reused tag list quietly under Checked', async () => {
      const d = detail();
      d.container.update = { ...d.container.update!, tagsFrom: '2026-10-15T07:02:00Z' };
      const el = await details(d);
      const note = text(el.querySelector('[data-testid="checked"] [data-testid="tags-from"]'));
      expect(note).toMatch(/^Tag list from Oct 15, 2026, .* \(the registry rate-limited the refresh\)$/);
      expect(el.querySelector('[data-testid="check-attempt-failed"]')).toBeNull();
    });

    it('says there was no successful check when the only attempt failed', async () => {
      const d = detail();
      d.container.update = { source: 'unknown', class: 'none', floating: false, newImage: false, level: 'none', error: 'not found, or private', failedAt: '2026-10-17T07:02:00Z' };
      const el = await details(d);

      expect(text(el.querySelector('header [data-testid="check-failed"]'))).toBe('check failed');
      expect(el.querySelector('header [data-testid="refresh-failed"]')).toBeNull();
      const checked = el.querySelector('[data-testid="checked"]')!;
      expect(text(checked)).toContain('No successful check yet');
      expect(text(checked)).toContain('check failed, see above');
      const failed = text(el.querySelector('[data-testid="check-attempt-failed"]'));
      expect(failed).toContain('Check failed at Oct 17, 2026');
      expect(failed).not.toContain('Showing the result');
      expect(text(el.querySelector('[data-testid="details"]'))).not.toContain('from unknown');
    });
  });

  describe('Header tide gauge', () => {
    const behind = (el: HTMLElement) => el.querySelector('header [data-testid="behind"]');
    function withReleases(n: number, partial = false): ContainerDetailData {
      const d = detail();
      const r = d.changelog!.releases[0];
      d.changelog = { ...d.changelog!, partial, releases: Array.from({ length: n }, (_, i) => ({ ...r, tag: `v${i}` })) };
      return d;
    }

    it('says how many releases behind, singular and plural', async () => {
      let el = await render('one', (req) => req.flush(withReleases(1)));
      expect(behind(el)?.textContent?.trim()).toBe('1 release behind');
      expect(behind(el)?.querySelector('tm-tide-gauge')?.getAttribute('aria-label')).toBe('1 release behind');

      el = await render('six', (req) => req.flush(withReleases(6)));
      expect(behind(el)?.textContent?.trim()).toBe('6 releases behind');
      expect((behind(el)?.querySelector('tm-tide-gauge') as HTMLElement).style.getPropertyValue('--tm-level')).toBe('0.5');
    });

    it('adds "+" when the list may be cut off', async () => {
      let el = await render('partial', (req) => req.flush(withReleases(30, true)));
      expect(behind(el)?.textContent?.trim()).toBe('30+ releases behind');
      // Far behind still leaves some water.
      expect((behind(el)?.querySelector('tm-tide-gauge') as HTMLElement).style.getPropertyValue('--tm-level')).toBe('0.15');

      el = await render('partial-one', (req) => req.flush(withReleases(1, true)));
      expect(behind(el)?.textContent?.trim()).toBe('1+ releases behind');
    });

    it('is hidden without releases, without an update or with an unknown running version', async () => {
      let el = await render('none', (req) => req.flush(withReleases(0)));
      expect(behind(el)).toBeNull();

      const current = withReleases(2);
      current.container.update = { ...current.container.update!, class: 'none', level: 'none', latest: undefined };
      el = await render('current', (req) => req.flush(current));
      expect(behind(el)).toBeNull();

      const unknown = withReleases(2);
      unknown.container.update = { ...unknown.container.update!, running: undefined, source: 'unknown' };
      el = await render('unknown', (req) => req.flush(unknown));
      expect(behind(el)).toBeNull();
    });
  });

  describe('Severity frame', () => {
    const header = (el: HTMLElement) => el.querySelector('[data-testid="detail-header"]')!;
    async function withLevel(level: 'none' | 'attention' | 'breaking') {
      const d = detail();
      d.container.update = { ...d.container.update!, level };
      return render(`level-${level}`, (req) => req.flush(d));
    }

    it('frames the header for breaking and attention, next to the level badge', async () => {
      for (const level of ['breaking', 'attention'] as const) {
        const el = await withLevel(level);
        expect(header(el).classList).toContain('tm-severity-frame');
        expect(header(el).getAttribute('data-level')).toBe(level);
        expect(header(el).querySelector('[data-testid="level"]')?.textContent).toContain(level === 'breaking' ? 'Breaking' : 'Attention');
      }
    });

    it('has no frame otherwise', async () => {
      const el = await withLevel('none');
      expect(header(el).classList).not.toContain('tm-severity-frame');
      expect(header(el).hasAttribute('data-level')).toBe(false);
    });
  });

  it('shows hover help as tooltips, not native titles', async () => {
    const el = await render('homeassistant', (req) => req.flush(detail()));
    expect(el.querySelector('header [data-tooltip="homeassistant/home-assistant:latest"]')).not.toBeNull();
    expect(el.querySelector('[title]')).toBeNull();
  });

  it('shows a not-found state for unknown containers', async () => {
    const el = await render('ghost', (req) => req.flush({ error: 'no container named ghost' }, { status: 404, statusText: 'Not Found' }));
    expect(el.textContent).toContain('No container named ghost');
  });
});
