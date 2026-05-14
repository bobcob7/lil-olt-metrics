# Plan 20 — Logs Integration Tests & Documentation

## Summary

End-to-end integration test that exercises OTLP logs over both transports into the live sessions API, plus README and `docs/` updates explaining how to enable the feature and how to point Claude Code at the server. After this plan the feature is shippable.

## Dependencies

- **All of plans 14–19**

## Scope

### In Scope

- `internal/integration/logs_e2e_test.go`:
  - One test per transport (`TestLogs_gRPC_EndToEnd`, `TestLogs_HTTP_EndToEnd`), both `t.Parallel()`
  - Per test:
    1. Build a `*config.Config` with logs enabled, `CaptureContent=false`, bbolt path under `t.TempDir()`
    2. Start the full server in-process on ephemeral ports (use `:0` and read back the actual port — follow the pattern in the existing `internal/integration/*_test.go`)
    3. Build an OTLP `ExportLogsServiceRequest` containing two Claude Code events for `session.id=integ-1`:
       - `claude_code.user_prompt` with body `"how do I X"`
       - `claude_code.tool_result` with attr `tool_name=Read`
    4. Send via the corresponding transport
    5. `GET /api/v1/sessions?since=1m` → expect one session with `id=integ-1`, `last_event_name=claude_code.tool_result`, `last_tool_name=Read`, `event_count=2`
    6. `GET /api/v1/sessions/integ-1/events?limit=10` → expect two events in chronological order, both with empty `body` (CaptureContent=false)
    7. Repeat step 3 with `CaptureContent=true` config in a separate sub-test → body is populated
  - Use `require` for preconditions (server start, request send), `assert` for response checks
- `internal/integration/logs_e2e_test.go` also covers:
  - Sessions endpoint returns 404 when `Logs.Enabled=false` (separate test that starts the server with logs off)
  - Retention TTL: append an event, advance time using the existing test clock pattern (or set a tiny retention like `100ms`, sleep, then assert session is gone). If the project doesn't have a test clock yet, use the tiny-retention approach
- `README.md` updates:
  - New "Live Sessions" section under Features explaining what it does and screenshotting (or text-describing) the dashboard panel
  - "Enabling logs ingestion" subsection with the YAML snippet:
    ```yaml
    otlp:
      logs:
        enabled: true
    logs:
      enabled: true
      path: ./data/sessions.db
      retention: 24h
      max_events_per_session: 500
      capture_content: false
    ```
  - Env var equivalents
- `docs/config-reference.md`:
  - Document every new field with type, default, env var
  - Note the validation rule (OTLP.LOGS requires Logs.Enabled)
- New `docs/claude-code-logs.md`:
  - How to point Claude Code at this server using OTel env vars:
    ```bash
    export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
    export OTEL_LOGS_EXPORTER=otlp
    export OTEL_LOG_USER_PROMPTS=1   # opt-in to content capture on the client
    export OTEL_METRICS_EXPORTER=otlp
    ```
  - Link from README's Quick Start
  - Privacy note: `OTEL_LOG_USER_PROMPTS=1` AND `logs.capture_content=true` are both required to store prompt content; either one off means the server discards body content
- `CHANGELOG.md`:
  - New entry under the next version: "Added OTLP logs ingestion and Live Sessions dashboard panel"
- `.context.md` (root) update:
  - Add `internal/sessions/` to the Directories table
  - Add "OTLP logs ingestion with per-session storage (bbolt)" to Functionality

### Out of Scope

- Performance benchmarks
- Authentication on the sessions API (still no auth on any endpoint — out of scope per original spec)

## Acceptance Criteria

1. `go test ./internal/integration/... -race` passes including the new tests
2. `make test` passes for the whole repo
3. `make lint` clean
4. `cd web && yarn build && yarn test` passes (dashboard now embeds the new components into `dashboard/dist`)
5. README quick start covers the new feature in under 30 lines
6. `docs/config-reference.md` includes every new field
7. `docs/claude-code-logs.md` exists and is linked from README
8. Manual smoke test: with Claude Code pointed at the server using the documented env vars, the dashboard shows live sessions while a Claude Code conversation is in progress

## Key Snippets

```go
// internal/integration/logs_e2e_test.go
func TestLogs_gRPC_EndToEnd(t *testing.T) {
    t.Parallel()
    ctx := t.Context()
    cfg := newTestConfig(t)
    cfg.OTLP.LOGS.Enabled = true
    cfg.Logs.Enabled = true
    cfg.Logs.Path = filepath.Join(t.TempDir(), "sessions.db")
    cfg.Logs.Retention = config.Duration(time.Hour)
    cfg.Logs.MaxEventsPerSession = 100

    srv := startTestServer(t, cfg)
    defer srv.Stop()

    sendLogsGRPC(t, srv.GRPCAddr(), buildClaudeCodeLogs("integ-1"))

    require.Eventually(t, func() bool {
        sessions := getJSON[sessionsResp](t, srv.QueryAddr()+"/api/v1/sessions?since=1m")
        return len(sessions.Data) == 1 && sessions.Data[0].ID == "integ-1"
    }, 2*time.Second, 50*time.Millisecond)
    // ...
}
```
