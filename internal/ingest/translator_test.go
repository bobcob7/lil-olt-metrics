package ingest

import (
	"io"
	"log/slog"
	"testing"

	"github.com/bobcob7/lil-olt-metrics/internal/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func defaultTranslationConfig() config.TranslationConfig {
	return config.TranslationConfig{
		ResourceAttributes: config.ResourceAttributesConfig{
			LabelMap: map[string]string{
				"service.name":        "job",
				"service.instance.id": "instance",
			},
			Promote: []string{},
		},
		SanitizeMetricNames: true,
		AddUnitSuffix:       true,
		AddTypeSuffix:       true,
	}
}

func TestTranslateGauge(t *testing.T) {
	t.Parallel()
	tr := NewTranslator(testLogger(), defaultTranslationConfig())
	app := &recordingAppender{}
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{strAttr("service.name", "myapp")},
			},
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Scope: &commonpb.InstrumentationScope{Name: "test", Version: "1.0"},
				Metrics: []*metricspb.Metric{{
					Name: "process.cpu.time",
					Unit: "s",
					Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
						DataPoints: []*metricspb.NumberDataPoint{{
							Attributes:   []*commonpb.KeyValue{strAttr("state", "user")},
							TimeUnixNano: 1000000000000,
							Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 42.5},
						}},
					}},
				}},
			}},
		}},
	}
	count, err := tr.Translate(req, app)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	require.Len(t, app.samples, 1)
	s := app.samples[0]
	assert.Equal(t, "process_cpu_time_seconds", s.labels.Get("__name__"))
	assert.Equal(t, "myapp", s.labels.Get("job"))
	assert.Equal(t, "user", s.labels.Get("state"))
	assert.Equal(t, "test", s.labels.Get("otel_scope_name"))
	assert.Equal(t, "1.0", s.labels.Get("otel_scope_version"))
	assert.Equal(t, int64(1000000), s.t)
	assert.Equal(t, 42.5, s.v)
}

func TestTranslateGaugeInt(t *testing.T) {
	t.Parallel()
	tr := NewTranslator(testLogger(), defaultTranslationConfig())
	app := &recordingAppender{}
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "active_connections",
					Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
						DataPoints: []*metricspb.NumberDataPoint{{
							TimeUnixNano: 2000000000000,
							Value:        &metricspb.NumberDataPoint_AsInt{AsInt: 100},
						}},
					}},
				}},
			}},
		}},
	}
	count, err := tr.Translate(req, app)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, float64(100), app.samples[0].v)
}

func TestTranslateMonotonicSum(t *testing.T) {
	t.Parallel()
	tr := NewTranslator(testLogger(), defaultTranslationConfig())
	app := &recordingAppender{}
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "http.server.requests",
					Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
						AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
						IsMonotonic:            true,
						DataPoints: []*metricspb.NumberDataPoint{{
							Attributes:   []*commonpb.KeyValue{strAttr("method", "GET")},
							TimeUnixNano: 1000000000000,
							Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 150},
						}},
					}},
				}},
			}},
		}},
	}
	count, err := tr.Translate(req, app)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	s := app.samples[0]
	assert.Equal(t, "http_server_requests_total", s.labels.Get("__name__"))
	assert.Equal(t, "GET", s.labels.Get("method"))
	assert.Equal(t, 150.0, s.v)
}

func TestTranslateNonMonotonicSum(t *testing.T) {
	t.Parallel()
	tr := NewTranslator(testLogger(), defaultTranslationConfig())
	app := &recordingAppender{}
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "queue.depth",
					Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
						AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
						IsMonotonic:            false,
						DataPoints: []*metricspb.NumberDataPoint{{
							TimeUnixNano: 1000000000000,
							Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 42},
						}},
					}},
				}},
			}},
		}},
	}
	count, err := tr.Translate(req, app)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, "queue_depth", app.samples[0].labels.Get("__name__"))
}

