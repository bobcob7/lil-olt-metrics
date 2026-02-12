# Configuration Schema

lil-olt-metrics uses a single YAML configuration file. All settings have sensible defaults, so a zero-config startup is possible.

## Default Config Path

```
./lil-olt-metrics.yaml
```

Override with `--config` flag or `LOM_CONFIG` environment variable.

## Environment Variable Override

Any config field can be overridden via environment variable using the `LOM_` prefix with underscore-separated path:

```
LOM_OTLP_GRPC_LISTEN=:4317
LOM_STORAGE_ENGINE=fs
LOM_STORAGE_FS_PATH=/data/metrics
```

## Full Schema

```yaml
# Server-level settings
server:
  # Log level: debug, info, warn, error
  log_level: info
  # Log format: json, text
  log_format: json

# OTLP ingestion settings
otlp:
  grpc:
    # Enable gRPC OTLP ingestion
    enabled: true
    # Listen address for gRPC
    listen: ":4317"
    # Max receive message size in bytes (default 4MB)
    max_recv_msg_size: 4194304
    # Enable gzip decompression
    gzip: true
  http:
    # Enable HTTP OTLP ingestion
    enabled: true
    # Listen address for HTTP (shared with Prometheus API)
    listen: ":4318"
    # Max request body size in bytes (default 4MB)
    max_body_size: 4194304
    # Enable gzip decompression
    gzip: true

# Prometheus-compatible query API settings
prometheus:
  # Listen address for the Prometheus HTTP API
  # If same as otlp.http.listen, they share the same server
  listen: ":9090"
  # Read timeout for query requests
  read_timeout: 30s
  # Max number of samples a single query can load
  max_samples: 50000000
  # Default evaluation interval for range queries
  default_step: 15s
  # Max points per timeseries in a range query response
  max_points_per_series: 11000
  # Lookback delta for staleness (how far back to look for a sample)
  lookback_delta: 5m

# Storage backend settings
storage:
  # Storage engine: "fs", "prometheus", "victoriametrics"
  engine: fs

  # Built-in filesystem storage
  fs:
    # Data directory path
    path: ./data
    # Write-ahead log (WAL) settings
    wal:
      # Enable WAL for crash recovery
      enabled: true
      # WAL segment size
      segment_size: 128MB
    # Block compaction settings
    compaction:
      # Enable background compaction
      enabled: true
      # Min block duration before compaction
      min_block_duration: 2h
      # Max block duration after compaction
      max_block_duration: 24h

  # Prometheus remote write/read backend
  prometheus:
    # Remote write URL
    write_url: ""
    # Remote read URL
    read_url: ""
    # Timeout for remote operations
    timeout: 30s
    # Basic auth (optional)
    basic_auth:
      username: ""
      password: ""
    # TLS settings (optional)
    tls:
      cert_file: ""
      key_file: ""
      ca_file: ""
      insecure_skip_verify: false

  # VictoriaMetrics backend
  victoriametrics:
    # Import URL (vmimport API)
    write_url: ""
    # Export/query URL
    read_url: ""
    timeout: 30s
    basic_auth:
      username: ""
      password: ""

# Data retention settings
retention:
  # How long to keep data (0 = unlimited)
  duration: 15d
  # Max storage size (0 = unlimited). Oldest data is dropped first.
  max_size: 0

# OTLP-to-Prometheus translation settings
translation:
  # How to handle resource attributes
  resource_attributes:
    # Promote these resource attributes to metric labels
    # Default: service.name -> job, service.instance.id -> instance
    label_map:
      "service.name": "job"
      "service.instance.id": "instance"
    # Additional resource attributes to promote (key becomes label name)
    promote: []
  # Metric name sanitization for Prometheus compatibility
  # Prometheus metric names must match [a-zA-Z_:][a-zA-Z0-9_:]*
  sanitize_metric_names: true
  # Convert OTLP unit to Prometheus suffix (e.g., "s" -> "_seconds")
  add_unit_suffix: true
  # Convert monotonic Sum to Prometheus counter (_total suffix)
  add_type_suffix: true
```

