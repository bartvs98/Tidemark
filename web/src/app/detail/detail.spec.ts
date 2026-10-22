import { ApplicationRef } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { ContainerDetail } from './detail';
import { Toasts } from '../ui';
import { ContainerDetailData, HistoryEntry } from './detail-types';

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
      update: { running: '2026.9.0', source: 'label', latest: '2026.10.1', reviewKey: '2026.10.1', class: 'minor', floating: true, newImage: true, checkedAt: '2026-10-16T12:00:00Z', level: 'breaking',
        reasons: [{ text: 'Major version: 2026 → 2027' }, { text: 'removed X', tag: '2026.10.0', line: '- removed X' }] },
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
    history: [],
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
    const items = [...reasons.querySelectorAll('[data-testid="reason"]')];
    expect(items.map((li) => li.querySelector('[data-testid="reason-text"]')?.textContent)).toEqual(['Major version: 2026 → 2027', 'removed X']);
    expect(items[0].querySelector('button')).toBeNull(); // no quote, nothing to jump to
    expect(items[1].querySelector('[data-testid="reason-jump"]')?.textContent?.trim()).toBe('2026.10.0');

    const [calm, flagged] = [...el.querySelectorAll('[data-testid="changes"] details')];
    expect(calm.querySelector('summary tm-level')?.textContent?.trim()).toBe('');
    expect(flagged.querySelector('summary tm-level')?.textContent).toContain('Breaking');
    expect(flagged.querySelector('h2')?.className).toBe('tm-flag-breaking');
    expect(flagged.querySelector('li')?.className).toBe('tm-flag-breaking');
    // All notes are capped behind "Show all", flagged ones too (the button counts hidden flags).
    expect(calm.querySelector('tm-clamp')).not.toBeNull();
    expect(flagged.querySelector('tm-clamp')).not.toBeNull();
  });

  it('shows a long reason whole', async () => {
    const d = detail();
    const long = 'removed '.repeat(40) + 'https://example.com/a/very/long/url/without/any/spaces/'.repeat(4);
    d.container.update!.reasons = [{ text: long, tag: '2026.10.0', line: 'not in the notes' }];
    const el = await render('homeassistant', (req) => req.flush(d));
    const li = el.querySelector('[data-testid="reason"]')!;
    expect(li.querySelector('[data-testid="reason-text"]')?.textContent).toBe(long);
    expect(li.classList).toContain('wrap-anywhere');
    // The line isn't among the release's hits: the tag shows, without a jump.
    expect(li.querySelector('button')).toBeNull();
    expect(li.querySelector('.font-mono')?.textContent).toBe('2026.10.0');
  });

  describe('AI summary', () => {
    const AI = { provider: 'anthropic', model: 'claude-haiku-5-5', autoMinReleases: 0, releases: 2, generating: false } as const;

    it('puts the summary card below the verdict callout and above the links, only when the server sends ai', async () => {
      const el = await render('homeassistant', (req) => req.flush(detail({ ai: AI })));
      const card = el.querySelector('[data-testid="ai-summary"]')!;
      expect(card).not.toBeNull();
      expect(el.querySelector('[data-testid="reasons"]')!.compareDocumentPosition(card) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      expect(card.compareDocumentPosition(el.querySelector('[data-testid="links"]')!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });

    it('has no summary card when AI is off', async () => {
      const el = await render('homeassistant', (req) => req.flush(detail()));
      expect(el.querySelector('[data-testid="ai-summary"]')).toBeNull();
    });

    it('reloads the detail after a summary was made, since the level may rise', async () => {
      const el = await render('homeassistant', (req) => req.flush(detail({ ai: AI })));
      (el.querySelector('[data-testid="ai-generate"]') as HTMLButtonElement).click();
      await TestBed.inject(ApplicationRef).whenStable();
      http.expectOne('/api/containers/homeassistant/summary').flush({ ...AI });
      await new Promise((r) => setTimeout(r));
      TestBed.inject(ApplicationRef).tick();
      http.expectOne('/api/containers/homeassistant').flush(detail({ ai: AI }));
      await TestBed.inject(ApplicationRef).whenStable();
    });

    it('marks AI reasons, with no jump button even though a heuristic quote has one', async () => {
      const d = detail({ ai: AI });
      d.container.update!.reasons = [
        { text: 'removed X', tag: '2026.10.0', line: '- removed X' },
        { text: 'The config key was renamed.', tag: '2026.10.0', source: 'ai' },
        { text: 'No way back.', source: 'ai' },
      ];
      const el = await render('homeassistant', (req) => req.flush(d));
      const items = [...el.querySelectorAll('[data-testid="reasons"] [data-testid="reason"]')];
      expect(items.map((li) => li.querySelector('[data-testid="ai-marker"]') !== null)).toEqual([false, true, true]);
      expect(items[0].querySelector('[data-testid="reason-jump"]')).not.toBeNull();
      expect(items[1].querySelector('button')).toBeNull();
      expect(items[1].querySelector('.font-mono')?.textContent).toBe('2026.10.0');
      expect(items[2].querySelector('button')).toBeNull();
    });
  });

  it('jumps from a quoted reason to its line: Changes tab, release opened, scrolled, focused, highlighted', async () => {
    const original = Element.prototype.scrollIntoView;
    const scrollIntoView = vi.fn();
    Element.prototype.scrollIntoView = scrollIntoView;
    try {
      const d = detail();
      // An attention release stays closed until jumped to.
      const release = d.changelog!.releases[1];
      release.level = 'attention';
      release.hits = release.hits.map((h) => ({ ...h, level: 'attention' }));
      const el = await render('homeassistant', (req) => req.flush(d));
      const app = TestBed.inject(ApplicationRef);

      (el.querySelector('[ngTab][value="details"]') as HTMLElement).click();
      await app.whenStable();
      expect(el.querySelector('[data-testid="changes"]')).toBeNull();

      (el.querySelector('[data-testid="reason-jump"]') as HTMLButtonElement).click();
      await app.whenStable();
      await app.whenStable();

      const flagged = el.querySelector<HTMLDetailsElement>('details[data-release="2026.10.0"]')!;
      expect(flagged.open).toBe(true);
      const line = flagged.querySelector<HTMLElement>('[data-flag~="1"]')!;
      expect(line.textContent).toBe('removed X');
      expect(scrollIntoView).toHaveBeenCalledWith(expect.objectContaining({ block: 'center' }));
      expect(scrollIntoView.mock.contexts[0]).toBe(line);
      expect(document.activeElement).toBe(line);
      expect(line.tabIndex).toBe(-1);
      expect(line.classList).toContain('tm-flag-jumped');
    } finally {
      Element.prototype.scrollIntoView = original;
    }
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

  describe('Review actions', () => {
    const URL = '/api/containers/homeassistant';
    const text = (e: Element | null | undefined) => e?.textContent?.replace(/\s+/g, ' ').trim();
    const header = (el: HTMLElement) => el.querySelector('[data-testid="detail-header"]')!;
    const toasts = () => TestBed.inject(Toasts).items();
    const settle = () => TestBed.inject(ApplicationRef).whenStable();
    /** Lets a resolved request's promise chain run (whenStable would wait for the reload it starts). */
    const tick = async () => {
      await new Promise((r) => setTimeout(r));
      TestBed.inject(ApplicationRef).tick();
    };

    /** Answers the PUT/DELETE and expects the page to reload the detail afterwards. */
    async function answer(method: 'PUT' | 'DELETE', reload: ContainerDetailData, url = URL) {
      const req = http.expectOne(`${url}/review`);
      expect(req.request.method).toBe(method);
      const body = req.request.body;
      if (method === 'PUT') req.flush({ ...body, at: '2026-10-20T08:00:00Z' });
      else req.flush(null, { status: 204, statusText: 'No Content' });
      await tick();
      http.expectOne(url).flush(reload);
      await settle();
      return body;
    }

    async function pickSnooze(el: HTMLElement, label: string) {
      el.querySelector<HTMLButtonElement>('[data-testid="snooze"] button')!.click();
      await settle();
      const item = [...el.querySelectorAll<HTMLElement>('[data-testid="snooze"] [role="menuitem"]')].find((i) => text(i) === label)!;
      item.click();
      await settle();
    }

    it('marks the update as reviewed up to the latest version, toasts and reloads', async () => {
      const el = await render('homeassistant', (req) => req.flush(detail()));
      expect(el.querySelector('[data-testid="review-actions"] [data-testid="review-status"]')).toBeNull();
      // They sit on the tab row, not in the header card (keeps the header short).
      expect(el.querySelector('header [data-testid="review-actions"]')).toBeNull();
      el.querySelector<HTMLButtonElement>('[data-testid="review-actions"] [data-testid="mark-reviewed"]')!.click();
      await settle();

      const reviewed = detail();
      reviewed.container.review = { kind: 'reviewed', version: '2026.10.1', at: '2026-10-20T08:00:00Z' };
      expect(await answer('PUT', reviewed)).toEqual({ kind: 'reviewed', version: '2026.10.1' });
      expect(toasts().at(-1)).toEqual(expect.objectContaining({ tone: 'ok', message: 'Marked as reviewed up to 2026.10.1' }));
      expect(text(el.querySelector('[data-testid="review-actions"] [data-testid="review-status"]'))).toBe('Reviewed up to 2026.10.1 · 20 Oct');
      expect(el.querySelector('[data-testid="review-actions"] [data-testid="mark-reviewed"]')).toBeNull();
      // The status line says it once: no dashboard "Reviewed" marker in the header.
      expect(el.querySelector('header [data-testid="reviewed"]')).toBeNull();
    });

    it('shows a snooze once, in the status line, not as the dashboard "until" line', async () => {
      const snoozed = detail();
      snoozed.container.review = { kind: 'snoozed', until: '2026-10-27T08:00:00Z', at: '2026-10-20T08:00:00Z' };
      const el = await render('homeassistant', (req) => req.flush(snoozed));
      expect(el.querySelector('[data-testid="review-actions"] [data-testid="review-status"]')).not.toBeNull();
      expect(el.querySelector('header [data-testid="snoozed"]')).toBeNull();
    });

    it('disables the controls while the request runs', async () => {
      const el = await render('homeassistant', (req) => req.flush(detail()));
      el.querySelector<HTMLButtonElement>('[data-testid="review-actions"] [data-testid="mark-reviewed"]')!.click();
      await settle();
      expect(el.querySelector<HTMLButtonElement>('[data-testid="mark-reviewed"]')!.disabled).toBe(true);
      expect(el.querySelector<HTMLButtonElement>('[data-testid="snooze"] button')!.disabled).toBe(true);
      await answer('PUT', detail());
      expect(el.querySelector<HTMLButtonElement>('[data-testid="mark-reviewed"]')!.disabled).toBe(false);
    });

    it.each([
      ['1 day', 1],
      ['1 week', 7],
      ['1 month', 30],
    ])('snoozes for %s with an until computed from now', async (label, days) => {
      const el = await render('homeassistant', (req) => req.flush(detail()));
      const before = Date.now();
      await pickSnooze(el, label);
      const body = await answer('PUT', detail());
      expect(body.kind).toBe('snoozed');
      expect(body.version).toBeUndefined();
      const offset = Date.parse(body.until) - before;
      expect(Math.abs(offset - days * 24 * 3600 * 1000)).toBeLessThan(60_000);
      expect(toasts().at(-1)?.message).toBe(`Snoozed for ${label}`);
    });

    it('snoozes until a newer version than the latest', async () => {
      const el = await render('homeassistant', (req) => req.flush(detail()));
      await pickSnooze(el, 'Until a newer version');
      expect(await answer('PUT', detail())).toEqual({ kind: 'snoozed', version: '2026.10.1' });
    });

    it('reviews and snoozes against the build when the tag has no usable latest version', async () => {
      const build = 'sha256:3f9a1c0d2b7e8a41';
      const d = detail();
      d.container.update = { ...d.container.update!, latest: undefined, tagDigest: build, reviewKey: build };
      let el = await render('homeassistant', (req) => req.flush(d));
      expect(el.querySelector('[data-testid="review-actions"] [data-testid="mark-reviewed"]')!.parentElement?.getAttribute('data-tooltip')).toContain(
        'looked at this build',
      );
      el.querySelector<HTMLButtonElement>('[data-testid="review-actions"] [data-testid="mark-reviewed"]')!.click();
      await settle();
      expect(await answer('PUT', d)).toEqual({ kind: 'reviewed', version: build });
      expect(toasts().at(-1)?.message).toBe('Marked this build as reviewed');

      await pickSnooze(el, 'Until a new build');
      expect(await answer('PUT', d)).toEqual({ kind: 'snoozed', version: build });
      expect(toasts().at(-1)?.message).toBe('Snoozed until a new build');

      const reviewed = detail();
      reviewed.container.name = 'other';
      reviewed.container.update = d.container.update;
      reviewed.container.review = { kind: 'reviewed', version: build, at: '2026-10-20T08:00:00Z' };
      el = await render('other', (req) => req.flush(reviewed));
      expect(text(el.querySelector('[data-testid="review-actions"] [data-testid="review-status"]'))).toBe('Reviewed this build · 20 Oct');

      const snoozed = detail();
      snoozed.container.name = 'third';
      snoozed.container.update = d.container.update;
      snoozed.container.review = { kind: 'snoozed', version: build, at: '2026-10-20T08:00:00Z' };
      el = await render('third', (req) => req.flush(snoozed));
      expect(text(el.querySelector('[data-testid="review-actions"] [data-testid="review-status"]'))).toBe('Snoozed until a new build');
    });

    it("can't mark or snooze by version without a latest version, and says why", async () => {
      const d = detail();
      d.container.update = { ...d.container.update!, latest: undefined, reviewKey: undefined };
      const el = await render('homeassistant', (req) => req.flush(d));
      const mark = el.querySelector<HTMLButtonElement>('[data-testid="review-actions"] [data-testid="mark-reviewed"]')!;
      expect(mark.disabled).toBe(true);
      expect(mark.parentElement?.getAttribute('data-tooltip')).toContain('no version or build');
      const newer = [...el.querySelectorAll<HTMLElement>('[data-testid="snooze"] [role="menuitem"]')].find((i) => text(i) === 'Until a newer version')!;
      expect(newer.getAttribute('aria-disabled')).toBe('true');
      expect(newer.getAttribute('data-tooltip')).toContain('no version or build');
      await pickSnooze(el, 'Until a newer version');
      http.expectNone(`${URL}/review`);
    });

    it('undoes a review with DELETE and stills the severity frame while reviewed', async () => {
      const d = detail();
      d.container.review = { kind: 'reviewed', version: '2026.10.1', at: '2026-10-20T08:00:00Z' };
      const el = await render('homeassistant', (req) => req.flush(d));
      // The frame keeps its colour but stops circling; the level and reasons stay.
      expect(header(el).classList).toContain('tm-severity-frame');
      expect(header(el).classList).toContain('before:hidden');
      expect(text(header(el).querySelector('[data-testid="level"]'))).toBe('Breaking');
      expect(el.querySelector('[data-testid="reasons"]')).not.toBeNull();

      const undo = el.querySelector<HTMLButtonElement>('[data-testid="review-actions"] [data-testid="review-undo"]')!;
      expect(text(undo)).toBe('Undo');
      undo.click();
      await settle();
      await answer('DELETE', detail());
      expect(toasts().at(-1)).toEqual(expect.objectContaining({ tone: 'ok' }));
      expect(el.querySelector('[data-testid="review-actions"] [data-testid="mark-reviewed"]')).not.toBeNull();
      expect(header(el).classList).not.toContain('before:hidden');
    });

    it('shows a snooze by time or by version, with Unsnooze', async () => {
      const byTime = detail();
      byTime.container.review = { kind: 'snoozed', until: '2026-11-03T08:00:00Z', at: '2026-10-20T08:00:00Z' };
      let el = await render('homeassistant', (req) => req.flush(byTime));
      expect(text(el.querySelector('[data-testid="review-actions"] [data-testid="review-status"]'))).toBe('Snoozed until 3 Nov');
      expect(text(el.querySelector('[data-testid="review-actions"] [data-testid="review-undo"]'))).toBe('Unsnooze');

      const byVersion = detail();
      byVersion.container.name = 'other';
      byVersion.container.review = { kind: 'snoozed', version: '2026.10.1', at: '2026-10-20T08:00:00Z' };
      el = await render('other', (req) => req.flush(byVersion));
      expect(text(el.querySelector('[data-testid="review-actions"] [data-testid="review-status"]'))).toBe('Snoozed until a version newer than 2026.10.1');
      el.querySelector<HTMLButtonElement>('[data-testid="review-actions"] [data-testid="review-undo"]')!.click();
      await settle();
      await answer('DELETE', detail(), '/api/containers/other');
      expect(toasts().at(-1)?.message).toBe('Unsnoozed');
    });

    it("toasts a failure and doesn't reload", async () => {
      const el = await render('homeassistant', (req) => req.flush(detail()));
      el.querySelector<HTMLButtonElement>('[data-testid="review-actions"] [data-testid="mark-reviewed"]')!.click();
      await settle();
      http.expectOne(`${URL}/review`).flush({ error: 'version is required' }, { status: 400, statusText: 'Bad Request' });
      await tick();
      await settle();
      expect(toasts().at(-1)).toEqual(expect.objectContaining({ tone: 'attention', message: "Couldn't save that: version is required" }));
      http.expectNone(URL);
      expect(el.querySelector<HTMLButtonElement>('[data-testid="mark-reviewed"]')!.disabled).toBe(false);
    });

    it('is hidden without an update, and for ignored containers', async () => {
      const upToDate = detail();
      upToDate.container.update = { ...upToDate.container.update!, class: 'none', level: 'none', reasons: [] };
      let el = await render('a', (req) => req.flush(upToDate));
      expect(el.querySelector('[data-testid="review-actions"]')).toBeNull();

      const ignored = detail();
      ignored.container.rules = { ignore: true, from: { ignore: 'label' } };
      el = await render('b', (req) => req.flush(ignored));
      expect(el.querySelector('[data-testid="review-actions"]')).toBeNull();
    });
  });

  describe('History tab', () => {
    const text = (e: Element | null | undefined) => e?.textContent?.replace(/\s+/g, ' ').trim();
    async function historyTab(history: HistoryEntry[]) {
      const el = await render('homeassistant', (req) => req.flush(detail({ history })));
      (el.querySelector('[ngTab][value="history"]') as HTMLElement).click();
      await TestBed.inject(ApplicationRef).whenStable();
      return el;
    }

    it('lists image changes newest first, with how long the previous version ran', async () => {
      const el = await historyTab([
        { at: '2026-10-18T07:00:00Z', running: '2026.10.1', imageId: 'sha256:cccccccccccc1111', prevRunning: '2026.10.1', prevImageId: 'sha256:bbbbbbbbbbbb2222' },
        { at: '2026-10-14T07:00:00Z', running: '2026.10.1', imageId: 'sha256:bbbbbbbbbbbb2222', prevRunning: '2026.9.0', prevImageId: 'sha256:aaaaaaaaaaaa3333' },
        { at: '2026-10-02T07:00:00Z', running: '2026.9.0', imageId: 'sha256:aaaaaaaaaaaa3333' },
      ]);
      expect(text(el.querySelector('[ngTab][value="history"]'))).toBe('History 3');
      const rows = [...el.querySelectorAll('[data-testid="history-row"]')];
      expect(rows.map((r) => text(r.querySelector('[data-testid="history-title"]')))).toEqual([
        'New image · 2026.10.1',
        '2026.9.0 → 2026.10.1',
        'Tracking started · running 2026.9.0',
      ]);
      expect(text(rows[0].querySelector('[data-testid="history-ids"]'))).toBe('bbbbbbbbbbbb → cccccccccccc');
      expect(rows.map((r) => text(r.querySelector('[data-testid="history-ran"]')))).toEqual([
        'ran 2026.10.1 for 4 days',
        'ran 2026.9.0 for 12 days',
        undefined,
      ]);
      expect(rows.map((r) => text(r.querySelector('time')))).toEqual(['18 Oct', '14 Oct', '2 Oct']);
    });

    it('says "New image" with short IDs when the version is unknown', async () => {
      const el = await historyTab([{ at: '2026-10-18T07:00:00Z', imageId: 'sha256:0123456789abcdef', prevImageId: 'sha256:fedcba9876543210' }]);
      const row = el.querySelector('[data-testid="history-row"]')!;
      expect(text(row.querySelector('[data-testid="history-title"]'))).toBe('New image');
      expect(text(row.querySelector('[data-testid="history-ids"]'))).toBe('fedcba987654 → 0123456789ab');
      expect(row.querySelector('[data-testid="history-ran"]')).toBeNull(); // the older row isn't there
    });

    it('shows an empty state before the first successful check', async () => {
      const el = await historyTab([]);
      expect(text(el.querySelector('[ngTab][value="history"]'))).toBe('History');
      expect(el.querySelector('[data-testid="history"]')).toBeNull();
      expect(text(el.querySelector('[data-testid="history-empty"]'))).toBe('No history yetIt starts at the first successful check.');
    });
  });
});
