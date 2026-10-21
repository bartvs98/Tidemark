import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { TmField } from './field';
import { TmInput } from '../input/input';

@Component({
  imports: [TmField, TmInput],
  template: `
    <tm-field label="Schedule" [hint]="hint()" [error]="error()" [locked]="locked()" [required]="required()">
      <input tmInput />
    </tm-field>
  `,
})
class Host {
  hint = signal<string | undefined>('Cron, or off.');
  error = signal<string | undefined>(undefined);
  locked = signal<string | undefined>(undefined);
  required = signal(false);
}

describe('TmField', () => {
  async function setup() {
    const fixture = TestBed.createComponent(Host);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    const label = el.querySelector('label')!;
    const input = el.querySelector('input')!;
    const message = () => el.querySelector<HTMLElement>('p');
    return { fixture, host: fixture.componentInstance, el, label, input, message };
  }

  it('links the label to the control and describes it with the hint', async () => {
    const { label, input, message } = await setup();
    expect(label.textContent?.trim()).toBe('Schedule');
    expect(input.id).toBeTruthy();
    expect(label.htmlFor).toBe(input.id);
    expect(message()!.textContent?.trim()).toBe('Cron, or off.');
    expect(input.getAttribute('aria-describedby')).toBe(message()!.id);
    expect(input.getAttribute('aria-invalid')).toBeNull();
  });

  it('shows an error with an icon instead of the hint and marks the control invalid', async () => {
    const { fixture, host, el, input, message } = await setup();
    host.error.set('invalid cron spec: expected 5 fields');
    await fixture.whenStable();
    expect(el.querySelectorAll('p').length).toBe(1);
    expect(message()!.textContent).toContain('invalid cron spec: expected 5 fields');
    expect(message()!.textContent).toContain('Error:');
    expect(message()!.classList.contains('text-attention')).toBe(true);
    expect(message()!.querySelector('tm-icon')).not.toBeNull();
    expect(input.getAttribute('aria-invalid')).toBe('true');
    expect(input.getAttribute('aria-describedby')).toBe(message()!.id);
  });

  it('shows the locked note with the env var in mono', async () => {
    const { fixture, host, message } = await setup();
    host.locked.set('Set by TIDEMARK_SCHEDULE');
    await fixture.whenStable();
    expect(message()!.textContent?.replace(/\s+/g, ' ').trim()).toBe('Set by TIDEMARK_SCHEDULE');
    expect(message()!.querySelector('.font-mono')!.textContent).toBe('TIDEMARK_SCHEDULE');
  });

  it('drops aria-describedby without a message and marks required quietly', async () => {
    const { fixture, host, label, input } = await setup();
    host.hint.set(undefined);
    host.required.set(true);
    await fixture.whenStable();
    expect(input.getAttribute('aria-describedby')).toBeNull();
    expect(input.getAttribute('aria-required')).toBe('true');
    expect(label.querySelector('[aria-hidden="true"]')!.textContent).toBe('*');
  });
});
