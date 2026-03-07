package query

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bobcob7/lil-olt-metrics/internal/store"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedStore(t *testing.T, s *store.MemStore) {
	t.Helper()
	app := s.Appender(t.Context())
	baseT := int64(1000000)
	for i := range 10 {
		ts := baseT + int64(i)*15000
		_, err := app.Append(0, labels.FromStrings("__name__", "up", "job", "myapp", "instance", "localhost:8080"), ts, 1)
		require.NoError(t, err)
		_, err = app.Append(0, labels.FromStrings("__name__", "http_requests_total", "job", "myapp", "method", "GET"), ts, float64(100+i*10))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
}

func newTestAPI(t *testing.T) (*API, *store.MemStore) {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	ms := store.NewMemStore(logger, 24*time.Hour)
	t.Cleanup(func() { _ = ms.Close() })
	seedStore(t, ms)
	queryable := store.NewQueryable(ms)
	engine := promql.NewEngine(promql.EngineOpts{
		Logger:               logger,
		MaxSamples:           50000000,
		Timeout:              30 * time.Second,
		LookbackDelta:        5 * time.Minute,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
	})
	api := NewAPI(logger, queryable, engine, 5*time.Minute, BuildInfo{
		Version:  "0.1.0-test",
		Revision: "abc123",
		Branch:   "main",
	})
	return api, ms
}

func doGet(handler http.Handler, path string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	handler.ServeHTTP(rr, req)
	return rr
}

func doPost(handler http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rr, req)
	return rr
}

func parseResponse(t *testing.T, rr *httptest.ResponseRecorder) apiResponse {
	t.Helper()
	var resp apiResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	return resp
}

func TestInstantQuery(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	ts := strconv.FormatInt(1000000+9*15000, 10)
	rr := doGet(handler, "/api/v1/query?query=up&time="+ts+"e-3")
	assert.Equal(t, http.StatusOK, rr.Code)
	resp := parseResponse(t, rr)
	assert.Equal(t, "success", resp.Status)
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "vector", data["resultType"])
}

func TestInstantQueryPOST(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	ts := strconv.FormatFloat(float64(1000000+9*15000)/1000.0, 'f', 3, 64)
	form := url.Values{"query": {"up"}, "time": {ts}}
	rr := doPost(handler, "/api/v1/query", form)
	resp := parseResponse(t, rr)
	assert.Equal(t, "success", resp.Status)
}

func TestRangeQuery(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	startSec := strconv.FormatFloat(float64(1000000)/1000.0, 'f', 3, 64)
	endSec := strconv.FormatFloat(float64(1000000+9*15000)/1000.0, 'f', 3, 64)
	rr := doGet(handler, "/api/v1/query_range?query=up&start="+startSec+"&end="+endSec+"&step=15s")
	assert.Equal(t, http.StatusOK, rr.Code)
	resp := parseResponse(t, rr)
	assert.Equal(t, "success", resp.Status)
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "matrix", data["resultType"])
}

func TestSeriesEndpoint(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	rr := doGet(handler, "/api/v1/series?match[]=up")
	assert.Equal(t, http.StatusOK, rr.Code)
	resp := parseResponse(t, rr)
	assert.Equal(t, "success", resp.Status)
	data, ok := resp.Data.([]any)
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(data), 1)
}

func TestSeriesEndpointMissingMatch(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	rr := doGet(handler, "/api/v1/series")
	resp := parseResponse(t, rr)
	assert.Equal(t, "error", resp.Status)
	assert.Equal(t, "bad_data", resp.ErrorType)
}

func TestLabelNames(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	rr := doGet(handler, "/api/v1/labels")
	assert.Equal(t, http.StatusOK, rr.Code)
	resp := parseResponse(t, rr)
	assert.Equal(t, "success", resp.Status)
	data, ok := resp.Data.([]any)
	require.True(t, ok)
	names := make([]string, len(data))
	for i, v := range data {
		names[i] = v.(string)
	}
	assert.Contains(t, names, "__name__")
	assert.Contains(t, names, "job")
}

func TestLabelValues(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	rr := doGet(handler, "/api/v1/label/__name__/values")
	assert.Equal(t, http.StatusOK, rr.Code)
	resp := parseResponse(t, rr)
	assert.Equal(t, "success", resp.Status)
	data, ok := resp.Data.([]any)
	require.True(t, ok)
	values := make([]string, len(data))
	for i, v := range data {
		values[i] = v.(string)
	}
	assert.Contains(t, values, "up")
	assert.Contains(t, values, "http_requests_total")
}

func TestMetadata(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	rr := doGet(handler, "/api/v1/metadata")
	assert.Equal(t, http.StatusOK, rr.Code)
	resp := parseResponse(t, rr)
	assert.Equal(t, "success", resp.Status)
}

func TestBuildInfo(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	rr := doGet(handler, "/api/v1/status/buildinfo")
	assert.Equal(t, http.StatusOK, rr.Code)
	resp := parseResponse(t, rr)
	assert.Equal(t, "success", resp.Status)
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "0.1.0-test", data["version"])
	assert.Equal(t, "abc123", data["revision"])
}

func TestInvalidQuery(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	rr := doGet(handler, "/api/v1/query?query=invalid{{{&time=1000")
	assert.Equal(t, http.StatusOK, rr.Code)
	resp := parseResponse(t, rr)
	assert.Equal(t, "error", resp.Status)
	assert.Equal(t, "bad_data", resp.ErrorType)
}

func TestMissingQueryParam(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	rr := doGet(handler, "/api/v1/query")
	resp := parseResponse(t, rr)
	assert.Equal(t, "error", resp.Status)
	assert.Equal(t, "bad_data", resp.ErrorType)
}

func TestRangeQueryMissingParams(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	rr := doGet(handler, "/api/v1/query_range?query=up")
	resp := parseResponse(t, rr)
	assert.Equal(t, "error", resp.Status)
}

func TestCORSHeaders(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	rr := doGet(handler, "/api/v1/labels")
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, POST, OPTIONS", rr.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type", rr.Header().Get("Access-Control-Allow-Headers"))
}

func TestCORSPreflight(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/query", nil)
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, rr.Body.String())
}

func TestDashboardRedirect(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	rr := doGet(handler, "/dashboard")
	assert.Equal(t, http.StatusMovedPermanently, rr.Code)
	assert.Equal(t, "/dashboard/", rr.Header().Get("Location"))
}

func TestDashboardSPA(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	rr := doGet(handler, "/dashboard/")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, rr.Body.String(), "Claude Code Metrics")
}

func TestRangeQueryDayStep(t *testing.T) {
	t.Parallel()
	api, _ := newTestAPI(t)
	handler := api.Handler()
	startSec := strconv.FormatFloat(float64(1000000)/1000.0, 'f', 3, 64)
	endSec := strconv.FormatFloat(float64(1000000+9*15000)/1000.0, 'f', 3, 64)
	rr := doGet(handler, "/api/v1/query_range?query=up&start="+startSec+"&end="+endSec+"&step=1d")
	assert.Equal(t, http.StatusOK, rr.Code)
	resp := parseResponse(t, rr)
	assert.Equal(t, "success", resp.Status)
}
