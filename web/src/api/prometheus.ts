import type { PromResult, PromRangeResult, PromResponse } from './types';

const PROM_URL = '';

export const promQuery = async (query: string): Promise<readonly PromResult[]> => {
  const url = `${PROM_URL}/api/v1/query?query=${encodeURIComponent(query)}`;
  const resp = await fetch(url);
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  const json = (await resp.json()) as PromResponse<PromResult>;
  if (json.status !== 'success') throw new Error(json.error ?? 'query failed');
  return json.data.result;
};

export const promRangeQuery = async (
  query: string,
  start: number,
  end: number,
  step: string,
): Promise<readonly PromRangeResult[]> => {
  const resolved = query.replaceAll('{step}', step);
  const url = `${PROM_URL}/api/v1/query_range?query=${encodeURIComponent(resolved)}&start=${start}&end=${end}&step=${step}`;
  const resp = await fetch(url);
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  const json = (await resp.json()) as PromResponse<PromRangeResult>;
  if (json.status !== 'success') throw new Error(json.error ?? 'range query failed');
  return json.data.result ?? [];
};

export const getVal = (result: readonly PromResult[]): number => {
  if (result.length === 0) return 0;
  return result.reduce((sum, r) => sum + parseFloat(r.value[1] ?? '0'), 0);
};
