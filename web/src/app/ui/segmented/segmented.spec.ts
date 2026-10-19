import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { TmSegmented } from './segmented';

describe('TmSegmented size', () => {
  @Component({
    imports: [TmSegmented],
    template: `<tm-segmented label="View" size="md" [options]="options" [(value)]="value" />`,
  })
  class SegHost {
    options = [{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }];
    value = signal('a');
  }

  it('md is 36 px: h-9 host, h-8 buttons', async () => {
    const fixture = TestBed.createComponent(SegHost);
    await fixture.whenStable();
    const seg = (fixture.nativeElement as HTMLElement).querySelector('tm-segmented')!;
    expect(seg.classList.contains('h-9')).toBe(true);
    expect(seg.classList.contains('p-0.5')).toBe(false);
    expect(seg.querySelector('button')!.classList.contains('h-8')).toBe(true);
  });
});
