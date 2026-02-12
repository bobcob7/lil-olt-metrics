package store

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

// FSStoreConfig holds configuration for the filesystem store.
type FSStoreConfig struct {
	Path             string
	WALSegmentSize   int64
	FlushAge         time.Duration
	CompactionPeriod time.Duration
	RetentionAge     time.Duration
	RetentionMaxSize int64
}

// FSStore is a Store backed by a WAL and on-disk blocks for persistence.
type FSStore struct {
	logger *slog.Logger
	cfg    FSStoreConfig
	mem    *MemStore
	wal    *WAL
	mu     sync.RWMutex
	blocks []*block
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewFSStore creates and opens a filesystem-backed store.
// It replays the WAL to recover any unflushed data.
func NewFSStore(logger *slog.Logger, cfg FSStoreConfig) (*FSStore, error) {
	walDir := filepath.Join(cfg.Path, "wal")
	wal, err := NewWAL(walDir, cfg.WALSegmentSize)
	if err != nil {
		return nil, fmt.Errorf("opening WAL: %w", err)
	}
	mem := NewMemStore(logger, 0)
	s := &FSStore{
		logger: logger,
		cfg:    cfg,
		mem:    mem,
		wal:    wal,
		stopCh: make(chan struct{}),
	}
	if err := s.openBlocks(); err != nil {
		_ = wal.Close()
		return nil, fmt.Errorf("opening blocks: %w", err)
	}
	replayCount := 0
	if err := wal.Replay(func(rec walRecord) {
		app := mem.Appender(context.Background())
		_, _ = app.Append(0, rec.Labels, rec.T, rec.V)
		_ = app.Commit()
		replayCount++
	}); err != nil {
		_ = wal.Close()
		return nil, fmt.Errorf("replaying WAL: %w", err)
	}
	if replayCount > 0 {
		logger.Info("WAL replay complete", "samples", replayCount)
	}
	if cfg.CompactionPeriod > 0 {
		s.wg.Add(1)
		go s.compactionLoop()
	}
	return s, nil
}

// Appender implements Store.
func (s *FSStore) Appender(ctx context.Context) Appender {
	return &fsAppender{store: s, inner: s.mem.Appender(ctx).(*memAppender)}
}

// Select implements Store.
func (s *FSStore) Select(ctx context.Context, sortSeries bool, mint, maxt int64, matchers ...*labels.Matcher) SeriesSet {
	memSS := s.mem.Select(ctx, sortSeries, mint, maxt, matchers...)
	var allSeries []Series
	for memSS.Next() {
		allSeries = append(allSeries, memSS.At())
	}
	s.mu.RLock()
	for _, b := range s.blocks {
		if b.meta.MaxTime < mint || b.meta.MinTime > maxt {
			continue
		}
		blockSeries := b.selectSeries(false, mint, maxt, matchers...)
		allSeries = append(allSeries, blockSeries...)
	}
	s.mu.RUnlock()
	if sortSeries {
		sort.Slice(allSeries, func(i, j int) bool {
			return labels.Compare(allSeries[i].Labels(), allSeries[j].Labels()) < 0
		})
	}
	return &sliceSeriesSet{series: allSeries, idx: -1}
}

// LabelNames implements Store.
func (s *FSStore) LabelNames(ctx context.Context, mint, maxt int64, matchers ...*labels.Matcher) ([]string, error) {
	names, err := s.mem.LabelNames(ctx, mint, maxt, matchers...)
	if err != nil {
		return nil, err
	}
	nameSet := make(map[string]struct{})
	for _, n := range names {
		nameSet[n] = struct{}{}
	}
	s.mu.RLock()
	for _, b := range s.blocks {
		if b.meta.MaxTime < mint || b.meta.MinTime > maxt {
			continue
		}
		for _, bs := range b.series {
			if !matchesAll(bs.Labels, matchers) {
				continue
			}
			bs.Labels.Range(func(l labels.Label) {
				nameSet[l.Name] = struct{}{}
			})
		}
	}
	s.mu.RUnlock()
	result := make([]string, 0, len(nameSet))
	for n := range nameSet {
		result = append(result, n)
	}
	sort.Strings(result)
	return result, nil
}

// LabelValues implements Store.
func (s *FSStore) LabelValues(ctx context.Context, name string, mint, maxt int64, matchers ...*labels.Matcher) ([]string, error) {
	values, err := s.mem.LabelValues(ctx, name, mint, maxt, matchers...)
	if err != nil {
		return nil, err
	}
	valueSet := make(map[string]struct{})
	for _, v := range values {
		valueSet[v] = struct{}{}
	}
	s.mu.RLock()
	for _, b := range s.blocks {
		if b.meta.MaxTime < mint || b.meta.MinTime > maxt {
			continue
		}
		for _, bs := range b.series {
			if !matchesAll(bs.Labels, matchers) {
				continue
			}
			if v := bs.Labels.Get(name); v != "" {
				valueSet[v] = struct{}{}
			}
		}
	}
	s.mu.RUnlock()
	result := make([]string, 0, len(valueSet))
	for v := range valueSet {
		result = append(result, v)
	}
	sort.Strings(result)
	return result, nil
}

// Close implements Store.
func (s *FSStore) Close() error {
	close(s.stopCh)
	s.wg.Wait()
	if err := s.wal.Close(); err != nil {
		return fmt.Errorf("closing WAL: %w", err)
	}
	return s.mem.Close()
}

func (s *FSStore) openBlocks() error {
	blocksDir := filepath.Join(s.cfg.Path, "blocks")
	if err := os.MkdirAll(blocksDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(blocksDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := openBlock(filepath.Join(blocksDir, e.Name()))
		if err != nil {
			s.logger.Warn("skipping corrupted block", "dir", e.Name(), "error", err)
			continue
		}
		s.blocks = append(s.blocks, b)
	}
	sort.Slice(s.blocks, func(i, j int) bool {
		return s.blocks[i].meta.MinTime < s.blocks[j].meta.MinTime
	})
	return nil
}

// FlushHead persists the head block to disk as an immutable block.
func (s *FSStore) FlushHead() error {
	s.mem.mu.RLock()
	var allSeries []blockSeries
	var mint, maxt int64
	first := true
	for _, bucket := range s.mem.series {
		for _, ms := range bucket {
			if len(ms.samples) == 0 {
				continue
			}
			samples := append([]Sample(nil), ms.samples...)
			allSeries = append(allSeries, blockSeries{Labels: ms.lset, Samples: samples})
			for _, sample := range samples {
				if first || sample.T < mint {
					mint = sample.T
				}
				if first || sample.T > maxt {
					maxt = sample.T
				}
				first = false
			}
		}
	}
	s.mem.mu.RUnlock()
	if len(allSeries) == 0 {
		return nil
	}
	blocksDir := filepath.Join(s.cfg.Path, "blocks")
	blockDir := filepath.Join(blocksDir, fmt.Sprintf("b%020d", mint))
	b, err := writeBlock(blockDir, allSeries, mint, maxt)
	if err != nil {
		return fmt.Errorf("flushing head: %w", err)
	}
	s.mu.Lock()
	s.blocks = append(s.blocks, b)
	s.mu.Unlock()
	s.mem.mu.Lock()
	s.mem.series = make(map[uint64][]*memSeries)
	s.mem.mu.Unlock()
	walSeq := s.wal.SegmentSeq()
	if err := s.wal.Truncate(walSeq); err != nil {
		s.logger.Warn("WAL truncation failed", "error", err)
	}
	s.logger.Info("flushed head to block", "dir", blockDir, "samples", b.meta.Samples)
	return nil
}

func (s *FSStore) compactionLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.cfg.CompactionPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if err := s.compact(); err != nil {
				s.logger.Error("compaction failed", "error", err)
			}
			s.applyRetention()
		}
	}
}

