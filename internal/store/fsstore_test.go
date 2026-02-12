package store

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestFSStore(t *testing.T) *FSStore {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dir := t.TempDir()
	s, err := NewFSStore(logger, FSStoreConfig{
		Path:             dir,
		WALSegmentSize:   1024 * 1024,
		CompactionPeriod: 0,
		RetentionAge:     0,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestFSStoreWriteAndQuery(t *testing.T) {
	t.Parallel()
	s := newTestFSStore(t)
	app := s.Appender(t.Context())
	_, err := app.Append(0, labels.FromStrings("__name__", "up", "job", "test"), 1000, 1.0)
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings("__name__", "up", "job", "test"), 2000, 2.0)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	ss := s.Select(t.Context(), false, 0, 3000)
	require.True(t, ss.Next())
	series := ss.At()
	assert.Equal(t, "up", series.Labels().Get("__name__"))
	assert.Len(t, series.Samples(), 2)
}

func TestFSStoreWALReplay(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dir := t.TempDir()
	cfg := FSStoreConfig{
		Path:           dir,
		WALSegmentSize: 1024 * 1024,
	}
	s1, err := NewFSStore(logger, cfg)
	require.NoError(t, err)
	app := s1.Appender(t.Context())
	_, _ = app.Append(0, labels.FromStrings("__name__", "up"), 1000, 42.0)
	require.NoError(t, app.Commit())
	require.NoError(t, s1.Close())
	s2, err := NewFSStore(logger, cfg)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	ss := s2.Select(t.Context(), false, 0, 2000)
	require.True(t, ss.Next())
	samples := ss.At().Samples()
	require.Len(t, samples, 1)
	assert.Equal(t, 42.0, samples[0].V)
}

func TestFSStoreFlushHead(t *testing.T) {
	t.Parallel()
	s := newTestFSStore(t)
	app := s.Appender(t.Context())
	_, _ = app.Append(0, labels.FromStrings("__name__", "metric"), 1000, 10.0)
	_, _ = app.Append(0, labels.FromStrings("__name__", "metric"), 2000, 20.0)
	require.NoError(t, app.Commit())
	require.NoError(t, s.FlushHead())
	s.mu.RLock()
	assert.Len(t, s.blocks, 1)
	s.mu.RUnlock()
	ss := s.Select(t.Context(), false, 0, 3000)
	require.True(t, ss.Next())
	assert.Len(t, ss.At().Samples(), 2)
}

func TestFSStoreCompaction(t *testing.T) {
	t.Parallel()
	s := newTestFSStore(t)
	app := s.Appender(t.Context())
	_, _ = app.Append(0, labels.FromStrings("__name__", "a"), 1000, 1.0)
	require.NoError(t, app.Commit())
	require.NoError(t, s.FlushHead())
	app = s.Appender(t.Context())
	_, _ = app.Append(0, labels.FromStrings("__name__", "a"), 2000, 2.0)
	require.NoError(t, app.Commit())
	require.NoError(t, s.FlushHead())
	s.mu.RLock()
	assert.Len(t, s.blocks, 2)
	s.mu.RUnlock()
	require.NoError(t, s.compact())
	s.mu.RLock()
	assert.Len(t, s.blocks, 1)
	assert.Equal(t, 2, s.blocks[0].meta.Samples)
	s.mu.RUnlock()
	ss := s.Select(t.Context(), false, 0, 3000)
	require.True(t, ss.Next())
	assert.Len(t, ss.At().Samples(), 2)
}

func TestFSStoreRetentionByAge(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dir := t.TempDir()
	s, err := NewFSStore(logger, FSStoreConfig{
		Path:           dir,
		WALSegmentSize: 1024 * 1024,
		RetentionAge:   1 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() { _ = s.Close() }()
	app := s.Appender(t.Context())
	_, _ = app.Append(0, labels.FromStrings("__name__", "old"), 1, 1.0)
	require.NoError(t, app.Commit())
	require.NoError(t, s.FlushHead())
	time.Sleep(10 * time.Millisecond)
	s.applyRetention()
	s.mu.RLock()
	assert.Len(t, s.blocks, 0)
	s.mu.RUnlock()
}

func TestFSStoreLabelNames(t *testing.T) {
	t.Parallel()
	s := newTestFSStore(t)
	app := s.Appender(t.Context())
	_, _ = app.Append(0, labels.FromStrings("__name__", "up", "job", "test"), 1000, 1.0)
	require.NoError(t, app.Commit())
	require.NoError(t, s.FlushHead())
	app = s.Appender(t.Context())
	_, _ = app.Append(0, labels.FromStrings("__name__", "up", "instance", "host1"), 2000, 1.0)
	require.NoError(t, app.Commit())
	names, err := s.LabelNames(t.Context(), 0, 3000)
	require.NoError(t, err)
	assert.Contains(t, names, "__name__")
	assert.Contains(t, names, "job")
	assert.Contains(t, names, "instance")
}

func TestFSStoreLabelValues(t *testing.T) {
	t.Parallel()
	s := newTestFSStore(t)
	app := s.Appender(t.Context())
	_, _ = app.Append(0, labels.FromStrings("__name__", "up"), 1000, 1.0)
	_, _ = app.Append(0, labels.FromStrings("__name__", "down"), 1000, 0.0)
	require.NoError(t, app.Commit())
	values, err := s.LabelValues(t.Context(), "__name__", 0, 2000)
	require.NoError(t, err)
	assert.Contains(t, values, "up")
	assert.Contains(t, values, "down")
}

func TestFSStoreAppenderRollback(t *testing.T) {
	t.Parallel()
	s := newTestFSStore(t)
	app := s.Appender(t.Context())
	_, _ = app.Append(0, labels.FromStrings("__name__", "up"), 1000, 1.0)
	require.NoError(t, app.Rollback())
	ss := s.Select(t.Context(), false, 0, 2000)
	assert.False(t, ss.Next())
}
