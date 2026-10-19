import { Component } from '@angular/core';
import { tideWavePath } from '../tide-gauge/tide-gauge';

// Back to front: long, low swells that drift very slowly. Paths are 6000 px long, which covers
// any screen plus one period.
const WAVES = [
  { period: 820, amp: 16, top: 0, seconds: 90, reverse: true, opacity: 0.25 },
  { period: 620, amp: 12, top: 26, seconds: 70, reverse: false, opacity: 0.32 },
  { period: 480, amp: 9, top: 52, seconds: 55, reverse: true, opacity: 0.4 },
].map((w) => ({ ...w, ...tideWavePath(w.period, w.amp, 6000) }));

/**
 * A very faint, slowly drifting tide along the bottom of the viewport, behind all page content.
 * Mounted once in the app shell. Decorative only: fixed, no pointer events, painted below
 * in-flow content (cards and the header cover it, the canvas colour shows through), and
 * still under reduced motion.
 *
 *   <tm-tide-backdrop />
 */
@Component({
  selector: 'tm-tide-backdrop',
  templateUrl: './tide-backdrop.html',
  styleUrl: './tide-backdrop.css',
  host: { 'aria-hidden': 'true' },
})
export class TmTideBackdrop {
  protected readonly waves = WAVES;
}
