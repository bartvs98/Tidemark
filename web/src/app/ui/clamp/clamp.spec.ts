import { Component, viewChild } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { TmClamp } from './clamp';

describe('TmClamp', () => {
  // jsdom has no layout: heights come from `rem` styles (the cap probe) or a data-h child,
  // and an element's top from its data-top.
  beforeEach(() => {
    vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockImplementation(function (this: HTMLElement) {
      if (this.style.height.endsWith('rem')) return parseFloat(this.style.height) * 16;
      return Number(this.querySelector<HTMLElement>('[data-h]')?.dataset['h'] ?? 0);
    });
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
      const top = Number((this as HTMLElement).dataset?.['top'] ?? 0);
      return { top, bottom: top, left: 0, right: 0, width: 0, height: 0, x: 0, y: top, toJSON: () => ({}) } as DOMRect;
    });
  });
  afterEach(() => vi.restoreAllMocks());

  // Cap 10rem = 160px, of which the last 4rem sit under the fade: marks starting below 96px are hidden.
  @Component({
    imports: [TmClamp],
    template: `
      <tm-clamp max="10rem" [marks]="marks">
        <div data-h="1000">
          <p class="m" data-top="0">visible</p>
          <p class="m" data-top="120">under the fade</p>
          <p class="m" data-top="600">far below</p>
          <p id="plain" data-top="700">not a mark</p>
        </div>
      </tm-clamp>
    `,
  })
  class Host {
    marks: string | undefined = '.m';
    readonly clamp = viewChild.required(TmClamp);
  }

  async function render(marks?: string) {
    const fixture = TestBed.createComponent(Host);
    fixture.componentInstance.marks = marks;
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    return { fixture, el, button: () => el.querySelector('button') };
  }

  it('caps tall content behind "Show all" and counts the marks it hides', async () => {
    const { fixture, el, button } = await render('.m');
    expect(el.querySelector('[data-clamped]')).not.toBeNull();
    expect(button()?.textContent?.trim()).toBe('Show all · 2 flagged lines');

    button()!.click();
    await fixture.whenStable();
    expect(el.querySelector('[data-clamped]')).toBeNull();
    expect(button()?.textContent?.trim()).toBe('Show less');
    expect(button()?.getAttribute('aria-expanded')).toBe('true');
  });

  it('says just "Show all" without marks', async () => {
    const { button } = await render(undefined);
    expect(button()?.textContent?.trim()).toBe('Show all');
  });

  it('opens on reveal() for an element inside it, and only then', async () => {
    const { fixture, el } = await render('.m');
    const clamp = fixture.componentInstance.clamp();
    expect(clamp.reveal(document.body)).toBe(false);
    await fixture.whenStable();
    expect(el.querySelector('[data-clamped]')).not.toBeNull();

    expect(clamp.reveal(el.querySelector('#plain')!)).toBe(true);
    await fixture.whenStable();
    expect(el.querySelector('[data-clamped]')).toBeNull();
  });
});
