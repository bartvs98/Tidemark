import { Component, computed, inject, input, LOCALE_ID } from '@angular/core';
import { HistoryEntry } from '../detail-types';
import { roughDuration, shortDate } from '../dates';

/** "sha256:0123456789abcdef…" → "0123456789ab". */
export function shortImageId(id: string | undefined): string {
  return (id ?? '').replace(/^sha256:/, '').slice(0, 12);
}

interface HistoryRow {
  at: string;
  date: string;
  kind: 'version' | 'image' | 'start';
  running?: string;
  prevRunning?: string;
  imageId: string;
  prevImageId: string;
  /** "ran 1.4.2 for 12 days": how long the previous image ran, when the older row is there. */
  ran?: { version?: string; duration: string };
}

/**
 * The container's update history as a quiet timeline, newest first: version changes
 * ("1.4.2 → 1.6.0"), new images without a (changed) version, and where tracking started.
 */
@Component({
  selector: 'tm-history',
  templateUrl: './history.html',
})
export class History {
  readonly entries = input.required<HistoryEntry[]>();

  private readonly locale = inject(LOCALE_ID);

  protected readonly rows = computed<HistoryRow[]>(() => {
    const list = this.entries();
    return list.map((h, i) => {
      const older = list[i + 1];
      const kind = !h.prevImageId ? 'start' : h.running && h.prevRunning && h.running !== h.prevRunning ? 'version' : 'image';
      const ran =
        h.prevImageId && older?.imageId === h.prevImageId ? { version: h.prevRunning, duration: roughDuration(older.at, h.at) } : undefined;
      return {
        at: h.at,
        date: shortDate(h.at, this.locale),
        kind,
        running: h.running,
        prevRunning: h.prevRunning,
        imageId: shortImageId(h.imageId),
        prevImageId: shortImageId(h.prevImageId),
        ran,
      };
    });
  });
}
