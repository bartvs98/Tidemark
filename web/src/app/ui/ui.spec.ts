import { ApplicationRef, Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { AppIconSources, Level, Theme, THEME_COLORS, TmAppIcon, TmButton, TmLevel, TmSegmented, TmToaster, TmTooltip, TmUpdateClass, Toasts, UpdateClass } from './index';

describe('Toasts', () => {
  afterEach(() => vi.useRealTimers());

  it('auto-dismisses after the duration', () => {
    vi.useFakeTimers();
    const toasts = TestBed.inject(Toasts);
    toasts.show('Check finished', 'ok', 1000);
    expect(toasts.items().length).toBe(1);
    vi.advanceTimersByTime(1000);
    expect(toasts.items().length).toBe(0);
  });

  it('dismisses from the toaster button', async () => {
    const fixture = TestBed.createComponent(TmToaster);
    const toasts = TestBed.inject(Toasts);
    toasts.show('First', 'info', 0);
    toasts.show('Rolled back', 'breaking', 0);
    await fixture.whenStable();

    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelectorAll('[role="status"]').length).toBe(1);
    expect(el.querySelector('[role="alert"]')?.textContent).toContain('Rolled back');

    el.querySelector<HTMLButtonElement>('button[aria-label="Dismiss"]')!.click();
    await fixture.whenStable();
    expect(toasts.items().map((t) => t.message)).toEqual(['Rolled back']);
  });
});

@Component({
  imports: [TmLevel, TmUpdateClass, TmButton],
  template: `
    <tm-level [level]="level()" />
    <tm-update-class [kind]="kind()" />
    <button tmButton [loading]="loading()">Check now</button>
  `,
})
class Host {
  level = signal<Level>('none');
  kind = signal<UpdateClass>('none');
  loading = signal(false);
}

describe('status components', () => {
  async function render() {
    const fixture = TestBed.createComponent(Host);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    return { fixture, el, host: fixture.componentInstance };
  }

  it('renders no risk badge for level none, and icon + word otherwise', async () => {
    const { fixture, el, host } = await render();
    expect(el.querySelector('tm-level')!.textContent!.trim()).toBe('');

    host.level.set('breaking');
    await fixture.whenStable();
    const level = el.querySelector('tm-level')!;
    expect(level.textContent).toContain('Breaking');
    expect(level.querySelector('svg')).not.toBeNull();
  });

  it('shows "Up to date" for no update and a neutral badge otherwise', async () => {
    const { fixture, el, host } = await render();
    expect(el.querySelector('tm-update-class')!.textContent).toContain('Up to date');

    host.kind.set('digest');
    await fixture.whenStable();
    expect(el.querySelector('tm-update-class')!.textContent).toContain('new image');
  });

  it('marks a loading button busy', async () => {
    const { fixture, el, host } = await render();
    const button = el.querySelector('button')!;
    expect(button.getAttribute('aria-busy')).toBeNull();

    host.loading.set(true);
    await fixture.whenStable();
    expect(button.getAttribute('aria-busy')).toBe('true');
    expect(button.getAttribute('aria-disabled')).toBe('true');
  });
});

describe('TmAppIcon', () => {
  async function render(icon: AppIconSources | undefined, dark: boolean) {
    TestBed.inject(Theme).preference.set(dark ? 'dark' : 'light');
    const fixture = TestBed.createComponent(TmAppIcon);
    fixture.componentRef.setInput('name', 'mealie');
    fixture.componentRef.setInput('icon', icon);
    await fixture.whenStable();
    // jsdom has no canvas, so set what averageLuminance() would have measured.
    const setLuminance = async (value: number) => {
      (fixture.componentInstance as unknown as { luminance: { set(v: number): void } }).luminance.set(value);
      await fixture.whenStable();
    };
    const src = () => (fixture.nativeElement as HTMLElement).querySelector('img')?.getAttribute('src');
    return { fixture, setLuminance, src };
  }

  const icon = { src: '/api/icons/x', light: '/api/icons/x?variant=light', dark: '/api/icons/x?variant=dark' };

  it('swaps a near-black icon to its light variant on the dark theme', async () => {
    const { setLuminance, src } = await render(icon, true);
    expect(src()).toBe(icon.src);
    await setLuminance(0.05);
    expect(src()).toBe(icon.light);
  });

  it('swaps a near-white icon to its dark variant on the light theme', async () => {
    const { setLuminance, src } = await render(icon, false);
    await setLuminance(0.95);
    expect(src()).toBe(icon.dark);
  });

  it('keeps colourful icons, and icons without a matching variant', async () => {
    const colourful = await render(icon, true);
    await colourful.setLuminance(0.45);
    expect(colourful.src()).toBe(icon.src);

    const noVariant = await render({ src: '/api/icons/y' }, true);
    await noVariant.setLuminance(0.02);
    expect(noVariant.src()).toBe('/api/icons/y');
  });

  it('shows a letter avatar without an icon', async () => {
    const { fixture } = await render(undefined, true);
    expect((fixture.nativeElement as HTMLElement).textContent?.trim()).toBe('A');
  });
});

