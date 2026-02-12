# Prometheus HTTP API Reference

This document describes the Prometheus HTTP API that lil-olt-metrics exposes for querying stored metrics.

## General Conventions

- All endpoints are prefixed with `/api/v1`
- All responses are JSON
- Both GET and POST are supported on query/metadata endpoints
- POST parameters are `application/x-www-form-urlencoded`

### Response Envelope

```json
{
  "status": "success" | "error",
  "data": <data>,
  "errorType": "<string>",
  "error": "<string>",
  "warnings": ["<string>"]
}
```

| Field | Present | Description |
|-------|---------|-------------|
| `status` | Always | `"success"` or `"error"` |
| `data` | On success | Shape depends on endpoint |
| `errorType` | On error | `timeout`, `canceled`, `execution`, `bad_data`, `internal`, `unavailable`, `not_found` |
| `error` | On error | Human-readable message |
| `warnings` | Optional | Non-fatal warnings from query execution |

### Timestamp Formats

All endpoints accept timestamps as:
- RFC3339: `2023-01-20T10:00:00Z`
- Unix seconds: `1674208800`
- Unix with fractional: `1674208800.123`

## Query Endpoints

### Instant Query: `GET|POST /api/v1/query`

Evaluates a PromQL expression at a single point in time.

| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| `query` | Yes | string | PromQL expression |
| `time` | No | timestamp | Evaluation time (defaults to now) |
| `timeout` | No | duration | Evaluation timeout (e.g., `5s`, `1m`) |

Response `data`:

```json
{
  "resultType": "vector",
  "result": [
    {
      "metric": {"__name__": "http_requests_total", "method": "GET"},
      "value": [1674208800, "123.45"]
    }
  ]
}
```

### Range Query: `GET|POST /api/v1/query_range`

Evaluates a PromQL expression over a time range.

| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| `query` | Yes | string | PromQL expression |
| `start` | Yes | timestamp | Range start (inclusive) |
| `end` | Yes | timestamp | Range end (inclusive) |
| `step` | Yes | duration/float | Resolution step (e.g., `15s`, `60`) |
| `timeout` | No | duration | Evaluation timeout |

Always returns `resultType: "matrix"`:

```json
{
  "resultType": "matrix",
  "result": [
    {
      "metric": {"__name__": "http_requests_total", "method": "GET"},
      "values": [
        [1674208800, "100"],
        [1674208815, "105"],
        [1674208830, "112"]
      ]
    }
  ]
}
```

## Result Data Types

### Vector (Instant)

Each series has one sample: `"value": [timestamp, "string_value"]`

### Matrix (Range)

Each series has multiple samples: `"values": [[ts, "val"], ...]`

### Scalar

Single numeric value: `"result": [timestamp, "string_value"]`

### String

Single string value: `"result": [timestamp, "string_value"]`

**Note**: Sample values are always JSON strings to preserve precision for `NaN`, `+Inf`, `-Inf`.

## Metadata Endpoints

### Find Series: `GET|POST /api/v1/series`

| Parameter | Required | Description |
|-----------|----------|-------------|
| `match[]` | Yes (1+) | Series selector(s) using label matchers |
| `start` | No | Time range start |
| `end` | No | Time range end |
| `limit` | No | Max series to return |

```json
{
  "data": [
    {"__name__": "up", "job": "prometheus", "instance": "localhost:9090"},
    {"__name__": "up", "job": "node", "instance": "localhost:9100"}
  ]
}
```

### Label Names: `GET|POST /api/v1/labels`

| Parameter | Required | Description |
|-----------|----------|-------------|
| `start` | No | Time range start |
| `end` | No | Time range end |
| `match[]` | No | Series selectors to constrain results |
| `limit` | No | Max label names to return |

```json
{
  "data": ["__name__", "instance", "job", "method"]
}
```

### Label Values: `GET /api/v1/label/<label_name>/values`

| Parameter | Required | Description |
|-----------|----------|-------------|
| `start` | No | Time range start |
| `end` | No | Time range end |
| `match[]` | No | Series selectors to constrain results |
| `limit` | No | Max values to return |

```json
{
  "data": ["node", "prometheus", "grafana"]
}
```

### Metric Metadata: `GET /api/v1/metadata`

| Parameter | Required | Description |
|-----------|----------|-------------|
| `limit` | No | Max metrics to return |
| `limit_per_metric` | No | Max metadata entries per metric |
| `metric` | No | Filter to single metric name |

```json
{
  "data": {
    "http_requests_total": [
      {"type": "counter", "help": "Total HTTP requests.", "unit": ""}
    ],
    "go_goroutines": [
      {"type": "gauge", "help": "Number of goroutines.", "unit": ""}
    ]
  }
}
```

Metric types: `counter`, `gauge`, `histogram`, `summary`, `info`, `stateset`, `unknown`.

### Build Info: `GET /api/v1/status/buildinfo`

```json
{
  "data": {
    "version": "0.1.0",
    "revision": "abc123",
    "branch": "main",
    "buildDate": "2024-01-01T00:00:00Z",
    "goVersion": "go1.23"
  }
}
```

## Metrics Exposition Format

### Prometheus Text Format

Content-Type: `text/plain; version=0.0.4; charset=utf-8`