func (s *FSStore) compact() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.blocks) < 2 {
		return nil
	}
	blocksDir := filepath.Join(s.cfg.Path, "blocks")
	mint := s.blocks[0].meta.MinTime
	targetDir := filepath.Join(blocksDir, fmt.Sprintf("c%020d", mint))
	merged, err := compactBlocks(targetDir, s.blocks)
	if err != nil {
		return fmt.Errorf("compacting blocks: %w", err)
	}
	for _, b := range s.blocks {
		_ = os.RemoveAll(b.dir)
	}
	s.blocks = []*block{merged}
	s.logger.Info("compacted blocks", "result", targetDir)
	return nil
}

func (s *FSStore) applyRetention() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.RetentionAge > 0 {
		cutoff := time.Now().Add(-s.cfg.RetentionAge).UnixMilli()
		var kept []*block
		for _, b := range s.blocks {
			if b.meta.MaxTime < cutoff {
				_ = os.RemoveAll(b.dir)
				s.logger.Info("retention: removed block", "dir", b.dir)
			} else {
				kept = append(kept, b)
			}
		}
		s.blocks = kept
	}
	if s.cfg.RetentionMaxSize > 0 {
		var totalSize int64
		for _, b := range s.blocks {
			sz, _ := blockDiskSize(b.dir)
			totalSize += sz
		}
		for totalSize > s.cfg.RetentionMaxSize && len(s.blocks) > 0 {
			oldest := s.blocks[0]
			sz, _ := blockDiskSize(oldest.dir)
			_ = os.RemoveAll(oldest.dir)
			s.logger.Info("retention: removed oldest block for size", "dir", oldest.dir)
			s.blocks = s.blocks[1:]
			totalSize -= sz
		}
	}
}

type fsAppender struct {
	store *FSStore
	inner *memAppender
}

func (a *fsAppender) Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	return a.inner.Append(ref, l, t, v)
}

func (a *fsAppender) Commit() error {
	records := make([]walRecord, len(a.inner.pending))
	for i, p := range a.inner.pending {
		records[i] = walRecord{Labels: p.labels, T: p.t, V: p.v}
	}
	if err := a.store.wal.Log(records); err != nil {
		return fmt.Errorf("WAL log: %w", err)
	}
	return a.inner.Commit()
}

func (a *fsAppender) Rollback() error {
	return a.inner.Rollback()
}
