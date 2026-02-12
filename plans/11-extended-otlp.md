# Plan 11 — Extended OTLP Features

## Summary

Extend the OTLP ingestion path beyond the MVP to handle the full range of OTLP metric types and features: delta-to-cumulative conversion, ExponentialHistogram translation, Summary passthrough, JSON content type for HTTP, and schema URL processing. After this plan, lil-olt-metrics handles all standard OTLP metric payloads.

## Dependencies

- **Plan 05** — OTLP-to-Prometheus Translator (base to extend)
- **Plan 06** — OTLP Ingestion Handlers (HTTP handler needs JSON support)

## Scope

### In Scope

- Delta-to-cumulative conversion in the translator:
  - Track last-seen cumulative value per series (keyed by metric name + label set)
  - Convert delta Sum and delta Histogram to cumulative by maintaining running totals
  - Handle series resets (value decreases) gracefully
  - Configurable: can be disabled if the source is known to send cumulative only
- ExponentialHistogram translation:
  - Convert OTLP ExponentialHistogram to Prometheus native histogram format (if supported) or to classic histogram buckets
  - Map exponential bucket boundaries to explicit `le` boundaries
- Summary passthrough:
  - Map OTLP Summary quantile values to Prometheus summary metrics (`{quantile="0.5"}`, etc.)
  - Map Summary sum and count to `_sum` and `_count`
- JSON content type for HTTP ingestion:
  - Accept `Content-Type: application/json` on `POST /v1/metrics`
  - Deserialize JSON-encoded `ExportMetricsServiceRequest` using protojson
- Schema URL processing:
  - Record schema URL from ResourceMetrics and ScopeMetrics
  - Store as a label or metadata (depending on approach)
- Unit tests for each feature: delta conversion with resets, ExponentialHistogram bucket mapping, Summary translation, JSON deserialization, schema URL propagation

### Out of Scope

- Full OpenTelemetry Collector compatibility
- Metric aggregation or downsampling at ingestion time
- Custom metric transformations beyond standard OTLP-to-Prometheus mapping

## Acceptance Criteria

1. Delta Sum data points are converted to cumulative values correctly
2. Delta Histogram data points are converted to cumulative bucket counts
3. Series resets (value decrease) in delta conversion are detected and handled without producing negative counters
4. Delta conversion state is cleaned up for series that stop reporting (bounded memory)
5. ExponentialHistogram data points produce valid Prometheus histogram samples with correct bucket boundaries
6. Summary data points produce `{quantile="..."}`, `_sum`, and `_count` samples
7. `Content-Type: application/json` requests to `/v1/metrics` are deserialized and processed correctly
8. Invalid JSON payloads return HTTP 400 with a descriptive error
9. Schema URLs are recorded and accessible (as labels or via metadata endpoint)
10. All tests use `t.Parallel()`, mock the Appender, and cover edge cases (empty histograms, NaN values, missing fields)

## Key Decisions

- **Delta conversion is stateful**: Requires a concurrent-safe map of series → last cumulative value; this state is ephemeral (not persisted) — a restart resets the conversion baseline, which is correct behavior (first delta after restart starts a new cumulative series)
- **ExponentialHistogram to classic buckets**: Converting to classic histogram format ensures compatibility with existing Grafana dashboards and `histogram_quantile()`; native histogram support can be added later
- **Bounded delta state**: Use an LRU or TTL eviction on the delta conversion map to prevent unbounded memory growth from abandoned series
- **protojson for JSON deserialization**: The official protobuf JSON serialization format; handles all proto3 JSON encoding rules correctly
- **Schema URL as label**: Storing as a label (`__schema_url__`) makes it queryable via PromQL; hidden labels (prefixed with `__`) are stripped from query results by convention
