# OTLP Metrics Standard Reference

This document describes the OpenTelemetry Protocol (OTLP) v1 metrics specification that lil-olt-metrics implements for ingestion.

## Proto Package Structure

| Package | Proto File | Purpose |
|---------|-----------|---------|
| `collector.metrics.v1` | `collector/metrics/v1/metrics_service.proto` | gRPC service definition, Export RPC |
| `metrics.v1` | `metrics/v1/metrics.proto` | Metric data model (all types, data points) |
| `resource.v1` | `resource/v1/resource.proto` | Resource message (service.name, etc.) |
| `common.v1` | `common/v1/common.proto` | Shared types: KeyValue, AnyValue, InstrumentationScope |

All packages are under the `opentelemetry.proto` namespace.

## Transport: gRPC

### Service Definition

```
/opentelemetry.proto.collector.metrics.v1.MetricsService/Export
```

- **Default port**: 4317
- **Protocol**: HTTP/2 (standard gRPC)
- **Compression**: `gzip` via `grpc-encoding` header

### Request/Response

**Request**: `ExportMetricsServiceRequest` containing `repeated ResourceMetrics`.

**Response**: `ExportMetricsServiceResponse` with optional `ExportMetricsPartialSuccess`:

```protobuf
message ExportMetricsPartialSuccess {
  int64 rejected_data_points = 1;
  string error_message = 2;
}
```

## Transport: HTTP

### Endpoint

```
POST /v1/metrics
```

- **Default port**: 4318

### Content Types

| Content-Type | Encoding |
|-------------|----------|
| `application/x-protobuf` | Binary protobuf (preferred) |
| `application/json` | Proto3 canonical JSON (camelCase fields, int64 as strings) |

### Headers

| Header | Required | Description |
|--------|----------|-------------|
| `Content-Type` | Yes | `application/x-protobuf` or `application/json` |
| `Content-Encoding` | No | `gzip` if body is compressed |

### HTTP Status Codes

| Code | Meaning | Retry |
|------|---------|-------|
| 200 | Success (full or partial) | No |
| 400 | Bad request | No |
| 401/403 | Auth failure | No |
| 408 | Timeout | Yes |
| 429 | Throttled | Yes (respect `Retry-After`) |
| 502/503/504 | Server error | Yes |

## Data Hierarchy

```
ExportMetricsServiceRequest
  └── repeated ResourceMetrics
        ├── Resource (attributes: service.name, etc.)
        ├── schema_url
        └── repeated ScopeMetrics
              ├── InstrumentationScope (name, version)
              ├── schema_url
              └── repeated Metric
```

### Resource

Identifies the entity producing metrics.

Key attributes (semantic conventions):

| Attribute | Example |
|-----------|---------|
| `service.name` | `"my-api"` |
| `service.version` | `"1.2.3"` |
| `service.namespace` | `"production"` |
| `service.instance.id` | `"host1:8080"` |
| `host.name` | `"server01"` |

### InstrumentationScope

Identifies the instrumentation library, not the service itself.

```protobuf
message InstrumentationScope {
  string name = 1;    // e.g., "go.opentelemetry.io/contrib/instrumentation/net/http"
  string version = 2;
  repeated KeyValue attributes = 3;
}
```

### Common Types

```protobuf
message KeyValue {
  string key = 1;
  AnyValue value = 2;
}

message AnyValue {
  oneof value {
    string string_value = 1;
    bool bool_value = 2;
    int64 int_value = 3;
    double double_value = 4;
    ArrayValue array_value = 5;
    KeyValueList kvlist_value = 6;
    bytes bytes_value = 7;
  }
}
```

## Metric Data Model

### Top-Level Metric

```protobuf
message Metric {
  string name = 1;        // e.g., "http.server.request.duration"
  string description = 2;
  string unit = 3;        // UCUM unit string: "s", "By", "{request}"

  oneof data {
    Gauge gauge = 5;
    Sum sum = 7;
    Histogram histogram = 9;
    ExponentialHistogram exponential_histogram = 10;
    Summary summary = 11;
  }
}
```

### Aggregation Temporality

```protobuf
enum AggregationTemporality {
  AGGREGATION_TEMPORALITY_UNSPECIFIED = 0;
  AGGREGATION_TEMPORALITY_DELTA = 1;       // Change since last report
  AGGREGATION_TEMPORALITY_CUMULATIVE = 2;  // Cumulative since start
}
```

### Timestamps

All timestamps are `fixed64` nanoseconds since Unix epoch:
- `time_unix_nano`: When the value was observed
- `start_time_unix_nano`: Start of observation window (required for Sum, Histogram)

## Metric Types

### Gauge

Instantaneous value that can go up or down. No aggregation temporality.

```protobuf
message Gauge {
  repeated NumberDataPoint data_points = 1;
}
```

Use cases: CPU utilization, queue depth, temperature.

### Sum

Cumulative or delta sum with monotonicity.

```protobuf
message Sum {
  repeated NumberDataPoint data_points = 1;
  AggregationTemporality aggregation_temporality = 2;
  bool is_monotonic = 3;
}
```

- `is_monotonic = true`: Counter (only increases, resets on restart)
- `is_monotonic = false`: UpDownCounter (increases and decreases)

### Histogram

Explicit-boundary histogram with pre-defined bucket boundaries.

```protobuf
message Histogram {
  repeated HistogramDataPoint data_points = 1;
  AggregationTemporality aggregation_temporality = 2;
}
```

Bucket layout for `N` explicit_bounds `[b0, b1, ..., bN-1]` produces `N+1` bucket_counts:
- Bucket 0: `(-inf, b0]`
- Bucket i: `(b[i-1], b[i]]`
- Bucket N: `(b[N-1], +inf)`

