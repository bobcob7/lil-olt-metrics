# Plan 17 — OTLP Logs Receivers & Server Wiring

## Summary

Add OTLP logs ingest handlers (gRPC + HTTP) that mirror the existing metrics handlers, then wire the sessions store, logs translator, and handlers into `cmd/server/main.go`. After this plan the server accepts `claude_code.*` log events on the same `:4317`/`:4318` listeners and persists them to bbolt.

## Dependencies

- **Plan 14** (logs config)
- **Plan 15** (sessions store)
- **Plan 16** (logs translator)

## Scope

### In Scope

- `internal/ingest/logs_grpc.go`:
  - `LogsGRPCHandler` embedding `collogspb.UnimplementedLogsServiceServer`, fields `logger`, `translator logsTranslator`, `store sessionsStore`
  - `NewLogsGRPCHandler(logger, translator, store)`
  - `Export(ctx, *ExportLogsServiceRequest) (*ExportLogsServiceResponse, error)`:
    * If `len(ResourceLogs)==0` → empty success
    * Translate → for each event, call `store.AppendEvent(ctx, evt)`; on per-event error, accumulate count and continue
    * If all events failed → `status.Error(codes.Internal, ...)`; otherwise return success (with `PartialSuccess.ErrorMessage` if any failed)
- `internal/ingest/logs_http.go`:
  - `LogsHTTPHandler` analogous to `HTTPHandler`; route is `/v1/logs`
  - Accepts `application/x-protobuf`, `application/json`, `Content-Encoding: gzip`
  - Same gzip + size-limit + protojson fallback pattern as `http.go`
  - Calls translator + appends events
- `internal/ingest/logs_grpc_test.go` and `logs_http_test.go`:
  - Use the generated mocks (`logsTranslatorMock`, `sessionsStoreMock`) from `moq_test.go`
  - Cases per handler:
    * Happy path: translator returns 2 events → store called twice → response OK
    * Translator error + zero events → 400/InvalidArgument
    * Store append fails on one event → response is PartialSuccess (gRPC) or 200 with partial body (HTTP) and logger warns
    * Empty request → 200 OK, no store calls
  - HTTP-only: gzip body decompresses correctly; JSON Content-Type round-trips via protojson
- `cmd/server/main.go` changes:
  - After `translator := ingest.NewTranslator(...)`, add:
    ```go
    var sessionStore sessions.Store
    var logsTranslator *ingest.LogsTranslator
    if cfg.Logs.Enabled {
        s, err := sessions.NewBBoltStore(logger.With("component", "sessions"), sessions.BBoltConfig{
            Path:                cfg.Logs.Path,
            Retention:           cfg.Logs.Retention.AsDuration(),
            MaxEventsPerSession: cfg.Logs.MaxEventsPerSession,
        })
        if err != nil { logger.Error("opening sessions store", "error", err); return 1 }
        sessionStore = s
        defer func() { _ = s.Close() }()
        logsTranslator = ingest.NewLogsTranslator(logger.With("component", "logs-translator"), cfg.Logs.CaptureContent)
    }
    ```
  - Extend `startGRPC` to take `logsTranslator` and `sessionStore` and, when `cfg.OTLP.LOGS.Enabled && sessionStore != nil`, register the logs service:
    ```go
    if logsTranslator != nil {
        collogspb.RegisterLogsServiceServer(srv, ingest.NewLogsGRPCHandler(grpcLogger, logsTranslator, sessionStore))
    }
    ```
  - Extend `startOTLPHTTP` to register `/v1/logs` on the same mux when logs are enabled
  - Logs are gated by both `cfg.OTLP.LOGS.Enabled` AND `cfg.Logs.Enabled` (validation already guarantees if the receiver is on the store is on)
- The bbolt file lives at `cfg.Logs.Path` (default `./data/sessions.db`); the FS metrics store still owns `./data/wal/` and `./data/blocks/`, so they coexist

### Out of Scope

- HTTP query API for sessions (Plan 18)
- Frontend (Plan 19)
- Documentation (Plan 20)

## Acceptance Criteria

1. `go build ./...` succeeds
2. `go test ./internal/ingest/...` passes including the four logs handler test cases × 2 transports
3. Starting the server with `LOM_LOGS_ENABLED=true LOM_OTLP_LOGS_ENABLED=true` opens `./data/sessions.db` and logs `listening` for the gRPC + HTTP servers with the logs route enabled
4. Sending a hand-crafted OTLP logs request (one Claude Code `user_prompt` event with `session.id=test`) via `grpcurl` or `curl` results in:
   - The bbolt file gaining a `sessions` bucket entry for `session.id=test`
   - With `LOM_LOGS_CAPTURE_CONTENT=false` (default) the stored event has empty `Body`
5. `make lint` clean
6. Shutting down the server closes the bbolt store cleanly (no goroutine leak)

## Key Snippets

```go
// startGRPC signature extension
func startGRPC(
    logger *slog.Logger, cfg config.OTLPGRPCConfig,
    metricsTranslator *ingest.Translator, metricsStore store.Store,
    logsCfg config.OTLPLogsConfig, logsTranslator *ingest.LogsTranslator, sessionStore sessions.Store,
    errCh chan<- error,
) *grpc.Server {
    // ...existing code that registers MetricsService...
    if logsCfg.Enabled && logsTranslator != nil && sessionStore != nil {
        collogspb.RegisterLogsServiceServer(srv, ingest.NewLogsGRPCHandler(grpcLogger, logsTranslator, sessionStore))
        grpcLogger.Info("logs service registered")
    }
    // ...
}
```