## Minimal Config Examples

### Zero Config (All Defaults)

```yaml
# Empty file or no file - uses all defaults:
# - OTLP gRPC on :4317
# - OTLP HTTP on :4318
# - Prometheus API on :9090
# - FS storage in ./data
# - 15 day retention
```

### Forward to External Prometheus

```yaml
storage:
  engine: prometheus
  prometheus:
    write_url: http://prometheus:9090/api/v1/write
    read_url: http://prometheus:9090/api/v1/read
```

### Forward to VictoriaMetrics

```yaml
storage:
  engine: victoriametrics
  victoriametrics:
    write_url: http://victoria:8428/api/v1/import
    read_url: http://victoria:8428
```

### Custom FS Storage with Retention

```yaml
storage:
  engine: fs
  fs:
    path: /var/lib/lil-olt-metrics/data
retention:
  duration: 30d
  max_size: 10GB
```

### Single Port Mode

```yaml
otlp:
  http:
    listen: ":8080"
prometheus:
  listen: ":8080"
```

When OTLP HTTP and Prometheus API share the same listen address, they are multiplexed on the same HTTP server. OTLP endpoints are routed under `/v1/metrics`, Prometheus API under `/api/v1/`.

## Config Go Struct

```go
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	OTLP       OTLPConfig       `yaml:"otlp"`
	Prometheus PrometheusConfig `yaml:"prometheus"`
	Storage    StorageConfig    `yaml:"storage"`
	Retention  RetentionConfig  `yaml:"retention"`
	Translation TranslationConfig `yaml:"translation"`
}

type ServerConfig struct {
	LogLevel  string `yaml:"log_level"`
	LogFormat string `yaml:"log_format"`
}

type OTLPConfig struct {
	GRPC OTLPGRPCConfig `yaml:"grpc"`
	HTTP OTLPHTTPConfig `yaml:"http"`
}

type OTLPGRPCConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Listen         string `yaml:"listen"`
	MaxRecvMsgSize int    `yaml:"max_recv_msg_size"`
	Gzip           bool   `yaml:"gzip"`
}

type OTLPHTTPConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Listen      string `yaml:"listen"`
	MaxBodySize int    `yaml:"max_body_size"`
	Gzip        bool   `yaml:"gzip"`
}

type PrometheusConfig struct {
	Listen            string        `yaml:"listen"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	MaxSamples        int           `yaml:"max_samples"`
	DefaultStep       time.Duration `yaml:"default_step"`
	MaxPointsPerSeries int          `yaml:"max_points_per_series"`
	LookbackDelta     time.Duration `yaml:"lookback_delta"`
}

type StorageConfig struct {
	Engine           string                  `yaml:"engine"`
	FS               FSStorageConfig         `yaml:"fs"`
	Prometheus       PrometheusStorageConfig `yaml:"prometheus"`
	VictoriaMetrics  VMStorageConfig         `yaml:"victoriametrics"`
}

type FSStorageConfig struct {
	Path       string           `yaml:"path"`
	WAL        WALConfig        `yaml:"wal"`
	Compaction CompactionConfig `yaml:"compaction"`
}

type RetentionConfig struct {
	Duration time.Duration `yaml:"duration"`
	MaxSize  int64         `yaml:"max_size"`
}

type TranslationConfig struct {
	ResourceAttributes ResourceAttributesConfig `yaml:"resource_attributes"`
	SanitizeMetricNames bool                    `yaml:"sanitize_metric_names"`
	AddUnitSuffix       bool                    `yaml:"add_unit_suffix"`
	AddTypeSuffix       bool                    `yaml:"add_type_suffix"`
}

type ResourceAttributesConfig struct {
	LabelMap map[string]string `yaml:"label_map"`
	Promote  []string          `yaml:"promote"`
}
```
