import { Component, computed, input } from '@angular/core';

export type TideGaugeSize = 'hero' | 'compact';

/**
 * SVG path data for one wave layer, in px (the SVGs have no viewBox, so nothing stretches).
 * The crest runs from y=0 (peaks) to y=2·amp (troughs) with the given period, `width` px long;
 * the fill closes it down to `depth`. Quadratic curves with smooth continuation (T) keep it short.
 */
export function tideWavePath(period: number, amp: number, width: number, depth = 2000): { crest: string; fill: string } {
  let crest = `M0 ${amp} Q${period / 4} ${-amp} ${period / 2} ${amp}`;
  for (let x = period; x <= width + period; x += period / 2) crest += ` T${x} ${amp}`;
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

function wave(period: number, amp: number, offset: number, seconds: number, reverse: boolean, opacity: number, front = false): Wave {
  return { period, offset, seconds, reverse, opacity, front, ...tideWavePath(period, amp, 3200) };
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
  template: `
    <span class="tm-gauge-line"></span>
    @for (w of waves(); track $index) {
      <div
        class="tm-gauge-wave"
        [class.tm-gauge-reverse]="w.reverse"
        [style.--tm-wave-period.px]="w.period"
        [style.--tm-wave-offset.px]="w.offset"
        [style.animation-duration.s]="w.seconds"
      >
        <svg width="100%" height="100%" aria-hidden="true" focusable="false">
          <path class="tm-gauge-fill" [attr.d]="w.fill" [attr.fill-opacity]="w.opacity" />
          @if (w.front) {
            <path class="tm-gauge-crest" [attr.d]="w.crest" />
          }
        </svg>
      </div>
    }
  `,
  styles: `
    /* In the components layer, so utilities on the host (absolute, h-48 …) win. */
    @layer components {
      :host {
        position: relative;
        overflow: hidden;
        /* Line position and the lowest water level (level 0), from the top. */
        --tm-gauge-line: 24%;
        --tm-gauge-floor: 80%;
      }
      :host(.tm-gauge-hero) {
        display: block;
        width: 100%;
        height: 100%;
      }
      :host(.tm-gauge-compact) {
        display: inline-block;
        flex-shrink: 0;
        width: 56px;
        height: 32px;
        border: 1px solid var(--color-line);
        border-radius: var(--radius-md);
        background-color: var(--color-surface);
        vertical-align: middle;
        --tm-gauge-line: 7px;
        --tm-gauge-floor: 24px;
      }

      .tm-gauge-line {
        position: absolute;
        top: var(--tm-gauge-line);
        left: 0;
        right: 0;
        height: 2px;
        margin-top: -1px;
        background-color: var(--color-line-strong);
      }
      :host(.tm-gauge-compact) .tm-gauge-line {
        left: 7px;
        right: 7px;
        border-radius: 1px;
        background-color: var(--color-fg);
      }

      .tm-gauge-wave {
        position: absolute;
        left: 0;
        bottom: 0;
        /* One period wider than the gauge, so sliding it by one period loops seamlessly. */
        width: calc(100% + var(--tm-wave-period));
        top: calc(
          var(--tm-gauge-line) + 1px + (1 - var(--tm-level)) * (var(--tm-gauge-floor) - var(--tm-gauge-line)) +
            var(--tm-wave-offset)
        );
        animation: tm-gauge-flow linear infinite;
        transition: top 1.4s cubic-bezier(0.4, 0, 0.2, 1);
        will-change: transform;
      }
      .tm-gauge-reverse {
        animation-direction: reverse;
      }
      :host(.tm-gauge-hero) .tm-gauge-wave {
        /* Strongest at the surface, fading with depth: text over the lower part stays readable. */
        mask-image: linear-gradient(to bottom, #000 0, #000 1.5rem, rgb(0 0 0 / 0.2) 100%);
      }
      svg {
        display: block;
      }
      .tm-gauge-fill {
        fill: var(--color-accent);
      }
      /* Hero water is the translucent --color-water (lighter in dark mode, where the accent is bright). */
      :host(.tm-gauge-hero) .tm-gauge-fill {
        fill: var(--color-water);
      }
      .tm-gauge-crest {
        fill: none;
        stroke: var(--color-accent);
        stroke-width: 2;
        stroke-opacity: 0.6;
      }
      :host(.tm-gauge-compact) .tm-gauge-crest {
        stroke-width: 1.5;
        stroke-opacity: 1;
      }

      @keyframes tm-gauge-flow {
        to {
          transform: translateX(calc(-1 * var(--tm-wave-period)));
        }
      }
      @media (prefers-reduced-motion: reduce) {
        .tm-gauge-wave {
          animation: none;
          transition: none;
        }
      }
    }
  `,
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
