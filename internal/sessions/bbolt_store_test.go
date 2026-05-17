package sessions

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T, maxEvents int, retention time.Duration) *BBoltStore {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	path := filepath.Join(t.TempDir(), "sessions.db")
	s, err := NewBBoltStore(logger, BBoltConfig{
		Path:                path,
		Retention:           retention,
		MaxEventsPerSession: maxEvents,
		RetentionInterval:   time.Hour,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestBBoltStore_AppendThenGet(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, 100, time.Hour)
	ctx := t.Context()
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	require.NoError(t, s.AppendEvent(ctx, Event{
		SessionID: "sess-1",
		Name:      "claude_code.user_prompt",
		Model:     "claude-opus-4-7",
		Timestamp: now,
		Attrs:     map[string]string{"cwd": "/home/dev/project", "user.id": "alice"},
	}))
	got, err := s.GetSession(ctx, "sess-1")
	require.NoError(t, err)
	assert.Equal(t, "sess-1", got.ID)
	assert.Equal(t, "alice", got.UserID)
	assert.Equal(t, "claude-opus-4-7", got.Model)
	assert.Equal(t, "/home/dev/project", got.CWD)
	assert.Equal(t, "claude_code.user_prompt", got.LastEventName)
	assert.Equal(t, 1, got.EventCount)
	assert.True(t, got.FirstSeen.Equal(now))
	assert.True(t, got.LastSeen.Equal(now))
}

func TestBBoltStore_ListSessionsNewestFirst(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, 100, time.Hour)
	ctx := t.Context()
	now := time.Now().UTC()
	require.NoError(t, s.AppendEvent(ctx, Event{
		SessionID: "old", Name: "x", Timestamp: now.Add(-2 * time.Minute),
	}))
	require.NoError(t, s.AppendEvent(ctx, Event{
		SessionID: "new", Name: "x", Timestamp: now,
	}))
	got, err := s.ListSessions(ctx, time.Time{})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "new", got[0].ID)
	assert.Equal(t, "old", got[1].ID)
}

func TestBBoltStore_ListSessionsSinceFilter(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, 100, time.Hour)
	ctx := t.Context()
	now := time.Now().UTC()
	require.NoError(t, s.AppendEvent(ctx, Event{
		SessionID: "old", Name: "x", Timestamp: now.Add(-10 * time.Minute),
	}))
	require.NoError(t, s.AppendEvent(ctx, Event{
		SessionID: "new", Name: "x", Timestamp: now,
	}))
	got, err := s.ListSessions(ctx, now.Add(-5*time.Minute))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "new", got[0].ID)
}

func TestBBoltStore_EnforcesMaxEventsPerSession(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, 3, time.Hour)
	ctx := t.Context()
	base := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		require.NoError(t, s.AppendEvent(ctx, Event{
			SessionID: "sess",
			Name:      "evt",
			Timestamp: base.Add(time.Duration(i) * time.Second),
		}))
	}
	events, err := s.GetEvents(ctx, "sess", time.Time{}, 100)
	require.NoError(t, err)
	require.Len(t, events, 3)
	assert.True(t, events[0].Timestamp.Equal(base.Add(2*time.Second)))
	assert.True(t, events[2].Timestamp.Equal(base.Add(4*time.Second)))
}

func TestBBoltStore_GetEventsLimitAndSince(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, 100, time.Hour)
	ctx := t.Context()
	base := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		require.NoError(t, s.AppendEvent(ctx, Event{
			SessionID: "sess",
			Name:      "evt",
			Timestamp: base.Add(time.Duration(i) * time.Second),
		}))
	}
	limited, err := s.GetEvents(ctx, "sess", time.Time{}, 4)
	require.NoError(t, err)
	require.Len(t, limited, 4)
	assert.True(t, limited[0].Timestamp.Equal(base))
	since, err := s.GetEvents(ctx, "sess", base.Add(7*time.Second), 100)
	require.NoError(t, err)
	require.Len(t, since, 3)
	assert.True(t, since[0].Timestamp.Equal(base.Add(7*time.Second)))
}

func TestBBoltStore_RetentionEvictsStaleSessions(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, 100, 24*time.Hour)
	ctx := t.Context()
	now := time.Now().UTC()
	require.NoError(t, s.AppendEvent(ctx, Event{
		SessionID: "stale", Name: "x", Timestamp: now.Add(-48 * time.Hour),
	}))
	require.NoError(t, s.AppendEvent(ctx, Event{
		SessionID: "fresh", Name: "x", Timestamp: now.Add(-time.Hour),
	}))
	require.NoError(t, s.runRetentionOnce(now))
	got, err := s.ListSessions(ctx, time.Time{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "fresh", got[0].ID)
	_, err = s.GetSession(ctx, "stale")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errSessionNotFound))
	staleEvents, err := s.GetEvents(ctx, "stale", time.Time{}, 100)
	require.NoError(t, err)
	assert.Empty(t, staleEvents)
}

func TestBBoltStore_GetSessionUnknownReturnsNotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, 100, time.Hour)
	_, err := s.GetSession(t.Context(), "missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errSessionNotFound))
}

func TestBBoltStore_CloseStopsRetentionGoroutine(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	path := filepath.Join(t.TempDir(), "sessions.db")
	before := runtime.NumGoroutine()
	s, err := NewBBoltStore(logger, BBoltConfig{
		Path:                path,
		Retention:           time.Hour,
		MaxEventsPerSession: 100,
		RetentionInterval:   time.Minute,
	})
	require.NoError(t, err)
	require.Greater(t, runtime.NumGoroutine(), before, "retention goroutine should be running")
	require.NoError(t, s.Close())
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.LessOrEqual(t, runtime.NumGoroutine(), before, "retention goroutine should have exited")
}
