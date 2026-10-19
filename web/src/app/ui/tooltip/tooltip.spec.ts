import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { TmTooltip } from './tooltip';

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
