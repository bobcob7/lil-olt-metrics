//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/bobcob7/lil-olt-metrics/internal/config"
	"github.com/bobcob7/lil-olt-metrics/internal/ingest"
	"github.com/bobcob7/lil-olt-metrics/internal/query"
	"github.com/bobcob7/lil-olt-metrics/internal/store"
	"github.com/prometheus/prometheus/promql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type testServer struct {
	grpcAddr  string
	httpAddr  string
	queryAddr string
	store     store.Store
	grpcSrv   *grpc.Server
	httpSrv   *http.Server
	querySrv  *http.Server
}

func startTestServer(t *testing.T) *testServer {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	s := store.NewMemStore(logger, 24*time.Hour)
	translator := ingest.NewTranslator(logger, config.TranslationConfig{
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
		DeltaConversion:     true,
		SchemaURL:           true,
	})
	grpcSrv := grpc.NewServer()
	grpcHandler := ingest.NewGRPCHandler(logger, translator, s)
	colmetricspb.RegisterMetricsServiceServer(grpcSrv, grpcHandler)
	grpcLis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = grpcSrv.Serve(grpcLis) }()
	httpHandler := ingest.NewHTTPHandler(logger, translator, s, 4194304)
	httpMux := http.NewServeMux()
	httpMux.Handle("/v1/metrics", httpHandler)
	httpSrv := &http.Server{Handler: httpMux}
	httpLis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = httpSrv.Serve(httpLis) }()
	queryable := store.NewQueryable(s)
	engine := promql.NewEngine(promql.EngineOpts{
		Logger:               logger,
		MaxSamples:           50000000,
		Timeout:              30 * time.Second,
		LookbackDelta:        5 * time.Minute,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
	})
	api := query.NewAPI(logger, queryable, engine, 5*time.Minute, query.BuildInfo{
		Version: "test", Revision: "test", Branch: "test",
	})
	queryMux := http.NewServeMux()
	queryMux.Handle("/", api.Handler())
	querySrv := &http.Server{Handler: queryMux}
	queryLis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = querySrv.Serve(queryLis) }()
	ts := &testServer{
		grpcAddr:  grpcLis.Addr().String(),
		httpAddr:  httpLis.Addr().String(),
		queryAddr: queryLis.Addr().String(),
		store:     s,
		grpcSrv:   grpcSrv,
		httpSrv:   httpSrv,
		querySrv:  querySrv,
	}
	t.Cleanup(func() {
		grpcSrv.GracefulStop()
		_ = httpSrv.Close()
		_ = querySrv.Close()
		_ = s.Close()
	})
	return ts
}

func (ts *testServer) grpcClient(t *testing.T) colmetricspb.MetricsServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(ts.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return colmetricspb.NewMetricsServiceClient(conn)
}

func (ts *testServer) queryURL(path string) string {
	return fmt.Sprintf("http://%s%s", ts.queryAddr, path)
}

func (ts *testServer) httpIngestURL() string {
	return fmt.Sprintf("http://%s/v1/metrics", ts.httpAddr)
}

type queryResult struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
	Error  string          `json:"error,omitempty"`
}

type instantData struct {
	ResultType string            `json:"resultType"`
	Result     []json.RawMessage `json:"result"`
}

type vectorSample struct {
	Metric map[string]string `json:"metric"`
	Value  [2]json.RawMessage
}

