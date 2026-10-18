import { Component } from '@angular/core';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { TmThemeSwitch, TmTideBackdrop, TmToaster } from './ui';

@Component({
  selector: 'tm-root',
  imports: [RouterOutlet, RouterLink, RouterLinkActive, TmThemeSwitch, TmTideBackdrop, TmToaster],
  templateUrl: './app.html',
})
export class App {}
