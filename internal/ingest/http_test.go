package ingest

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobcob7/lil-olt-metrics/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/proto"
)

func TestHTTPExportSuccess(t *testing.T) {
	t.Parallel()
	app := &recordingAppender{}
	h := newTestHTTPHandler(app, 5, nil)
	body := marshalRequest(t, validRequest())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-protobuf")
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/x-protobuf", rr.Header().Get("Content-Type"))
	assert.True(t, app.committed)
}

func TestHTTPExportGzip(t *testing.T) {
	t.Parallel()
	app := &recordingAppender{}
	h := newTestHTTPHandler(app, 5, nil)
	body := marshalRequest(t, validRequest())
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write(body)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/metrics", &buf)
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "gzip")
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, app.committed)
}

func TestHTTPExportEmptyRequest(t *testing.T) {
	t.Parallel()
	app := &recordingAppender{}
	h := newTestHTTPHandler(app, 0, nil)
	body := marshalRequest(t, &colmetricspb.ExportMetricsServiceRequest{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.False(t, app.committed)
}

func TestHTTPExportMalformedPayload(t *testing.T) {
	t.Parallel()
	app := &recordingAppender{}
	h := newTestHTTPHandler(app, 0, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader([]byte("not proto")))
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHTTPExportMethodNotAllowed(t *testing.T) {
	t.Parallel()
	app := &recordingAppender{}
	h := newTestHTTPHandler(app, 0, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/metrics", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHTTPExportPartialSuccess(t *testing.T) {
	t.Parallel()
	app := &recordingAppender{}
	h := newTestHTTPHandler(app, 3, errors.New("partial fail"))
	body := marshalRequest(t, validRequest())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	resp := &colmetricspb.ExportMetricsServiceResponse{}
	require.NoError(t, proto.Unmarshal(rr.Body.Bytes(), resp))
	require.NotNil(t, resp.PartialSuccess)
	assert.Contains(t, resp.PartialSuccess.ErrorMessage, "partial fail")
	assert.True(t, app.committed)
}

func TestHTTPExportTranslateError_ZeroSamples(t *testing.T) {
	t.Parallel()
	app := &recordingAppender{}
	h := newTestHTTPHandler(app, 0, errors.New("bad metric"))
	body := marshalRequest(t, validRequest())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.True(t, app.rolledBack)
}

func TestHTTPExportCommitError(t *testing.T) {
	t.Parallel()
	app := &recordingAppender{commitErr: errors.New("commit boom")}
	h := newTestHTTPHandler(app, 5, nil)
	body := marshalRequest(t, validRequest())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.True(t, app.rolledBack)
}

func newTestHTTPHandler(app *recordingAppender, translateCount int, translateErr error) *HTTPHandler {
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
	return NewHTTPHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), tr, st, 4194304)
}

func marshalRequest(t *testing.T, req *colmetricspb.ExportMetricsServiceRequest) []byte {
	t.Helper()
	data, err := proto.Marshal(req)
	require.NoError(t, err)
	return data
}
