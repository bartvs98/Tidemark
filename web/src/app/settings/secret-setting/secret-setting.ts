import { afterNextRender, Component, computed, ElementRef, inject, Injector, input, model, viewChild } from '@angular/core';
import { TmButton, TmField, TmIcon, TmPasswordInput } from '../../ui';
import type { SettingField } from '../settings-dialog';

/**
 * A write-only setting (a token) in the settings dialog: "Set · Replace · Clear". The server
 * never sends the secret, only whether one is in effect, so this shows that state with buttons
 * to type a new one (a `tm-password-input`, focused) or to clear it on Save (with an undo).
 * Set by an env var, it only shows the state and the locked note.
 *
 *   <tm-secret-setting label="Token" [field]="fields.githubToken" [error]="errors.githubToken" [(value)]="edit" />
 *
 * `value` is the pending edit: `undefined` = untouched, a string = the new secret being typed
 * (`''` while the input is open but empty), `null` = cleared on Save.
 */
@Component({
  selector: 'tm-secret-setting',
  imports: [TmButton, TmField, TmIcon, TmPasswordInput],
  templateUrl: './secret-setting.html',
  host: { class: 'block' },
})
export class SecretSetting {
  readonly label = input.required<string>();
  readonly field = input.required<SettingField>();
  readonly hint = input<string>();
  readonly error = input<string | null>();
  readonly placeholder = input('');
  readonly value = model<string | null | undefined>(undefined);

  protected readonly locked = computed(() => (this.field().source === 'env' ? `Set by ${this.field().env}` : null));
  protected readonly state = computed<'idle' | 'typing' | 'clearing'>(() => {
    const v = this.value();
    return v === undefined ? 'idle' : v === null ? 'clearing' : 'typing';
  });

  private readonly injector = inject(Injector);
  private readonly input = viewChild(TmPasswordInput);
  private readonly start = viewChild('start', { read: ElementRef<HTMLButtonElement> });
  private readonly undo = viewChild('undo', { read: ElementRef<HTMLButtonElement> });

  protected replace(): void {
    this.value.set('');
    this.afterRender(() => this.input()?.focus());
  }

  protected clear(): void {
    this.value.set(null);
    this.afterRender(() => this.undo()?.nativeElement.focus());
  }

  protected cancel(): void {
    this.value.set(undefined);
    this.afterRender(() => this.start()?.nativeElement.focus());
  }

  /** Moves focus into what replaced the button that had it. */
  private afterRender(fn: () => void): void {
    afterNextRender(fn, { injector: this.injector });
  }
}
