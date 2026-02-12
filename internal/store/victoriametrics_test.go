package store

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVMRemote_AppendAndCommit(t *testing.T) {
	t.Parallel()
	var received []vmImportRow
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/import", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		dec := json.NewDecoder(r.Body)
		for dec.More() {
			var row vmImportRow
			require.NoError(t, dec.Decode(&row))
			received = append(received, row)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	vm := NewVMRemote(logger, VMRemoteConfig{
		WriteURL: srv.URL,
	})
	ctx := t.Context()
	app := vm.Appender(ctx)
	lset := labels.FromStrings("__name__", "cpu_seconds_total", "host", "node1")
	now := time.Now().UnixMilli()
	_, err := app.Append(0, lset, now, 100.0)
	require.NoError(t, err)
	_, err = app.Append(0, lset, now+1000, 101.0)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Len(t, received, 1, "same series should be grouped into one row")
	assert.Equal(t, "cpu_seconds_total", received[0].Metric["__name__"])
	assert.Equal(t, []float64{100.0, 101.0}, received[0].Values)
	assert.Len(t, received[0].Timestamps, 2)
}

func TestVMRemote_BasicAuth(t *testing.T) {
	t.Parallel()
	var gotUser, gotPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	vm := NewVMRemote(logger, VMRemoteConfig{
		WriteURL: srv.URL,
		Username: "admin",
		Password: "secret",
	})
	app := vm.Appender(t.Context())
	_, err := app.Append(0, labels.FromStrings("__name__", "x"), 1, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	assert.Equal(t, "admin", gotUser)
	assert.Equal(t, "secret", gotPass)
}

func TestVMRemote_RetryOn5xx(t *testing.T) {
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
	vm := NewVMRemote(logger, VMRemoteConfig{
		WriteURL:   srv.URL,
		MaxRetries: 3,
	})
	app := vm.Appender(t.Context())
	_, err := app.Append(0, labels.FromStrings("__name__", "x"), 1, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	assert.Equal(t, int32(3), attempts.Load())
}

func TestVMRemote_4xxNoRetry(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	vm := NewVMRemote(logger, VMRemoteConfig{
		WriteURL:   srv.URL,
		MaxRetries: 3,
	})
	app := vm.Appender(t.Context())
	_, err := app.Append(0, labels.FromStrings("__name__", "x"), 1, 1)
	require.NoError(t, err)
	err = app.Commit()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
	assert.Equal(t, int32(1), attempts.Load())
}

func TestVMRemote_DropAfterMaxRetries(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	vm := NewVMRemote(logger, VMRemoteConfig{
		WriteURL:   srv.URL,
		MaxRetries: 2,
	})
	app := vm.Appender(t.Context())
	_, err := app.Append(0, labels.FromStrings("__name__", "x"), 1, 1)
	require.NoError(t, err)
	err = app.Commit()
	assert.NoError(t, err, "should drop batch instead of returning error")
}

func TestVMRemote_Rollback(t *testing.T) {
	t.Parallel()
	var called atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	vm := NewVMRemote(logger, VMRemoteConfig{
		WriteURL: srv.URL,
	})
	app := vm.Appender(t.Context())
	_, err := app.Append(0, labels.FromStrings("__name__", "x"), 1, 1)
	require.NoError(t, err)
	require.NoError(t, app.Rollback())
	assert.False(t, called.Load(), "rollback should not send any data")
}

func TestVMRemote_EmptyCommit(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	vm := NewVMRemote(logger, VMRemoteConfig{
		WriteURL: "http://localhost:1",
	})
	app := vm.Appender(t.Context())
	assert.NoError(t, app.Commit())
}

func TestVMRemote_Select(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/export", r.URL.Path)
		assert.NotEmpty(t, r.URL.Query().Get("match[]"))
		w.Header().Set("Content-Type", "application/json")
		row := vmImportRow{
			Metric:     map[string]string{"__name__": "up", "job": "test"},
			Values:     []float64{1.0, 1.0},
			Timestamps: []int64{1000, 2000},
		}
		enc := json.NewEncoder(w)
		_ = enc.Encode(row)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	vm := NewVMRemote(logger, VMRemoteConfig{
		WriteURL: srv.URL,
		ReadURL:  srv.URL,
	})
	ss := vm.Select(t.Context(), false, 0, 3000,
		labels.MustNewMatcher(labels.MatchEqual, "__name__", "up"))
	require.True(t, ss.Next())
	s := ss.At()
	assert.Equal(t, "up", s.Labels().Get("__name__"))
	assert.Equal(t, "test", s.Labels().Get("job"))
	samples := s.Samples()
	require.Len(t, samples, 2)
	assert.Equal(t, 1.0, samples[0].V)
	assert.False(t, ss.Next())
}

func TestVMRemote_SelectNoReadURL(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	vm := NewVMRemote(logger, VMRemoteConfig{
		WriteURL: "http://localhost:1",
	})
	ss := vm.Select(t.Context(), false, 0, 100)
	assert.False(t, ss.Next())
}

func TestVMRemote_SelectWithBasicAuth(t *testing.T) {
	t.Parallel()
	var gotUser, gotPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	vm := NewVMRemote(logger, VMRemoteConfig{
		WriteURL: srv.URL,
		ReadURL:  srv.URL,
		Username: "reader",
		Password: "pass123",
	})
	ss := vm.Select(t.Context(), false, 0, 100,
		labels.MustNewMatcher(labels.MatchEqual, "__name__", "x"))
	for ss.Next() {
	}
	assert.Equal(t, "reader", gotUser)
	assert.Equal(t, "pass123", gotPass)
}

func TestVMRemote_BatchSplit(t *testing.T) {
	t.Parallel()
	var batches atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		batches.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	vm := NewVMRemote(logger, VMRemoteConfig{
		WriteURL:  srv.URL,
		BatchSize: 2,
	})
	app := vm.Appender(t.Context())
	for i := range 5 {
		_, err := app.Append(0, labels.FromStrings("__name__", "x", "i", string(rune('0'+i))), int64(i), float64(i))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
	assert.Equal(t, int32(3), batches.Load(), "5 series / batch size 2 = 3 batches")
}

func TestBuildMatcherQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		matchers []*labels.Matcher
		want     string
	}{
		{
			name: "empty",
			want: `{__name__=~".+"}`,
		},
		{
			name:     "equal",
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "__name__", "up")},
			want:     `{__name__="up"}`,
		},
		{
			name:     "not_equal",
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, "job", "test")},
			want:     `{job!="test"}`,
		},
		{
			name:     "regex",
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "__name__", "cpu.*")},
			want:     `{__name__=~"cpu.*"}`,
		},
		{
			name: "multiple",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__name__", "up"),
				labels.MustNewMatcher(labels.MatchNotRegexp, "instance", "localhost.*"),
			},
			want: `{__name__="up",instance!~"localhost.*"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildMatcherQuery(tt.matchers)
			assert.Equal(t, tt.want, got)
		})
	}
}
