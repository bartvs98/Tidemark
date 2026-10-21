import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { disabled, form, FormField } from '@angular/forms/signals';
import { TmField } from '../field/field';
import { TmPasswordInput } from './password-input';

describe('TmPasswordInput', () => {
  @Component({
    imports: [TmPasswordInput, TmField],
    template: `
      <tm-field label="Bot token" hint="From @BotFather.">
        <tm-password-input placeholder="123456:ABC…" [(value)]="token" />
      </tm-field>
    `,
  })
  class Standalone {
    token = signal('');
  }

  async function setup() {
    const fixture = TestBed.createComponent(Standalone);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    const input = el.querySelector('input')!;
    const toggle = el.querySelector<HTMLButtonElement>('button')!;
    return { fixture, el, input, toggle };
  }

  it('is a mono password input without autocomplete, linked to its field', async () => {
    const { el, input } = await setup();
    expect(input.type).toBe('password');
    expect(input.getAttribute('autocomplete')).toBe('off');
    expect(input.placeholder).toBe('123456:ABC…');
    expect(input.classList.contains('font-mono')).toBe(true);
    expect(el.querySelector('label')!.htmlFor).toBe(input.id);
    expect(input.getAttribute('aria-describedby')).toBe(el.querySelector('p')!.id);
  });

  it('writes typing to the model and back', async () => {
    const { fixture, input } = await setup();
    input.value = 'abc:123';
    input.dispatchEvent(new Event('input'));
    await fixture.whenStable();
    expect(fixture.componentInstance.token()).toBe('abc:123');
    fixture.componentInstance.token.set('');
    await fixture.whenStable();
    expect(input.value).toBe('');
  });

  it('shows and hides the value with a labelled toggle', async () => {
    const { fixture, input, toggle } = await setup();
    expect(toggle.getAttribute('aria-label')).toBe('Show');
    expect(toggle.getAttribute('aria-pressed')).toBe('false');
    toggle.click();
    await fixture.whenStable();
    expect(input.type).toBe('text');
    expect(toggle.getAttribute('aria-pressed')).toBe('true');
    toggle.click();
    await fixture.whenStable();
    expect(input.type).toBe('password');
  });

  @Component({
    imports: [TmPasswordInput, FormField],
    template: `<tm-password-input [formField]="f.token" />`,
  })
  class WithForm {
    model = signal({ token: 'secret' });
    locked = signal(false);
    f = form(this.model, (p) => disabled(p.token, () => this.locked()));
  }

  it('binds to a Signal Forms field, including disabled and touched', async () => {
    const fixture = TestBed.createComponent(WithForm);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    const input = el.querySelector('input')!;
    expect(input.value).toBe('secret');

    input.value = 'other';
    input.dispatchEvent(new Event('input'));
    input.dispatchEvent(new Event('blur'));
    await fixture.whenStable();
    expect(fixture.componentInstance.model().token).toBe('other');
    expect(fixture.componentInstance.f.token().touched()).toBe(true);

    fixture.componentInstance.locked.set(true);
    await fixture.whenStable();
    expect(input.disabled).toBe(true);
    expect(el.querySelector('button')!.disabled).toBe(true);
  });
});
