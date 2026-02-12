# COMPLETE Plan 12 — Integration Tests

## Summary

Create a comprehensive integration test suite that exercises the full system end-to-end: OTLP ingestion through translation, storage, and Prometheus query API response. These tests validate that all components work together correctly, not just in isolation. They run against a real server instance (in-process, no network mocking) with each storage backend.

## Dependencies

- **All plans 01–11** — The full system must be implemented

## Scope

### In Scope

- Integration test package (`internal/integration/` or top-level `integration_test.go` with build tag)
- Test harness that starts a full server in-process (all listeners on ephemeral ports)
- End-to-end ingestion-to-query tests:
  - Ingest OTLP Gauge via gRPC → query via PromQL instant query → verify value
  - Ingest OTLP Sum (counter) → query `rate()` via range query → verify result
  - Ingest OTLP Histogram → query `histogram_quantile()` → verify buckets
  - Ingest via HTTP protobuf → query → verify
  - Ingest via HTTP JSON → query → verify
- Label and series API tests:
  - Ingest multiple metrics → verify `/api/v1/series` returns correct matches
  - Verify `/api/v1/labels` and `/api/v1/label/<name>/values` reflect ingested data
- FS persistence tests (with build tag or skip if short):
  - Ingest data → stop server → restart → verify data survives
  - WAL replay correctness after simulated crash
- Delta conversion integration:
  - Send delta Sum data points → query cumulative result
- Concurrent load test:
  - Multiple goroutines ingesting simultaneously while queries run → no races, no data loss
- Error handling tests:
  - Malformed OTLP payload → appropriate error response
  - Invalid PromQL → appropriate error response
  - Query timeout → timeout error in response
- Remote backend tests (with build tag, require external service):
  - If Prometheus/VM endpoint available, verify write-through and read-back

### Out of Scope

- Performance benchmarking (informational benchmarks in unit tests are fine)
- Chaos/fault injection testing
- Multi-node or distributed testing
- Browser-based testing (Grafana integration)

## Acceptance Criteria

1. All integration tests pass with `go test -tags integration ./...`
2. End-to-end: ingest Gauge via gRPC, query via PromQL, get correct value
3. End-to-end: ingest counter via HTTP, query `rate()`, get non-zero result
4. End-to-end: ingest Histogram, query `histogram_quantile(0.95, ...)`, get valid result
5. Series and label APIs return data consistent with what was ingested
6. FS persistence: data survives server restart (WAL replay)
7. Concurrent ingestion + query runs without data races (`-race` flag passes)
8. All tests clean up after themselves (temp dirs, ports, goroutines)
9. Tests run in parallel where independent
10. Remote backend tests are skipped by default (require `-tags remote` and a running endpoint)

## Key Decisions

- **In-process server, real listeners**: Start the actual server with ephemeral ports (`:0`) rather than mocking the network; this catches real integration issues (serialization, routing, content types)
- **Build tags for slow/external tests**: FS persistence tests and remote backend tests use build tags so `go test ./...` stays fast; CI runs the full suite
- **Test data is deterministic**: Use fixed timestamps and values so assertions are exact, not approximate
- **One test function per scenario**: Avoid mega-tests that assert everything; each test function tests one flow with clear setup and assertions
- **Shared test helpers**: Factor out server startup, OTLP request construction, and Prometheus response parsing into test helpers to keep individual tests concise
