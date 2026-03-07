export const fmtNum = (n: number): string => {
  if (n >= 1e9) return (n / 1e9).toFixed(2) + 'B';
  if (n >= 1e6) return (n / 1e6).toFixed(2) + 'M';
  if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
  return Math.round(n).toLocaleString();
};

export const fmtCost = (n: number): string => {
  if (n >= 1) return '$' + n.toFixed(2);
  if (n >= 0.01) return '$' + n.toFixed(3);
  return '$' + n.toFixed(4);
};

export const fmtDuration = (secs: number): string => {
  if (secs >= 3600) return (secs / 3600).toFixed(1) + 'h';
  if (secs >= 60) return (secs / 60).toFixed(1) + 'm';
  return Math.round(secs) + 's';
};
