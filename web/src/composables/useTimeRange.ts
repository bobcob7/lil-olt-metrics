import { ref } from 'vue';

export const STEP_MAP: Record<string, string> = {
  '1m': '5s', '5m': '15s', '30m': '30s', '1h': '1m', '4h': '5m',
  '8h': '10m', '24h': '30m', '2d': '1h', '7d': '4h', '14d': '8h',
  '30d': '12h', '90d': '24h',
};

export const RANGE_SECONDS: Record<string, number> = {
  '1m': 60, '5m': 300, '30m': 1800, '1h': 3600, '4h': 14400,
  '8h': 28800, '24h': 86400, '2d': 172800, '7d': 604800, '14d': 1209600,
  '30d': 2592000, '90d': 7776000,
};

export const TIME_RANGE_OPTIONS = [
  '1m', '5m', '30m', '1h', '4h', '8h', '24h', '2d', '7d', '14d', '30d', '90d',
] as const;

export const useTimeRange = () => {
  const selectedRange = ref('30d');
  return { selectedRange, STEP_MAP, RANGE_SECONDS, TIME_RANGE_OPTIONS };
};
