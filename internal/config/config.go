package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration for YAML string parsing (supports "15d" for days).
type Duration int64

// AsDuration returns the underlying time.Duration.
func (d Duration) AsDuration() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		var n int64
		if nerr := value.Decode(&n); nerr != nil {
			return err
		}
		*d = Duration(n)
		return nil
	}
	parsed, err := parseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(parsed)
	return nil
}

// ByteSize is an int64 byte count that parses human-readable sizes ("128MB", "10GB").
type ByteSize int64

const (
	KB ByteSize = 1024
	MB ByteSize = 1024 * KB
	GB ByteSize = 1024 * MB
)

func (b *ByteSize) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		var n int64
		if nerr := value.Decode(&n); nerr != nil {
			return err
		}
		*b = ByteSize(n)
		return nil
	}
	parsed, err := parseByteSize(s)
	if err != nil {
		return err
	}
	*b = ByteSize(parsed)
	return nil
}

// Config is the top-level configuration for lil-olt-metrics.
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	OTLP        OTLPConfig        `yaml:"otlp"`
	Prometheus  PrometheusConfig  `yaml:"prometheus"`
	Storage     StorageConfig     `yaml:"storage"`
	Retention   RetentionConfig   `yaml:"retention"`
	Translation TranslationConfig `yaml:"translation"`
}

// ServerConfig holds server-level settings.
type ServerConfig struct {
	LogLevel  string `yaml:"log_level"`
	LogFormat string `yaml:"log_format"`
}

// OTLPConfig holds OTLP ingestion settings.
type OTLPConfig struct {
	GRPC OTLPGRPCConfig `yaml:"grpc"`
	HTTP OTLPHTTPConfig `yaml:"http"`
}

// OTLPGRPCConfig holds gRPC OTLP settings.
type OTLPGRPCConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Listen         string `yaml:"listen"`
	MaxRecvMsgSize int    `yaml:"max_recv_msg_size"`
	Gzip           bool   `yaml:"gzip"`
}

// OTLPHTTPConfig holds HTTP OTLP settings.
type OTLPHTTPConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Listen      string `yaml:"listen"`
	MaxBodySize int    `yaml:"max_body_size"`
	Gzip        bool   `yaml:"gzip"`
}

// PrometheusConfig holds Prometheus query API settings.
type PrometheusConfig struct {
	Listen             string   `yaml:"listen"`
	ReadTimeout        Duration `yaml:"read_timeout"`
	MaxSamples         int      `yaml:"max_samples"`
	DefaultStep        Duration `yaml:"default_step"`
	MaxPointsPerSeries int      `yaml:"max_points_per_series"`
	LookbackDelta      Duration `yaml:"lookback_delta"`
}

// StorageConfig holds storage backend settings.
type StorageConfig struct {
	Engine          string                  `yaml:"engine"`
	FS              FSStorageConfig         `yaml:"fs"`
	Prometheus      PrometheusStorageConfig `yaml:"prometheus"`
	VictoriaMetrics VMStorageConfig         `yaml:"victoriametrics"`
}

// FSStorageConfig holds built-in filesystem storage settings.
type FSStorageConfig struct {
	Path       string           `yaml:"path"`
	WAL        WALConfig        `yaml:"wal"`
	Compaction CompactionConfig `yaml:"compaction"`
}

// WALConfig holds write-ahead log settings.
type WALConfig struct {
	Enabled     bool     `yaml:"enabled"`
	SegmentSize ByteSize `yaml:"segment_size"`
}

// CompactionConfig holds block compaction settings.
type CompactionConfig struct {
	Enabled          bool     `yaml:"enabled"`
	MinBlockDuration Duration `yaml:"min_block_duration"`
	MaxBlockDuration Duration `yaml:"max_block_duration"`
}

// PrometheusStorageConfig holds Prometheus remote write/read settings.
type PrometheusStorageConfig struct {
	WriteURL  string    `yaml:"write_url"`
	ReadURL   string    `yaml:"read_url"`
	Timeout   Duration  `yaml:"timeout"`
	BasicAuth BasicAuth `yaml:"basic_auth"`
	TLS       TLSConfig `yaml:"tls"`
}

// VMStorageConfig holds VictoriaMetrics backend settings.
type VMStorageConfig struct {
	WriteURL  string    `yaml:"write_url"`
	ReadURL   string    `yaml:"read_url"`
	Timeout   Duration  `yaml:"timeout"`
	BasicAuth BasicAuth `yaml:"basic_auth"`
}