### ExponentialHistogram

Auto-scaling histogram using base-2 exponential bucket boundaries.

```protobuf
message ExponentialHistogram {
  repeated ExponentialHistogramDataPoint data_points = 1;
  AggregationTemporality aggregation_temporality = 2;
}
```

Key fields: `scale` (bucket resolution), `positive`/`negative` buckets, `zero_count`, `zero_threshold`.

### Summary (Legacy)

Pre-calculated quantiles. Not recommended for new implementations.

```protobuf
message Summary {
  repeated SummaryDataPoint data_points = 1;
}
```

## Data Point Types

### NumberDataPoint (Gauge, Sum)

```protobuf
message NumberDataPoint {
  repeated KeyValue attributes = 7;
  fixed64 start_time_unix_nano = 2;
  fixed64 time_unix_nano = 3;
  oneof value {
    double as_double = 4;
    sfixed64 as_int = 6;
  }
  repeated Exemplar exemplars = 5;
  uint32 flags = 8;
}
```

### HistogramDataPoint

```protobuf
message HistogramDataPoint {
  repeated KeyValue attributes = 9;
  fixed64 start_time_unix_nano = 2;
  fixed64 time_unix_nano = 3;
  fixed64 count = 4;
  optional double sum = 5;
  repeated fixed64 bucket_counts = 6;
  repeated double explicit_bounds = 7;
  repeated Exemplar exemplars = 8;
  uint32 flags = 10;
  optional double min = 11;
  optional double max = 12;
}
```

### ExponentialHistogramDataPoint

```protobuf
message ExponentialHistogramDataPoint {
  repeated KeyValue attributes = 1;
  fixed64 start_time_unix_nano = 2;
  fixed64 time_unix_nano = 3;
  fixed64 count = 4;
  optional double sum = 5;
  sint32 scale = 6;
  fixed64 zero_count = 7;
  Buckets positive = 8;
  Buckets negative = 9;
  uint32 flags = 10;
  repeated Exemplar exemplars = 11;
  optional double min = 12;
  optional double max = 13;
  double zero_threshold = 14;

  message Buckets {
    sint32 offset = 1;
    repeated uint64 bucket_counts = 2;
  }
}
```

### SummaryDataPoint

```protobuf
message SummaryDataPoint {
  repeated KeyValue attributes = 7;
  fixed64 start_time_unix_nano = 2;
  fixed64 time_unix_nano = 3;
  fixed64 count = 4;
  double sum = 5;
  repeated ValueAtQuantile quantile_values = 6;

  message ValueAtQuantile {
    double quantile = 1;  // 0.0 to 1.0
    double value = 2;
  }
}
```

### Exemplar

Links metric data points to traces.

```protobuf
message Exemplar {
  repeated KeyValue filtered_attributes = 7;
  fixed64 time_unix_nano = 2;
  oneof value {
    double as_double = 3;
    sfixed64 as_int = 6;
  }
  bytes span_id = 4;   // 8 bytes
  bytes trace_id = 5;  // 16 bytes
}
```

### Data Point Flags

Bit 0 (`flags & 1 == 1`): No recorded value — used for gap/staleness signals.

## Metric Type Summary

| Type | Data Point | Temporality | Monotonicity | Use Case |
|------|-----------|-------------|-------------|----------|
| Gauge | NumberDataPoint | No | No | CPU%, queue depth |
| Sum (monotonic) | NumberDataPoint | Yes | Yes | Total requests |
| Sum (non-monotonic) | NumberDataPoint | Yes | No | Active connections |
| Histogram | HistogramDataPoint | Yes | No | Latency distributions |
| ExponentialHistogram | ExponentialHistogramDataPoint | Yes | No | Auto-scaling distributions |
| Summary | SummaryDataPoint | No | No | Pre-computed quantiles (legacy) |

## Implementation Scope

### Phase 1 (MVP)

- OTLP/HTTP with `application/x-protobuf` on `/v1/metrics`
- OTLP/gRPC on port 4317
- Metric types: Gauge, Sum (monotonic cumulative), Histogram
- Resource and Scope extraction
- gzip decompression

### Phase 2

- OTLP/HTTP JSON (`application/json`)
- Sum (delta, non-monotonic) with delta-to-cumulative conversion
- ExponentialHistogram (convert to explicit buckets for Prometheus)
- Partial success responses
- Exemplar storage

### Phase 3

- Summary passthrough
- Schema URL handling
- Full retry/throttling semantics (429 with `Retry-After`)

## JSON Wire Format Example

```json
{
  "resourceMetrics": [{
    "resource": {
      "attributes": [
        {"key": "service.name", "value": {"stringValue": "my-service"}}
      ]
    },
    "scopeMetrics": [{
      "scope": {"name": "my-library", "version": "0.1.0"},
      "metrics": [{
        "name": "http.server.request.duration",
        "unit": "s",
        "histogram": {
          "dataPoints": [{
            "attributes": [
              {"key": "http.request.method", "value": {"stringValue": "GET"}}
            ],
            "startTimeUnixNano": "1694000000000000000",
            "timeUnixNano": "1694000060000000000",
            "count": "150",
            "sum": 12.5,
            "bucketCounts": ["0", "20", "80", "40", "8", "2", "0"],
            "explicitBounds": [0.005, 0.01, 0.025, 0.05, 0.1, 0.25]
          }],
          "aggregationTemporality": 2
        }
      }]
    }]
  }]
}
```

Note: `int64`/`uint64`/`fixed64` fields are JSON strings per proto3 convention.
