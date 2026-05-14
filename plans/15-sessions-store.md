# Plan 15 — Sessions Store (bbolt)

## Summary

Create a new `internal/sessions` package providing a persistent, bounded, per-session event store backed by bbolt. The Store exposes append/list/query operations the OTLP logs receiver and HTTP API will use, plus a background retention loop that prunes stale sessions and bounds events per session.

## Dependencies

- **Plan 14** (logs config) — uses `LogsConfig` for path, retention, max events
- Existing project conventions only — no other plans needed

## Scope

### In Scope

- New module dep: `go.etcd.io/bbolt` added to `go.mod` via `go get`
- `internal/sessions/interfaces.go`:
  - `Store` interface with: `AppendEvent(ctx, evt) error`, `GetSession(ctx, id) (Session, error)`, `ListSessions(ctx, since time.Time) ([]Session, error)`, `GetEvents(ctx, sessionID string, since time.Time, limit int) ([]Event, error)`, `Close() error`
  - Sentinel: `errSessionNotFound` (unexported)
  - `//go:generate moq -out moq_test.go . Store` directive
- `internal/sessions/types.go`:
  - `Session` struct: `ID, UserID, Model, CWD, LastEventName, LastToolName string`; `FirstSeen, LastSeen time.Time`; `EventCount int`
  - `Event` struct: `SessionID, Name, ToolName, Model string`; `Timestamp time.Time`; `Attrs map[string]string`; `Body string` (empty when content-capture off)
- `internal/sessions/bbolt_store.go`:
  - `NewBBoltStore(logger, cfg BBoltConfig) (*BBoltStore, error)` opens the file, creates buckets, starts retention goroutine
  - `BBoltConfig{Path string, Retention time.Duration, MaxEventsPerSession int, RetentionInterval time.Duration}` — `RetentionInterval` defaults to `min(Retention/10, 5min)` when zero
  - Bucket layout:
    * top-level `sessions` bucket: `key=sessionID`, `value=json.Marshal(Session)`
    * top-level `events` bucket containing one nested bucket per `sessionID`; within each nested bucket `key=timestamp` encoded as 8-byte big-endian unix-nano (lexicographically sortable), `value=json.Marshal(Event)`
  - `AppendEvent`:
    * Upsert the session record (set `FirstSeen` only if zero; always update `LastSeen`, `LastEventName`, `LastToolName`, `Model`, `CWD`, `UserID` when non-empty; increment `EventCount`)
    * Insert into the per-session events bucket
    * If the bucket size exceeds `MaxEventsPerSession`, delete the oldest keys via a forward cursor until within bound
    * All operations within one `db.Update` txn
  - `ListSessions`: iterate the `sessions` bucket, decode each, filter by `LastSeen >= since`, sort by `LastSeen` descending
  - `GetEvents`: open the per-session nested bucket; if `since` is non-zero, seek to its encoded key; iterate forward up to `limit` records (default `limit=100` if `<=0`); return in chronological order
  - `GetSession`: single bucket lookup, returns `errSessionNotFound` when absent
  - `Close`: stops retention goroutine, calls `db.Close()`
  - Retention loop: `time.NewTicker(RetentionInterval)`; on each tick, in a single `db.Update` txn, walk `sessions` bucket and delete each session whose `LastSeen < now-Retention`, along with its nested events bucket
- `internal/sessions/bbolt_store_test.go`:
  - `t.Parallel()`, `t.Context()`, `slog.New(slog.NewJSONHandler(io.Discard, nil))` per project standards
  - Use `t.TempDir()` for the bbolt file path
  - Cases:
    * Append → GetSession returns the same data
    * Append two sessions → ListSessions returns both, newest-first
    * `since` filter on ListSessions drops old sessions
    * Append > `MaxEventsPerSession` events → only the last N remain
    * GetEvents respects `limit` and `since`
    * Retention TTL eviction: force `LastSeen` into the past, call internal `runRetentionOnce(ctx, now)` (export for tests via `_test.go` helper), then verify the session and its events bucket are gone
    * GetSession on unknown id returns `errSessionNotFound`
- `internal/sessions/moq_test.go` generated (in-package mock for the `Store` interface)
- `internal/tools/tools.go` unchanged — moq is already pinned

### Out of Scope

- Anything that calls this store (translator, receivers, HTTP API)
- WAL-style replay or crash testing beyond what bbolt provides natively

## Acceptance Criteria

1. `go build ./...` succeeds
2. `go test ./internal/sessions/...` passes with `-race`
3. `make lint` clean
4. `go mod tidy` leaves `go.etcd.io/bbolt` as a direct dependency
5. Tests cover the seven cases above, all using `t.Parallel()`
6. Closing the store stops the retention goroutine (verified via `goleak`-style assertion or a channel-close check in the test)

## Key Snippets

```go
// internal/sessions/interfaces.go
package sessions

import (
    "context"
    "time"
)

//go:generate moq -out moq_test.go . Store

type Store interface {
    AppendEvent(ctx context.Context, evt Event) error
    GetSession(ctx context.Context, id string) (Session, error)
    ListSessions(ctx context.Context, since time.Time) ([]Session, error)
    GetEvents(ctx context.Context, sessionID string, since time.Time, limit int) ([]Event, error)
    Close() error
}

var errSessionNotFound = errors.New("session not found")
```

```go
// Key encoding for events bucket
func encodeTS(t time.Time) []byte {
    b := make([]byte, 8)
    binary.BigEndian.PutUint64(b, uint64(t.UnixNano()))
    return b
}
```

```go
// AppendEvent skeleton
func (s *BBoltStore) AppendEvent(ctx context.Context, evt Event) error {
    return s.db.Update(func(tx *bbolt.Tx) error {
        sessions := tx.Bucket(bucketSessions)
        var sess Session
        if raw := sessions.Get([]byte(evt.SessionID)); raw != nil {
            _ = json.Unmarshal(raw, &sess)
        } else {
            sess = Session{ID: evt.SessionID, FirstSeen: evt.Timestamp}
        }
        sess.LastSeen = evt.Timestamp
        sess.LastEventName = evt.Name
        if evt.ToolName != "" { sess.LastToolName = evt.ToolName }
        if evt.Model != "" { sess.Model = evt.Model }
        // ...userID, cwd similar
        sess.EventCount++
        encSess, _ := json.Marshal(sess)
        if err := sessions.Put([]byte(evt.SessionID), encSess); err != nil { return err }

        eventsRoot := tx.Bucket(bucketEvents)
        eb, err := eventsRoot.CreateBucketIfNotExists([]byte(evt.SessionID))
        if err != nil { return err }
        encEvt, _ := json.Marshal(evt)
        if err := eb.Put(encodeTS(evt.Timestamp), encEvt); err != nil { return err }

        return s.enforceBound(eb)
    })
}
```
