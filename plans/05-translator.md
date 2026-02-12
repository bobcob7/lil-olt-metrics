# Plan 05 — OTLP-to-Prometheus Translator

## Summary

Implement the translation layer that converts OTLP metric data points into Prometheus-compatible samples and labels. This is the bridge between the ingestion handlers (Plan 06) and the Store (Plan 04). It handles metric type mapping, label construction from resource/scope/data-point attributes, metric name sanitization, and the semantic differences between OTLP and Prometheus data models.

## Dependencies

- **Plan 03** — Core types (`labels.Labels`, `Sample`) and `Appender` interface

## Scope

### In Scope

- Translator in `internal/ingest/translator.go` that accepts OTLP `ExportMetricsServiceRequest` and writes to an `Appender`
- OTLP-to-Prometheus type mapping per `docs/architecture.md`:
  - Gauge → Prometheus gauge
  - Sum (monotonic, cumulative) → Prometheus counter (with `_total` suffix)
  - Sum (non-monotonic) → Prometheus gauge
  - Histogram (cumulative) → Prometheus histogram (`_bucket`, `_count`, `_sum`)
- Label construction:
  - Resource attributes → target labels (configurable prefix/mapping per `docs/config.md`)
  - Scope name/version → `otel_scope_name`, `otel_scope_version`
  - Data point attributes → metric labels
  - `__name__` label from metric name (sanitized)
- Metric name sanitization: replace non-alphanumeric/underscore characters, collapse consecutive underscores, lowercase
- Unit suffix handling: append unit to metric name when present (e.g., `http_request_duration_seconds`)
- Exemplar extraction: convert OTLP exemplars to Prometheus exemplars (trace ID, span ID, value, timestamp)
- Constructor accepting translation config (resource attribute mapping settings) and `*slog.Logger`
- Unit tests covering: each metric type mapping, label construction, name sanitization, unit suffixes, exemplar extraction, empty/nil input handling

### Out of Scope

- Delta-to-cumulative conversion (Plan 11)
- ExponentialHistogram translation (Plan 11)
- Summary passthrough (Plan 11)
- gRPC/HTTP transport handling (Plan 06)
- Schema URL processing (Plan 11)

## Acceptance Criteria

1. OTLP Gauge data points produce Prometheus gauge samples with correct labels and values
2. OTLP monotonic cumulative Sum produces counter samples with `_total` suffix
3. OTLP non-monotonic Sum produces gauge samples without suffix
4. OTLP cumulative Histogram produces `_bucket` (with `le` label), `_count`, and `_sum` samples
5. Resource attributes appear as labels with configurable mapping
6. Scope name and version appear as `otel_scope_name` and `otel_scope_version` labels
7. Metric names are sanitized to valid Prometheus metric names
8. Unit suffixes are appended when present and not already in the metric name
9. Exemplars are extracted with trace/span IDs preserved
10. Nil or empty requests produce zero samples without errors
11. All tests use `t.Parallel()` and mock the `Appender` via moq

## Key Decisions

- **Translator writes directly to Appender**: Avoids an intermediate representation; samples flow straight from OTLP decoding to the store's write interface
- **Configurable resource attribute mapping**: Some deployments want `service.name` as `job`, others want it as `service_name`; make this configurable from the start per `docs/config.md`
- **Sanitization follows Prometheus conventions**: Use the same rules as the Prometheus OTLP receiver to ensure metric names are compatible with PromQL
- **No delta-to-cumulative in MVP**: Delta temporality requires stateful tracking of last-seen values per series, which is complex; defer to Plan 11
- **Exemplars are best-effort**: If exemplar data is malformed, log a warning and skip rather than failing the entire batch