func TestTranslateHistogram(t *testing.T) {
	t.Parallel()
	tr := NewTranslator(testLogger(), defaultTranslationConfig())
	app := &recordingAppender{}
	sum := 123.45
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "http.request.duration",
					Unit: "s",
					Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{
						AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
						DataPoints: []*metricspb.HistogramDataPoint{{
							TimeUnixNano:   1000000000000,
							Count:          100,
							Sum:            &sum,
							ExplicitBounds: []float64{0.005, 0.01, 0.025, 0.05, 0.1},
							BucketCounts:   []uint64{10, 15, 25, 30, 10, 10},
						}},
					}},
				}},
			}},
		}},
	}
	count, err := tr.Translate(req, app)
	require.NoError(t, err)
	assert.Equal(t, 8, count)
	nameMap := make(map[string]int)
	for _, s := range app.samples {
		nameMap[s.labels.Get("__name__")]++
	}
	assert.Equal(t, 6, nameMap["http_request_duration_seconds_bucket"])
	assert.Equal(t, 1, nameMap["http_request_duration_seconds_count"])
	assert.Equal(t, 1, nameMap["http_request_duration_seconds_sum"])
	var infBucket *recordedSample
	for i := range app.samples {
		if app.samples[i].labels.Get("le") == "+Inf" {
			infBucket = &app.samples[i]
			break
		}
	}
	require.NotNil(t, infBucket)
	assert.Equal(t, float64(100), infBucket.v)
}

func TestTranslateResourceAttributes(t *testing.T) {
	t.Parallel()
	cfg := defaultTranslationConfig()
	cfg.ResourceAttributes.Promote = []string{"deployment.environment"}
	tr := NewTranslator(testLogger(), cfg)
	app := &recordingAppender{}
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{
					strAttr("service.name", "api"),
					strAttr("service.instance.id", "abc123"),
					strAttr("deployment.environment", "production"),
					strAttr("host.name", "server01"),
				},
			},
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "up",
					Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
						DataPoints: []*metricspb.NumberDataPoint{{
							TimeUnixNano: 1000000000000,
							Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 1},
						}},
					}},
				}},
			}},
		}},
	}
	count, err := tr.Translate(req, app)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	s := app.samples[0]
	assert.Equal(t, "api", s.labels.Get("job"))
	assert.Equal(t, "abc123", s.labels.Get("instance"))
	assert.Equal(t, "production", s.labels.Get("deployment_environment"))
	assert.Equal(t, "", s.labels.Get("host_name"))
}

func TestSanitizeMetricName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "dots to underscores", input: "http.server.requests", expected: "http_server_requests"},
		{name: "dashes to underscores", input: "my-metric-name", expected: "my_metric_name"},
		{name: "collapse underscores", input: "foo__bar___baz", expected: "foo_bar_baz"},
		{name: "leading digit", input: "123metric", expected: "_123metric"},
		{name: "already valid", input: "valid_metric_name", expected: "valid_metric_name"},
		{name: "special chars", input: "metric@#$name", expected: "metric_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, sanitizeMetricName(tt.input))
		})
	}
}

func TestUnitSuffix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		metric   string
		unit     string
		expected string
	}{
		{name: "seconds", metric: "duration", unit: "s", expected: "duration_seconds"},
		{name: "bytes", metric: "size", unit: "By", expected: "size_bytes"},
		{name: "already has suffix", metric: "duration_seconds", unit: "s", expected: "duration_seconds"},
		{name: "unknown unit", metric: "custom", unit: "widgets", expected: "custom_widgets"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, addUnitSuffix(tt.metric, tt.unit))
		})
	}
}

func TestTranslateNilRequest(t *testing.T) {
	t.Parallel()
	tr := NewTranslator(testLogger(), defaultTranslationConfig())
	app := &recordingAppender{}
	count, err := tr.Translate(nil, app)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Empty(t, app.samples)
}

