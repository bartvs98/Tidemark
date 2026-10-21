import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { form, FormField } from '@angular/forms/signals';
import { TmField } from '../field/field';
import { SelectOption, TmSelect } from './select';

type Level = 'all' | 'attention' | 'breaking';
const LEVELS: SelectOption<Level>[] = [
  { value: 'all', label: 'Every update' },
  { value: 'attention', label: 'Attention and breaking', hint: 'Skips plain updates' },
  { value: 'breaking', label: 'Breaking only' },
];

const key = (target: Element, k: string) => target.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true }));

describe('TmSelect', () => {
  @Component({
    imports: [TmSelect, TmField],
    template: `
      <tm-field label="Notify about" hint="Which updates send a message.">
        <tm-select [options]="levels" [disabled]="disabled()" [(value)]="level" />
      </tm-field>
    `,
  })
  class Standalone {
    levels = LEVELS;
    level = signal<Level>('all');
    disabled = signal(false);
  }

  async function setup() {
    const fixture = TestBed.createComponent(Standalone);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    const trigger = el.querySelector<HTMLElement>('[role="combobox"]')!;
    const listbox = () => el.querySelector<HTMLElement>('[role="listbox"]');
    const options = () => [...el.querySelectorAll<HTMLElement>('[role="option"]')];
    const settle = () => fixture.whenStable();
    return { fixture, el, trigger, listbox, options, settle };
  }

  it('shows the chosen label on a combobox named by the field', async () => {
    const { el, trigger, listbox } = await setup();
    expect(trigger.textContent).toContain('Every update');
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(trigger.tabIndex).toBe(0);
    const label = el.querySelector('label')!;
    expect(trigger.id).toBe(label.htmlFor);
    expect(trigger.getAttribute('aria-labelledby')).toBe(label.id);
    expect(trigger.getAttribute('aria-describedby')).toBe(el.querySelector('p')!.id);
    expect(listbox()).toBeNull();
  });

  it('opens on click with the chosen option checked and active, and chooses by click', async () => {
    const { fixture, trigger, listbox, options, settle } = await setup();
    trigger.click();
    await settle();
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    expect(listbox()).not.toBeNull();
    expect(trigger.getAttribute('aria-controls')).toBe(listbox()!.id);
    expect(options().map((o) => o.getAttribute('aria-selected'))).toEqual(['true', 'false', 'false']);
    expect(options()[1].textContent).toContain('Skips plain updates');

    options()[2].click();
    await settle();
    expect(fixture.componentInstance.level()).toBe('breaking');
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(document.activeElement).toBe(trigger);
  });

  it('works from the keyboard: arrows, Home/End, typeahead, Enter, Escape', async () => {
    const { fixture, trigger, options, settle } = await setup();
    trigger.focus();
    key(trigger, 'ArrowDown');
    await settle();
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    expect(trigger.getAttribute('aria-activedescendant')).toBe(options()[0].id);

    key(trigger, 'ArrowDown');
    await settle();
    expect(trigger.getAttribute('aria-activedescendant')).toBe(options()[1].id);
    expect(options()[1].getAttribute('data-active')).toBe('true');

    key(trigger, 'End');
    await settle();
    expect(trigger.getAttribute('aria-activedescendant')).toBe(options()[2].id);
    key(trigger, 'Home');
    await settle();
    expect(trigger.getAttribute('aria-activedescendant')).toBe(options()[0].id);
    key(trigger, 'a');
    await settle();
    expect(trigger.getAttribute('aria-activedescendant')).toBe(options()[1].id);

    key(trigger, 'Enter');
    await settle();
    expect(fixture.componentInstance.level()).toBe('attention');
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(document.activeElement).toBe(trigger);

    key(trigger, ' ');
    await settle();
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    key(trigger, 'ArrowDown');
    await settle();
    key(trigger, 'Escape');
    await settle();
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(fixture.componentInstance.level()).toBe('attention');
  });

  it('keeps the value when the chosen option is picked again', async () => {
    const { fixture, trigger, options, settle } = await setup();
    trigger.click();
    await settle();
    options()[0].click();
    await settle();
    expect(fixture.componentInstance.level()).toBe('all');
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
  });

  it("can't be opened when disabled", async () => {
    const { fixture, trigger, settle } = await setup();
    fixture.componentInstance.disabled.set(true);
    await settle();
    expect(trigger.getAttribute('aria-disabled')).toBe('true');
    expect(trigger.tabIndex).toBe(-1);
    trigger.click();
    await settle();
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
  });

  @Component({
    imports: [TmSelect, FormField],
    template: `<tm-select label="Notify about" placeholder="Pick a level" [options]="levels" [formField]="f.level" />`,
  })
  class WithForm {
    levels: SelectOption<Level | ''>[] = LEVELS;
    model = signal({ level: '' as Level | '' });
    f = form(this.model);
  }

  it('binds to a Signal Forms field and shows the placeholder while empty', async () => {
    const fixture = TestBed.createComponent(WithForm);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    const trigger = el.querySelector<HTMLElement>('[role="combobox"]')!;
    expect(trigger.getAttribute('aria-label')).toBe('Notify about');
    expect(trigger.textContent).toContain('Pick a level');

    trigger.click();
    await fixture.whenStable();
    el.querySelectorAll<HTMLElement>('[role="option"]')[1].click();
    await fixture.whenStable();
    expect(fixture.componentInstance.model().level).toBe('attention');
    expect(trigger.textContent).toContain('Attention and breaking');

    fixture.componentInstance.model.set({ level: 'breaking' });
    await fixture.whenStable();
    expect(trigger.textContent).toContain('Breaking only');
  });
});