func TestGRPCIngestGaugeAndQuery(t *testing.T) {
	t.Parallel()
	ts := startTestServer(t)
	client := ts.grpcClient(t)
	sampleTime := time.Now().Truncate(time.Second)
	tsNano := uint64(sampleTime.UnixNano())
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{strAttr("service.name", "myapp")},
			},
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "temperature",
					Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
						DataPoints: []*metricspb.NumberDataPoint{{
							TimeUnixNano: tsNano,
							Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 72.5},
						}},
					}},
				}},
			}},
		}},
	}
	resp, err := client.Export(t.Context(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	result := instantQuery(t, ts, "temperature", sampleTime.Add(1*time.Second))
	require.Equal(t, "success", result.Status)
	var data instantData
	require.NoError(t, json.Unmarshal(result.Data, &data))
	assert.Equal(t, "vector", data.ResultType)
	require.Len(t, data.Result, 1)
	var sample vectorSample
	require.NoError(t, json.Unmarshal(data.Result[0], &sample))
	assert.Equal(t, "temperature", sample.Metric["__name__"])
	assert.Equal(t, "myapp", sample.Metric["job"])
	var valStr string
	require.NoError(t, json.Unmarshal(sample.Value[1], &valStr))
	assert.Equal(t, "72.5", valStr)
}

func TestHTTPProtobufIngestAndQuery(t *testing.T) {
	t.Parallel()
	ts := startTestServer(t)
	now := time.Now()
	tsNano := uint64(now.UnixNano())
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "http_requests",
					Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
						AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
						IsMonotonic:            true,
						DataPoints: []*metricspb.NumberDataPoint{{
							TimeUnixNano: tsNano,
							Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 100},
						}},
					}},
				}},
			}},
		}},
	}
	body, err := proto.Marshal(req)
	require.NoError(t, err)
	httpReq, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.httpIngestURL(), bytes.NewReader(body))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpResp, err := http.DefaultClient.Do(httpReq)
	require.NoError(t, err)
	defer httpResp.Body.Close()
	assert.Equal(t, http.StatusOK, httpResp.StatusCode)
	result := instantQuery(t, ts, "http_requests_total", now.Add(1*time.Second))
	require.Equal(t, "success", result.Status)
	var data instantData
	require.NoError(t, json.Unmarshal(result.Data, &data))
	require.Len(t, data.Result, 1)
}

func TestHTTPJSONIngestAndQuery(t *testing.T) {
	t.Parallel()
	ts := startTestServer(t)
	now := time.Now()
	tsNano := uint64(now.UnixNano())
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "json_gauge",
					Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
						DataPoints: []*metricspb.NumberDataPoint{{
							TimeUnixNano: tsNano,
							Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 99.9},
						}},
					}},
				}},
			}},
		}},
	}
	body, err := protojson.Marshal(req)
	require.NoError(t, err)
	httpReq, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.httpIngestURL(), bytes.NewReader(body))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := http.DefaultClient.Do(httpReq)
	require.NoError(t, err)
	defer httpResp.Body.Close()
	assert.Equal(t, http.StatusOK, httpResp.StatusCode)
	result := instantQuery(t, ts, "json_gauge", now.Add(1*time.Second))
	require.Equal(t, "success", result.Status)
	var data instantData
	require.NoError(t, json.Unmarshal(result.Data, &data))
	require.Len(t, data.Result, 1)
	var sample vectorSample
	require.NoError(t, json.Unmarshal(data.Result[0], &sample))
	var valStr string
	require.NoError(t, json.Unmarshal(sample.Value[1], &valStr))
	assert.Equal(t, "99.9", valStr)
}

func TestIngestHistogramAndQuery(t *testing.T) {
	t.Parallel()
	ts := startTestServer(t)
	client := ts.grpcClient(t)
	now := time.Now()
	tsNano := uint64(now.UnixNano())
	sum := 45.0
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "request_duration",
					Unit: "s",
					Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{
						AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
						DataPoints: []*metricspb.HistogramDataPoint{{
							TimeUnixNano:   tsNano,
							Count:          50,
							Sum:            &sum,
							ExplicitBounds: []float64{0.01, 0.05, 0.1, 0.5, 1.0},
							BucketCounts:   []uint64{5, 10, 15, 10, 5, 5},
						}},
					}},
				}},
			}},
		}},
	}
	resp, err := client.Export(t.Context(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	result := instantQuery(t, ts, `histogram_quantile(0.95, request_duration_seconds_bucket)`, now.Add(1*time.Second))
	require.Equal(t, "success", result.Status)
	var data instantData
	require.NoError(t, json.Unmarshal(result.Data, &data))
	require.Len(t, data.Result, 1)
}

func TestSeriesAndLabelAPIs(t *testing.T) {
	t.Parallel()
	ts := startTestServer(t)
	client := ts.grpcClient(t)
	now := time.Now()
	tsNano := uint64(now.UnixNano())
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{strAttr("service.name", "web")},
			},
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{
					{
						Name: "cpu_usage",
						Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
							DataPoints: []*metricspb.NumberDataPoint{{
								Attributes:   []*commonpb.KeyValue{strAttr("host", "server1")},
								TimeUnixNano: tsNano,
								Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 0.8},
							}},
						}},
					},
					{
						Name: "mem_usage",
						Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
							DataPoints: []*metricspb.NumberDataPoint{{
								Attributes:   []*commonpb.KeyValue{strAttr("host", "server1")},
								TimeUnixNano: tsNano,
								Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 0.6},
							}},
						}},
					},
				},
			}},
		}},
	}
	_, err := client.Export(t.Context(), req)
	require.NoError(t, err)
	start := now.Add(-1 * time.Minute).Unix()
	end := now.Add(1 * time.Minute).Unix()
	labelsResp := httpGet(t, ts.queryURL(fmt.Sprintf("/api/v1/labels?start=%d&end=%d", start, end)))
	require.Equal(t, "success", labelsResp.Status)
	var labelNames []string
	require.NoError(t, json.Unmarshal(labelsResp.Data, &labelNames))
	assert.Contains(t, labelNames, "__name__")
	assert.Contains(t, labelNames, "job")
	assert.Contains(t, labelNames, "host")
	valuesResp := httpGet(t, ts.queryURL(fmt.Sprintf("/api/v1/label/__name__/values?start=%d&end=%d", start, end)))
	require.Equal(t, "success", valuesResp.Status)
	var values []string
	require.NoError(t, json.Unmarshal(valuesResp.Data, &values))
	assert.Contains(t, values, "cpu_usage")
	assert.Contains(t, values, "mem_usage")
}

