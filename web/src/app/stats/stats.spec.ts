import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { Stats } from './stats';
import { Stats as StatsData } from './stats-types';

function fixture(overrides: Partial<StatsData> = {}): StatsData {
  return {
    generatedAt: '2026-10-20T12:00:00Z',
    range: '90d',
    since: '2026-07-22T12:00:00Z',
    behind: {
      containers: [
        {
          name: 'grafana',
          stack: 'monitoring',
          running: '11.6.0',
          latest: '12.2.1',
          class: 'major',
          level: 'breaking',
          releasesBehind: 9,
          behindSince: '2026-07-20T00:00:00Z',
        },
        { name: 'nginx', running: '1.29.0', latest: '1.31.7', class: 'minor', level: 'none', review: 'reviewed', releasesBehind: 0 },
      ],
      applied: {
        total: 14,
        perStack: [{ stack: 'home', count: 8 }, { stack: '', count: 6 }],
        perWeek: [{ week: '2026-10-12', count: 3 }, { week: '2026-10-19', count: 0 }],
      },
      timeToUpdate: { count: 6, medianMs: 3 * 86_400_000 },
      tide: [
        { at: '2026-10-19T12:00:00Z', checked: 8, upToDate: 6, updates: 2, attention: 0, breaking: 1, releasesBehind: 14 },
        { at: '2026-10-20T12:00:00Z', checked: 8, upToDate: 7, updates: 1, attention: 0, breaking: 1, releasesBehind: 9 },
      ],
    },
    upstream: {
      repos: [
        {
          repo: 'grafana/grafana',
          url: 'https://github.com/grafana/grafana',
          containers: ['grafana'],
          releases: 12,
          breaking: 2,
          attention: 1,
          lastRelease: '2026-10-15T00:00:00Z',
          medianGapDays: 6.5,
        },
      ],
      perMonth: [{ month: '2026-09', releases: 7, breaking: 1, attention: 1 }, { month: '2026-10', releases: 5, breaking: 1, attention: 0 }],
      biggestJumps: [{ name: 'grafana', from: '10.4.0', to: '11.6.0', at: '2026-09-01T00:00:00Z', ms: 1, releases: 24, class: 'major' }],
    },
    health: {
      runs: { total: 60, failed: 2, byTrigger: { startup: 3, schedule: 55, manual: 2 }, medianMs: 4200, slowestMs: 31_000 },
      durations: [
        { at: '2026-10-19T06:00:00Z', ms: 4000, images: 9, failed: false },
        { at: '2026-10-20T06:00:00Z', ms: 5000, images: 9, failed: false },
      ],
      failing: [{ name: 'mosquitto', kind: 'rate-limit', error: 'toomanyrequests', since: '2026-10-20T06:00:00Z' }],
      failuresByKind: [{ kind: 'rate-limit', count: 4 }, { kind: 'other', count: 1 }],
      requests: [{ day: '2026-10-19', registry: 40, github: 12, gitlab: 0, limited: 1 }],
      requestsPer: 'day',
      requestTotals: { registry: 40, github: 12, gitlab: 0, limited: 1 },
      github: { token: false },
    },
    ai: {
      months: [{ month: '2026-10', calls: 3, failed: 0, inputTokens: 12_000, outputTokens: 900 }],
      byModel: [{ provider: 'anthropic', model: 'claude-haiku', calls: 3, failed: 0, inputTokens: 12_000, outputTokens: 900 }],
      calls: 3,
      failed: 0,
      manual: 1,
      auto: 2,
      extended: 0,
      medianMs: 4200,
    },
    lifetime: {
      since: '2026-07-01T00:00:00Z',
      startedAt: '2026-10-20T08:00:00Z',
      containersTracked: 9,
      releasesStored: 410,
      breakingFlagged: 17,
    },
    ...overrides,
  };
}

describe('Stats', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    localStorage.removeItem('tidemark.statsRange');
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])] });
    http = TestBed.inject(HttpTestingController);
  });

  async function render(stats: StatsData = fixture()) {
    const component = TestBed.createComponent(Stats);
    component.detectChanges();
    http.expectOne('/api/stats?range=90d').flush(stats);
    await component.whenStable();
    component.detectChanges();
    return component;
  }

  it('shows a skeleton first, then the sections', async () => {
    const component = TestBed.createComponent(Stats);
    component.detectChanges();
    const el = component.nativeElement as HTMLElement;
    expect(el.querySelector('tm-stats-skeleton')).not.toBeNull();
    http.expectOne('/api/stats?range=90d').flush(fixture());
    await component.whenStable();
    component.detectChanges();
    expect(el.querySelector('tm-stats-skeleton')).toBeNull();
    for (const id of ['behind', 'upstream', 'health', 'ai', 'lifetime']) {
      expect(el.querySelector(`[data-testid="stats-${id}"]`), id).not.toBeNull();
    }
    // Longest behind: only containers with a known release date (nginx has no notes).
    const behind = el.querySelector('[data-testid="longest-behind"]')!;
    expect([...behind.querySelectorAll('a')].map((a) => a.textContent?.trim())).toEqual(['grafana']);
    expect(behind.textContent).toContain('9 releases');
    expect(el.querySelector('[data-testid="repo-flagged"]')?.textContent).toMatch(/2 breaking\s+1 attention/);
    expect(el.querySelector('[data-testid="failing"]')?.textContent).toContain('mosquitto');
  });

  it('hides the AI section without ai stats', async () => {
    const component = await render(fixture({ ai: undefined }));
    expect((component.nativeElement as HTMLElement).querySelector('[data-testid="stats-ai"]')).toBeNull();
  });

  it('refetches with the new range when it changes', async () => {
    const component = await render();
    const el = component.nativeElement as HTMLElement;
    const button = [...el.querySelectorAll<HTMLButtonElement>('[data-testid="range"] button')].find((b) => b.textContent?.includes('1 year'))!;
    button.click();
    component.detectChanges();
    TestBed.tick();
    http.expectOne('/api/stats?range=1y').flush(fixture({ range: '1y' }));
    await component.whenStable();
    component.detectChanges();
    expect(localStorage.getItem('tidemark.statsRange')).toBe('1y');
    // The previous data stays while loading and the page keeps its sections.
    expect(el.querySelector('[data-testid="stats-behind"]')).not.toBeNull();
  });

  it('says when Docker could not be reached', async () => {
    const component = await render(fixture({ partial: true, behind: { ...fixture().behind, containers: [] } }));
    const el = component.nativeElement as HTMLElement;
    expect(el.querySelector('[data-testid="stats-partial"]')?.textContent).toContain("Docker couldn't be reached");
    expect(el.querySelector('[data-testid="longest-behind"]')?.textContent).toContain('Not available without Docker');
  });

  it('shows an error callout when the first load fails', async () => {
    const component = TestBed.createComponent(Stats);
    component.detectChanges();
    http.expectOne('/api/stats?range=90d').flush({ error: 'boom' }, { status: 500, statusText: 'Server Error' });
    await component.whenStable();
    component.detectChanges();
    const callout = (component.nativeElement as HTMLElement).querySelector('[data-testid="stats-error"]');
    expect(callout?.textContent).toContain("Couldn't load stats.");
    expect(callout?.textContent).toContain('boom');
  });
});
