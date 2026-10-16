import { Component } from '@angular/core';
import { httpResource } from '@angular/common/http';
import { TmCard, TmEmptyState, TmSkeleton } from '../ui';

interface Health {
  status: string;
  version: string;
}

/** Placeholder until step 1 (container inventory). */
@Component({
  selector: 'tm-dashboard',
  imports: [TmCard, TmEmptyState, TmSkeleton],
  template: `
    <h1 class="mb-6 text-2xl font-semibold tracking-tight">Containers</h1>

    <tm-card>
      @if (health.hasValue()) {
        <tm-empty-state
          heading="No containers yet"
          [description]="'Connected to Tidemark ' + health.value().version + '. Container discovery arrives in the next step.'"
        />
      } @else if (health.error()) {
        <tm-empty-state
          icon="triangle-alert"
          heading="Can't reach the API"
          description="Is the Go server running on :8080?"
        />
      } @else {
        <div class="flex flex-col gap-3 p-2" aria-label="Loading">
          <tm-skeleton width="40%" />
          <tm-skeleton width="70%" />
          <tm-skeleton width="55%" />
        </div>
      }
    </tm-card>
  `,
})
export class Dashboard {
  protected readonly health = httpResource<Health>(() => '/api/health');
}
