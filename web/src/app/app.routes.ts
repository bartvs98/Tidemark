import { Routes } from '@angular/router';

export const routes: Routes = [
  {
    path: '',
    title: 'Tidemark',
    loadComponent: () => import('./dashboard/dashboard').then((m) => m.Dashboard),
  },
  {
    path: 'containers/:name',
    title: 'Container · Tidemark',
    loadComponent: () => import('./detail/detail').then((m) => m.ContainerDetail),
  },
  {
    path: 'stats',
    title: 'Stats · Tidemark',
    loadComponent: () => import('./stats/stats').then((m) => m.Stats),
  },
  {
    path: 'styleguide',
    title: 'Styleguide · Tidemark',
    loadComponent: () => import('./styleguide/styleguide').then((m) => m.Styleguide),
  },
  { path: '**', redirectTo: '' },
];