```
# HELP http_requests_total Total number of HTTP requests.
# TYPE http_requests_total counter
http_requests_total{method="GET",handler="/api/v1/query"} 1234 1674208800000
http_requests_total{method="POST"} 567

# TYPE go_goroutines gauge
go_goroutines 42

# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.005"} 24
http_request_duration_seconds_bucket{le="0.01"} 33
http_request_duration_seconds_bucket{le="+Inf"} 145
http_request_duration_seconds_sum 53.21
http_request_duration_seconds_count 145
```

Format rules:
- `# HELP metric_name description`
- `# TYPE metric_name type` (counter, gauge, histogram, summary, untyped)
- `metric_name{label="value"} sample_value [timestamp_ms]`
- Timestamp is optional (milliseconds since epoch)
- Metric names: `[a-zA-Z_:][a-zA-Z0-9_:]*`
- Label names: `[a-zA-Z_][a-zA-Z0-9_]*`; `__` prefix is reserved

### Histogram Exposition

Counters use `_total` suffix. Histograms produce `_bucket`, `_sum`, `_count`:

```
http_request_duration_seconds_bucket{le="0.005"} 24
http_request_duration_seconds_bucket{le="0.01"} 33
http_request_duration_seconds_bucket{le="+Inf"} 145
http_request_duration_seconds_sum 53.21
http_request_duration_seconds_count 145
```

## PromQL Reference

### Selectors

```promql
# Instant vector
http_requests_total
http_requests_total{job="prometheus", method="GET"}
http_requests_total{job=~"prom.*"}

# Range vector
http_requests_total[5m]

# Offset
http_requests_total offset 5m
```

Duration units: `ms`, `s`, `m`, `h`, `d` (24h), `w` (7d), `y` (365d).

### Label Matchers

| Op | Description |
|----|-------------|
| `=` | Exact equality |
| `!=` | Not equal |
| `=~` | Regex match (RE2, fully anchored) |
| `!~` | Negative regex match |

### Arithmetic Operators

`+`, `-`, `*`, `/`, `%`, `^`

### Comparison Operators

`==`, `!=`, `>`, `<`, `>=`, `<=`

Use `bool` modifier to return 0/1 instead of filtering.

### Logical/Set Operators

`and` (intersection), `or` (union), `unless` (complement)

### Vector Matching

```promql
errors / ignoring(code) requests
errors / on(method) group_left requests
```

### Aggregation Operators

All support `by(labels...)` or `without(labels...)`.

| Operator | Description |
|----------|-------------|
| `sum` | Sum over dimensions |
| `min` | Minimum |
| `max` | Maximum |
| `avg` | Average |
| `count` | Count elements |
| `stddev` | Population standard deviation |
| `stdvar` | Population standard variance |
| `topk` | Largest k elements |
| `bottomk` | Smallest k elements |
| `quantile` | Calculate quantile (0-1) |
| `count_values` | Count elements with same value |
| `group` | All values become 1 |

### Key Functions

**Rate functions** (range vector -> instant vector):

| Function | Description |
|----------|-------------|
| `rate(v[d])` | Per-second average rate of increase (counters) |
| `irate(v[d])` | Instantaneous rate (last two points) |
| `increase(v[d])` | Total increase over range |
| `delta(v[d])` | Difference first to last (gauges) |
| `idelta(v[d])` | Difference last two points (gauges) |
| `deriv(v[d])` | Per-second derivative via linear regression |

**Aggregation over time** (range vector -> instant vector):

`avg_over_time`, `min_over_time`, `max_over_time`, `sum_over_time`, `count_over_time`, `quantile_over_time`, `stddev_over_time`, `last_over_time`, `present_over_time`

**Histogram**:

| Function | Description |
|----------|-------------|
| `histogram_quantile(q, v)` | Calculate quantile from bucket metrics |

**Math**: `abs`, `ceil`, `floor`, `round`, `exp`, `ln`, `log2`, `log10`, `sqrt`, `clamp`, `clamp_min`, `clamp_max`, `sgn`

**Label manipulation**: `label_replace`, `label_join`

**Utility**: `vector`, `scalar`, `sort`, `sort_desc`, `time`, `timestamp`, `absent`, `absent_over_time`, `changes`, `resets`

### Staleness

A series is stale if no sample exists within a 5-minute lookback window.

## Implementation Scope

### Phase 1 (MVP)

- `/api/v1/query` (instant query, basic PromQL)
- `/api/v1/query_range` (range query)
- `/api/v1/series`
- `/api/v1/labels`
- `/api/v1/label/<name>/values`
- `/api/v1/metadata`
- `/api/v1/status/buildinfo`
- Basic PromQL: selectors, `rate`, `sum`, `avg`, `min`, `max`, `count`, `topk`, `bottomk`, arithmetic/comparison operators, `histogram_quantile`

### Phase 2

- Full aggregation operators
- `increase`, `irate`, `delta`, `deriv`
- `*_over_time` functions
- Label manipulation functions
- Vector matching (`on`, `ignoring`, `group_left`, `group_right`)
- Subquery syntax

### Phase 3

- `/api/v1/write` (remote write ingestion)
- `/api/v1/read` (remote read)
- `/metrics` self-exposition endpoint
- `predict_linear`, `absent`, `changes`, `resets`
- Full PromQL compatibility
