// Package sessions provides a persistent, bounded, per-session event store
// backed by bbolt for the OTLP logs receiver and the Live Sessions HTTP API.
package sessions

import (
	"context"
	"errors"
	"time"
)

//go:generate moq -out moq_test.go . Store

// Store persists per-session events emitted by the OTLP logs receiver.
type Store interface {
	// AppendEvent records an event and upserts the parent session record.
	AppendEvent(ctx context.Context, evt Event) error
	// GetSession returns a single session by id, or errSessionNotFound.
	GetSession(ctx context.Context, id string) (Session, error)
	// ListSessions returns sessions whose LastSeen is at or after since,
	// sorted newest-first.
	ListSessions(ctx context.Context, since time.Time) ([]Session, error)
	// GetEvents returns events for a session in chronological order.
	// since may be zero (no filter); limit <= 0 falls back to a default.
	GetEvents(ctx context.Context, sessionID string, since time.Time, limit int) ([]Event, error)
	// Close flushes pending writes, stops background work, and releases the file.
	Close() error
}

var errSessionNotFound = errors.New("session not found")
