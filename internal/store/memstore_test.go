package store

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestMemStoreWriteAndReadBack(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s := NewMemStore(testLogger(), 0)
	app := s.Appender(ctx)
	lset := labels.FromStrings("__name__", "test_metric", "job", "test")
	_, err := app.Append(0, lset, 1000, 1.0)
	require.NoError(t, err)
	_, err = app.Append(0, lset, 2000, 2.0)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	matcher := labels.MustNewMatcher(labels.MatchEqual, "__name__", "test_metric")
	ss := s.Select(ctx, false, 0, 3000, matcher)
	require.True(t, ss.Next())
	series := ss.At()
	assert.Equal(t, "test_metric", series.Labels().Get("__name__"))
	samples := series.Samples()
	require.Len(t, samples, 2)
	assert.Equal(t, int64(1000), samples[0].T)
	assert.Equal(t, 1.0, samples[0].V)
	assert.Equal(t, int64(2000), samples[1].T)
	assert.Equal(t, 2.0, samples[1].V)
	assert.False(t, ss.Next())
	assert.NoError(t, ss.Err())
}

func TestMemStoreMatcherFiltering(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s := NewMemStore(testLogger(), 0)
	app := s.Appender(ctx)
	_, _ = app.Append(0, labels.FromStrings("__name__", "http_total", "method", "GET"), 1000, 10)
	_, _ = app.Append(0, labels.FromStrings("__name__", "http_total", "method", "POST"), 1000, 5)
	_, _ = app.Append(0, labels.FromStrings("__name__", "cpu_seconds", "core", "0"), 1000, 42)
	require.NoError(t, app.Commit())
	t.Run("equality matcher", func(t *testing.T) {
		t.Parallel()
		m := labels.MustNewMatcher(labels.MatchEqual, "method", "GET")
		ss := s.Select(ctx, false, 0, 2000, m)
		require.True(t, ss.Next())
		assert.Equal(t, "GET", ss.At().Labels().Get("method"))
		assert.False(t, ss.Next())
	})
	t.Run("not-equal matcher", func(t *testing.T) {
		t.Parallel()
		m := labels.MustNewMatcher(labels.MatchNotEqual, "method", "GET")
		nameM := labels.MustNewMatcher(labels.MatchEqual, "__name__", "http_total")
		ss := s.Select(ctx, false, 0, 2000, nameM, m)
		require.True(t, ss.Next())
		assert.Equal(t, "POST", ss.At().Labels().Get("method"))
		assert.False(t, ss.Next())
	})
	t.Run("regex matcher", func(t *testing.T) {
		t.Parallel()
		m := labels.MustNewMatcher(labels.MatchRegexp, "__name__", "http.*")
		ss := s.Select(ctx, true, 0, 2000, m)
		count := 0
		for ss.Next() {
			count++
		}
		assert.Equal(t, 2, count)
	})
	t.Run("not-regex matcher", func(t *testing.T) {
		t.Parallel()
		m := labels.MustNewMatcher(labels.MatchNotRegexp, "__name__", "http.*")
		ss := s.Select(ctx, false, 0, 2000, m)
		require.True(t, ss.Next())
		assert.Equal(t, "cpu_seconds", ss.At().Labels().Get("__name__"))
		assert.False(t, ss.Next())
	})
}

func TestMemStoreLabelNames(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s := NewMemStore(testLogger(), 0)
	app := s.Appender(ctx)
	_, _ = app.Append(0, labels.FromStrings("__name__", "metric_a", "job", "svc"), 1000, 1)
	_, _ = app.Append(0, labels.FromStrings("__name__", "metric_b", "env", "prod"), 1000, 2)
	require.NoError(t, app.Commit())
	names, err := s.LabelNames(ctx, 0, 2000)
	require.NoError(t, err)
	assert.Equal(t, []string{"__name__", "env", "job"}, names)
}

func TestMemStoreLabelValues(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s := NewMemStore(testLogger(), 0)
	app := s.Appender(ctx)
	_, _ = app.Append(0, labels.FromStrings("__name__", "metric_a"), 1000, 1)
	_, _ = app.Append(0, labels.FromStrings("__name__", "metric_b"), 1000, 2)
	_, _ = app.Append(0, labels.FromStrings("__name__", "metric_a"), 2000, 3)
	require.NoError(t, app.Commit())
	values, err := s.LabelValues(ctx, "__name__", 0, 3000)
	require.NoError(t, err)
	assert.Equal(t, []string{"metric_a", "metric_b"}, values)
}

