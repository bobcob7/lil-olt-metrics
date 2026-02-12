package store

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrometheusRemote_AppendAndCommit(t *testing.T) {
	t.Parallel()
	var received atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		decoded, err := snappy.Decode(nil, body)
		if err != nil {
			http.Error(w, "bad snappy", http.StatusBadRequest)
			return
		}
		var wr prompb.WriteRequest
		if err := wr.Unmarshal(decoded); err != nil {
			http.Error(w, "bad proto", http.StatusBadRequest)
			return
		}
		received.Store(&wr)
		assert.Equal(t, "application/x-protobuf", r.Header.Get("Content-Type"))
		assert.Equal(t, "snappy", r.Header.Get("Content-Encoding"))
		assert.Equal(t, "0.1.0", r.Header.Get("X-Prometheus-Remote-Write-Version"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	remote := NewPrometheusRemote(logger, PrometheusRemoteConfig{
		WriteURL: srv.URL,
	})
	ctx := t.Context()
	app := remote.Appender(ctx)
	lset := labels.FromStrings("__name__", "test_total", "job", "myapp")
	now := time.Now().UnixMilli()
	_, err := app.Append(0, lset, now, 42.5)
	require.NoError(t, err)
	_, err = app.Append(0, lset, now+1000, 43.5)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	wr := received.Load().(*prompb.WriteRequest)
	require.Len(t, wr.Timeseries, 2)
	assert.Equal(t, "test_total", labelValue(wr.Timeseries[0].Labels, "__name__"))
	assert.Equal(t, 42.5, wr.Timeseries[0].Samples[0].Value)
}

func TestPrometheusRemote_BasicAuth(t *testing.T) {
	t.Parallel()
	var gotUser, gotPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	remote := NewPrometheusRemote(logger, PrometheusRemoteConfig{
		WriteURL: srv.URL,
		Username: "admin",
		Password: "secret",
	})
	app := remote.Appender(t.Context())
	_, err := app.Append(0, labels.FromStrings("__name__", "x"), 1, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	assert.Equal(t, "admin", gotUser)
	assert.Equal(t, "secret", gotPass)
}

func TestPrometheusRemote_RetryOn5xx(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	remote := NewPrometheusRemote(logger, PrometheusRemoteConfig{
		WriteURL:   srv.URL,
		MaxRetries: 3,
	})
	app := remote.Appender(t.Context())
	_, err := app.Append(0, labels.FromStrings("__name__", "x"), 1, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	assert.Equal(t, int32(3), attempts.Load())
}

func TestPrometheusRemote_4xxNoRetry(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	remote := NewPrometheusRemote(logger, PrometheusRemoteConfig{
		WriteURL:   srv.URL,
		MaxRetries: 3,
	})
	app := remote.Appender(t.Context())
	_, err := app.Append(0, labels.FromStrings("__name__", "x"), 1, 1)
	require.NoError(t, err)
	err = app.Commit()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
	assert.Equal(t, int32(1), attempts.Load())
}

func TestPrometheusRemote_DropAfterMaxRetries(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	remote := NewPrometheusRemote(logger, PrometheusRemoteConfig{
		WriteURL:   srv.URL,
		MaxRetries: 2,
	})
	app := remote.Appender(t.Context())
	_, err := app.Append(0, labels.FromStrings("__name__", "x"), 1, 1)
	require.NoError(t, err)
	err = app.Commit()
	assert.NoError(t, err, "should drop batch instead of returning error after max retries")
}

func TestPrometheusRemote_Rollback(t *testing.T) {
	t.Parallel()
	var called atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	remote := NewPrometheusRemote(logger, PrometheusRemoteConfig{
		WriteURL: srv.URL,
	})
	app := remote.Appender(t.Context())
	_, err := app.Append(0, labels.FromStrings("__name__", "x"), 1, 1)
	require.NoError(t, err)
	require.NoError(t, app.Rollback())
	assert.False(t, called.Load(), "rollback should not send any data")
}

func TestPrometheusRemote_EmptyCommit(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	remote := NewPrometheusRemote(logger, PrometheusRemoteConfig{
		WriteURL: "http://localhost:1",
	})
	app := remote.Appender(t.Context())
	assert.NoError(t, app.Commit())
}

func TestPrometheusRemote_SelectReturnsEmpty(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	remote := NewPrometheusRemote(logger, PrometheusRemoteConfig{
		WriteURL: "http://localhost:1",
	})
	ss := remote.Select(t.Context(), false, 0, 100)
	assert.False(t, ss.Next())
}

func TestPrometheusRemote_BatchSplit(t *testing.T) {
	t.Parallel()
	var batches atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		batches.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	remote := NewPrometheusRemote(logger, PrometheusRemoteConfig{
		WriteURL:  srv.URL,
		BatchSize: 2,
	})
	app := remote.Appender(t.Context())
	for i := range 5 {
		_, err := app.Append(0, labels.FromStrings("__name__", "x", "i", string(rune('0'+i))), int64(i), float64(i))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
	assert.Equal(t, int32(3), batches.Load(), "5 samples / batch size 2 = 3 batches")
}

func labelValue(lbls []prompb.Label, name string) string {
	for _, l := range lbls {
		if l.Name == name {
			return l.Value
		}
	}
	return ""
}
