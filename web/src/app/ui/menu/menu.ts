import { booleanAttribute, Component, input, output } from '@angular/core';
import { Menu, MenuItem, MenuTrigger } from '@angular/aria/menu';
import { ButtonSize, ButtonVariant, TmButton } from '../button/button';
import { IconName, TmIcon } from '../icon/icon';
import { TmTooltip } from '../tooltip/tooltip';

export interface MenuOption<T extends string = string> {
  value: T;
  label: string;
  icon?: IconName;
  disabled?: boolean;
  /** Tooltip on the item, e.g. why it's disabled. */
  hint?: string;
}

/**
 * A button that opens a short list of actions. Built on Angular Aria's menu directives
 * (roles, roving focus, arrow keys, Home/End, typeahead, Escape, Enter/Space), styled like
 * the buttons and the dialog. Disabled items stay focusable (so their hint can be read) but
 * can't be chosen.
 *
 *   <tm-menu label="Snooze" icon="clock" size="sm" [items]="snoozeOptions" (selected)="snooze($event)" />
 */
@Component({
  selector: 'tm-menu',
  imports: [Menu, MenuItem, MenuTrigger, TmButton, TmIcon, TmTooltip],
  templateUrl: './menu.html',
  host: { class: 'relative inline-flex' },
})
export class TmMenu<T extends string> {
  readonly label = input.required<string>();
  readonly items = input.required<MenuOption<T>[]>();
  readonly icon = input<IconName>();
  readonly variant = input<ButtonVariant>('secondary');
  readonly size = input<ButtonSize>('md');
  readonly disabled = input(false, { transform: booleanAttribute });
  /** Which edge of the trigger the menu lines up with. */
  readonly align = input<'start' | 'end'>('start');

  /** The chosen item's value. */
  readonly selected = output<T>();

  protected select(value: T | undefined): void {
    if (value !== undefined) this.selected.emit(value);
  }
}
