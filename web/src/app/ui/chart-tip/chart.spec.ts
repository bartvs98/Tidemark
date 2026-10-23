import { columnPath, niceStep, stepIndex, yScale } from './chart';

describe('chart helpers', () => {
  it('rounds steps up to 1, 2 or 5 × 10ⁿ', () => {
    expect(niceStep(0.3)).toBe(0.5);
    expect(niceStep(2.3)).toBe(5);
    expect(niceStep(17)).toBe(20);
    expect(niceStep(0)).toBe(1);
  });

  it('builds 3–4 gridlines from the data, or inside a given max', () => {
    expect(yScale(52, 0, undefined, true)).toEqual({ min: 0, max: 60, ticks: [0, 20, 40, 60] });
    expect(yScale(3, 0, undefined, true).ticks).toEqual([0, 1, 2, 3]);
    expect(yScale(1, 0, undefined, true).ticks).toEqual([0, 1]);
    expect(yScale(0.8, 0, 1)).toEqual({ min: 0, max: 1, ticks: [0, 0.5, 1] });
    expect(yScale(0.73)).toEqual({ min: 0, max: 1, ticks: [0, 0.5, 1] });
  });

  it('rounds only the data end of a column', () => {
    expect(columnPath(0, 10, 20, 30)).toBe('M0 40V14A4 4 0 0 1 4 10H16A4 4 0 0 1 20 14V40Z');
    expect(columnPath(0, 10, 20, 30, 0)).toBe('M0 40V10H20V40Z');
    expect(columnPath(0, 10, 20, 0)).toBe('');
  });

  it('steps an index with arrow keys, Home and End', () => {
    expect(stepIndex('ArrowLeft', null, 5)).toBe(4);
    expect(stepIndex('ArrowLeft', 2, 5)).toBe(1);
    expect(stepIndex('ArrowRight', 4, 5)).toBe(4);
    expect(stepIndex('Home', 3, 5)).toBe(0);
    expect(stepIndex('End', 0, 5)).toBe(4);
    expect(stepIndex('a', 0, 5)).toBeNull();
  });
});
