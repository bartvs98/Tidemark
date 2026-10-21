import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { disabled, form, FormField } from '@angular/forms/signals';
import { TmField } from '../field/field';
import { TmSwitch } from './switch';

describe('TmSwitch', () => {
  @Component({
    imports: [TmSwitch, TmField],
    template: `
      <tm-switch label="Dim quiet rows" size="sm" [(checked)]="dim" />
      <tm-field label="App icons" hint="Loaded from the icons URL." inline>
        <tm-switch [(checked)]="icons" />
      </tm-field>
    `,
  })
  class Standalone {
    dim = signal(false);
    icons = signal(true);
  }

  async function setup() {
    const fixture = TestBed.createComponent(Standalone);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    const [alone, inField] = [...el.querySelectorAll<HTMLButtonElement>('button[role="switch"]')];
    return { fixture, el, alone, inField };
  }

  it('is a native switch button named by its label', async () => {
    const { alone } = await setup();
    expect(alone.type).toBe('button');
    expect(alone.getAttribute('aria-label')).toBe('Dim quiet rows');
    expect(alone.getAttribute('aria-checked')).toBe('false');
    expect(alone.classList.contains('h-4')).toBe(true);
  });

  it('toggles the model on click and follows it', async () => {
    const { fixture, alone } = await setup();
    alone.click();
    await fixture.whenStable();
    expect(fixture.componentInstance.dim()).toBe(true);
    expect(alone.getAttribute('aria-checked')).toBe('true');
    fixture.componentInstance.dim.set(false);
    await fixture.whenStable();
    expect(alone.getAttribute('aria-checked')).toBe('false');
  });

  it('in a field, the label names and toggles it', async () => {
    const { fixture, el, inField } = await setup();
    const label = el.querySelector('label')!;
    expect(inField.getAttribute('aria-label')).toBeNull();
    expect(label.htmlFor).toBe(inField.id);
    expect(inField.getAttribute('aria-describedby')).toBe(el.querySelector('p')!.id);
    label.click();
    await fixture.whenStable();
    expect(fixture.componentInstance.icons()).toBe(false);
  });

  @Component({
    imports: [TmSwitch, FormField],
    template: `<tm-switch label="Icons" [formField]="f.icons" />`,
  })
  class WithForm {
    model = signal({ icons: false });
    locked = signal(false);
    f = form(this.model, (p) => disabled(p.icons, () => this.locked()));
  }

  it('binds to a Signal Forms checkbox field', async () => {
    const fixture = TestBed.createComponent(WithForm);
    await fixture.whenStable();
    const button = (fixture.nativeElement as HTMLElement).querySelector('button')!;
    expect(button.getAttribute('aria-checked')).toBe('false');
    button.click();
    await fixture.whenStable();
    expect(fixture.componentInstance.model().icons).toBe(true);

    fixture.componentInstance.locked.set(true);
    await fixture.whenStable();
    expect(button.disabled).toBe(true);
  });
});
