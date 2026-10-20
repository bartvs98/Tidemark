import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MenuOption, TmMenu } from './menu';

describe('TmMenu', () => {
  @Component({
    imports: [TmMenu],
    template: `<tm-menu label="Snooze" icon="clock" [items]="items" [disabled]="disabled()" (selected)="chosen.push($event)" />`,
  })
  class Host {
    items: MenuOption[] = [
      { value: 'day', label: '1 day' },
      { value: 'week', label: '1 week', icon: 'clock' },
      { value: 'newer', label: 'Until a newer version', disabled: true, hint: 'No latest version known' },
    ];
    disabled = signal(false);
    chosen: string[] = [];
  }

  async function render() {
    const fixture = TestBed.createComponent(Host);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    const trigger = el.querySelector('button')!;
    const menu = el.querySelector<HTMLElement>('[role="menu"]')!;
    const items = () => [...el.querySelectorAll<HTMLElement>('[role="menuitem"]')];
    return { fixture, el, trigger, menu, items };
  }

  const key = (target: HTMLElement, k: string) => target.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true }));

  it('has a menu-button trigger and a hidden menu until opened', async () => {
    const { fixture, trigger, menu, items } = await render();
    expect(trigger.textContent).toContain('Snooze');
    expect(trigger.getAttribute('aria-haspopup')).toBe('true');
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(trigger.getAttribute('aria-controls')).toBe(menu.id);
    expect(menu.getAttribute('data-visible')).toBe('false');
    expect(items().map((i) => i.textContent?.trim())).toEqual(['1 day', '1 week', 'Until a newer version']);

    trigger.click();
    await fixture.whenStable();
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    expect(menu.getAttribute('data-visible')).toBe('true');
  });

  it('opens from the keyboard, moves with the arrow keys and chooses with Enter', async () => {
    const { fixture, trigger, menu, items } = await render();
    trigger.focus();
    key(trigger, 'ArrowDown');
    await fixture.whenStable();
    expect(menu.getAttribute('data-visible')).toBe('true');
    expect(document.activeElement).toBe(items()[0]);

    key(menu, 'ArrowDown');
    await fixture.whenStable();
    expect(document.activeElement).toBe(items()[1]);
    expect(items()[1].getAttribute('data-active')).toBe('true');

    key(menu, 'Enter');
    await fixture.whenStable();
    expect(fixture.componentInstance.chosen).toEqual(['week']);
    expect(menu.getAttribute('data-visible')).toBe('false');
    expect(document.activeElement).toBe(trigger);
  });

  it('closes on Escape without choosing', async () => {
    const { fixture, trigger, menu } = await render();
    trigger.focus();
    key(trigger, 'Enter');
    await fixture.whenStable();
    key(menu, 'Escape');
    await fixture.whenStable();
    expect(menu.getAttribute('data-visible')).toBe('false');
    expect(fixture.componentInstance.chosen).toEqual([]);
  });

  it('emits the clicked item', async () => {
    const { fixture, trigger, items } = await render();
    trigger.click();
    await fixture.whenStable();
    items()[0].click();
    await fixture.whenStable();
    expect(fixture.componentInstance.chosen).toEqual(['day']);
  });

  it("doesn't choose a disabled item, and gives it its hint", async () => {
    const { fixture, trigger, items } = await render();
    const off = items()[2];
    expect(off.getAttribute('aria-disabled')).toBe('true');
    expect(off.getAttribute('data-tooltip')).toBe('No latest version known');

    trigger.click();
    await fixture.whenStable();
    off.click();
    await fixture.whenStable();
    expect(fixture.componentInstance.chosen).toEqual([]);
  });

  it('can be disabled as a whole', async () => {
    const { fixture, trigger, menu } = await render();
    fixture.componentInstance.disabled.set(true);
    await fixture.whenStable();
    expect(trigger.disabled).toBe(true);
    trigger.click();
    await fixture.whenStable();
    expect(menu.getAttribute('data-visible')).toBe('false');
  });
});