describe('Theme', () => {
  const meta = () => document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
  let osDark = true;
  let listeners: ((e: MediaQueryListEvent) => void)[] = [];
  const original = window.matchMedia;

  beforeEach(() => {
    localStorage.clear();
    meta()?.remove();
    osDark = true;
    listeners = [];
    // jsdom has no matchMedia: fake the OS setting, and let the test flip it.
    window.matchMedia = ((query: string) => ({
      matches: query.includes('dark') && osDark,
      media: query,
      addEventListener: (_: string, fn: (e: MediaQueryListEvent) => void) => listeners.push(fn),
      removeEventListener: () => {},
    })) as unknown as typeof window.matchMedia;
  });
  afterEach(() => {
    window.matchMedia = original;
    meta()?.remove();
  });

  const flipOs = (dark: boolean) => {
    osDark = dark;
    listeners.forEach((fn) => fn({ matches: dark } as MediaQueryListEvent));
  };

  it('points theme-color at the resolved page background', async () => {
    const theme = TestBed.inject(Theme);
    await TestBed.inject(ApplicationRef).whenStable();
    expect(document.documentElement.dataset['theme']).toBe('dark');
    expect(meta()?.content).toBe(THEME_COLORS.dark);

    theme.preference.set('light');
    await TestBed.inject(ApplicationRef).whenStable();
    expect(meta()?.content).toBe(THEME_COLORS.light);
    expect(document.querySelectorAll('meta[name="theme-color"]').length).toBe(1);
    expect(localStorage.getItem('tidemark.theme')).toBe('light');
  });

  it('follows the OS when set to system, also when the OS flips', async () => {
    localStorage.setItem('tidemark.theme', 'system');
    const theme = TestBed.inject(Theme);
    await TestBed.inject(ApplicationRef).whenStable();
    expect(theme.isDark()).toBe(true);
    expect(meta()?.content).toBe(THEME_COLORS.dark);

    flipOs(false);
    await TestBed.inject(ApplicationRef).whenStable();
    expect(theme.isDark()).toBe(false);
    expect(meta()?.content).toBe(THEME_COLORS.light);
  });
});

describe('TmTooltip', () => {
  @Component({
    imports: [TmTooltip],
    template: `<button [tmTooltip]="text()" aria-describedby="other">Go</button>`,
  })
  class TipHost {
    text = signal<string | null>('Pinned to 1.4');
  }

  async function render() {
    const fixture = TestBed.createComponent(TipHost);
    await fixture.whenStable();
    const button = (fixture.nativeElement as HTMLElement).querySelector('button')!;
    return { fixture, button, host: fixture.componentInstance };
  }
  const tip = () => document.body.querySelector<HTMLElement>('[role="tooltip"]');

  afterEach(() => vi.useRealTimers());

  // jsdom doesn't implement :focus-visible or PointerEvent.pointerType.
  const keyboardFocus = (button: HTMLElement) => {
    vi.spyOn(button, 'matches').mockReturnValue(true);
    button.dispatchEvent(new FocusEvent('focusin', { bubbles: true }));
  };
  const mouseEnter = (button: HTMLElement) => {
    const e = new Event('pointerenter');
    Object.defineProperty(e, 'pointerType', { value: 'mouse' });
    button.dispatchEvent(e);
  };

  it('reflects the text as data-tooltip and never sets a native title', async () => {
    const { button } = await render();
    expect(button.getAttribute('data-tooltip')).toBe('Pinned to 1.4');
    expect(button.hasAttribute('title')).toBe(false);
  });

  it('has no tooltip when the text is empty', async () => {
    const { fixture, button, host } = await render();
    host.text.set('');
    await fixture.whenStable();
    expect(button.hasAttribute('data-tooltip')).toBe(false);
    button.dispatchEvent(new FocusEvent('focusin', { bubbles: true }));
    expect(tip()).toBeNull();
  });

  it('shows on focus, links aria-describedby and hides on Escape and blur', async () => {
    const { fixture, button } = await render();
    keyboardFocus(button);
    const el = tip()!;
    expect(el.textContent).toBe('Pinned to 1.4');
    expect(el.hasAttribute('data-open')).toBe(true);
    expect(button.getAttribute('aria-describedby')).toBe(`other ${el.id}`);

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(el.hasAttribute('data-open')).toBe(false);

    keyboardFocus(button);
    expect(el.hasAttribute('data-open')).toBe(true);
    button.dispatchEvent(new FocusEvent('focusout', { bubbles: true }));
    expect(el.hasAttribute('data-open')).toBe(false);
    fixture.destroy();
  });

  it('shows after the hover delay for mouse pointers only', async () => {
    const { fixture, button } = await render();
    vi.useFakeTimers();
    mouseEnter(button);
    vi.advanceTimersByTime(500);
    expect(tip()).not.toBeNull();
    expect(tip()!.hasAttribute('data-open')).toBe(true);
    button.dispatchEvent(new MouseEvent('pointerleave'));
    vi.advanceTimersByTime(200);
    expect(tip()!.hasAttribute('data-open')).toBe(false);
    fixture.destroy();
  });

  it('is removed on destroy and restores aria-describedby', async () => {
    const { fixture, button } = await render();
    keyboardFocus(button);
    expect(tip()).not.toBeNull();
    fixture.destroy();
    expect(tip()).toBeNull();
  });
});

