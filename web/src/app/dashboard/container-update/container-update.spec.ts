import { TestBed } from '@angular/core/testing';
import { ContainerUpdate } from './container-update';
import { ContainerSummary, Reason } from '../containers';

function container(reasons: Reason[]): ContainerSummary {
  return {
    id: 'x',
    name: 'app',
    image: { ref: 'app:1', display: 'app:1', registry: 'docker.io', repository: 'app', tag: '1', id: 'sha256:abc' },
    state: 'running',
    status: 'Up',
    created: '2026-10-01T00:00:00Z',
    update: { running: '1.0.0', source: 'tag', latest: '2.0.0', reviewKey: '2.0.0', class: 'major', checkedAt: '2026-10-16T12:00:00Z', level: 'breaking', reasons },
  } as ContainerSummary;
}

describe('ContainerUpdate level tooltip', () => {
  function tooltip(reasons: Reason[]): string | null {
    const fixture = TestBed.createComponent(ContainerUpdate);
    fixture.componentRef.setInput('container', container(reasons));
    fixture.detectChanges();
    return (fixture.nativeElement as HTMLElement).querySelector('[data-testid="level"]')!.getAttribute('data-tooltip');
  }

  it('prefixes reasons written by the AI summary with "AI: "', () => {
    const text = tooltip([
      { text: 'Major version: 1 → 2' },
      { text: 'removed X', tag: 'v1.9.0', line: '- removed X' },
      { text: 'The config key was renamed.', tag: 'v2.0.0', source: 'ai' },
      { text: 'No way back.', source: 'ai' },
    ]);
    expect(text?.split('\n')).toEqual(['Major version: 1 → 2', 'v1.9.0: removed X', 'AI: v2.0.0: The config key was renamed.', 'AI: No way back.']);
  });
});
