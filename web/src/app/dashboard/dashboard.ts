import { Component, computed } from '@angular/core';
import { DatePipe } from '@angular/common';
import { HttpErrorResponse, httpResource } from '@angular/common/http';
import { TmBadge, TmButton, TmCard, TmEmptyState, TmSkeleton } from '../ui';
import { ContainerList, groupByStack, imageName } from './containers';

@Component({
  selector: 'tm-dashboard',
  imports: [DatePipe, TmBadge, TmButton, TmCard, TmEmptyState, TmSkeleton],
  templateUrl: './dashboard.html',
})
export class Dashboard {
  protected readonly list = httpResource<ContainerList>(() => '/api/containers');

  // value() throws while the resource is in an error state, so always go through hasValue().
  private readonly data = computed(() => (this.list.hasValue() ? this.list.value() : undefined));

  protected readonly stacks = computed(() => groupByStack(this.data()?.containers ?? []));
  protected readonly source = computed(() => this.data()?.source);

  protected readonly summary = computed(() => {
    const containers = this.data()?.containers ?? [];
    const running = containers.filter((c) => c.state === 'running').length;
    const stacks = this.stacks().filter((s) => s.name !== null).length;
    const parts = [`${containers.length} containers`, `${stacks} stacks`];
    if (running !== containers.length) parts.push(`${containers.length - running} not running`);
    return parts.join(' · ');
  });

  protected readonly errorMessage = computed(() => {
    const err = this.list.error();
    if (err instanceof HttpErrorResponse) {
      return (err.error as { error?: string } | null)?.error ?? `The API answered ${err.status} ${err.statusText}.`;
    }
    return err ? String(err) : '';
  });

  protected readonly imageName = imageName;
}
