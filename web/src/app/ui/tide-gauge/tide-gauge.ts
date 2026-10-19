import { Component, computed, input } from '@angular/core';

export type TideGaugeSize = 'hero' | 'compact';

/**
 * SVG path data for one wave layer, in px (the SVGs have no viewBox, so nothing stretches).
 * The crest runs from y=pad (peaks) to y=pad+2·amp (troughs) with the given period, `width` px
 * long; the fill closes it down to `depth`. Quadratic curves with smooth continuation (T) keep it
 * short. `pad` leaves room above the peaks for a stroked crest, which the SVG would clip.
 */
export function tideWavePath(period: number, amp: number, width: number, depth = 2000, pad = 0): { crest: string; fill: string } {
  const mid = amp + pad;
  let crest = `M0 ${mid} Q${period / 4} ${pad - amp} ${period / 2} ${mid}`;
  for (let x = period; x <= width + period; x += period / 2) crest += ` T${x} ${mid}`;
  return { crest, fill: `${crest} V${depth} H0Z` };
}

interface Wave {
  period: number;
  /** Extra px below the water level, so the layers overlap like depth. */
  offset: number;
  /** Loop length in seconds; `reverse` flows the other way. */
  seconds: number;
  reverse: boolean;
  /** Fill opacity of the layer. */
  opacity: number;
  /** Only the front wave gets a crest line. */
  front: boolean;
  crest: string;
  fill: string;
}

/** Room above the peaks for the 2 px crest stroke (the wave box moves up by as much, see .tm-gauge-wave). */
const CREST_PAD = 2;

function wave(period: number, amp: number, offset: number, seconds: number, reverse: boolean, opacity: number, front = false): Wave {
  return { period, offset, seconds, reverse, opacity, front, ...tideWavePath(period, amp, 3200, 2000, CREST_PAD) };
}

// Back to front. Different periods + speeds + directions keep the crests from lining up.
const WAVES: Record<TideGaugeSize, Wave[]> = {
  hero: [wave(340, 9, 0, 19, true, 0.6), wave(250, 7, 6, 14, false, 0.8), wave(190, 5, 12, 11, true, 1, true)],
  compact: [wave(30, 1.6, 0, 9, true, 0.25), wave(22, 1.4, 2.5, 7, false, 0.4, true)],
};

/**
 * The tide gauge: the logo as a meter. A straight line at the top is the high-water mark
 * (upstream, the latest versions); the water below is where you are. At `level` 1 the waves
 * reach the line; lower water means further behind. The gap between them is the point.
 *
 * Accent colour only (never amber/coral: colour means risk). No text inside: `label` is the
 * accessible name, visible numbers are the caller's job. Slow CSS motion, none with reduced
 * motion; a new level eases the water to its height.
 *
 *   <tm-tide-gauge size="hero" [level]="0.72" label="18 of 25 containers up to date" class="h-48" />
 *   <tm-tide-gauge [level]="0.5" label="6 releases behind" />
 *
 * `hero` fills its parent (give the host or parent a height). The line sits at 24% of the
 * height; the water spans 24% (level 1) … 80% (level 0) and fades towards the bottom so
 * text over it stays readable. `compact` is a fixed 56×32 box for headers.
 */
@Component({
  selector: 'tm-tide-gauge',
  templateUrl: './tide-gauge.html',
  styleUrl: './tide-gauge.css',
  host: {
    role: 'img',
    '[attr.aria-label]': 'label()',
    '[class.tm-gauge-hero]': "size() === 'hero'",
    '[class.tm-gauge-compact]': "size() === 'compact'",
    '[style.--tm-level]': 'clamped()',
  },
})
export class TmTideGauge {
  /** Water height, 0 (far behind) … 1 (at the mark). Clamped. */
  readonly level = input.required<number>();
  readonly size = input<TideGaugeSize>('compact');
  /** Accessible description, e.g. "18 of 25 containers up to date". */
  readonly label = input.required<string>();

  /** The level as used, 0 … 1 (NaN counts as 0). */
  readonly clamped = computed(() => {
    const l = this.level();
    return Number.isFinite(l) ? Math.min(1, Math.max(0, l)) : l === Infinity ? 1 : 0;
  });
  protected readonly waves = computed(() => WAVES[this.size()]);
}
