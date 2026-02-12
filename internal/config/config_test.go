package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	t.Parallel()
	c, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, "info", c.Server.LogLevel)
	assert.Equal(t, "json", c.Server.LogFormat)
	assert.True(t, c.OTLP.GRPC.Enabled)
	assert.Equal(t, ":4317", c.OTLP.GRPC.Listen)
	assert.Equal(t, 4194304, c.OTLP.GRPC.MaxRecvMsgSize)
	assert.True(t, c.OTLP.HTTP.Enabled)
	assert.Equal(t, ":4318", c.OTLP.HTTP.Listen)
	assert.Equal(t, ":9090", c.Prometheus.Listen)
	assert.Equal(t, 30*time.Second, c.Prometheus.ReadTimeout.AsDuration())
	assert.Equal(t, 50000000, c.Prometheus.MaxSamples)
	assert.Equal(t, 15*time.Second, c.Prometheus.DefaultStep.AsDuration())
	assert.Equal(t, 5*time.Minute, c.Prometheus.LookbackDelta.AsDuration())
	assert.Equal(t, "fs", c.Storage.Engine)
	assert.Equal(t, "./data", c.Storage.FS.Path)
	assert.True(t, c.Storage.FS.WAL.Enabled)
	assert.Equal(t, ByteSize(128*MB), c.Storage.FS.WAL.SegmentSize)
	assert.True(t, c.Storage.FS.Compaction.Enabled)
	assert.Equal(t, 2*time.Hour, c.Storage.FS.Compaction.MinBlockDuration.AsDuration())
	assert.Equal(t, 24*time.Hour, c.Storage.FS.Compaction.MaxBlockDuration.AsDuration())
	assert.Equal(t, 15*24*time.Hour, c.Retention.Duration.AsDuration())
	assert.Equal(t, ByteSize(0), c.Retention.MaxSize)
	assert.True(t, c.Translation.SanitizeMetricNames)
	assert.True(t, c.Translation.AddUnitSuffix)
	assert.True(t, c.Translation.AddTypeSuffix)
	assert.Equal(t, "job", c.Translation.ResourceAttributes.LabelMap["service.name"])
	assert.Equal(t, "instance", c.Translation.ResourceAttributes.LabelMap["service.instance.id"])
}

func TestLoadYAMLOverride(t *testing.T) {
	t.Parallel()
	yamlContent := `
server:
  log_level: debug
  log_format: text
otlp:
  grpc:
    listen: ":5317"
prometheus:
  listen: ":8080"
  read_timeout: 60s
storage:
  engine: fs
  fs:
    path: /tmp/metrics
    wal:
      segment_size: 256MB
retention:
  duration: 30d
  max_size: 10GB
`
	path := writeTestYAML(t, yamlContent)
	c, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "debug", c.Server.LogLevel)
	assert.Equal(t, "text", c.Server.LogFormat)
	assert.Equal(t, ":5317", c.OTLP.GRPC.Listen)
	assert.Equal(t, ":8080", c.Prometheus.Listen)
	assert.Equal(t, 60*time.Second, c.Prometheus.ReadTimeout.AsDuration())
	assert.Equal(t, "/tmp/metrics", c.Storage.FS.Path)
	assert.Equal(t, ByteSize(256*MB), c.Storage.FS.WAL.SegmentSize)
	assert.Equal(t, 30*24*time.Hour, c.Retention.Duration.AsDuration())
	assert.Equal(t, ByteSize(10*GB), c.Retention.MaxSize)
}

func TestLoadYAMLPreservesDefaults(t *testing.T) {
	t.Parallel()
	yamlContent := `
server:
  log_level: warn
`
	path := writeTestYAML(t, yamlContent)
	c, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "warn", c.Server.LogLevel)
	assert.Equal(t, "json", c.Server.LogFormat)
	assert.True(t, c.OTLP.GRPC.Enabled)
	assert.Equal(t, ":4317", c.OTLP.GRPC.Listen)
	assert.Equal(t, "fs", c.Storage.Engine)
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("LOM_SERVER_LOG_LEVEL", "error")
	t.Setenv("LOM_OTLP_GRPC_LISTEN", ":6317")
	t.Setenv("LOM_PROMETHEUS_LISTEN", ":7090")
	t.Setenv("LOM_STORAGE_ENGINE", "fs")
	t.Setenv("LOM_STORAGE_FS_PATH", "/env/data")
	c, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, "error", c.Server.LogLevel)
	assert.Equal(t, ":6317", c.OTLP.GRPC.Listen)
	assert.Equal(t, ":7090", c.Prometheus.Listen)
	assert.Equal(t, "/env/data", c.Storage.FS.Path)
}

