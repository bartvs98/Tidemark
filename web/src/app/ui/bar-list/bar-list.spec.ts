import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { BarListItem, TmBarList } from './bar-list';

describe('TmBarList', () => {
  @Component({
    imports: [TmBarList],
    template: `<tm-bar-list label="Updates per stack" [items]="items()" />`,
  })
  class Host {
    items = signal<BarListItem[]>([
      { label: 'media', value: 12, href: '/containers/jellyfin' },
      { label: 'immich-app/immich', value: 6, href: 'https://github.com/immich-app/immich', mono: true, hint: '2 containers' },
      { label: 'No stack', value: 3, display: '3 updates', tone: 'muted' },
    ]);
  }

  async function render() {
    TestBed.configureTestingModule({ providers: [provideRouter([])] });
    const fixture = TestBed.createComponent(Host);
    await fixture.whenStable();
    return { fixture, host: fixture.componentInstance, el: fixture.nativeElement as HTMLElement };
  }

  it('is a named list with values and proportional bars', async () => {
    const { el } = await render();
    expect(el.querySelector('ul')!.getAttribute('aria-label')).toBe('Updates per stack');
    const bars = [...el.querySelectorAll<HTMLElement>('[aria-hidden="true"] > div')];
    expect(bars.map((b) => b.style.width)).toEqual(['100%', '50%', '25%']);
    expect(bars[2].style.backgroundColor).toBe('var(--color-fg-subtle)');
    const values = [...el.querySelectorAll('li .tabular-nums')].map((v) => v.textContent);
    expect(values).toEqual(['12', '6', '3 updates']);
  });

  it('links router paths and external urls', async () => {
    const { el } = await render();
    const links = el.querySelectorAll('a');
    expect(links[0].getAttribute('href')).toBe('/containers/jellyfin');
    expect(links[0].hasAttribute('target')).toBe(false);
    expect(links[1].getAttribute('href')).toBe('https://github.com/immich-app/immich');
    expect(links[1].getAttribute('target')).toBe('_blank');
    expect(links[1].classList).toContain('font-mono');
    expect(el.textContent).toContain('2 containers');
  });

  it('shows the empty text without items', async () => {
    const { fixture, host, el } = await render();
    host.items.set([]);
    await fixture.whenStable();
    expect(el.querySelector('ul')).toBeNull();
    expect(el.textContent).toContain('Nothing yet');
  });
});
