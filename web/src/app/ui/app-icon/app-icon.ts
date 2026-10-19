import { Component, computed, inject, input, linkedSignal } from '@angular/core';
import { Theme } from '../theme-switch/theme';

export interface AppIconSources {
  src: string;
  /** White variant, for icons that are near-black on a dark theme. */
  light?: string;
  /** Black variant, for icons that are near-white on a light theme. */
  dark?: string;
}

/**
 * An app's icon, or a letter avatar when there's no icon or it fails to load.
 * Decorative: the app name is always shown next to it, so the image has empty alt text.
 *
 * Once the default icon loads, its average brightness is measured. A near-black icon
 * on the dark theme (or near-white on the light theme) swaps to its contrasting variant,
 * while colourful icons keep their colours.
 *
 *   <tm-app-icon [icon]="c.icon" [name]="c.name" [size]="32" />
 */
@Component({
  selector: 'tm-app-icon',
  templateUrl: './app-icon.html',
  host: {
    class: 'grid shrink-0 place-items-center overflow-hidden',
    '[class.rounded-md]': '!src()',
    '[class.bg-hover]': '!src()',
    '[style.width.px]': 'size()',
    '[style.height.px]': 'size()',
    'aria-hidden': 'true',
  },
})
export class TmAppIcon {
  readonly icon = input<AppIconSources | undefined>();
  readonly name = input.required<string>();
  readonly size = input(24);

  private readonly theme = inject(Theme);

  // Both reset when the icon changes.
  protected readonly failed = linkedSignal({ source: this.icon, computation: () => false });
  private readonly luminance = linkedSignal<AppIconSources | undefined, number | null>({
    source: this.icon,
    computation: () => null,
  });

  protected readonly src = computed(() => {
    const icon = this.icon();
    if (!icon || this.failed()) return undefined;
    const lum = this.luminance();
    if (lum !== null) {
      if (this.theme.isDark() && lum < 0.2 && icon.light) return icon.light;
      if (!this.theme.isDark() && lum > 0.85 && icon.dark) return icon.dark;
    }
    return icon.src;
  });

  protected readonly letter = computed(() => (this.name().match(/[a-z0-9]/i)?.[0] ?? '?').toUpperCase());

  protected measure(event: Event): void {
    const img = event.target as HTMLImageElement;
    // Only the default icon is measured; variants are the result of a measurement.
    if (this.luminance() === null && img.getAttribute('src') === this.icon()?.src) {
      this.luminance.set(averageLuminance(img));
    }
  }
}

/** Average relative luminance (0 = black, 1 = white) of the image's opaque pixels, or null. */
export function averageLuminance(img: HTMLImageElement): number | null {
  try {
    const size = 32;
    const canvas = document.createElement('canvas');
    canvas.width = canvas.height = size;
    const ctx = canvas.getContext('2d', { willReadFrequently: true });
    if (!ctx) return null;
    ctx.drawImage(img, 0, 0, size, size);
    const { data } = ctx.getImageData(0, 0, size, size);
    let sum = 0;
    let weight = 0;
    for (let i = 0; i < data.length; i += 4) {
      const alpha = data[i + 3] / 255;
      if (alpha < 0.5) continue;
      sum += alpha * ((0.2126 * data[i] + 0.7152 * data[i + 1] + 0.0722 * data[i + 2]) / 255);
      weight += alpha;
    }
    return weight > 0 ? sum / weight : null;
  } catch {
    return null; // canvas unavailable (tests) or tainted
  }
}
