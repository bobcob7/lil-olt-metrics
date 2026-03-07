import { describe, it, expect } from 'vitest';
import { fmtNum, fmtCost, fmtDuration } from '../utils/format';

describe('fmtNum', () => {
  it('formats billions', () => {
    expect(fmtNum(1_500_000_000)).toBe('1.50B');
  });
  it('formats millions', () => {
    expect(fmtNum(2_345_678)).toBe('2.35M');
  });
  it('formats thousands', () => {
    expect(fmtNum(12_345)).toBe('12.3K');
  });
  it('formats small numbers', () => {
    expect(fmtNum(42)).toBe('42');
  });
  it('formats zero', () => {
    expect(fmtNum(0)).toBe('0');
  });
});

describe('fmtCost', () => {
  it('formats dollars', () => {
    expect(fmtCost(12.5)).toBe('$12.50');
  });
  it('formats cents', () => {
    expect(fmtCost(0.05)).toBe('$0.050');
  });
  it('formats sub-cent', () => {
    expect(fmtCost(0.001)).toBe('$0.0010');
  });
});

describe('fmtDuration', () => {
  it('formats hours', () => {
    expect(fmtDuration(7200)).toBe('2.0h');
  });
  it('formats minutes', () => {
    expect(fmtDuration(150)).toBe('2.5m');
  });
  it('formats seconds', () => {
    expect(fmtDuration(30)).toBe('30s');
  });
});