func TestTranslateEmptyRequest(t *testing.T) {
	t.Parallel()
	tr := NewTranslator(testLogger(), defaultTranslationConfig())
	app := &recordingAppender{}
	count, err := tr.Translate(&colmetricspb.ExportMetricsServiceRequest{}, app)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Empty(t, app.samples)
}

func TestExtractExemplars(t *testing.T) {
	t.Parallel()
	traceID := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	spanID := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x11, 0x22}
	exemplars := []*metricspb.Exemplar{{
		TimeUnixNano: 5000000000,
		Value:        &metricspb.Exemplar_AsDouble{AsDouble: 0.123},
		TraceId:      traceID,
		SpanId:       spanID,
		FilteredAttributes: []*commonpb.KeyValue{
			strAttr("custom", "value"),
		},
	}}
	result := ExtractExemplars(exemplars)
	require.Len(t, result, 1)
	ex := result[0]
	assert.Equal(t, 0.123, ex.Value)
	assert.Equal(t, int64(5000), ex.Ts)
	assert.Equal(t, "0102030405060708090a0b0c0d0e0f10", ex.TraceID)
	assert.Equal(t, "aabbccddeeff1122", ex.SpanID)
	assert.Equal(t, "0102030405060708090a0b0c0d0e0f10", ex.Labels.Get("trace_id"))
	assert.Equal(t, "aabbccddeeff1122", ex.Labels.Get("span_id"))
	assert.Equal(t, "value", ex.Labels.Get("custom"))
}

func TestExtractExemplarsEmpty(t *testing.T) {
	t.Parallel()
	assert.Nil(t, ExtractExemplars(nil))
	assert.Nil(t, ExtractExemplars([]*metricspb.Exemplar{}))
}

func TestTranslateDeltaTemporalitySkipped(t *testing.T) {
	t.Parallel()
	tr := NewTranslator(testLogger(), defaultTranslationConfig())
	app := &recordingAppender{}
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "delta_counter",
					Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
						AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
						IsMonotonic:            true,
						DataPoints: []*metricspb.NumberDataPoint{{
							TimeUnixNano: 1000000000000,
							Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 10},
						}},
					}},
				}},
			}},
		}},
	}
	count, err := tr.Translate(req, app)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Empty(t, app.samples)
}

func TestTranslateDeltaSumConversion(t *testing.T) {
	t.Parallel()
	cfg := defaultTranslationConfig()
	cfg.DeltaConversion = true
	tr := NewTranslator(testLogger(), cfg)
	app := &recordingAppender{}
	makeReq := func(val float64, ts uint64) *colmetricspb.ExportMetricsServiceRequest {
		return &colmetricspb.ExportMetricsServiceRequest{
			ResourceMetrics: []*metricspb.ResourceMetrics{{
				ScopeMetrics: []*metricspb.ScopeMetrics{{
					Metrics: []*metricspb.Metric{{
						Name: "delta_counter",
						Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
							AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
							IsMonotonic:            true,
							DataPoints: []*metricspb.NumberDataPoint{{
								TimeUnixNano: ts,
								Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: val},
							}},
						}},
					}},
				}},
			}},
		}
	}
	count, err := tr.Translate(makeReq(10, 1000000000000), app)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, 10.0, app.samples[0].v)
	count, err = tr.Translate(makeReq(5, 2000000000000), app)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, 15.0, app.samples[1].v)
	count, err = tr.Translate(makeReq(7, 3000000000000), app)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, 22.0, app.samples[2].v)
}

