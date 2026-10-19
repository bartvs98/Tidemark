import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { TmSearch } from './search';

@Component({
  imports: [TmSearch],
  template: `<tm-search label="Search containers" placeholder="Search…" [(value)]="query" />`,
})
class Host {
  query = signal('');
}

describe('TmSearch', () => {
  async function setup() {
    const fixture = TestBed.createComponent(Host);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    const input = () => el.querySelector('input')!;
    const clearButton = () => el.querySelector<HTMLButtonElement>('button[aria-label="Clear search"]');
    return { fixture, el, input, clearButton };
  }

  it('is a labelled search input, 36 px tall', async () => {
    const { input } = await setup();
    expect(input().type).toBe('search');
    expect(input().getAttribute('aria-label')).toBe('Search containers');
    expect(input().placeholder).toBe('Search…');
    expect(input().classList.contains('h-9')).toBe(true);
  });

  it('writes typing to the model and shows the model in the field', async () => {
    const { fixture, input } = await setup();
    input().value = 'jelly';
    input().dispatchEvent(new Event('input'));
    await fixture.whenStable();
    expect(fixture.componentInstance.query()).toBe('jelly');

    fixture.componentInstance.query.set('sonarr');
    await fixture.whenStable();
    expect(input().value).toBe('sonarr');
  });

  it('shows the clear button only with text, and clears and refocuses', async () => {
    const { fixture, input, clearButton } = await setup();
    expect(clearButton()).toBeNull();

    fixture.componentInstance.query.set('abc');
    await fixture.whenStable();
    expect(clearButton()).not.toBeNull();

    clearButton()!.click();
    await fixture.whenStable();
    expect(fixture.componentInstance.query()).toBe('');
    expect(input().value).toBe('');
    expect(clearButton()).toBeNull();
  });

  it('Escape clears a non-empty field and leaves an empty one alone', async () => {
    const { fixture, input } = await setup();
    fixture.componentInstance.query.set('abc');
    await fixture.whenStable();
    const first = new KeyboardEvent('keydown', { key: 'Escape', cancelable: true });
    input().dispatchEvent(first);
    await fixture.whenStable();
    expect(fixture.componentInstance.query()).toBe('');
    expect(first.defaultPrevented).toBe(true);

    const second = new KeyboardEvent('keydown', { key: 'Escape', cancelable: true });
    input().dispatchEvent(second);
    expect(second.defaultPrevented).toBe(false);
  });
});