func TestEnvOverrideDuration(t *testing.T) {
	t.Setenv("LOM_PROMETHEUS_READ_TIMEOUT", "45s")
	t.Setenv("LOM_RETENTION_DURATION", "7d")
	c, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, 45*time.Second, c.Prometheus.ReadTimeout.AsDuration())
	assert.Equal(t, 7*24*time.Hour, c.Retention.Duration.AsDuration())
}

func TestEnvOverrideByteSize(t *testing.T) {
	t.Setenv("LOM_STORAGE_FS_WAL_SEGMENT_SIZE", "256MB")
	t.Setenv("LOM_RETENTION_MAX_SIZE", "5GB")
	c, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, ByteSize(256*MB), c.Storage.FS.WAL.SegmentSize)
	assert.Equal(t, ByteSize(5*GB), c.Retention.MaxSize)
}

func TestEnvTakesPrecedenceOverYAML(t *testing.T) {
	yamlContent := `
server:
  log_level: debug
otlp:
  grpc:
    listen: ":5317"
`
	path := writeTestYAML(t, yamlContent)
	t.Setenv("LOM_SERVER_LOG_LEVEL", "warn")
	t.Setenv("LOM_OTLP_GRPC_LISTEN", ":6317")
	c, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "warn", c.Server.LogLevel)
	assert.Equal(t, ":6317", c.OTLP.GRPC.Listen)
}

func TestValidateInvalidLogLevel(t *testing.T) {
	t.Setenv("LOM_SERVER_LOG_LEVEL", "verbose")
	_, err := Load("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server.log_level")
}

func TestValidateInvalidLogFormat(t *testing.T) {
	t.Setenv("LOM_SERVER_LOG_FORMAT", "xml")
	_, err := Load("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server.log_format")
}

func TestValidateInvalidEngine(t *testing.T) {
	t.Setenv("LOM_STORAGE_ENGINE", "redis")
	_, err := Load("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage.engine")
}

func TestValidatePrometheusEngineRequiresURLs(t *testing.T) {
	t.Parallel()
	yamlContent := `
storage:
  engine: prometheus
`
	path := writeTestYAML(t, yamlContent)
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage.prometheus.write_url")
	assert.Contains(t, err.Error(), "storage.prometheus.read_url")
}

func TestValidateVictoriaMetricsEngineRequiresURLs(t *testing.T) {
	t.Parallel()
	yamlContent := `
storage:
  engine: victoriametrics
`
	path := writeTestYAML(t, yamlContent)
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage.victoriametrics.write_url")
	assert.Contains(t, err.Error(), "storage.victoriametrics.read_url")
}

func TestValidatePrometheusEngineWithURLsSucceeds(t *testing.T) {
	t.Parallel()
	yamlContent := `
storage:
  engine: prometheus
  prometheus:
    write_url: http://prom:9090/api/v1/write
    read_url: http://prom:9090/api/v1/read
`
	path := writeTestYAML(t, yamlContent)
	c, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "prometheus", c.Storage.Engine)
}

func TestValidateInvalidPort(t *testing.T) {
	t.Setenv("LOM_OTLP_GRPC_LISTEN", "noport")
	_, err := Load("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "otlp.grpc.listen")
}

func TestValidateNegativeMaxRecvMsgSize(t *testing.T) {
	t.Parallel()
	yamlContent := `
otlp:
  grpc:
    max_recv_msg_size: -1
`
	path := writeTestYAML(t, yamlContent)
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_recv_msg_size")
}

func TestLoadInvalidYAMLPath(t *testing.T) {
	t.Parallel()
	_, err := Load("/nonexistent/path.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config file")
}

func TestParseDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{name: "seconds", input: "30s", expected: 30 * time.Second},
		{name: "minutes", input: "5m", expected: 5 * time.Minute},
		{name: "hours", input: "2h", expected: 2 * time.Hour},
		{name: "days", input: "15d", expected: 15 * 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d, err := parseDuration(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, d.AsDuration())
		})
	}
}

func TestParseByteSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected ByteSize
	}{
		{name: "zero", input: "0", expected: 0},
		{name: "kilobytes", input: "512KB", expected: 512 * KB},
		{name: "megabytes", input: "128MB", expected: 128 * MB},
		{name: "gigabytes", input: "10GB", expected: 10 * GB},
		{name: "raw_bytes", input: "4194304", expected: 4194304},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, err := parseByteSize(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, b)
		})
	}
}

func TestEnvOverrideBool(t *testing.T) {
	t.Setenv("LOM_OTLP_GRPC_ENABLED", "false")
	t.Setenv("LOM_TRANSLATION_SANITIZE_METRIC_NAMES", "false")
	c, err := Load("")
	require.NoError(t, err)
	assert.False(t, c.OTLP.GRPC.Enabled)
	assert.False(t, c.Translation.SanitizeMetricNames)
}

func writeTestYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}