func TestTranslateDeltaHistogramConversion(t *testing.T) {
	t.Parallel()
	cfg := defaultTranslationConfig()
	cfg.DeltaConversion = true
	cfg.AddUnitSuffix = false
	tr := NewTranslator(testLogger(), cfg)
	app := &recordingAppender{}
	sum1 := 10.0
	makeReq := func(counts []uint64, total uint64, sum float64, ts uint64) *colmetricspb.ExportMetricsServiceRequest {
		return &colmetricspb.ExportMetricsServiceRequest{
			ResourceMetrics: []*metricspb.ResourceMetrics{{
				ScopeMetrics: []*metricspb.ScopeMetrics{{
					Metrics: []*metricspb.Metric{{
						Name: "latency",
						Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{
							AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
							DataPoints: []*metricspb.HistogramDataPoint{{
								TimeUnixNano:   ts,
								Count:          total,
								Sum:            &sum,
								ExplicitBounds: []float64{1.0},
								BucketCounts:   counts,
							}},
						}},
					}},
				}},
			}},
		}
	}
	count, err := tr.Translate(makeReq([]uint64{2, 3}, 5, sum1, 1000000000000), app)
	require.NoError(t, err)
	assert.Equal(t, 4, count)
	sum2 := 5.0
	count, err = tr.Translate(makeReq([]uint64{1, 2}, 3, sum2, 2000000000000), app)
	require.NoError(t, err)
	assert.Equal(t, 4, count)
	countSamples := findSamples(app.samples, "latency_count")
	require.Len(t, countSamples, 2)
	assert.Equal(t, 5.0, countSamples[0].v)
	assert.Equal(t, 8.0, countSamples[1].v)
}

func TestTranslateExponentialHistogram(t *testing.T) {
	t.Parallel()
	cfg := defaultTranslationConfig()
	cfg.AddUnitSuffix = false
	tr := NewTranslator(testLogger(), cfg)
	app := &recordingAppender{}
	sum := 42.0
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "exp_hist",
					Data: &metricspb.Metric_ExponentialHistogram{ExponentialHistogram: &metricspb.ExponentialHistogram{
						AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
						DataPoints: []*metricspb.ExponentialHistogramDataPoint{{
							TimeUnixNano: 1000000000000,
							Count:        10,
							Sum:          &sum,
							ZeroCount:    2,
							Scale:        0,
							Positive: &metricspb.ExponentialHistogramDataPoint_Buckets{
								Offset:       0,
								BucketCounts: []uint64{3, 5},
							},
						}},
					}},
				}},
			}},
		}},
	}
	count, err := tr.Translate(req, app)
	require.NoError(t, err)
	nameMap := make(map[string]int)
	for _, s := range app.samples {
		nameMap[s.labels.Get("__name__")]++
	}
	assert.Equal(t, 3, nameMap["exp_hist_bucket"])
	assert.Equal(t, 1, nameMap["exp_hist_count"])
	assert.Equal(t, 1, nameMap["exp_hist_sum"])
	assert.Equal(t, 5, count)
	infSamples := findSamplesWithLabel(app.samples, "le", "+Inf")
	require.Len(t, infSamples, 1)
	assert.Equal(t, 10.0, infSamples[0].v)
}

func TestTranslateSummary(t *testing.T) {
	t.Parallel()
	cfg := defaultTranslationConfig()
	cfg.AddUnitSuffix = false
	tr := NewTranslator(testLogger(), cfg)
	app := &recordingAppender{}
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "request_duration",
					Data: &metricspb.Metric_Summary{Summary: &metricspb.Summary{
						DataPoints: []*metricspb.SummaryDataPoint{{
							TimeUnixNano: 1000000000000,
							Count:        100,
							Sum:          456.78,
							QuantileValues: []*metricspb.SummaryDataPoint_ValueAtQuantile{
								{Quantile: 0.5, Value: 1.2},
								{Quantile: 0.9, Value: 3.4},
								{Quantile: 0.99, Value: 8.9},
							},
						}},
					}},
				}},
			}},
		}},
	}
	count, err := tr.Translate(req, app)
	require.NoError(t, err)
	assert.Equal(t, 5, count)
	quantileSamples := findSamples(app.samples, "request_duration")
	require.Len(t, quantileSamples, 3)
	assert.Equal(t, "0.5", quantileSamples[0].labels.Get("quantile"))
	assert.Equal(t, 1.2, quantileSamples[0].v)
	assert.Equal(t, "0.9", quantileSamples[1].labels.Get("quantile"))
	assert.Equal(t, 3.4, quantileSamples[1].v)
	assert.Equal(t, "0.99", quantileSamples[2].labels.Get("quantile"))
	assert.Equal(t, 8.9, quantileSamples[2].v)
	sumSamples := findSamples(app.samples, "request_duration_sum")
	require.Len(t, sumSamples, 1)
	assert.Equal(t, 456.78, sumSamples[0].v)
	countSamples := findSamples(app.samples, "request_duration_count")
	require.Len(t, countSamples, 1)
	assert.Equal(t, 100.0, countSamples[0].v)
}

