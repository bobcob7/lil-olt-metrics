# ✅ COMPLETE — Plan 14 — Logs Config & OTLP.LOGS Toggle

## Summary

Extend `internal/config` with (a) an `OTLP.LOGS` block toggling the logs receiver on the existing gRPC/HTTP listeners and (b) a top-level `Logs` block holding sessions-store settings (path, retention, max events per session, content-capture flag). After this plan, the existing build still passes and the config struct can carry every setting the later plans need — no behavior change yet.

## Dependencies

- **Plan 02** (configuration) — extends the existing config package

## Scope

### In Scope

- `internal/config/config.go`:
  - Add `OTLPLogsConfig` (`Enabled bool`) and embed as `OTLPConfig.LOGS`
  - Add top-level `LogsConfig` with fields: `Enabled bool`, `Path string`, `Retention Duration`, `MaxEventsPerSession int`, `CaptureContent bool`
  - Add `Logs LogsConfig` to `Config`
  - Defaults: `OTLP.LOGS.Enabled=false`, `Logs.Enabled=false`, `Logs.Path="./data/sessions.db"`, `Logs.Retention=24h`, `Logs.MaxEventsPerSession=500`, `Logs.CaptureContent=false`
  - Validation: if `OTLP.LOGS.Enabled=true` then `Logs.Enabled` must also be `true` (the receiver needs a store); `Logs.MaxEventsPerSession > 0`; `Logs.Retention >= 0`; `Logs.Path` non-empty when `Logs.Enabled`
- `internal/config/config_test.go` table additions:
  - Defaults case asserts the new fields
  - Validation case for OTLP.LOGS enabled but Logs disabled returns a clear error
  - Env var override `LOM_LOGS_PATH=/tmp/x.db` applies via existing reflection walker
- Update `docs/config-reference.md` with the new fields, defaults, and env var names (`LOM_LOGS_*`, `LOM_OTLP_LOGS_ENABLED`)

### Out of Scope

- Wiring receivers, opening the bbolt file, anything that *uses* the config

## Acceptance Criteria

1. `go build ./...` succeeds
2. `go test ./internal/config/...` passes including the new cases
3. `make lint` clean
4. Loading default config yields `cfg.OTLP.LOGS.Enabled == false` and `cfg.Logs.Path == "./data/sessions.db"`
5. Loading a YAML with `otlp.logs.enabled: true` and no `logs.enabled: true` fails validation with a message naming both fields
6. Setting `LOM_LOGS_RETENTION=2h` overrides the default to `2h`

## Key Snippets

```go
// OTLPConfig gains:
type OTLPConfig struct {
    GRPC OTLPGRPCConfig `yaml:"grpc"`
    HTTP OTLPHTTPConfig `yaml:"http"`
    LOGS OTLPLogsConfig `yaml:"logs"`
}

type OTLPLogsConfig struct {
    Enabled bool `yaml:"enabled"`
}

// New top-level block:
type LogsConfig struct {
    Enabled             bool     `yaml:"enabled"`
    Path                string   `yaml:"path"`
    Retention           Duration `yaml:"retention"`
    MaxEventsPerSession int      `yaml:"max_events_per_session"`
    CaptureContent      bool     `yaml:"capture_content"`
}
```
