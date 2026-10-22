import { Component } from '@angular/core';
import { TmBadge } from '../badge/badge';

/**
 * A small neutral "AI" label for text a model wrote (the summary card, reasons in the verdict).
 * Not a risk, so it has no colour; the screen-reader text says what it means.
 *
 *   <tm-ai-marker /> The config key was renamed.
 */
@Component({
  selector: 'tm-ai-marker',
  imports: [TmBadge],
  templateUrl: './ai-marker.html',
  host: { class: 'inline-flex align-middle' },
})
export class TmAiMarker {}
