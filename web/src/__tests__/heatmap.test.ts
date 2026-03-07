import { describe, it, expect } from 'vitest';
import { computeLevels } from '../utils/heatmap';

describe('computeLevels', () => {
  it('returns 0 for all zeros', () => {
    const getLevel = computeLevels([0, 0, 0]);
    expect(getLevel(0)).toBe(0);
    expect(getLevel(5)).toBe(0);
  });

  it('returns 0 for empty array', () => {
    const getLevel = computeLevels([]);
    expect(getLevel(10)).toBe(0);
  });

  it('assigns levels based on percentiles', () => {
    const values = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12];
    const getLevel = computeLevels(values);
    expect(getLevel(0)).toBe(0);
    expect(getLevel(1)).toBe(1);
    expect(getLevel(12)).toBe(4);
  });

  it('returns 0 for negative values', () => {
    const getLevel = computeLevels([1, 2, 3, 4]);
    expect(getLevel(-1)).toBe(0);
  });
});
