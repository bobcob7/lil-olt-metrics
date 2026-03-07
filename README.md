# lil-olt-metrics

A minimal, single-process metrics server that ingests [OTLP](https://opentelemetry.io/docs/specs/otlp/) metrics and serves them via the [Prometheus HTTP API](https://prometheus.io/docs/prometheus/latest/querying/api/). Designed for edge deployments, dev environments, and lightweight production use.

## Features

- **OTLP ingestion** via gRPC (`:4317`) and HTTP (`:4318`) with protobuf and JSON support
- **Prometheus-compatible query API** (`:9090`) with full PromQL engine - works with Grafana out of the box
- **Pluggable storage**: built-in filesystem engine with WAL and compaction, or remote backends (Prometheus remote write, VictoriaMetrics)
- **Full OTLP metric type support**: Gauge, Sum (counter), Histogram, ExponentialHistogram, Summary
- **Delta-to-cumulative conversion** for delta temporality metrics
- **Zero-config startup** with sensible defaults - just run the binary
- **Single binary**, no external dependencies

## Installation

Install from a GitHub release with a single command (Linux and macOS):

```bash
curl -fsSL https://github.com/bobcob7/lil-olt-metrics/releases/latest/download/install.sh | sudo bash
```

The script auto-detects your OS and architecture, downloads the correct binary, installs it to `/usr/local/bin`, writes a default config, and registers a system service (systemd on Linux, launchd on macOS). It is idempotent and safe to re-run for updates.

To install a specific version or skip service setup:

```bash
# Specific version
curl -fsSL https://github.com/bobcob7/lil-olt-metrics/releases/latest/download/install.sh | sudo bash -s -- --version v0.2.0

# Binary only, no service
curl -fsSL https://github.com/bobcob7/lil-olt-metrics/releases/latest/download/install.sh | sudo bash -s -- --no-service
```

See the [project website](https://bobcob7.github.io/lil-olt-metrics/) for more details.

## Quick Start

### Build and run

```bash
# Build
make build

# Run with defaults (data stored in ./data)
./bin/lil-olt-metrics

# Or with a config file
./bin/lil-olt-metrics -config myconfig.yaml
```

### Send test data

Using the OpenTelemetry Collector's [telemetrygen](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/cmd/telemetrygen):

```bash
telemetrygen metrics --otlp-insecure --duration 10s
```

### Query via Prometheus API

```bash
# Instant query
curl 'http://localhost:9090/api/v1/query?query=up'

# Range query
curl 'http://localhost:9090/api/v1/query_range?query=rate(http_requests_total[5m])&start=2024-01-01T00:00:00Z&end=2024-01-01T01:00:00Z&step=15s'

# List all metric names
curl 'http://localhost:9090/api/v1/label/__name__/values'
```

### Use with Grafana

Add a Prometheus datasource pointing to `http://localhost:9090` and start building dashboards.

## Architecture

```
                    +-------------------+
  OTLP gRPC :4317 --->|                   |
                    |   lil-olt-metrics  |---> Prometheus API :9090 ---> Grafana
  OTLP HTTP :4318 --->|                   |
                    +-------------------+
                            |
                    +-------+-------+
                    |       |       |
                   FS   Prometheus  VM
                  store   remote   remote
```

Incoming OTLP metrics are translated to the Prometheus data model and stored. The Prometheus HTTP API serves queries using the embedded PromQL engine.

**Internal pipeline:**

1. **Ingestion** (`internal/ingest/`) - gRPC and HTTP handlers accept OTLP `ExportMetricsServiceRequest`
2. **Translation** (`internal/ingest/translator.go`) - Converts OTLP metric types to Prometheus samples with label mapping, unit suffixes, and delta-to-cumulative conversion
3. **Storage** (`internal/store/`) - Samples are written via the `Appender` interface to the configured backend
4. **Query** (`internal/query/`) - Prometheus HTTP API backed by the PromQL engine

## Configuration

Configuration is loaded from (in order of precedence):
1. Environment variables (prefix: `LOM_`)
2. YAML config file (optional, via `-config` flag)
3. Built-in defaults

See [docs/config-reference.md](docs/config-reference.md) for the full configuration reference.

### Example: minimal config

```yaml
# Use all defaults - just change the storage path
storage:
  fs:
    path: /var/lib/lil-olt-metrics
```

### Example: remote backend

```yaml
storage:
  engine: prometheus
  prometheus:
    write_url: http://prometheus:9090/api/v1/write
    read_url: http://prometheus:9090/api/v1/read
```

## Supported OTLP Features

| Feature | Status |
|---------|--------|
| Gauge | Supported |
| Sum (Counter) | Supported |
| Histogram | Supported |
| ExponentialHistogram | Converted to classic buckets |
| Summary | Supported |
| Delta-to-cumulative conversion | Supported (configurable) |
| Resource attribute mapping | Supported |
| Schema URL recording | Supported |
| Protobuf content type | Supported |
| JSON content type | Supported |
| gzip compression | Supported |

## Prometheus API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/query` | GET/POST | Instant query |
| `/api/v1/query_range` | GET/POST | Range query |
| `/api/v1/series` | GET/POST | Series metadata |
| `/api/v1/labels` | GET/POST | Label names |
| `/api/v1/label/<name>/values` | GET | Label values |
| `/api/v1/metadata` | GET | Metric metadata |
| `/api/v1/status/buildinfo` | GET | Build information |

## Development

### Prerequisites

- Go 1.24+

### Build, test, lint

```bash
make build       # Compile the server binary
make test        # Run all unit tests
make lint        # Run golangci-lint
make fmt         # Format code with gofumpt
make generate    # Regenerate mocks
make vet         # Run go vet
```

### Integration tests

```bash
go test -tags integration ./internal/integration/
go test -tags integration -race ./internal/integration/  # with race detector
```

### Project structure

```
cmd/server/         Application entrypoint
internal/
  config/           Configuration loading and defaults
  ingest/           OTLP ingestion (gRPC, HTTP, translator)
  store/            Storage backends (FS, MemStore, Prometheus, VictoriaMetrics)
  query/            Prometheus HTTP API and PromQL engine
  integration/      End-to-end integration tests
  tools/            Dev tool version pinning
docs/               Architecture and reference documentation
plans/              Implementation roadmap
```

## License

See [LICENSE](LICENSE) for details.
