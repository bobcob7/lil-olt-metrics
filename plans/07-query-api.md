# Plan 07 — Prometheus Query API & PromQL Engine

## Summary

Implement the Prometheus-compatible HTTP query API that lets dashboards (Grafana), alerting tools, and ad-hoc queries read metrics via PromQL. This plan wires the Prometheus PromQL engine to the Store's Queryable adapter (Plan 03) and exposes the standard API endpoints.

## Dependencies

- **Plan 03** — `storage.Queryable` adapter
- **Plan 04** — In-memory Store (read source)

## Scope

### In Scope

- Prometheus PromQL engine instantiation with configurable timeout and max samples
- HTTP API endpoints in `internal/query/` per `docs/prometheus-api.md`:
  - `GET/POST /api/v1/query` — instant query
  - `GET/POST /api/v1/query_range` — range query
  - `GET/POST /api/v1/series` — find matching series
  - `GET/POST /api/v1/labels` — list all label names
  - `GET/POST /api/v1/label/<name>/values` — list values for a label
  - `GET /api/v1/metadata` — metric metadata (minimal: name + type)
  - `GET /api/v1/status/buildinfo` — version/build info
- Prometheus JSON response format for all endpoints (status, data, errorType, error)
- Result types: vector, matrix, scalar, string
- Query parameter parsing: `query`, `time`, `start`, `end`, `step`, `match[]`, `timeout`
- Default lookback delta from config
- Constructor accepting `storage.Queryable`, config, build info, and `*slog.Logger`
- Unit tests covering: instant query returning a vector, range query returning a matrix, series endpoint, label name/value endpoints, malformed query errors, timeout behavior

### Out of Scope

- Full PromQL compatibility testing (Plan 12 integration tests)
- Remote read/write endpoints (Plan 10)
- Prometheus text exposition format (`/metrics`)
- Advanced query features beyond what the Prometheus engine provides out of the box

## Acceptance Criteria

1. `/api/v1/query?query=up&time=<ts>` returns a valid Prometheus JSON response with vector result type
2. `/api/v1/query_range?query=rate(x[5m])&start=...&end=...&step=...` returns a matrix result
3. `/api/v1/series?match[]=up` returns matching series labels
4. `/api/v1/labels` returns all label names from the Store
5. `/api/v1/label/__name__/values` returns all metric names
6. `/api/v1/metadata` returns metric type information
7. `/api/v1/status/buildinfo` returns version and Go runtime info
8. Invalid PromQL returns HTTP 200 with `"status": "error"` and `"errorType": "bad_data"` (per Prometheus convention)
9. Query timeout is enforced — long-running queries are cancelled and return a timeout error
10. Both GET and POST are supported for query endpoints
11. All tests use `t.Parallel()`, seed the Store with known data, and verify JSON response structure

## Key Decisions

- **Embed Prometheus PromQL engine**: Reimplementing PromQL is not feasible; the Prometheus engine is battle-tested and provides full compatibility for free
- **Standard HTTP mux, not a framework**: Use `net/http` with a simple router; the API surface is small and well-defined
- **Prometheus JSON format exactly**: Grafana and other tools depend on exact response shapes; match the Prometheus API response format byte-for-byte where possible
- **Metadata is best-effort**: Without OTLP metadata persistence, return what's available from the Store (metric name → type mapping from translation); this can be enhanced later
- **POST support from the start**: Grafana sends PromQL queries via POST to avoid URL length limits; support both methods on all query endpoints