// BasicAuth holds basic authentication credentials.
type BasicAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// TLSConfig holds TLS settings.
type TLSConfig struct {
	CertFile           string `yaml:"cert_file"`
	KeyFile            string `yaml:"key_file"`
	CAFile             string `yaml:"ca_file"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

// RetentionConfig holds data retention settings.
type RetentionConfig struct {
	Duration Duration `yaml:"duration"`
	MaxSize  ByteSize `yaml:"max_size"`
}

// TranslationConfig holds OTLP-to-Prometheus translation settings.
type TranslationConfig struct {
	ResourceAttributes  ResourceAttributesConfig `yaml:"resource_attributes"`
	SanitizeMetricNames bool                     `yaml:"sanitize_metric_names"`
	AddUnitSuffix       bool                     `yaml:"add_unit_suffix"`
	AddTypeSuffix       bool                     `yaml:"add_type_suffix"`
}

// ResourceAttributesConfig holds resource attribute mapping settings.
type ResourceAttributesConfig struct {
	LabelMap map[string]string `yaml:"label_map"`
	Promote  []string          `yaml:"promote"`
}

// Load reads configuration from defaults, optional YAML file, and environment
// variable overrides. Precedence: env vars > YAML file > defaults.
func Load(path string) (*Config, error) {
	c := newDefaults()
	if path != "" {
		if err := loadYAML(c, path); err != nil {
			return nil, fmt.Errorf("loading config file: %w", err)
		}
	}
	applyEnvOverrides(reflect.ValueOf(c).Elem(), "LOM")
	if err := validate(c); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return c, nil
}

func newDefaults() *Config {
	return &Config{
		Server: ServerConfig{
			LogLevel:  "info",
			LogFormat: "json",
		},
		OTLP: OTLPConfig{
			GRPC: OTLPGRPCConfig{
				Enabled:        true,
				Listen:         ":4317",
				MaxRecvMsgSize: 4194304,
				Gzip:           true,
			},
			HTTP: OTLPHTTPConfig{
				Enabled:     true,
				Listen:      ":4318",
				MaxBodySize: 4194304,
				Gzip:        true,
			},
		},
		Prometheus: PrometheusConfig{
			Listen:             ":9090",
			ReadTimeout:        Duration(30 * time.Second),
			MaxSamples:         50000000,
			DefaultStep:        Duration(15 * time.Second),
			MaxPointsPerSeries: 11000,
			LookbackDelta:      Duration(5 * time.Minute),
		},
		Storage: StorageConfig{
			Engine: "fs",
			FS: FSStorageConfig{
				Path: "./data",
				WAL: WALConfig{
					Enabled:     true,
					SegmentSize: 128 * MB,
				},
				Compaction: CompactionConfig{
					Enabled:          true,
					MinBlockDuration: Duration(2 * time.Hour),
					MaxBlockDuration: Duration(24 * time.Hour),
				},
			},
			Prometheus: PrometheusStorageConfig{
				Timeout: Duration(30 * time.Second),
			},
			VictoriaMetrics: VMStorageConfig{
				Timeout: Duration(30 * time.Second),
			},
		},
		Retention: RetentionConfig{
			Duration: Duration(15 * 24 * time.Hour),
			MaxSize:  0,
		},
		Translation: TranslationConfig{
			ResourceAttributes: ResourceAttributesConfig{
				LabelMap: map[string]string{
					"service.name":        "job",
					"service.instance.id": "instance",
				},
				Promote: []string{},
			},
			SanitizeMetricNames: true,
			AddUnitSuffix:       true,
			AddTypeSuffix:       true,
		},
	}
}

func loadYAML(c *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, c)
}

var (
	durationType = reflect.TypeFor[Duration]()
	byteSizeType = reflect.TypeFor[ByteSize]()
)

func applyEnvOverrides(v reflect.Value, prefix string) {
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		fv := v.Field(i)
		tag := field.Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		tag, _, _ = strings.Cut(tag, ",")
		envKey := prefix + "_" + strings.ToUpper(tag)
		envVal, ok := os.LookupEnv(envKey)
		switch {
		case field.Type == durationType:
			if ok {
				if d, err := parseDuration(envVal); err == nil {
					fv.SetInt(int64(d))
				}
			}
		case field.Type == byteSizeType:
			if ok {
				if b, err := parseByteSize(envVal); err == nil {
					fv.SetInt(int64(b))
				}
			}
		case field.Type.Kind() == reflect.Struct:
			applyEnvOverrides(fv, envKey)
		case field.Type.Kind() == reflect.Map || field.Type.Kind() == reflect.Slice:
			continue
		case ok:
			setFieldFromString(fv, envVal)
		}
	}
}

func setFieldFromString(fv reflect.Value, val string) {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(val)
	case reflect.Int, reflect.Int64:
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			fv.SetInt(n)
		}
	case reflect.Bool:
		if b, err := strconv.ParseBool(val); err == nil {
			fv.SetBool(b)
		}
	}
}

var validEngines = map[string]bool{
	"fs":              true,
	"prometheus":      true,
	"victoriametrics": true,
}

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

var validLogFormats = map[string]bool{
	"json": true,
	"text": true,
}

func validate(c *Config) error {
	var errs []error
	if !validLogLevels[c.Server.LogLevel] {
		errs = append(errs, fmt.Errorf("server.log_level: invalid value %q (must be debug, info, warn, or error)", c.Server.LogLevel))
	}
	if !validLogFormats[c.Server.LogFormat] {
		errs = append(errs, fmt.Errorf("server.log_format: invalid value %q (must be json or text)", c.Server.LogFormat))
	}
	if c.OTLP.GRPC.Enabled {
		if err := validateListen(c.OTLP.GRPC.Listen, "otlp.grpc.listen"); err != nil {
			errs = append(errs, err)
		}
		if c.OTLP.GRPC.MaxRecvMsgSize <= 0 {
			errs = append(errs, fmt.Errorf("otlp.grpc.max_recv_msg_size: must be positive"))
		}
	}
	if c.OTLP.HTTP.Enabled {
		if err := validateListen(c.OTLP.HTTP.Listen, "otlp.http.listen"); err != nil {
			errs = append(errs, err)
		}
		if c.OTLP.HTTP.MaxBodySize <= 0 {
			errs = append(errs, fmt.Errorf("otlp.http.max_body_size: must be positive"))
		}
	}
	if err := validateListen(c.Prometheus.Listen, "prometheus.listen"); err != nil {
		errs = append(errs, err)
	}
	if c.Prometheus.ReadTimeout.AsDuration() <= 0 {
		errs = append(errs, fmt.Errorf("prometheus.read_timeout: must be positive"))
	}
	if c.Prometheus.MaxSamples <= 0 {
		errs = append(errs, fmt.Errorf("prometheus.max_samples: must be positive"))
	}
	if !validEngines[c.Storage.Engine] {
		errs = append(errs, fmt.Errorf("storage.engine: invalid value %q (must be fs, prometheus, or victoriametrics)", c.Storage.Engine))
	}
	if c.Storage.Engine == "prometheus" {
		if c.Storage.Prometheus.WriteURL == "" {
			errs = append(errs, fmt.Errorf("storage.prometheus.write_url: required when engine is prometheus"))
		}
		if c.Storage.Prometheus.ReadURL == "" {
			errs = append(errs, fmt.Errorf("storage.prometheus.read_url: required when engine is prometheus"))
		}
	}
	if c.Storage.Engine == "victoriametrics" {
		if c.Storage.VictoriaMetrics.WriteURL == "" {
			errs = append(errs, fmt.Errorf("storage.victoriametrics.write_url: required when engine is victoriametrics"))
		}
		if c.Storage.VictoriaMetrics.ReadURL == "" {
			errs = append(errs, fmt.Errorf("storage.victoriametrics.read_url: required when engine is victoriametrics"))
		}
	}
	if c.Retention.Duration.AsDuration() < 0 {
		errs = append(errs, fmt.Errorf("retention.duration: must not be negative"))
	}
	if c.Retention.MaxSize < 0 {
		errs = append(errs, fmt.Errorf("retention.max_size: must not be negative"))
	}
	return errors.Join(errs...)
}

func validateListen(addr, field string) error {
	if addr == "" {
		return fmt.Errorf("%s: must not be empty", field)
	}
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return fmt.Errorf("%s: must contain a port (e.g., :8080 or 0.0.0.0:8080)", field)
	}
	portStr := addr[idx+1:]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("%s: invalid port %q", field, portStr)
	}
	if port < 0 || port > 65535 {
		return fmt.Errorf("%s: port must be 0-65535, got %d", field, port)
	}
	return nil
}

func parseDuration(s string) (Duration, error) {
	if numStr, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.Atoi(numStr)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		return Duration(time.Duration(days) * 24 * time.Hour), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return Duration(d), nil
}

func parseByteSize(s string) (ByteSize, error) {
	s = strings.TrimSpace(s)
	if s == "0" {
		return 0, nil
	}
	suffixes := []struct {
		suffix string
		mult   ByteSize
	}{
		{"GB", GB},
		{"MB", MB},
		{"KB", KB},
	}
	for _, sf := range suffixes {
		if numStr, ok := strings.CutSuffix(s, sf.suffix); ok {
			n, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid byte size %q: %w", s, err)
			}
			return ByteSize(n * float64(sf.mult)), nil
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size %q: %w", s, err)
	}
	return ByteSize(n), nil
}
