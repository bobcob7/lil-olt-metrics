# ✅ COMPLETE — Plan 03 — Core Types, Store Interface & Prometheus Adapter

## Summary

Define the central abstractions that all downstream packages depend on: the internal time-series data model, the `Store` interface (write path + read path), and the `storage.Queryable` / `storage.ChunkQuerier` adapter that lets the Prometheus PromQL engine read from any Store implementation. No concrete Store implementation yet — this plan is purely about contracts and the adapter layer.

## Dependencies

- **Plan 01** — Go module and dependencies (Prometheus client/engine packages must be available)

## Scope

### In Scope

- Internal time-series types in `internal/store/`: `Sample`, `Series`, `Labels`, `Label` (or reuse Prometheus `labels.Labels` if appropriate)
- `Store` interface in `internal/store/interfaces.go` with methods for: writing samples, selecting series by matchers, listing label names, listing label values, closing
- `Appender` interface for batched writes (matching Prometheus TSDB's appender pattern)
- Prometheus `storage.Queryable` adapter that wraps any `Store` and satisfies the interface the PromQL engine expects
- Prometheus `storage.Querier` / `storage.ChunkQuerier` adapter translating PromQL `Select` calls into `Store.Select` calls
- `SeriesSet` implementation to bridge internal series iteration to Prometheus's `storage.SeriesSet`
- Unit tests for the adapter layer using a trivial stub Store (hardcoded data, not the real in-memory store)
- `moq` generation directive in `interfaces.go` for the `Store` and `Appender` interfaces

### Out of Scope

- Concrete Store implementations (Plan 04, 09, 10)
- OTLP-to-Prometheus type mapping (Plan 05)
- PromQL engine instantiation (Plan 07)

## Acceptance Criteria

1. `Store` interface is defined with write, select, label-name, label-value, and close methods
2. `Appender` interface supports add-sample, add-exemplar, commit, and rollback
3. The Prometheus `storage.Queryable` adapter compiles and satisfies the interface
4. A unit test creates a stub Store, wraps it in the Queryable adapter, and verifies that `Select` returns the expected series
5. A unit test verifies `LabelNames` and `LabelValues` pass through the adapter correctly
6. `SeriesSet` correctly implements the Prometheus `storage.SeriesSet` interface (iteration, error propagation)
7. `go generate ./internal/store/...` produces `moq_test.go` with mocks for `Store` and `Appender`
8. No concrete storage logic exists in this plan — only interfaces and the adapter

## Key Decisions

- **Reuse Prometheus `labels.Labels`**: Avoids a translation layer between internal types and what the PromQL engine expects; reduces allocations on the query path
- **Separate `Appender` from `Store`**: Matches the Prometheus TSDB pattern where writes go through a short-lived Appender that is committed atomically; makes the in-memory store (Plan 04) and FS store (Plan 09) easier to implement
- **Adapter lives in `internal/store/`**: Keeps the Prometheus coupling in one package; query API (Plan 07) only sees the `storage.Queryable` interface
- **Interfaces defined at consumer boundary**: The `Store` and `Appender` interfaces are consumed by the adapter and the ingestion path, so defining them in `internal/store/` (the lowest common package) is appropriate here despite the general "interfaces at consumer" rule
- **ChunkQuerier returns not-implemented**: The PromQL engine can use either Querier or ChunkQuerier; implementing only Querier is sufficient for MVP and ChunkQuerier can return an appropriate error
