# Configuration Reference

Configuration is loaded from (in order of precedence):

1. **Environment variables** (prefix: `LOM_`, underscore-separated path)
2. **YAML config file** (optional, via `-config` flag)
3. **Built-in defaults**

Environment variable names follow the pattern `LOM_<SECTION>_<FIELD>` using the YAML key names in uppercase. For example, `server.log_level` becomes `LOM_SERVER_LOG_LEVEL`.

## Server

| Field | Type | Default | Env Var | Description |
|-------|------|---------|---------|-------------|
| `server.log_level` | string | `"info"` | `LOM_SERVER_LOG_LEVEL` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `server.log_format` | string | `"json"` | `LOM_SERVER_LOG_FORMAT` | Log output format: `json`, `text` |

## OTLP Ingestion

### gRPC

| Field | Type | Default | Env Var | Description |
|-------|------|---------|---------|-------------|
| `otlp.grpc.enabled` | bool | `true` | `LOM_OTLP_GRPC_ENABLED` | Enable gRPC OTLP endpoint |
| `otlp.grpc.listen` | string | `":4317"` | `LOM_OTLP_GRPC_LISTEN` | gRPC listen address |
| `otlp.grpc.max_recv_msg_size` | int | `4194304` | `LOM_OTLP_GRPC_MAX_RECV_MSG_SIZE` | Maximum gRPC message size in bytes (4 MB) |
| `otlp.grpc.gzip` | bool | `true` | `LOM_OTLP_GRPC_GZIP` | Enable gzip compression support |

### HTTP

| Field | Type | Default | Env Var | Description |
|-------|------|---------|---------|-------------|
| `otlp.http.enabled` | bool | `true` | `LOM_OTLP_HTTP_ENABLED` | Enable HTTP OTLP endpoint |
| `otlp.http.listen` | string | `":4318"` | `LOM_OTLP_HTTP_LISTEN` | HTTP listen address |
| `otlp.http.max_body_size` | int | `4194304` | `LOM_OTLP_HTTP_MAX_BODY_SIZE` | Maximum HTTP body size in bytes (4 MB) |
| `otlp.http.gzip` | bool | `true` | `LOM_OTLP_HTTP_GZIP` | Enable gzip decompression |

## Prometheus Query API

| Field | Type | Default | Env Var | Description |
|-------|------|---------|---------|-------------|
| `prometheus.listen` | string | `":9090"` | `LOM_PROMETHEUS_LISTEN` | Prometheus API listen address |
| `prometheus.read_timeout` | duration | `"30s"` | `LOM_PROMETHEUS_READ_TIMEOUT` | HTTP read timeout |
| `prometheus.max_samples` | int | `50000000` | `LOM_PROMETHEUS_MAX_SAMPLES` | Maximum samples per query |
| `prometheus.default_step` | duration | `"15s"` | `LOM_PROMETHEUS_DEFAULT_STEP` | Default step for range queries |
| `prometheus.max_points_per_series` | int | `11000` | `LOM_PROMETHEUS_MAX_POINTS_PER_SERIES` | Maximum data points per series in a query |
| `prometheus.lookback_delta` | duration | `"5m"` | `LOM_PROMETHEUS_LOOKBACK_DELTA` | PromQL lookback delta for stale sample detection |

## Storage

| Field | Type | Default | Env Var | Description |
|-------|------|---------|---------|-------------|
| `storage.engine` | string | `"fs"` | `LOM_STORAGE_ENGINE` | Storage backend: `fs`, `prometheus`, `victoriametrics` |

### Filesystem (fs)

| Field | Type | Default | Env Var | Description |
|-------|------|---------|---------|-------------|
| `storage.fs.path` | string | `"./data"` | `LOM_STORAGE_FS_PATH` | Data directory path |
| `storage.fs.wal.enabled` | bool | `true` | `LOM_STORAGE_FS_WAL_ENABLED` | Enable write-ahead log |
| `storage.fs.wal.segment_size` | bytesize | `"128MB"` | `LOM_STORAGE_FS_WAL_SEGMENT_SIZE` | WAL segment file size |
| `storage.fs.compaction.enabled` | bool | `true` | `LOM_STORAGE_FS_COMPACTION_ENABLED` | Enable background compaction |
| `storage.fs.compaction.min_block_duration` | duration | `"2h"` | `LOM_STORAGE_FS_COMPACTION_MIN_BLOCK_DURATION` | Minimum block duration before compaction |
| `storage.fs.compaction.max_block_duration` | duration | `"24h"` | `LOM_STORAGE_FS_COMPACTION_MAX_BLOCK_DURATION` | Maximum block duration |

### Prometheus Remote