func TestDeltaConversionEndToEnd(t *testing.T) {
	t.Parallel()
	ts := startTestServer(t)
	client := ts.grpcClient(t)
	now := time.Now()
	for i, val := range []float64{10, 5, 3} {
		tsNano := uint64(now.Add(time.Duration(i) * time.Second).UnixNano())
		req := &colmetricspb.ExportMetricsServiceRequest{
			ResourceMetrics: []*metricspb.ResourceMetrics{{
				ScopeMetrics: []*metricspb.ScopeMetrics{{
					Metrics: []*metricspb.Metric{{
						Name: "delta_requests",
						Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
							AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
							IsMonotonic:            true,
							DataPoints: []*metricspb.NumberDataPoint{{
								TimeUnixNano: tsNano,
								Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: val},
							}},
						}},
					}},
				}},
			}},
		}
		_, err := client.Export(t.Context(), req)
		require.NoError(t, err)
	}
	result := instantQuery(t, ts, "delta_requests_total", now.Add(3*time.Second))
	require.Equal(t, "success", result.Status)
	var data instantData
	require.NoError(t, json.Unmarshal(result.Data, &data))
	require.Len(t, data.Result, 1)
	var sample vectorSample
	require.NoError(t, json.Unmarshal(data.Result[0], &sample))
	var valStr string
	require.NoError(t, json.Unmarshal(sample.Value[1], &valStr))
	assert.Equal(t, "18", valStr)
}

func TestConcurrentIngestAndQuery(t *testing.T) {
	t.Parallel()
	ts := startTestServer(t)
	client := ts.grpcClient(t)
	now := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 50 {
			tsNano := uint64(now.Add(time.Duration(i) * time.Millisecond).UnixNano())
			req := &colmetricspb.ExportMetricsServiceRequest{
				ResourceMetrics: []*metricspb.ResourceMetrics{{
					ScopeMetrics: []*metricspb.ScopeMetrics{{
						Metrics: []*metricspb.Metric{{
							Name: "concurrent_gauge",
							Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
								DataPoints: []*metricspb.NumberDataPoint{{
									TimeUnixNano: tsNano,
									Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: float64(i)},
								}},
							}},
						}},
					}},
				}},
			}
			_, _ = client.Export(t.Context(), req)
		}
	}()
	for range 10 {
		_ = instantQuery(t, ts, "concurrent_gauge", now.Add(1*time.Second))
	}
	<-done
	result := instantQuery(t, ts, "concurrent_gauge", now.Add(1*time.Second))
	assert.Equal(t, "success", result.Status)
}

func TestCounterRateRangeQuery(t *testing.T) {
	t.Parallel()
	ts := startTestServer(t)
	client := ts.grpcClient(t)
	now := time.Now().Truncate(time.Second)
	for i := range 5 {
		tsNano := uint64(now.Add(time.Duration(i*15) * time.Second).UnixNano())
		req := &colmetricspb.ExportMetricsServiceRequest{
			ResourceMetrics: []*metricspb.ResourceMetrics{{
				ScopeMetrics: []*metricspb.ScopeMetrics{{
					Metrics: []*metricspb.Metric{{
						Name: "api_calls",
						Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
							AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
							IsMonotonic:            true,
							DataPoints: []*metricspb.NumberDataPoint{{
								TimeUnixNano: tsNano,
								Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: float64((i + 1) * 100)},
							}},
						}},
					}},
				}},
			}},
		}
		_, err := client.Export(t.Context(), req)
		require.NoError(t, err)
	}
	startSec := fmt.Sprintf("%d", now.Unix())
	endSec := fmt.Sprintf("%d", now.Add(61*time.Second).Unix())
	u := fmt.Sprintf("%s?query=%s&start=%s&end=%s&step=15s",
		ts.queryURL("/api/v1/query_range"),
		url.QueryEscape("rate(api_calls_total[1m])"),
		startSec, endSec)
	result := httpGet(t, u)
	require.Equal(t, "success", result.Status)
	var data instantData
	require.NoError(t, json.Unmarshal(result.Data, &data))
	assert.Equal(t, "matrix", data.ResultType)
	require.NotEmpty(t, data.Result)
}

