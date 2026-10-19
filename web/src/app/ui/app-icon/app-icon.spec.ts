import { TestBed } from '@angular/core/testing';
import { Theme } from '../theme-switch/theme';
import { AppIconSources, TmAppIcon } from './app-icon';

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
