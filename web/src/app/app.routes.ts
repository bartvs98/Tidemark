import { Routes } from '@angular/router';

export const routes: Routes = [
  {
    path: '',
    title: 'Tidemark',
    loadComponent: () => import('./dashboard/dashboard').then((m) => m.Dashboard),
  },
  {
    path: 'styleguide',
    title: 'Styleguide · Tidemark',
    loadComponent: () => import('./styleguide/styleguide').then((m) => m.Styleguide),
  },
  { path: '**', redirectTo: '' },
];
