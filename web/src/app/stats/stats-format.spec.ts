import { behindFor, capitalize, dayLabel, longDuration, monthLabel, percent } from './stats-format';

describe('stats format helpers', () => {
  it('rounds durations to a unit', () => {
    expect(longDuration(30_000)).toBe('under a minute');
    expect(longDuration(12 * 60_000)).toBe('12 min');
    expect(longDuration(3 * 3_600_000)).toBe('3 h');
    expect(longDuration(2 * 86_400_000)).toBe('2 days');
    expect(longDuration(35 * 86_400_000)).toBe('5 weeks');
    expect(longDuration(100 * 86_400_000)).toBe('3 months');
    expect(longDuration(800 * 86_400_000)).toBe('2 years');
  });

  it('formats ratios, months and days', () => {
    expect(percent(0.874)).toBe('87%');
    expect(monthLabel('2026-09', 'en-US')).toBe('Sep');
    expect(monthLabel('2026-09', 'en-US', true)).toBe('Sep 26');
    expect(dayLabel('2026-10-12', 'en-US')).toBe('12 Oct');
    expect(capitalize('rate limited')).toBe('Rate limited');
  });

  it('says how long a container has been behind', () => {
    const now = Date.parse('2026-10-20T00:00:00Z');
    expect(behindFor(undefined, now)).toBe('');
    expect(behindFor('2026-10-19T12:00:00Z', now)).toBe('less than a day');
    expect(behindFor('2026-07-20T00:00:00Z', now)).toBe('3 months');
  });
});
