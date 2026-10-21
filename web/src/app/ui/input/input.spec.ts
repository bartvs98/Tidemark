import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { form, FormField, required } from '@angular/forms/signals';
import { TmInput } from './input';

describe('TmInput', () => {
  @Component({
    imports: [TmInput],
    template: `
      <input tmInput id="plain" placeholder="https://…" />
      <input tmInput size="sm" mono [invalid]="invalid()" [touched]="touched()" />
      <textarea tmInput></textarea>
    `,
  })
  class Plain {
    invalid = signal(false);
    touched = signal(false);
  }

  it('styles inputs and textareas by size, mono and invalid', async () => {
    const fixture = TestBed.createComponent(Plain);
    await fixture.whenStable();
    const [md, sm] = [...(fixture.nativeElement as HTMLElement).querySelectorAll('input')];
    const area = (fixture.nativeElement as HTMLElement).querySelector('textarea')!;
    expect(md.id).toBe('plain');
    expect(md.classList.contains('h-9')).toBe(true);
    expect(md.classList.contains('font-sans')).toBe(true);
    expect(sm.classList.contains('h-8')).toBe(true);
    expect(sm.classList.contains('font-mono')).toBe(true);
    expect(area.classList.contains('h-9')).toBe(false);
    expect(area.classList.contains('min-h-20')).toBe(true);

    expect(sm.getAttribute('aria-invalid')).toBeNull();
    fixture.componentInstance.invalid.set(true);
    await fixture.whenStable();
    expect(sm.getAttribute('aria-invalid')).toBeNull();
    fixture.componentInstance.touched.set(true);
    await fixture.whenStable();
    expect(sm.getAttribute('aria-invalid')).toBe('true');
  });

  @Component({
    imports: [TmInput, FormField],
    template: `<input tmInput mono [formField]="f.schedule" />`,
  })
  class WithForm {
    model = signal({ schedule: '0 */6 * * *' });
    f = form(this.model, (p) => required(p.schedule));
  }

  it('works with a Signal Forms field', async () => {
    const fixture = TestBed.createComponent(WithForm);
    await fixture.whenStable();
    const input = (fixture.nativeElement as HTMLElement).querySelector('input')!;
    expect(input.value).toBe('0 */6 * * *');

    input.value = '@daily';
    input.dispatchEvent(new Event('input'));
    await fixture.whenStable();
    expect(fixture.componentInstance.model().schedule).toBe('@daily');

    fixture.componentInstance.model.set({ schedule: 'off' });
    await fixture.whenStable();
    expect(input.value).toBe('off');

    // Client-side validity colours the input only once it's touched.
    fixture.componentInstance.model.set({ schedule: '' });
    await fixture.whenStable();
    expect(fixture.componentInstance.f.schedule().invalid()).toBe(true);
    expect(input.getAttribute('aria-invalid')).toBeNull();
    input.dispatchEvent(new Event('blur'));
    await fixture.whenStable();
    expect(input.getAttribute('aria-invalid')).toBe('true');
  });
});