func TestMemStoreTimeRangeFiltering(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s := NewMemStore(testLogger(), 0)
	app := s.Appender(ctx)
	lset := labels.FromStrings("__name__", "test")
	_, _ = app.Append(0, lset, 1000, 1)
	_, _ = app.Append(0, lset, 2000, 2)
	_, _ = app.Append(0, lset, 3000, 3)
	require.NoError(t, app.Commit())
	m := labels.MustNewMatcher(labels.MatchEqual, "__name__", "test")
	ss := s.Select(ctx, false, 1500, 2500, m)
	require.True(t, ss.Next())
	samples := ss.At().Samples()
	require.Len(t, samples, 1)
	assert.Equal(t, int64(2000), samples[0].T)
}

func TestMemStoreConcurrentAccess(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s := NewMemStore(testLogger(), 0)
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			app := s.Appender(ctx)
			lset := labels.FromStrings("__name__", "concurrent", "worker", string(rune('0'+i)))
			for j := range 100 {
				_, _ = app.Append(0, lset, int64(j*1000), float64(j))
			}
			_ = app.Commit()
		}(i)
	}
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m := labels.MustNewMatcher(labels.MatchEqual, "__name__", "concurrent")
			ss := s.Select(ctx, false, 0, 100000, m)
			for ss.Next() {
				_ = ss.At().Samples()
			}
		}()
	}
	wg.Wait()
	m := labels.MustNewMatcher(labels.MatchEqual, "__name__", "concurrent")
	names, err := s.LabelNames(ctx, 0, 100000, m)
	require.NoError(t, err)
	assert.Contains(t, names, "__name__")
	assert.Contains(t, names, "worker")
}

func TestMemStoreRetentionPruning(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	retention := 1 * time.Hour
	s := NewMemStore(testLogger(), retention)
	app := s.Appender(ctx)
	lset := labels.FromStrings("__name__", "test")
	now := time.Now()
	_, _ = app.Append(0, lset, now.Add(-2*time.Hour).UnixMilli(), 1)
	_, _ = app.Append(0, lset, now.Add(-30*time.Minute).UnixMilli(), 2)
	_, _ = app.Append(0, lset, now.UnixMilli(), 3)
	require.NoError(t, app.Commit())
	s.Prune(now)
	m := labels.MustNewMatcher(labels.MatchEqual, "__name__", "test")
	ss := s.Select(ctx, false, 0, now.UnixMilli()+1, m)
	require.True(t, ss.Next())
	samples := ss.At().Samples()
	require.Len(t, samples, 2)
	assert.Equal(t, 2.0, samples[0].V)
	assert.Equal(t, 3.0, samples[1].V)
}

func TestMemStoreAppenderRollback(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s := NewMemStore(testLogger(), 0)
	app := s.Appender(ctx)
	lset := labels.FromStrings("__name__", "rollback_test")
	_, _ = app.Append(0, lset, 1000, 42)
	require.NoError(t, app.Rollback())
	m := labels.MustNewMatcher(labels.MatchEqual, "__name__", "rollback_test")
	ss := s.Select(ctx, false, 0, 2000, m)
	assert.False(t, ss.Next())
}

func TestMemStoreEmptyStore(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s := NewMemStore(testLogger(), 0)
	m := labels.MustNewMatcher(labels.MatchEqual, "__name__", "anything")
	ss := s.Select(ctx, false, 0, 1000, m)
	assert.False(t, ss.Next())
	assert.NoError(t, ss.Err())
	names, err := s.LabelNames(ctx, 0, 1000)
	require.NoError(t, err)
	assert.Empty(t, names)
	values, err := s.LabelValues(ctx, "__name__", 0, 1000)
	require.NoError(t, err)
	assert.Empty(t, values)
}

func TestMemStoreQueryableAdapterIntegration(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s := NewMemStore(testLogger(), 0)
	app := s.Appender(ctx)
	_, _ = app.Append(0, labels.FromStrings("__name__", "up", "job", "api"), 1000, 1)
	require.NoError(t, app.Commit())
	q := NewQueryable(s)
	querier, err := q.Querier(0, 2000)
	require.NoError(t, err)
	defer func() {
		if err := querier.Close(); err != nil {
			t.Log("closing querier:", err)
		}
	}()
	m := labels.MustNewMatcher(labels.MatchEqual, "__name__", "up")
	ss := querier.Select(ctx, false, nil, m)
	require.True(t, ss.Next())
	assert.Equal(t, "up", ss.At().Labels().Get("__name__"))
	assert.Equal(t, "api", ss.At().Labels().Get("job"))
	assert.False(t, ss.Next())
}

func BenchmarkMemStoreWrite(b *testing.B) {
	s := NewMemStore(testLogger(), 0)
	ctx := b.Context()
	lset := labels.FromStrings("__name__", "bench_metric", "job", "bench")
	b.ResetTimer()
	for i := range b.N {
		app := s.Appender(ctx)
		_, _ = app.Append(0, lset, int64(i*1000), float64(i))
		_ = app.Commit()
	}
}