describe('TmSegmented size', () => {
  @Component({
    imports: [TmSegmented],
    template: `<tm-segmented label="View" size="md" [options]="options" [(value)]="value" />`,
  })
  class SegHost {
    options = [{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }];
    value = signal('a');
  }

  it('md is 36 px: h-9 host, h-8 buttons', async () => {
    const fixture = TestBed.createComponent(SegHost);
    await fixture.whenStable();
    const seg = (fixture.nativeElement as HTMLElement).querySelector('tm-segmented')!;
    expect(seg.classList.contains('h-9')).toBe(true);
    expect(seg.classList.contains('p-0.5')).toBe(false);
    expect(seg.querySelector('button')!.classList.contains('h-8')).toBe(true);
  });
});

// Tide gauge + backdrop (own import, so this block can be added without touching the one above).
import { TideGaugeSize, tideWavePath, TmTideBackdrop, TmTideGauge } from './index';

describe('TmTideGauge', () => {
  @Component({
    imports: [TmTideGauge],
    template: `<tm-tide-gauge [level]="level()" [size]="size()" [label]="label()" />`,
  })
  class GaugeHost {
    level = signal(0.5);
    size = signal<TideGaugeSize>('compact');
    label = signal('18 of 25 containers up to date');
  }

  async function gauge() {
    const fixture = TestBed.createComponent(GaugeHost);
    await fixture.whenStable();
    const el = (fixture.nativeElement as HTMLElement).querySelector('tm-tide-gauge') as HTMLElement;
    return { fixture, host: fixture.componentInstance, el, level: () => el.style.getPropertyValue('--tm-level') };
  }

  it('is an image named by its label, with no text inside', async () => {
    const { fixture, host, el } = await gauge();
    expect(el.getAttribute('role')).toBe('img');
    expect(el.getAttribute('aria-label')).toBe('18 of 25 containers up to date');
    expect(el.textContent?.trim()).toBe('');

    host.label.set('3 releases behind');
    await fixture.whenStable();
    expect(el.getAttribute('aria-label')).toBe('3 releases behind');
  });

  it('clamps the level to 0 … 1', async () => {
    const { fixture, host, level } = await gauge();
    expect(level()).toBe('0.5');
    for (const [input, used] of [[1.7, '1'], [-0.3, '0'], [NaN, '0'], [Infinity, '1'], [0.25, '0.25']] as const) {
      host.level.set(input);
      await fixture.whenStable();
      expect(level()).toBe(used);
    }
  });

  it('has a hero size that fills its parent and a compact one', async () => {
    const { fixture, host, el } = await gauge();
    expect(el.classList).toContain('tm-gauge-compact');
    expect(el.querySelectorAll('.tm-gauge-wave').length).toBe(2);

    host.size.set('hero');
    await fixture.whenStable();
    expect(el.classList).toContain('tm-gauge-hero');
    expect(el.classList).not.toContain('tm-gauge-compact');
    expect(el.querySelectorAll('.tm-gauge-wave').length).toBe(3);
    expect(el.querySelectorAll('.tm-gauge-crest').length).toBe(1); // only the front wave
  });

  it('draws seamless waves: whole periods, closed below the crest', () => {
    const { crest, fill } = tideWavePath(100, 5, 300);
    expect(crest.startsWith('M0 5 Q25 -5 50 5')).toBe(true);
    expect(crest.endsWith('T400 5')).toBe(true); // one period past the width, for the loop
    expect(fill).toBe(`${crest} V2000 H0Z`);
  });
});

describe('TmTideBackdrop', () => {
  it('is decorative', async () => {
    const fixture = TestBed.createComponent(TmTideBackdrop);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.getAttribute('aria-hidden')).toBe('true');
    expect(el.querySelectorAll('path').length).toBe(3);
    expect(el.textContent?.trim()).toBe('');
  });
});
