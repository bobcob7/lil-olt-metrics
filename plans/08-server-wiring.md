# ✅ COMPLETE — Plan 08 — Server Entrypoint & End-to-End Startup

## Summary

Wire everything together in `cmd/server/main.go`: load config, construct all components, start gRPC and HTTP listeners, and handle graceful shutdown. After this plan, the binary is a working MVP — it can ingest OTLP metrics and serve them via the Prometheus query API, all in-memory.

## Dependencies

- **Plan 02** — Configuration loading
- **Plan 06** — OTLP ingestion handlers (gRPC + HTTP)
- **Plan 07** — Prometheus query API & PromQL engine

## Scope

### In Scope

- `cmd/server/main.go` implementing the full startup sequence:
  1. Parse CLI flag for config file path
  2. Call `config.Load()` to get validated config
  3. Initialize `slog.Logger` based on config (level, format)
  4. Create in-memory Store
  5. Create translator with translation config
  6. Create OTLP gRPC and HTTP handlers
  7. Create Prometheus Queryable adapter
  8. Create PromQL engine and query API handler
  9. Start gRPC listener, HTTP OTLP listener, and HTTP query API listener
  10. Block on OS signal (SIGINT, SIGTERM)
  11. Graceful shutdown: stop accepting new connections, drain in-flight requests, close Store
- Structured logging for all lifecycle events (starting, listening, shutting down)
- Exit codes: 0 for clean shutdown, 1 for startup failure
- Health check endpoint (`GET /healthz`) returning 200 when the server is ready
- Build-time version injection via ldflags (`-X main.version=...`)
- Makefile `build` target updated with ldflags

### Out of Scope

- Daemonization, systemd units, Docker (Plan 13)
- FS persistence startup (replaying WAL) — deferred to Plan 09
- Remote backend initialization — deferred to Plan 10
- Prometheus self-metrics exposition

## Acceptance Criteria

1. `./lil-olt-metrics` starts with zero config and listens on default ports (gRPC 4317, HTTP OTLP 4318, HTTP query 9090)
2. `./lil-olt-metrics --config lom.yaml` loads the specified config file
3. Sending an OTLP protobuf request to `localhost:4318/v1/metrics` succeeds
4. Querying `localhost:9090/api/v1/query?query=up` returns a valid Prometheus JSON response
5. End-to-end: ingest a metric via OTLP, then query it via PromQL and get the correct value back
6. `GET /healthz` on the query API port returns 200
7. Sending SIGINT triggers graceful shutdown: logs shutdown, drains requests, exits 0
8. Startup failure (e.g., port already in use) logs the error and exits 1
9. `--version` flag prints the version and exits
10. All lifecycle events are logged with structured fields (component, address, duration)

## Key Decisions

- **Three listeners**: gRPC (4317), HTTP OTLP (4318), and HTTP query API (9090) on separate ports matches the standard OTLP/Prometheus convention; keeps concerns separated
- **Constructor injection in main**: All components are wired manually in `main()` — no dependency injection framework
- **Graceful shutdown with timeout**: Wait up to a configurable duration (default 30s) for in-flight requests to complete, then force-close
- **Health check on query port**: Single healthz endpoint is sufficient for liveness probes; readiness can be the same endpoint since the server only starts listening after full initialization
- **No signal handler library**: `os/signal.NotifyContext` is sufficient for SIGINT/SIGTERM handling
