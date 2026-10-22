import { Component, computed, effect, inject, input, linkedSignal, LOCALE_ID, output, signal } from '@angular/core';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';
import { TmAiMarker, TmButton, TmCallout, TmCard, TmIcon, TmLevel, TmSkeleton } from '../../ui';
import { timeAgo } from '../dates';
import { callStats } from '../ai-format';
import { AiState } from '../detail-types';

/** How often the detail is reloaded while the server is generating a summary (an automatic run). */
export const AI_POLL_MS = 4000;

const PROVIDERS: Record<string, string> = {
  anthropic: 'Anthropic',
  openai: 'an OpenAI-compatible endpoint',
  gemini: 'Google Gemini',
};

/**
 * The AI summary of an update on the detail page, below the verdict callout: "What changed",
 * "What you need to do", the AI's verdict with its reasons and where it came from, or (without
 * a summary yet) a button to make one. A stale summary stays, with a note. A failed attempt
 * shows inline with Retry. Everything the model wrote is plain text (interpolation only).
 *
 * Posts /api/containers/{name}/summary and shows the answer at once; `changed` tells the page to
 * reload, since the verdict can raise the level. While the server says `generating` (an
 * automatic run) it emits `changed` every few seconds until that stops.
 *
 *   <tm-ai-summary [name]="d.container.name" [ai]="d.ai" (changed)="detail.reload()" />
 */
@Component({
  selector: 'tm-ai-summary',
  imports: [TmAiMarker, TmButton, TmCallout, TmCard, TmIcon, TmLevel, TmSkeleton],
  templateUrl: './ai-summary.html',
  host: { class: 'block', 'data-testid': 'ai-summary' },
})
export class AiSummary {
  readonly name = input.required<string>();
  readonly ai = input.required<AiState>();
  /** The state changed on the server: reload the detail. */
  readonly changed = output<void>();

  private readonly http = inject(HttpClient);
  private readonly locale = inject(LOCALE_ID);

  /** The latest POST's answer until the page hands over a newer state. */
  private readonly answer = linkedSignal<AiState, AiState | undefined>({ source: () => this.ai(), computation: () => undefined });
  protected readonly state = computed(() => this.answer() ?? this.ai());
  protected readonly summary = computed(() => this.state().summary);
  /** A request of ours is in flight. */
  protected readonly posting = signal(false);
  /** Why our last request failed; replaced by the stored error once the page reloads. */
  protected readonly failure = signal<string | null>(null);
  protected readonly busy = computed(() => this.posting() || this.state().generating);
  protected readonly error = computed(() => (this.busy() ? null : (this.failure() ?? this.state().error ?? null)));
  protected readonly provider = computed(() => PROVIDERS[this.state().provider] ?? this.state().provider);
  protected readonly created = computed(() => {
    const at = this.summary()?.createdAt;
    return at ? timeAgo(at, this.locale) : '';
  });
  protected readonly stats = computed(() => {
    const s = this.summary();
    return s ? callStats(s.inputTokens ?? 0, s.outputTokens ?? 0, s.durationMs ?? 0) : '';
  });
  protected readonly count = computed(() => this.plural(this.state().releases, 'release', 'releases'));

  constructor() {
    // Poll while the server generates on its own; the effect re-runs only when `generating` flips.
    effect((onCleanup) => {
      if (!this.ai().generating) return;
      const id = setInterval(() => this.changed.emit(), AI_POLL_MS);
      onCleanup(() => clearInterval(id));
    });
  }

  protected plural(n: number, one: string, many: string): string {
    return `${n} ${n === 1 ? one : many}`;
  }

  /** Summarise, Regenerate or Retry. */
  protected async generate(): Promise<void> {
    if (this.busy()) return;
    this.posting.set(true);
    this.failure.set(null);
    try {
      this.answer.set(await firstValueFrom(this.http.post<AiState>(`/api/containers/${encodeURIComponent(this.name())}/summary`, null)));
    } catch (err) {
      const reason = err instanceof HttpErrorResponse ? ((err.error as { error?: string } | null)?.error ?? err.message) : String(err);
      this.failure.set(reason);
    } finally {
      this.posting.set(false);
      // The verdict may raise the level, and a failure is stored on the server too.
      this.changed.emit();
    }
  }
}
