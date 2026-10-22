/** A token count, short: 640, 9.1k, 182k, 1.2M. */
export function formatTokens(n: number): string {
  const short = (v: number) => (v >= 100 ? Math.round(v) : Math.round(v * 10) / 10).toString();
  if (n < 1000) return String(n);
  if (n < 999_500) return `${short(n / 1000)}k`;
  return `${short(n / 1_000_000)}M`;
}

/** How long a call took: 420 ms, 4.2 s, 1 m 12 s. */
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms} ms`;
  if (ms < 59_950) return `${Math.round(ms / 100) / 10} s`;
  const s = Math.round(ms / 1000);
  return `${Math.floor(s / 60)} m ${s % 60} s`;
}

/**
 * The tokens and duration of the call behind a summary, parts left out when 0:
 * "12.3k tokens in · 640 out · 4.2 s". Empty when nothing is known.
 */
export function callStats(inputTokens: number, outputTokens: number, durationMs: number): string {
  const parts: string[] = [];
  if (inputTokens > 0) parts.push(`${formatTokens(inputTokens)} tokens in`);
  if (outputTokens > 0)
    parts.push(
      inputTokens > 0
        ? `${formatTokens(outputTokens)} out`
        : `${formatTokens(outputTokens)} tokens out`,
    );
  if (durationMs > 0) parts.push(formatDuration(durationMs));
  return parts.join(' · ');
}

/** The month's usage line: "14 summaries (9 extended) · 182k tokens in · 9.1k out · 2 failed". */
export function usageLine(u: {
  calls: number;
  failed: number;
  extended?: number;
  inputTokens: number;
  outputTokens: number;
}): string {
  if (u.calls === 0) return 'No summaries yet this month';
  const parts = [
    `${u.calls} ${u.calls === 1 ? 'summary' : 'summaries'}${u.extended ? ` (${u.extended} extended)` : ''}`,
  ];
  if (u.inputTokens > 0) parts.push(`${formatTokens(u.inputTokens)} tokens in`);
  if (u.outputTokens > 0)
    parts.push(
      u.inputTokens > 0
        ? `${formatTokens(u.outputTokens)} out`
        : `${formatTokens(u.outputTokens)} tokens out`,
    );
  if (u.failed > 0) parts.push(`${u.failed} failed`);
  return parts.join(' · ');
}
