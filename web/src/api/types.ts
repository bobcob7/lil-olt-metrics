export interface PromResult {
  readonly metric: Record<string, string>;
  readonly value: readonly [number, string];
}

export interface PromRangeResult {
  readonly metric: Record<string, string>;
  readonly values: ReadonlyArray<readonly [number, string]>;
}

export interface PromResponse<T> {
  readonly status: string;
  readonly data: {
    readonly resultType: string;
    readonly result: readonly T[];
  };
  readonly error?: string;
}