func TestTranslateSchemaURL(t *testing.T) {
	t.Parallel()
	cfg := defaultTranslationConfig()
	cfg.SchemaURL = true
	tr := NewTranslator(testLogger(), cfg)
	app := &recordingAppender{}
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			SchemaUrl: "https://opentelemetry.io/schemas/1.21.0",
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "up",
					Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
						DataPoints: []*metricspb.NumberDataPoint{{
							TimeUnixNano: 1000000000000,
							Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 1},
						}},
					}},
				}},
			}},
		}},
	}
	count, err := tr.Translate(req, app)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, "https://opentelemetry.io/schemas/1.21.0", app.samples[0].labels.Get("__schema_url__"))
}

func TestTranslateSchemaURLScopeOverride(t *testing.T) {
	t.Parallel()
	cfg := defaultTranslationConfig()
	cfg.SchemaURL = true
	tr := NewTranslator(testLogger(), cfg)
	app := &recordingAppender{}
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			SchemaUrl: "https://opentelemetry.io/schemas/1.20.0",
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				SchemaUrl: "https://opentelemetry.io/schemas/1.21.0",
				Metrics: []*metricspb.Metric{{
					Name: "up",
					Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
						DataPoints: []*metricspb.NumberDataPoint{{
							TimeUnixNano: 1000000000000,
							Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 1},
						}},
					}},
				}},
			}},
		}},
	}
	count, err := tr.Translate(req, app)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, "https://opentelemetry.io/schemas/1.21.0", app.samples[0].labels.Get("__schema_url__"))
}

func TestTranslateSchemaURLDisabled(t *testing.T) {
	t.Parallel()
	cfg := defaultTranslationConfig()
	cfg.SchemaURL = false
	tr := NewTranslator(testLogger(), cfg)
	app := &recordingAppender{}
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			SchemaUrl: "https://opentelemetry.io/schemas/1.21.0",
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "up",
					Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
						DataPoints: []*metricspb.NumberDataPoint{{
							TimeUnixNano: 1000000000000,
							Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 1},
						}},
					}},
				}},
			}},
		}},
	}
	count, err := tr.Translate(req, app)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, "", app.samples[0].labels.Get("__schema_url__"))
}

func findSamples(samples []recordedSample, name string) []recordedSample {
	var result []recordedSample
	for _, s := range samples {
		if s.labels.Get("__name__") == name {
			result = append(result, s)
		}
	}
	return result
}

func findSamplesWithLabel(samples []recordedSample, labelName, labelValue string) []recordedSample {
	var result []recordedSample
	for _, s := range samples {
		if s.labels.Get(labelName) == labelValue {
			result = append(result, s)
		}
	}
	return result
}

// Test helpers

type recordedSample struct {
	labels labels.Labels
	t      int64
	v      float64
}

type recordingAppender struct {
	samples    []recordedSample
	committed  bool
	rolledBack bool
	commitErr  error
}

func (a *recordingAppender) Append(_ storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	a.samples = append(a.samples, recordedSample{labels: l, t: t, v: v})
	return 0, nil
}

func (a *recordingAppender) Commit() error {
	if a.commitErr != nil {
		return a.commitErr
	}
	a.committed = true
	return nil
}

func (a *recordingAppender) Rollback() error {
	a.rolledBack = true
	return nil
}

func strAttr(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key:   key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}},
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}