| Field | Type | Default | Env Var | Description |
|-------|------|---------|---------|-------------|
| `storage.prometheus.write_url` | string | `""` | `LOM_STORAGE_PROMETHEUS_WRITE_URL` | Prometheus remote write URL (required when engine=prometheus) |
| `storage.prometheus.read_url` | string | `""` | `LOM_STORAGE_PROMETHEUS_READ_URL` | Prometheus remote read URL (required when engine=prometheus) |
| `storage.prometheus.timeout` | duration | `"30s"` | `LOM_STORAGE_PROMETHEUS_TIMEOUT` | Request timeout |
| `storage.prometheus.basic_auth.username` | string | `""` | `LOM_STORAGE_PROMETHEUS_BASIC_AUTH_USERNAME` | Basic auth username |
| `storage.prometheus.basic_auth.password` | string | `""` | `LOM_STORAGE_PROMETHEUS_BASIC_AUTH_PASSWORD` | Basic auth password |
| `storage.prometheus.tls.cert_file` | string | `""` | `LOM_STORAGE_PROMETHEUS_TLS_CERT_FILE` | TLS client certificate |
| `storage.prometheus.tls.key_file` | string | `""` | `LOM_STORAGE_PROMETHEUS_TLS_KEY_FILE` | TLS client key |
| `storage.prometheus.tls.ca_file` | string | `""` | `LOM_STORAGE_PROMETHEUS_TLS_CA_FILE` | TLS CA certificate |
| `storage.prometheus.tls.insecure_skip_verify` | bool | `false` | `LOM_STORAGE_PROMETHEUS_TLS_INSECURE_SKIP_VERIFY` | Skip TLS verification |

### VictoriaMetrics Remote

| Field | Type | Default | Env Var | Description |
|-------|------|---------|---------|-------------|
| `storage.victoriametrics.write_url` | string | `""` | `LOM_STORAGE_VICTORIAMETRICS_WRITE_URL` | VM import URL (required when engine=victoriametrics) |
| `storage.victoriametrics.read_url` | string | `""` | `LOM_STORAGE_VICTORIAMETRICS_READ_URL` | VM export URL (required when engine=victoriametrics) |
| `storage.victoriametrics.timeout` | duration | `"30s"` | `LOM_STORAGE_VICTORIAMETRICS_TIMEOUT` | Request timeout |
| `storage.victoriametrics.basic_auth.username` | string | `""` | `LOM_STORAGE_VICTORIAMETRICS_BASIC_AUTH_USERNAME` | Basic auth username |
| `storage.victoriametrics.basic_auth.password` | string | `""` | `LOM_STORAGE_VICTORIAMETRICS_BASIC_AUTH_PASSWORD` | Basic auth password |

## Retention

| Field | Type | Default | Env Var | Description |
|-------|------|---------|---------|-------------|
| `retention.duration` | duration | `"15d"` | `LOM_RETENTION_DURATION` | How long to keep data (supports `d` suffix for days) |
| `retention.max_size` | bytesize | `0` | `LOM_RETENTION_MAX_SIZE` | Maximum storage size (0 = unlimited) |

## Translation

| Field | Type | Default | Env Var | Description |
|-------|------|---------|---------|-------------|
| `translation.sanitize_metric_names` | bool | `true` | `LOM_TRANSLATION_SANITIZE_METRIC_NAMES` | Sanitize metric names to Prometheus conventions |
| `translation.add_unit_suffix` | bool | `true` | `LOM_TRANSLATION_ADD_UNIT_SUFFIX` | Append OTLP unit as metric name suffix |
| `translation.add_type_suffix` | bool | `true` | `LOM_TRANSLATION_ADD_TYPE_SUFFIX` | Append `_total` suffix for monotonic counters |
| `translation.delta_conversion` | bool | `true` | `LOM_TRANSLATION_DELTA_CONVERSION` | Convert delta temporality to cumulative |
| `translation.schema_url` | bool | `true` | `LOM_TRANSLATION_SCHEMA_URL` | Record schema URL as `__schema_url__` label |

Resource attribute mapping (`translation.resource_attributes.label_map` and `translation.resource_attributes.promote`) is configured via YAML only (not supported via environment variables).

### Default resource attribute mapping

```yaml
translation:
  resource_attributes:
    label_map:
      service.name: job
      service.instance.id: instance
    promote: []
```

## Type Reference

- **duration**: Go duration string (`"30s"`, `"5m"`, `"2h"`) or days (`"15d"`)
- **bytesize**: Integer bytes or human-readable (`"128MB"`, `"1GB"`)
- **bool**: `true` / `false`
- **int**: Integer number
- **string**: Text value

## Example: Full Config

```yaml
server:
  log_level: info
  log_format: json

otlp:
  grpc:
    enabled: true
    listen: ":4317"
    max_recv_msg_size: 4194304
    gzip: true
  http:
    enabled: true
    listen: ":4318"
    max_body_size: 4194304
    gzip: true

prometheus:
  listen: ":9090"
  read_timeout: 30s
  max_samples: 50000000
  default_step: 15s
  max_points_per_series: 11000
  lookback_delta: 5m

storage:
  engine: fs
  fs:
    path: ./data
    wal:
      enabled: true
      segment_size: 128MB
    compaction:
      enabled: true
      min_block_duration: 2h
      max_block_duration: 24h

retention:
  duration: 15d
  max_size: 0

translation:
  sanitize_metric_names: true
  add_unit_suffix: true
  add_type_suffix: true
  delta_conversion: true
  schema_url: true
  resource_attributes:
    label_map:
      service.name: job
      service.instance.id: instance
    promote: []
```
