# Plan 10 — Remote Storage Backends (Prometheus + VictoriaMetrics)

## Summary

Implement the Prometheus remote-write and VictoriaMetrics remote-write/read storage backends. These allow lil-olt-metrics to forward ingested metrics to an external TSDB rather than (or in addition to) storing them locally. Each backend implements the same `Store` interface from Plan 03.

## Dependencies

- **Plan 02** — Configuration (remote backend URLs, timeouts, auth settings)
- **Plan 03** — `Store` and `Appender` interfaces

## Scope

### In Scope

- Prometheus remote-write backend in `internal/store/prometheus.go`:
  - Implements `Store` interface (write path only for MVP)
  - Converts internal samples to Prometheus remote-write protobuf format
  - HTTP POST to configured remote-write endpoint with snappy compression
  - Configurable batch size, flush interval, retry with backoff
  - Basic auth and bearer token support
- VictoriaMetrics remote backend in `internal/store/victoriametrics.go`:
  - Implements `Store` interface (write and read paths)
  - Write: POST to VictoriaMetrics import endpoint
  - Read: query VictoriaMetrics via its Prometheus-compatible query API
  - Configurable endpoint URL, auth, timeouts
- Remote read support for Prometheus backend:
  - Implements `Select`, `LabelNames`, `LabelValues` by proxying to a Prometheus-compatible read endpoint
- Batching and buffering: accumulate samples and flush periodically or when batch is full
- Error handling: log failures, retry transient errors, drop after max retries with a warning
- Constructor for each backend accepting config and `*slog.Logger`
- Unit tests: mock the HTTP endpoints, verify correct protobuf/request format, test retry behavior, test batch flushing

### Out of Scope

- Fan-out to multiple backends simultaneously (could be added as a composite Store later)
- Queue-based buffering with disk spillover
- mTLS client certificates
- Prometheus remote-read protocol (different from VictoriaMetrics approach)

## Acceptance Criteria

1. Prometheus remote-write backend sends correctly formatted protobuf to the configured endpoint
2. Snappy compression is applied to Prometheus remote-write payloads
3. VictoriaMetrics backend writes to the import endpoint in the expected format
4. VictoriaMetrics backend reads (Select, LabelNames, LabelValues) return data from the remote
5. Basic auth credentials are included in requests when configured
6. Samples are batched and flushed at the configured interval or batch size
7. Transient HTTP errors (5xx, connection refused) trigger retries with exponential backoff
8. After max retries, failed batches are dropped with a structured log warning (not a crash)
9. Backends are selectable via the `storage.engine` config field
10. All tests use `t.Parallel()`, use `httptest.Server` for mock endpoints

## Key Decisions

- **Store interface, not a separate "remote" abstraction**: Remote backends implement the same `Store` interface as the in-memory and FS stores; the server doesn't need to know where data goes
- **Write-path priority**: Remote write is the primary use case; remote read is secondary and only fully implemented for VictoriaMetrics (which has a Prometheus-compatible API)
- **No disk-based buffer queue**: For simplicity, buffer in memory only; if the remote is down for extended periods, data is lost after retry exhaustion — this is acceptable for the target use case (edge/dev deployments)
- **Separate backend files**: Each backend is its own file with its own struct; no inheritance or shared base — keeps each implementation self-contained and testable
- **Snappy for Prometheus, configurable for VM**: Prometheus remote-write requires snappy; VictoriaMetrics supports multiple encodings
