# ✅ COMPLETE Plan 16 — OTLP Logs Translator

## Summary

Add a translator that converts an OTLP `ExportLogsServiceRequest` into a stream of `sessions.Event` records, applying the privacy gate (content-capture off by default) and pulling structured fields (session.id, model, tool name, cwd, user id) from resource and log-record attributes. Parallels the existing metrics translator in `internal/ingest/translator.go` but emits `sessions.Event` instead of Prometheus samples.

## Dependencies

- **Plan 14** (logs config) — uses `LogsConfig.CaptureContent`
- **Plan 15** (sessions store) — uses `sessions.Event` type

## Scope

### In Scope

- `internal/ingest/logs_translator.go`:
  - `LogsTranslator` struct with `logger`, `captureContent bool` fields
  - `NewLogsTranslator(logger *slog.Logger, captureContent bool) *LogsTranslator`
  - `func (t *LogsTranslator) Translate(req *collogspb.ExportLogsServiceRequest) ([]sessions.Event, error)` — walks `ResourceLogs → ScopeLogs → LogRecord`, returns the event slice plus a (possibly nil) error describing partial failures
  - Per record:
    * Read resource attributes once per ResourceLogs; map `session.id`, `user.id`, `host.name`, `os.type` to event fields
    * Read log record attributes; extract `event.name` (Claude Code event types like `claude_code.user_prompt`, `claude_code.tool_result`, `claude_code.api_request`, `claude_code.tool_decision`); store `tool_name`, `model`, `cwd`, `decision`, `language` into `evt.Attrs` (string-coerced)
    * If `event.name` falls back from a record attribute, also accept the log record's `Body` field containing a JSON object with `event_name` key (Claude Code variant)
    * `evt.Timestamp` = `time.Unix(0, int64(rec.TimeUnixNano))` if non-zero; otherwise `time.Unix(0, int64(rec.ObservedTimeUnixNano))`; otherwise `time.Now()`
    * Drop the record (no error) if `session.id` is missing; log at debug
    * `evt.Body`: if `captureContent` is false, leave empty; otherwise marshal `rec.Body` (an `AnyValue`) using `attrToString` helper
  - Helper: `attrToString(v *commonpb.AnyValue) string` — handles `StringValue`, `IntValue`, `DoubleValue`, `BoolValue`, `ArrayValue` (JSON-encoded), `KvlistValue` (JSON-encoded), `BytesValue` (base64). Return `""` for nil.
  - Helper: `attrMapToStrings(attrs []*commonpb.KeyValue) map[string]string` — produces the flat `Attrs` map used by `sessions.Event`
- `internal/ingest/logs_translator_test.go`:
  - Table-driven cases constructed by hand (no fixture files): build `*collogspb.ExportLogsServiceRequest` directly in Go
  - Cases:
    * `user_prompt` event with `captureContent=false` → body empty, name/session/timestamp populated
    * `user_prompt` event with `captureContent=true` → body equals the AnyValue string content
    * `tool_result` event → `tool_name` attribute promoted to `evt.ToolName`
    * `api_request` event → `model` attribute promoted to `evt.Model`
    * Resource missing `session.id` → record dropped, no error, no event
    * Multiple ResourceLogs / ScopeLogs / LogRecords in one request → all events returned in order
    * Record with `ObservedTimeUnixNano` only → that timestamp is used
- Update `internal/ingest/interfaces.go`:
  - Add `logsTranslator` interface mirroring the metrics-translator pattern:
    ```go
    type logsTranslator interface {
        Translate(req *collogspb.ExportLogsServiceRequest) ([]sessions.Event, error)
    }
    type sessionsStore interface {
        AppendEvent(ctx context.Context, evt sessions.Event) error
    }
    ```
  - Extend the `//go:generate moq` directive to include the new interfaces
  - Regenerate `moq_test.go`

### Out of Scope

- gRPC/HTTP handlers (those land in Plan 17)
- Any wiring in `main.go`
- Span/trace correlation (events only carry attributes, not span context here)

## Acceptance Criteria

1. `go build ./...` succeeds
2. `go test ./internal/ingest/...` passes; new cases all use `t.Parallel()`
3. `make generate` regenerates `moq_test.go` cleanly (no diff on re-run)
4. `make lint` clean
5. Coverage for all seven test cases above

## Key Snippets

```go
package ingest

import (
    collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
    commonpb "go.opentelemetry.io/proto/otlp/common/v1"
    logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

type LogsTranslator struct {
    logger         *slog.Logger
    captureContent bool
}

func (t *LogsTranslator) Translate(req *collogspb.ExportLogsServiceRequest) ([]sessions.Event, error) {
    var events []sessions.Event
    for _, rl := range req.GetResourceLogs() {
        resAttrs := attrMapToStrings(rl.GetResource().GetAttributes())
        sessionID, ok := resAttrs["session.id"]
        if !ok || sessionID == "" {
            // also check log-record-level attrs below before dropping
        }
        for _, sl := range rl.GetScopeLogs() {
            for _, rec := range sl.GetLogRecords() {
                evt, ok := t.recordToEvent(rec, resAttrs, sessionID)
                if !ok { continue }
                events = append(events, evt)
            }
        }
    }
    return events, nil
}
```