func TestFSPersistenceWALReplay(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dir := t.TempDir()
	sampleTime := time.Now().Truncate(time.Second)
	fsCfg := store.FSStoreConfig{
		Path:           dir,
		WALSegmentSize: 1024 * 1024,
		FlushAge:       1 * time.Hour,
	}
	fs1, err := store.NewFSStore(logger, fsCfg)
	require.NoError(t, err)
	translator := ingest.NewTranslator(logger, config.TranslationConfig{
		SanitizeMetricNames: true,
	})
	tsNano := uint64(sampleTime.UnixNano())
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "persistent_gauge",
					Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
						DataPoints: []*metricspb.NumberDataPoint{{
							TimeUnixNano: tsNano,
							Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 42.0},
						}},
					}},
				}},
			}},
		}},
	}
	app := fs1.Appender(t.Context())
	_, err = translator.Translate(req, app)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.NoError(t, fs1.Close())
	fs2, err := store.NewFSStore(logger, fsCfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = fs2.Close() })
	queryable := store.NewQueryable(fs2)
	engine := promql.NewEngine(promql.EngineOpts{
		Logger:               logger,
		MaxSamples:           50000000,
		Timeout:              30 * time.Second,
		LookbackDelta:        5 * time.Minute,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
	})
	api := query.NewAPI(logger, queryable, engine, 5*time.Minute, query.BuildInfo{
		Version: "test", Revision: "test", Branch: "test",
	})
	queryMux := http.NewServeMux()
	queryMux.Handle("/", api.Handler())
	querySrv := &http.Server{Handler: queryMux}
	queryLis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = querySrv.Serve(queryLis) }()
	t.Cleanup(func() { _ = querySrv.Close() })
	queryAddr := queryLis.Addr().String()
	u := fmt.Sprintf("http://%s/api/v1/query?query=%s&time=%d",
		queryAddr, url.QueryEscape("persistent_gauge"), sampleTime.Add(1*time.Second).Unix())
	result := httpGet(t, u)
	require.Equal(t, "success", result.Status)
	var data instantData
	require.NoError(t, json.Unmarshal(result.Data, &data))
	require.Len(t, data.Result, 1)
	var sample vectorSample
	require.NoError(t, json.Unmarshal(data.Result[0], &sample))
	var valStr string
	require.NoError(t, json.Unmarshal(sample.Value[1], &valStr))
	assert.Equal(t, "42", valStr)
}

func TestMalformedPayload(t *testing.T) {
	t.Parallel()
	ts := startTestServer(t)
	httpReq, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.httpIngestURL(), bytes.NewReader([]byte("not valid protobuf")))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpResp, err := http.DefaultClient.Do(httpReq)
	require.NoError(t, err)
	defer httpResp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, httpResp.StatusCode)
}

func TestInvalidPromQL(t *testing.T) {
	t.Parallel()
	ts := startTestServer(t)
	result := httpGet(t, ts.queryURL("/api/v1/query?query=invalid{{{"))
	assert.Equal(t, "error", result.Status)
	assert.NotEmpty(t, result.Error)
}

func TestBuildInfo(t *testing.T) {
	t.Parallel()
	ts := startTestServer(t)
	result := httpGet(t, ts.queryURL("/api/v1/status/buildinfo"))
	assert.Equal(t, "success", result.Status)
}

// Helpers

func instantQuery(t *testing.T, ts *testServer, expr string, at time.Time) queryResult {
	t.Helper()
	u := fmt.Sprintf("%s?query=%s&time=%d", ts.queryURL("/api/v1/query"), url.QueryEscape(expr), at.Unix())
	return httpGet(t, u)
}

func httpGet(t *testing.T, url string) queryResult {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var result queryResult
	require.NoError(t, json.Unmarshal(body, &result))
	return result
}

func strAttr(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key:   key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}},
	}
}
