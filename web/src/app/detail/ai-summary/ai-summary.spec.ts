import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { AI_POLL_MS, AiSummary } from './ai-summary';
import { AiState } from '../detail-types';

const BASE: AiState = {
  provider: 'anthropic',
  model: 'claude-haiku-5-5',
  autoMinReleases: 0,
  releases: 3,
  generating: false,
};
const SUMMARY: NonNullable<AiState['summary']> = {
  changes: ['Adds a backup job.', 'Fixes a crash <b>on start</b>.'],
  actions: ['Rename DB_URL to DATABASE_URL.'],
  verdict: 'breaking',
  reasons: [{ text: 'DB_URL was renamed.', tag: 'v2.0.0' }, { text: 'No rollback.' }],
  provider: 'anthropic',
  model: 'claude-haiku-5-5',
  releases: 3,
  omitted: 0,
  createdAt: new Date(Date.now() - 3 * 3600_000).toISOString(),
  stale: false,
  inputTokens: 12340,
  outputTokens: 640,
  durationMs: 4200,
};
const URL = '/api/containers/home%20assistant/summary';

describe('AiSummary', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    vi.useRealTimers();
    http.verify();
  });

  async function render(ai: AiState) {
    const fixture = TestBed.createComponent(AiSummary);
    fixture.componentRef.setInput('name', 'home assistant');
    fixture.componentRef.setInput('ai', ai);
    const changed = vi.fn();
    fixture.componentInstance.changed.subscribe(changed);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    const q = (id: string) => el.querySelector<HTMLElement>(`[data-testid="${id}"]`);
    return { fixture, el, q, changed };
  }

  it('shows the summary as plain text: changes, actions, the verdict with its reasons, and where it came from', async () => {
    const { el, q } = await render({ ...BASE, summary: SUMMARY });
    expect(el.querySelector('h3')?.textContent).toBe('Summary');
    expect(el.querySelector('[data-testid="ai-marker"]')).not.toBeNull();
    // Model text is interpolated, never parsed as HTML.
    expect(q('ai-changes')?.textContent).toContain('Fixes a crash <b>on start</b>.');
    expect(q('ai-changes')?.querySelector('b')).toBeNull();
    expect(q('ai-changes')?.querySelectorAll('li').length).toBe(2);
    expect(q('ai-actions')?.textContent).toContain('What you need to do');
    expect(q('ai-verdict')?.textContent).toContain('Breaking');
    const reasons = [...el.querySelectorAll('[data-testid="ai-reason"]')];
    expect(reasons.map((r) => r.textContent?.replace(/\s+/g, ' ').trim())).toEqual([
      'v2.0.0 DB_URL was renamed.',
      'No rollback.',
    ]);
    expect(reasons[0].querySelector('.font-mono')?.textContent).toBe('v2.0.0');
    expect(q('ai-meta')?.querySelector('.font-mono')?.textContent).toBe('claude-haiku-5-5');
    expect(q('ai-meta')?.textContent).toContain('3 releases');
    expect(q('ai-meta')?.textContent).toContain('3 hours ago');
    expect(q('ai-stats')?.textContent?.trim()).toBe('· 12.3k tokens in · 640 out · 4.2 s');
    expect(q('ai-omitted')).toBeNull();
    expect(q('ai-stale')).toBeNull();
    expect(q('ai-regenerate')).not.toBeNull();
  });

  it('leaves out the actions when empty, says nothing-to-flag for verdict none and counts what was left out', async () => {
    const { q } = await render({
      ...BASE,
      summary: { ...SUMMARY, actions: [], verdict: 'none', reasons: [], omitted: 4, releases: 20 },
    });
    expect(q('ai-actions')).toBeNull();
    expect(q('ai-verdict')?.textContent).toContain('Nothing to flag');
    expect(q('ai-omitted')?.textContent).toContain('4 left out');
    expect(q('ai-meta')?.textContent).toContain('20 releases');
  });

  it('notes a stale summary quietly and keeps it', async () => {
    const { q } = await render({ ...BASE, summary: { ...SUMMARY, stale: true } });
    expect(q('ai-stale')?.textContent).toContain('The notes changed since this summary');
    expect(q('ai-changes')).not.toBeNull();
  });

  it('offers to summarise when there is none, naming where the notes go', async () => {
    const { el, q } = await render({ ...BASE, autoMinReleases: 5 });
    expect(el.querySelector('h3')).toBeNull(); // the compact card
    expect(q('ai-generate')?.textContent?.trim()).toBe('Summarise 3 releases');
    expect(q('ai-empty')?.textContent).toContain(
      'Sends the release notes to Anthropic (claude-haiku-5-5).',
    );
    expect(q('ai-empty')?.textContent).toContain(
      'Updates with 5+ releases are summarised after each check.',
    );
    expect(q('ai-changes')).toBeNull();
  });

  it('names an OpenAI-compatible endpoint, has no automatic line when off and counts one release', async () => {
    const { q } = await render({ ...BASE, provider: 'openai', model: 'llama3.1:8b', releases: 1 });
    expect(q('ai-generate')?.textContent?.trim()).toBe('Summarise 1 release');
    expect(q('ai-empty')?.textContent).toContain('an OpenAI-compatible endpoint (llama3.1:8b)');
    expect(q('ai-empty')?.textContent).not.toContain('summarised after each check');
  });

  it('disables the button when there are no releases to summarise', async () => {
    const { q } = await render({ ...BASE, releases: 0 });
    expect((q('ai-generate') as HTMLButtonElement).disabled).toBe(true);
  });

  it('shows placeholder lines while the server generates, with no button to press', async () => {
    const { q } = await render({ ...BASE, generating: true });
    expect(q('ai-generating')?.textContent).toContain('Summarising 3 releases…');
    expect(q('ai-generate')).toBeNull();
  });

  it('keeps a summary visible while regenerating, with a busy button', async () => {
    const { q } = await render({ ...BASE, generating: true, summary: SUMMARY });
    expect(q('ai-changes')).not.toBeNull();
    expect(q('ai-regenerate')?.getAttribute('aria-busy')).toBe('true');
  });

  it('posts to the summary endpoint, shows the answer at once and tells the page to reload', async () => {
    const { fixture, q, changed } = await render(BASE);
    q('ai-generate')!.click();
    await fixture.whenStable();
    expect(q('ai-generating')).not.toBeNull(); // busy while the request runs
    const req = http.expectOne(URL);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({}); // the server may extend an earlier summary
    req.flush({ ...BASE, summary: SUMMARY });
    await new Promise((r) => setTimeout(r));
    await fixture.whenStable();
    expect(q('ai-changes')?.textContent).toContain('Adds a backup job.');
    expect(changed).toHaveBeenCalledTimes(1);
  });

  it('regenerates a kept summary in full', async () => {
    const { fixture, q } = await render({ ...BASE, summary: SUMMARY });
    q('ai-regenerate')!.click();
    await fixture.whenStable();
    const req = http.expectOne(URL);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ mode: 'full' });
    req.flush({ ...BASE, summary: { ...SUMMARY, changes: ['Newer.'] } });
    await new Promise((r) => setTimeout(r));
    await fixture.whenStable();
    expect(q('ai-changes')?.textContent).toContain('Newer.');
  });

  it('says which version an extended summary was built on', async () => {
    const { q } = await render({ ...BASE, summary: { ...SUMMARY, extendedFrom: '3.1.0' } });
    expect(q('ai-extended')?.textContent).toContain('extended from 3.1.0');
    const { q: q2 } = await render({ ...BASE, summary: SUMMARY });
    expect(q2('ai-extended')).toBeNull();
  });

  it('shows the stored error with Retry, and a kept summary stays', async () => {
    const { q } = await render({
      ...BASE,
      summary: SUMMARY,
      error: 'rate limited by Anthropic',
      failedAt: '2026-10-22T08:00:00Z',
    });
    expect(q('ai-error')?.textContent).toContain("Couldn't summarise");
    expect(q('ai-error')?.textContent).toContain('rate limited by Anthropic');
    expect(q('ai-retry')).not.toBeNull();
    expect(q('ai-changes')).not.toBeNull();
  });

  it('shows the server message when the request fails, then retries', async () => {
    const { fixture, q, changed } = await render({ ...BASE, error: 'old' });
    q('ai-retry')!.click();
    await fixture.whenStable();
    http
      .expectOne(URL)
      .flush({ error: 'the API key was rejected' }, { status: 502, statusText: 'Bad Gateway' });
    await new Promise((r) => setTimeout(r));
    await fixture.whenStable();
    expect(q('ai-error')?.textContent).toContain('the API key was rejected');
    expect(q('ai-empty')).not.toBeNull();
    expect(changed).toHaveBeenCalledTimes(1); // a failure is stored too: reload

    q('ai-retry')!.click();
    await fixture.whenStable();
    http.expectOne(URL).flush({ ...BASE, summary: SUMMARY });
    await new Promise((r) => setTimeout(r));
    await fixture.whenStable();
    expect(q('ai-error')).toBeNull();
  });

  it('asks the page to reload every few seconds while the server generates, and stops after', async () => {
    vi.useFakeTimers();
    const fixture = TestBed.createComponent(AiSummary);
    fixture.componentRef.setInput('name', 'x');
    fixture.componentRef.setInput('ai', { ...BASE, generating: true });
    const changed = vi.fn();
    fixture.componentInstance.changed.subscribe(changed);
    fixture.detectChanges();
    await vi.advanceTimersByTimeAsync(AI_POLL_MS * 2 + 10);
    expect(changed).toHaveBeenCalledTimes(2);

    // The next response says it's done: no more polling.
    fixture.componentRef.setInput('ai', { ...BASE, summary: SUMMARY });
    fixture.detectChanges();
    await vi.advanceTimersByTimeAsync(AI_POLL_MS * 3);
    expect(changed).toHaveBeenCalledTimes(2);
  });

  it("doesn't poll in a hidden tab, and asks once when it's shown again", async () => {
    vi.useFakeTimers();
    const setHidden = (hidden: boolean) => {
      Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => (hidden ? 'hidden' : 'visible') });
      document.dispatchEvent(new Event('visibilitychange'));
    };
    try {
      const fixture = TestBed.createComponent(AiSummary);
      fixture.componentRef.setInput('name', 'x');
      fixture.componentRef.setInput('ai', { ...BASE, generating: true });
      const changed = vi.fn();
      fixture.componentInstance.changed.subscribe(changed);
      fixture.detectChanges();
      setHidden(true);
      await vi.advanceTimersByTimeAsync(AI_POLL_MS * 3);
      expect(changed).not.toHaveBeenCalled();

      setHidden(false);
      await vi.advanceTimersByTimeAsync(10);
      expect(changed).toHaveBeenCalledTimes(1);
      await vi.advanceTimersByTimeAsync(AI_POLL_MS);
      expect(changed).toHaveBeenCalledTimes(2);
    } finally {
      delete (document as { visibilityState?: unknown }).visibilityState;
      vi.useRealTimers();
    }
  });

  it('leaves out the stats that are unknown', async () => {
    const a = await render({
      ...BASE,
      summary: { ...SUMMARY, inputTokens: 900, outputTokens: 0, durationMs: 0 },
    });
    expect(a.q('ai-stats')?.textContent?.trim()).toBe('· 900 tokens in');
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    http = TestBed.inject(HttpTestingController);
    const b = await render({
      ...BASE,
      summary: { ...SUMMARY, inputTokens: 0, outputTokens: 0, durationMs: 0 },
    });
    expect(b.q('ai-stats')).toBeNull();
  });

  it('wears the sheen, quicker (data-generating) while the model works', async () => {
    const { el, fixture } = await render({ ...BASE, summary: SUMMARY });
    const card = el.querySelector('tm-card')!;
    expect(card.classList.contains('tm-ai-sheen')).toBe(true);
    expect(card.hasAttribute('data-generating')).toBe(false);
    fixture.componentRef.setInput('ai', { ...BASE, summary: SUMMARY, generating: true });
    await fixture.whenStable();
    expect(card.hasAttribute('data-generating')).toBe(true);
  });
});
