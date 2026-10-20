import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { TmTooltip } from './tooltip';

describe('TmTooltip', () => {
  @Component({
    imports: [TmTooltip],
    template: `<button [tmTooltip]="text()" aria-describedby="other">Go</button>
      <span tmTooltip="Second tip" data-testid="second">Second</span>
      <p data-testid="outside">Elsewhere</p>`,
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

  const pointer = (type: string, el: Element, pointerType: string) => {
    const e = new Event(type, { bubbles: true, cancelable: true });
    Object.defineProperty(e, 'pointerType', { value: pointerType });
    el.dispatchEvent(e);
  };
  /** A press and its click, as a touch screen (or a mouse) sends them. Returns whether the click's default was prevented. */
  const tap = (el: Element, pointerType = 'touch') => {
    pointer('pointerdown', el, pointerType);
    pointer('pointerup', el, pointerType);
    pointer('pointerleave', el, pointerType);
    const click = new MouseEvent('click', { bubbles: true, cancelable: true });
    el.dispatchEvent(click);
    return click.defaultPrevented;
  };
  const byTestId = (fixture: { nativeElement: HTMLElement }, id: string) =>
    fixture.nativeElement.querySelector<HTMLElement>(`[data-testid="${id}"]`)!;
  const tipFor = (el: Element) => document.getElementById(el.getAttribute('aria-describedby')!.split(' ').pop()!);

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

  it('toggles on a touch tap and stays open when the finger lifts', async () => {
    const { fixture, button } = await render();
    vi.useFakeTimers();
    tap(button);
    vi.advanceTimersByTime(500);
    const el = tip()!;
    expect(el.textContent).toBe('Pinned to 1.4');
    expect(el.hasAttribute('data-open')).toBe(true);

    tap(button);
    expect(el.hasAttribute('data-open')).toBe(false);
    tap(button, 'pen');
    expect(el.hasAttribute('data-open')).toBe(true);
    fixture.destroy();
  });

  it('closes on a tap elsewhere', async () => {
    const { fixture, button } = await render();
    tap(button);
    expect(tip()!.hasAttribute('data-open')).toBe(true);
    tap(byTestId(fixture, 'outside'));
    expect(tip()!.hasAttribute('data-open')).toBe(false);
    fixture.destroy();
  });

  it('keeps only one tooltip open', async () => {
    const { fixture, button } = await render();
    const second = byTestId(fixture, 'second');
    tap(button);
    const first = tipFor(button)!;
    expect(first.hasAttribute('data-open')).toBe(true);

    tap(second);
    expect(tipFor(second)!.hasAttribute('data-open')).toBe(true);
    expect(first.hasAttribute('data-open')).toBe(false);

    keyboardFocus(button);
    expect(first.hasAttribute('data-open')).toBe(true);
    expect(tipFor(second)!.hasAttribute('data-open')).toBe(false);
    fixture.destroy();
  });

  it('prevents the click default only on a non-interactive host', async () => {
    const { fixture, button } = await render();
    expect(tap(byTestId(fixture, 'second'))).toBe(true);
    expect(tap(button)).toBe(false);
    fixture.destroy();
  });

  it('does not show on a mouse click', async () => {
    const { fixture, button } = await render();
    vi.useFakeTimers();
    expect(tap(button, 'mouse')).toBe(false);
    vi.advanceTimersByTime(500);
    expect(tip()).toBeNull();
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
