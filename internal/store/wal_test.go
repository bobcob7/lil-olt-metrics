package store

import (
	"path/filepath"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWALWriteAndReplay(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")
	w, err := NewWAL(walDir, 1024*1024)
	require.NoError(t, err)
	records := []walRecord{
		{Labels: labels.FromStrings("__name__", "up", "job", "test"), T: 1000, V: 1.0},
		{Labels: labels.FromStrings("__name__", "up", "job", "test"), T: 2000, V: 2.0},
		{Labels: labels.FromStrings("__name__", "requests", "method", "GET"), T: 1000, V: 42.5},
	}
	require.NoError(t, w.Log(records))
	require.NoError(t, w.Close())
	w2, err := NewWAL(walDir, 1024*1024)
	require.NoError(t, err)
	defer func() {
		if err := w2.Close(); err != nil {
			t.Log("closing WAL:", err)
		}
	}()
	var replayed []walRecord
	require.NoError(t, w2.Replay(func(rec walRecord) {
		replayed = append(replayed, rec)
	}))
	require.Len(t, replayed, 3)
	assert.Equal(t, "up", replayed[0].Labels.Get("__name__"))
	assert.Equal(t, int64(1000), replayed[0].T)
	assert.Equal(t, 1.0, replayed[0].V)
	assert.Equal(t, int64(2000), replayed[1].T)
	assert.Equal(t, 2.0, replayed[1].V)
	assert.Equal(t, 42.5, replayed[2].V)
}

func TestWALSegmentRotation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")
	w, err := NewWAL(walDir, 100)
	require.NoError(t, err)
	for range 50 {
		require.NoError(t, w.Log([]walRecord{
			{Labels: labels.FromStrings("__name__", "metric"), T: 1000, V: 1},
		}))
	}
	assert.Greater(t, w.SegmentSeq(), 0)
	require.NoError(t, w.Close())
	w2, err := NewWAL(walDir, 100)
	require.NoError(t, err)
	defer func() {
		if err := w2.Close(); err != nil {
			t.Log("closing WAL:", err)
		}
	}()
	count := 0
	require.NoError(t, w2.Replay(func(_ walRecord) {
		count++
	}))
	assert.Equal(t, 50, count)
}

func TestWALTruncate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")
	w, err := NewWAL(walDir, 50)
	require.NoError(t, err)
	for range 20 {
		require.NoError(t, w.Log([]walRecord{
			{Labels: labels.FromStrings("__name__", "x"), T: 1000, V: 1},
		}))
	}
	seq := w.SegmentSeq()
	require.Greater(t, seq, 0)
	require.NoError(t, w.Truncate(seq))
	require.NoError(t, w.Close())
	w2, err := NewWAL(walDir, 50)
	require.NoError(t, err)
	defer func() {
		if err := w2.Close(); err != nil {
			t.Log("closing WAL:", err)
		}
	}()
	count := 0
	require.NoError(t, w2.Replay(func(_ walRecord) {
		count++
	}))
	assert.Less(t, count, 20)
}

func TestWALEmptyReplay(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")
	w, err := NewWAL(walDir, 1024*1024)
	require.NoError(t, err)
	defer func() {
		if err := w.Close(); err != nil {
			t.Log("closing WAL:", err)
		}
	}()
	count := 0
	require.NoError(t, w.Replay(func(_ walRecord) {
		count++
	}))
	assert.Equal(t, 0, count)
}
