package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/bobcob7/lil-olt-metrics/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGRPCExportSuccess(t *testing.T) {
	t.Parallel()
	app := &recordingAppender{}
	h := newTestGRPCHandler(app, 5, nil)
	resp, err := h.Export(t.Context(), validRequest())
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Nil(t, resp.PartialSuccess)
	assert.True(t, app.committed)
}

func TestGRPCExportNilRequest(t *testing.T) {
	t.Parallel()
	app := &recordingAppender{}
	h := newTestGRPCHandler(app, 0, nil)
	resp, err := h.Export(t.Context(), nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, app.committed)
}

func TestGRPCExportEmptyRequest(t *testing.T) {
	t.Parallel()
	app := &recordingAppender{}
	h := newTestGRPCHandler(app, 0, nil)
	resp, err := h.Export(t.Context(), &colmetricspb.ExportMetricsServiceRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, app.committed)
}

func TestGRPCExportTranslateError_ZeroSamples(t *testing.T) {
	t.Parallel()
	app := &recordingAppender{}
	h := newTestGRPCHandler(app, 0, errors.New("bad metric"))
	_, err := h.Export(t.Context(), validRequest())
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.True(t, app.rolledBack)
}

func TestGRPCExportPartialSuccess(t *testing.T) {
	t.Parallel()
	app := &recordingAppender{}
	h := newTestGRPCHandler(app, 3, errors.New("partial fail"))
	resp, err := h.Export(t.Context(), validRequest())
	require.NoError(t, err)
	require.NotNil(t, resp.PartialSuccess)
	assert.Contains(t, resp.PartialSuccess.ErrorMessage, "partial fail")
	assert.True(t, app.committed)
}

func TestGRPCExportCommitError(t *testing.T) {
	t.Parallel()
	app := &recordingAppender{commitErr: errors.New("commit boom")}
	h := newTestGRPCHandler(app, 5, nil)
	_, err := h.Export(t.Context(), validRequest())
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.True(t, app.rolledBack)
}

func newTestGRPCHandler(app *recordingAppender, translateCount int, translateErr error) *GRPCHandler {
	tr := &metricsTranslatorMock{
		TranslateFunc: func(_ *colmetricspb.ExportMetricsServiceRequest, _ store.Appender) (int, error) {
			return translateCount, translateErr
		},
	}
	st := &metricsStoreMock{
		AppenderFunc: func(_ context.Context) store.Appender {
			return app
		},
	}
	return NewGRPCHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), tr, st)
}

func validRequest() *colmetricspb.ExportMetricsServiceRequest {
	return &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{{
					Name: "test_metric",
					Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
						DataPoints: []*metricspb.NumberDataPoint{{
							TimeUnixNano: 1000000000000,
							Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 1.0},
						}},
					}},
				}},
			}},
		}},
	}
}
