# ✅ COMPLETE — Plan 04 — In-Memory Store (Head Block)

## Summary

Implement the in-memory Store that holds the "head block" — recent samples not yet persisted to disk. This is the primary write target for ingested metrics and the primary read source for queries. It must be safe for concurrent reads and writes, memory-efficient, and fast enough to handle the target throughput.

## Dependencies

- **Plan 03** — `Store` and `Appender` interfaces, `labels.Labels` usage, `SeriesSet`

## Scope

### In Scope

- `memStore` struct in `internal/store/` implementing the `Store` interface
- In-memory series index: map from label set (fingerprint) to series data
- Per-series sample buffer holding timestamped float64 values
- `Appender` implementation that buffers writes and commits atomically
- Concurrent access: writes are serialized (or use fine-grained locking), reads can proceed concurrently with writes
- Series selection by label matchers (equality, not-equal, regex, not-regex)
- Label name and label value enumeration (with optional matchers)
- Pruning: ability to drop samples older than a configurable duration (in-memory retention)
- Constructor accepting `*slog.Logger` and retention duration
- Unit tests covering: write and read-back, concurrent write+read safety, matcher filtering, label enumeration, retention pruning, empty store behavior, appender rollback

### Out of Scope

- Disk persistence, WAL, compaction (Plan 09)
- Remote storage backends (Plan 10)
- Prometheus Queryable adapter (already done in Plan 03)
- Translation from OTLP (Plan 05)

## Acceptance Criteria

1. Writing samples via `Appender` and reading them back via `Select` returns correct data
2. Label matchers (eq, neq, regex, not-regex) filter series correctly
3. `LabelNames` and `LabelValues` return the correct unique sets
4. Concurrent goroutines can write and read without data races (`go test -race` passes)
5. Samples older than the retention window are pruned and no longer returned by queries
6. `Appender.Rollback()` discards uncommitted samples
7. The store can be wrapped in the Prometheus `Queryable` adapter from Plan 03 and return valid `SeriesSet` results
8. Benchmark test demonstrates write throughput (informational, no hard threshold)
9. All tests use `t.Parallel()`, `t.Context()`, and discard loggers

## Key Decisions

- **Fingerprint-based index**: Hash label sets to uint64 for O(1) series lookup on write path; fall back to full label comparison on collision
- **Slice-of-samples per series**: Append-only sorted slice is cache-friendly and sufficient for the target scale; no need for a skip list or B-tree
- **Serialized writes with concurrent reads**: A single write lock simplifies correctness; read path uses `sync.RWMutex` so queries don't block each other
- **In-memory retention separate from FS retention**: The head block trims old samples independently; FS persistence (Plan 09) manages its own block lifecycle
- **No chunk encoding in memory**: Prometheus TSDB uses XOR/delta encoding in chunks; for simplicity, store raw float64 samples in the head block — encoding happens only when persisting to disk (Plan 09)
