import { callStats, formatDuration, formatTokens, usageLine } from './ai-format';

describe('ai-format', () => {
  it('shortens token counts', () => {
    expect([640, 1000, 9100, 12340, 182000, 999600, 1_200_000].map(formatTokens)).toEqual(['640', '1k', '9.1k', '12.3k', '182k', '1M', '1.2M']);
  });

  it('formats durations', () => {
    expect([420, 4200, 59_960, 72_000].map(formatDuration)).toEqual(['420 ms', '4.2 s', '1 m 0 s', '1 m 12 s']);
  });

  it('leaves out the parts that are 0', () => {
    expect(callStats(12340, 640, 4200)).toBe('12.3k tokens in · 640 out · 4.2 s');
    expect(callStats(0, 640, 0)).toBe('640 tokens out');
    expect(callStats(900, 0, 1500)).toBe('900 tokens in · 1.5 s');
    expect(callStats(0, 0, 0)).toBe('');
  });

  it('writes the usage line', () => {
    expect(usageLine({ calls: 14, failed: 2, inputTokens: 182000, outputTokens: 9100 })).toBe('14 summaries · 182k tokens in · 9.1k out · 2 failed');
    expect(usageLine({ calls: 1, failed: 0, inputTokens: 500, outputTokens: 80 })).toBe('1 summary · 500 tokens in · 80 out');
    expect(usageLine({ calls: 3, failed: 3, inputTokens: 0, outputTokens: 0 })).toBe('3 summaries · 3 failed');
    expect(usageLine({ calls: 0, failed: 0, inputTokens: 0, outputTokens: 0 })).toBe('No summaries yet this month');
  });
});
