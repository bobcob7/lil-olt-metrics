export const computeLevels = (values: readonly number[]): ((v: number) => number) => {
  const nonZero = values.filter(v => v > 0).sort((a, b) => a - b);
  if (nonZero.length === 0) return () => 0;
  const p25 = nonZero[Math.floor(nonZero.length * 0.25)] ?? nonZero[0]!;
  const p50 = nonZero[Math.floor(nonZero.length * 0.50)] ?? p25;
  const p75 = nonZero[Math.floor(nonZero.length * 0.75)] ?? p50;
  return (v: number): number => {
    if (v <= 0) return 0;
    if (v <= p25) return 1;
    if (v <= p50) return 2;
    if (v <= p75) return 3;
    return 4;
  };
};

export const DAY_NAMES = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'] as const;
export const MONTH_NAMES = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'] as const;
