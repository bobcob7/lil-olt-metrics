# Architecture

lil-olt-metrics is a single-process metrics server that ingests OTLP metrics and serves them via the Prometheus HTTP API. It is designed to be minimal at rest, suitable for edge deployments, dev environments, and small-scale production use.

## Design Goals

- **Single binary, single process**: No external dependencies required
- **Minimal at rest**: Low memory and CPU when idle
- **OTLP-native ingestion**: gRPC and HTTP, protobuf and JSON
- **Prometheus-compatible queries**: Standard API that works with Grafana out of the box
- **Pluggable storage**: Built-in FS engine or forward to external Prometheus/VictoriaMetrics
- **Zero-config startup**: Sensible defaults for all settings

## System Architecture

![System Architecture](https://www.plantuml.com/plantuml/png/~1UDfbKzjk4p4GVT-l6F84GccBG88YX1e2Y4X1R9D07WWFxdPsruflsBDHmqNYgHzGyONz4iRURRC9JnxFvSoPu_5P8E6fqIfxG3Lg1AU4D5bOI-E45neAgvqrQ6XEMuNWiI1XAeLPHfBOeQGQMaL3ZUHb4U3spr-ORmEXXf4lQabSq7XETw9OYmfXC34L1fBM3E1d1l3rqsAQGpNBnolpyjdJ9y-_WGXGULVS1pyi5daRV962DP7BVlSW5rwOeCG1wEtrJUuj4wkn2IipjcQ3dj0d3EfAhDz3chE3ZHP56iukHT7v4IPtZa64a24wOxVtStXdAcbmPHukHrDu36-KBTRXKMfLXfZJfp93RGbXThnC2Ov3CUMBB1XHYe-jbl_bRVGUr_C6_RQRTuUUr89HFc75JazEJrAbSF8kwtpQSRrl_Lr5sAWu38Ul-yf4eGzzEpD5EX6ozeHNluwEui3zQnglx-0vCcQKwZi2xR67QEBeNbOrWItx301c66pZ-Lq4iL8N3I4ltlKDA761AghTAWX6y4O7OIJ97saxEFIPRyUqTHneob9UWHREILDnw2qg4empRdGJjsc3lFvnQK8iHNC9aTCXi_IyIy-RFXGrYajGDeAb3KZVHnHTwnaIVoIs0KrfcR_cwmxtZ2t-YVy0B4uwHW00)

The system has four major subsystems:

1. **Ingestion** — OTLP gRPC (`:4317`) and HTTP (`:4318`) receivers
2. **Translation** — Converts OTLP metric model to Prometheus time series model
3. **Storage** — Pluggable backend (FS engine, Prometheus remote, VictoriaMetrics remote)
4. **Query** — Prometheus HTTP API with PromQL engine

## Data Flow

![Data Flow](https://www.plantuml.com/plantuml/png/~1UDfzL4rJsp0GlVjNR3d5Cp2P7dfX9YqCIMbBQ4Bi3WV2C8gzE6fboKZhD73ggJ-Wqr_8B-bApeS3XPibxNlxzkskZnm9ItMfeZSqnnAXKaBgY2GfXBuW0H-L-GcFl_x2A1sEuRCkq94q6iZ0s9eI6LOxkAZHtaUHoCXOs1kbg23fdqrr5qwCKkYlzq0uo9H4JL75IMKcAw79Hm_7Kpt13EK2cp2xEkm6fbPefyIQYpR7tR24Rp14EF-r--WrNmodUg0BgPie39zhxQTMt0ejWlGWDiGKpMqKzNej3tgzA6egWeFt6z5m28DbPIoT8rcPkGJjGcOumRkQSLFTAIRZ4-1Sdz9q7FkySY48jqLlY7P9-vYPihBe77IA1rd5BmSwj58e-S3zj8RChBxjVab6NtS0M_iOSdKzGMTgc-5AT9JZSs5RhItlMpnFMGTBFcR410qt26BoMKteflhnznzGOeQAo_bkPliWDG-ZpZ1-ZJ8HMf8lh0mwGOiItMk0Sr4reqltrJfjQt33xMluFYeI33gfovhls8UfpencEvmVWOwy1OsOO_vKTXW-9hYqah1pTPtA4daK9BguRMxBoQszpU0s32sguwf2dVlLkpmUFd_-LkKUF4UHmqoPx0VFcjG79PR6thEzEIxZDUffcQEpjgWcln_siDXXq5Shu1Cs6yEeqq4ANL79xk9jzyv7tZ1dmGvz2AUUPRBEdAYwiNXtzQwyRetxQkrMN6tMiR0EeTdKrlkwp0Jz3wLJ2ihct8Lz87PsulF6BpZO9czQa201jyK6tvyL_XzDWuLGTP2z32l8lMTtVNtx88h2OY7yBo7UO1cyTkmG_FBvBQYCTkoErsxbqj_KYdR6OUNA4VUP_yh_05f1_u80)

### Ingestion Path

1. OTel SDK or Collector sends `ExportMetricsServiceRequest` via gRPC or HTTP
2. Receiver decompresses (gzip) and deserializes (protobuf or JSON)
3. Translator extracts resource attributes into labels (`service.name` -> `job`)
4. Translator sanitizes metric names for Prometheus compatibility
5. Translator maps OTLP types to Prometheus types (Sum monotonic -> counter, etc.)
6. Translator performs delta-to-cumulative conversion when needed
7. Translated time series are written to the storage backend

### Query Path

1. Grafana or client sends PromQL query to `/api/v1/query` or `/api/v1/query_range`
2. PromQL engine parses the expression
3. Engine requests matching series from storage via `Select(matchers, timeRange)`
4. Storage returns a `SeriesSet` iterator
5. Engine evaluates the expression (rate, aggregation, etc.)
6. JSON response is returned in Prometheus API format

## OTLP to Prometheus Translation

The translator converts between the OTLP and Prometheus data models:

| OTLP Type | Prometheus Type | Notes |
|-----------|----------------|-------|
| Gauge | gauge | Direct mapping |
| Sum (monotonic, cumulative) | counter | `_total` suffix added |
| Sum (non-monotonic) | gauge | No suffix |
| Sum (monotonic, delta) | counter | Delta-to-cumulative conversion required |
| Histogram | histogram | Explicit bounds -> `le` buckets, `_bucket`/`_sum`/`_count` |
| ExponentialHistogram | histogram | Convert to explicit bounds first |
| Summary | summary | Quantile values -> `quantile` label |

### Resource Attribute Mapping

By default:
- `service.name` -> `job` label
- `service.instance.id` -> `instance` label

Additional resource attributes can be promoted to labels via config.

### Metric Name Sanitization

OTLP metric names (e.g., `http.server.request.duration`) are converted to Prometheus-compatible names:
- Dots replaced with underscores: `http_server_request_duration`
- Unit suffix appended: `http_server_request_duration_seconds`
- Type suffix appended for counters: `http_server_requests_total`

## Storage Architecture

### Store Interface

All storage backends implement a common interface:

```go
type Store interface {
    // Write appends time series samples
    Write(ctx context.Context, series []TimeSeries) error

    // Select returns series matching the given label matchers within the time range
    Select(ctx context.Context, matchers []*LabelMatcher, mint, maxt int64) (SeriesSet, error)

    // LabelNames returns all label names within the time range
    LabelNames(ctx context.Context, matchers []*LabelMatcher, mint, maxt int64) ([]string, error)

    // LabelValues returns values for a label name within the time range
    LabelValues(ctx context.Context, name string, matchers []*LabelMatcher, mint, maxt int64) ([]string, error)

    // Close flushes pending data and releases resources
    Close() error
}
```

### FS Storage Engine

![FS Storage Layout](https://www.plantuml.com/plantuml/png/~1UDfzayzEma0GnkzzYf4J929GF7a2ZSO39o5HC1wMTg0h-wTff_o9CV4XV4AVnDbjHIM5dhRVjpk_sI-pWvn4HeLMmHabg15I9QG9I992k1l3c6mcbWYtPYaDmjV79rmh6wrW97Qse218HRHo6ngn8D5fm5i0iDSsGbKIm7EEIus6sbttzLv0v93tQgomjC8QgrzQVRRUWx-WkUkftUtTFqrdq0oJrfugJOnRVqp6smeN9OLPvHslHaabQYHnyPfRusNTEy6OdMA4TOombX4UPqoxNMPS9X1PpIvXV0JNhyFrQt5bR0mqbX0okKm8xAACSnYHN2Cy3aTG9Ux4D4KJjwrHEzXaaZ1dyG5vH7OOWpLG3ayQy2vmHGzB-nJBUN6zk16vOgKD97dKCc7upaqjpUGSjDXEMjupXPvZvZl3J7hcFr7eL5Lg5OxaLXIkKHqsChT4LhjJLL8tFsd28cD35ueaLsCdJC27pw4Qik2Yz_-_9GUyueVo3M8m-qC0)

The built-in FS engine uses a design inspired by Prometheus TSDB:

**Write-Ahead Log (WAL)**:
- Append-only log segments for crash recovery
- New samples are written to WAL before being indexed in memory
- WAL is replayed on startup to recover in-flight data
- Segments are rotated at a configurable size (default 128MB)

**Head Block**:
- In-memory block holding the most recent data (last 2 hours by default)
- Inverted index for fast label-based lookups
- Samples stored in compressed chunks

**Persistent Blocks**:
- Immutable, on-disk blocks created when the head block is cut
- Each block covers a time range (initially 2h, compacted up to 24h)
- Contains: index file, chunk files, meta.json
- Index maps label sets to chunk references

**Compaction**:
- Background goroutine merges small blocks into larger ones
- Reduces the number of blocks to scan during queries
- Drops data outside the retention window

**Retention**:
- Time-based: blocks older than `retention.duration` are deleted
- Size-based: oldest blocks deleted when `retention.max_size` is exceeded

### Prometheus Remote Backend

Forwards writes via Prometheus Remote Write protocol and reads via Remote Read. Acts as a transparent proxy — lil-olt-metrics handles OTLP translation and the external Prometheus handles storage and PromQL.

### VictoriaMetrics Remote Backend

Similar to Prometheus remote but uses VictoriaMetrics-specific import/export APIs for better performance with that backend.

## Package Structure

![Package Dependencies](https://www.plantuml.com/plantuml/png/~1UDfrKZ5kmp0CtFKADNkS3nH5XW9TknST28MMYTgoIj81WYP_hn9RWct46tbtF8e77KM1TUXQyw8DTcXZ2nICahPeFy7zW4VxZX732OExs0-6s1WJ9sRdkjD1aC_8E_jdhFtgqdZb-szpcwaeA7A0zk3wK9EVf6EpBQRWKNGIUuxk8KrrenfphGn1Mj2UjuqaiJZvFC0Q7CfTRq6iq1sl9Jj6xZjCOGm5EPDB3WG9PdAolUXtGBvCYo1IdiVWAM0UAn7P_VvjaSAAAIUly7B-4azth8Jv2QPOZVWgisBud5tsUPN9xkFGxCHPNSH2zlLQLINcqg_2BlYpo8_f4wQV-W-rl-Ks)

```
lil-olt-metrics/
├── cmd/
│   └── server/
│       └── main.go          # Entrypoint, wires all components
├── internal/
│   ├── config/
│   │   └── config.go        # YAML config loading, defaults, env overrides
│   ├── ingest/
│   │   ├── grpc.go          # OTLP gRPC server
│   │   ├── http.go          # OTLP HTTP handler
│   │   └── translator.go    # OTLP → Prometheus model conversion
│   ├── store/
│   │   ├── interfaces.go    # Store interface (consumed by ingest + query)
│   │   ├── fs.go            # Built-in FS storage engine
│   │   ├── prometheus.go    # Prometheus remote read/write backend
│   │   └── victoriametrics.go # VictoriaMetrics remote backend
│   └── query/
│       ├── api.go           # Prometheus HTTP API handlers
│       └── engine.go        # PromQL evaluation engine
├── docs/
│   ├── architecture.md      # This file
│   ├── otlp-standard.md     # OTLP protocol reference
│   ├── prometheus-api.md    # Prometheus API reference
│   └── config.md            # Configuration schema
└── go.mod
```

## Key Dependencies

| Dependency | Purpose |
|-----------|---------|
| `go.opentelemetry.io/proto-otlp` | OTLP protobuf definitions |
| `google.golang.org/grpc` | gRPC server |
| `github.com/prometheus/prometheus` | PromQL engine, TSDB primitives, model types |
| `github.com/prometheus/common` | Prometheus model and expfmt |
| `gopkg.in/yaml.v3` | Config file parsing |
| `log/slog` | Structured logging (stdlib) |

The Prometheus dependency is the most significant. We import its PromQL engine and storage interfaces rather than reimplementing them. This gives us full PromQL compatibility while keeping the codebase minimal.

## Concurrency Model

- **gRPC server**: Managed by `google.golang.org/grpc`, one goroutine per stream
- **HTTP server**: Standard `net/http` server, one goroutine per request
- **Storage writes**: Serialized through a write channel to the head block
- **Storage reads**: Concurrent, lock-free reads from immutable blocks; RWLock on head block
- **Compaction**: Single background goroutine, runs periodically
- **Retention**: Single background goroutine, runs periodically

## Startup Sequence

1. Load config (file -> env overrides -> defaults)
2. Initialize logger
3. Initialize storage backend
4. If FS engine: replay WAL, rebuild in-memory index
5. Initialize translator with config
6. Start OTLP gRPC server (if enabled)
7. Start HTTP server(s) for OTLP HTTP and Prometheus API
8. Start background goroutines (compaction, retention)
9. Block on signal (SIGINT/SIGTERM)
10. Graceful shutdown: drain in-flight requests, flush WAL, close storage

## Shutdown Sequence

1. Stop accepting new connections
2. Wait for in-flight requests to complete (with timeout)
3. Flush head block to WAL
4. Close storage backend
5. Exit

## PromQL Engine Strategy

Rather than implementing a PromQL engine from scratch, we embed the Prometheus PromQL engine (`github.com/prometheus/prometheus/promql`). This requires our storage to implement the Prometheus `storage.Queryable` interface:

```go
type Queryable interface {
    Querier(mint, maxt int64) (Querier, error)
}

type Querier interface {
    Select(ctx context.Context, sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet
    LabelValues(ctx context.Context, name string, hints *LabelHints, matchers ...*labels.Matcher) ([]string, Warnings, error)
    LabelNames(ctx context.Context, hints *LabelHints, matchers ...*labels.Matcher) ([]string, Warnings, error)
    Close() error
}
```

This approach ensures full PromQL compatibility (all functions, operators, and aggregations work) with minimal implementation effort.

## Resource Footprint Goals

| State | Memory | CPU |
|-------|--------|-----|
| Idle (no ingestion, no queries) | < 20 MB | < 0.1% |
| Light load (1k series, 1 sample/15s) | < 50 MB | < 1% |
| Moderate load (10k series, 1 sample/15s) | < 200 MB | < 5% |

Memory is dominated by the head block's in-memory index and sample buffer. Idle memory is kept low by not pre-allocating large buffers.
