package sessions

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"go.etcd.io/bbolt"
)

var (
	bucketSessions = []byte("sessions")
	bucketEvents   = []byte("events")
)

const defaultGetEventsLimit = 100

// BBoltConfig configures a BBoltStore.
type BBoltConfig struct {
	Path                string
	Retention           time.Duration
	MaxEventsPerSession int
	// RetentionInterval is how often the retention loop runs. When zero,
	// defaults to min(Retention/10, 5min) clamped to 1m minimum.
	RetentionInterval time.Duration
}

// BBoltStore is a bbolt-backed Store implementation.
type BBoltStore struct {
	logger              *slog.Logger
	db                  *bbolt.DB
	retention           time.Duration
	maxEventsPerSession int

	stop   chan struct{}
	wg     sync.WaitGroup
	closed bool
	mu     sync.Mutex
}

// NewBBoltStore opens (or creates) the bbolt file at cfg.Path, ensures the
// required buckets exist, and starts the retention loop.
func NewBBoltStore(logger *slog.Logger, cfg BBoltConfig) (*BBoltStore, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("bbolt store: path must not be empty")
	}
	if cfg.MaxEventsPerSession <= 0 {
		return nil, fmt.Errorf("bbolt store: max_events_per_session must be positive")
	}
	db, err := bbolt.Open(cfg.Path, 0o600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening bbolt at %s: %w", cfg.Path, err)
	}
	if err := db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketSessions); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketEvents); err != nil {
			return err
		}
		return nil
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initializing buckets: %w", err)
	}
	interval := cfg.RetentionInterval
	if interval <= 0 {
		interval = max(min(cfg.Retention/10, 5*time.Minute), time.Minute)
	}
	s := &BBoltStore{
		logger:              logger,
		db:                  db,
		retention:           cfg.Retention,
		maxEventsPerSession: cfg.MaxEventsPerSession,
		stop:                make(chan struct{}),
	}
	if cfg.Retention > 0 {
		s.wg.Add(1)
		go s.retentionLoop(interval)
	}
	return s, nil
}

// Close stops background work and closes the underlying database.
func (s *BBoltStore) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.stop)
	s.mu.Unlock()
	s.wg.Wait()
	return s.db.Close()
}

// AppendEvent records evt and updates the parent session's rolling metadata.
func (s *BBoltStore) AppendEvent(_ context.Context, evt Event) error {
	if evt.SessionID == "" {
		return fmt.Errorf("event session id must not be empty")
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		sessions := tx.Bucket(bucketSessions)
		var sess Session
		if raw := sessions.Get([]byte(evt.SessionID)); raw != nil {
			if err := json.Unmarshal(raw, &sess); err != nil {
				return fmt.Errorf("decoding existing session: %w", err)
			}
		}
		if sess.ID == "" {
			sess.ID = evt.SessionID
			sess.FirstSeen = evt.Timestamp
		}
		sess.LastSeen = evt.Timestamp
		sess.LastEventName = evt.Name
		if evt.ToolName != "" {
			sess.LastToolName = evt.ToolName
		}
		if evt.Model != "" {
			sess.Model = evt.Model
		}
		if cwd := evt.Attrs["cwd"]; cwd != "" {
			sess.CWD = cwd
		}
		if userID := evt.Attrs["user.id"]; userID != "" {
			sess.UserID = userID
		}
		sess.EventCount++
		encSess, err := json.Marshal(sess)
		if err != nil {
			return fmt.Errorf("encoding session: %w", err)
		}
		if err := sessions.Put([]byte(evt.SessionID), encSess); err != nil {
			return err
		}
		eventsRoot := tx.Bucket(bucketEvents)
		eb, err := eventsRoot.CreateBucketIfNotExists([]byte(evt.SessionID))
		if err != nil {
			return fmt.Errorf("creating events bucket: %w", err)
		}
		encEvt, err := json.Marshal(evt)
		if err != nil {
			return fmt.Errorf("encoding event: %w", err)
		}
		if err := eb.Put(encodeTS(evt.Timestamp), encEvt); err != nil {
			return err
		}
		return s.enforceBound(eb)
	})
}

func (s *BBoltStore) enforceBound(eb *bbolt.Bucket) error {
	count := 0
	if err := eb.ForEach(func(_, _ []byte) error {
		count++
		return nil
	}); err != nil {
		return err
	}
	excess := count - s.maxEventsPerSession
	if excess <= 0 {
		return nil
	}
	cur := eb.Cursor()
	k, _ := cur.First()
	for i := 0; i < excess && k != nil; i++ {
		if err := cur.Delete(); err != nil {
			return err
		}
		k, _ = cur.First()
	}
	return nil
}

// GetSession returns the session with the given id.
func (s *BBoltStore) GetSession(_ context.Context, id string) (Session, error) {
	var sess Session
	err := s.db.View(func(tx *bbolt.Tx) error {
		raw := tx.Bucket(bucketSessions).Get([]byte(id))
		if raw == nil {
			return errSessionNotFound
		}
		return json.Unmarshal(raw, &sess)
	})
	if err != nil {
		return Session{}, err
	}
	return sess, nil
}

// ListSessions returns sessions whose LastSeen is at or after since.
func (s *BBoltStore) ListSessions(_ context.Context, since time.Time) ([]Session, error) {
	var out []Session
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketSessions).ForEach(func(_, v []byte) error {
			var sess Session
			if err := json.Unmarshal(v, &sess); err != nil {
				return err
			}
			if !since.IsZero() && sess.LastSeen.Before(since) {
				return nil
			}
			out = append(out, sess)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeen.After(out[j].LastSeen)
	})
	return out, nil
}

// GetEvents returns events for a session in chronological order.
func (s *BBoltStore) GetEvents(_ context.Context, sessionID string, since time.Time, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = defaultGetEventsLimit
	}
	var out []Event
	err := s.db.View(func(tx *bbolt.Tx) error {
		eb := tx.Bucket(bucketEvents).Bucket([]byte(sessionID))
		if eb == nil {
			return nil
		}
		cur := eb.Cursor()
		var k, v []byte
		if since.IsZero() {
			k, v = cur.First()
		} else {
			k, v = cur.Seek(encodeTS(since))
		}
		for ; k != nil && len(out) < limit; k, v = cur.Next() {
			var evt Event
			if err := json.Unmarshal(v, &evt); err != nil {
				return err
			}
			out = append(out, evt)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *BBoltStore) retentionLoop(interval time.Duration) {
	defer s.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case now := <-ticker.C:
			if err := s.runRetentionOnce(now); err != nil {
				s.logger.Warn("retention sweep failed", "error", err)
			}
		}
	}
}

func (s *BBoltStore) runRetentionOnce(now time.Time) error {
	cutoff := now.Add(-s.retention)
	return s.db.Update(func(tx *bbolt.Tx) error {
		sessions := tx.Bucket(bucketSessions)
		eventsRoot := tx.Bucket(bucketEvents)
		var toDelete [][]byte
		if err := sessions.ForEach(func(k, v []byte) error {
			var sess Session
			if err := json.Unmarshal(v, &sess); err != nil {
				return err
			}
			if sess.LastSeen.Before(cutoff) {
				idCopy := make([]byte, len(k))
				copy(idCopy, k)
				toDelete = append(toDelete, idCopy)
			}
			return nil
		}); err != nil {
			return err
		}
		for _, id := range toDelete {
			if err := sessions.Delete(id); err != nil {
				return err
			}
			if eventsRoot.Bucket(id) != nil {
				if err := eventsRoot.DeleteBucket(id); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func encodeTS(t time.Time) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(t.UnixNano()))
	return b
}
