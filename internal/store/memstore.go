package store

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

// NewMemStore creates a new in-memory store with the given retention duration.
// If retention is 0, samples are never pruned.
func NewMemStore(logger *slog.Logger, retention time.Duration) *MemStore {
	return &MemStore{
		logger:    logger,
		retention: retention,
		series:    make(map[uint64][]*memSeries),
	}
}

// MemStore is an in-memory implementation of the Store interface holding
// the "head block" of recent metric samples.
type MemStore struct {
	logger    *slog.Logger
	retention time.Duration
	mu        sync.RWMutex
	series    map[uint64][]*memSeries // fingerprint -> series (slice for collision handling)
	nextRef   storage.SeriesRef
}

type memSeries struct {
	ref     storage.SeriesRef
	lset    labels.Labels
	samples []Sample
}

// Appender implements Store.
func (s *MemStore) Appender(_ context.Context) Appender {
	return &memAppender{store: s}
}

// Select implements Store.
func (s *MemStore) Select(_ context.Context, sortSeries bool, mint, maxt int64, matchers ...*labels.Matcher) SeriesSet {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Series
	for _, bucket := range s.series {
		for _, ms := range bucket {
			if !matchesAll(ms.lset, matchers) {
				continue
			}
			samples := filterSamples(ms.samples, mint, maxt)
			if len(samples) == 0 {
				continue
			}
			result = append(result, &concreteSeries{lset: ms.lset, samples: samples})
		}
	}
	if sortSeries {
		sort.Slice(result, func(i, j int) bool {
			return labels.Compare(result[i].Labels(), result[j].Labels()) < 0
		})
	}
	return &sliceSeriesSet{series: result, idx: -1}
}

// LabelNames implements Store.
func (s *MemStore) LabelNames(_ context.Context, mint, maxt int64, matchers ...*labels.Matcher) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	nameSet := make(map[string]struct{})
	for _, bucket := range s.series {
		for _, ms := range bucket {
			if !matchesAll(ms.lset, matchers) {
				continue
			}
			if !hasOverlap(ms.samples, mint, maxt) {
				continue
			}
			ms.lset.Range(func(l labels.Label) {
				nameSet[l.Name] = struct{}{}
			})
		}
	}
	names := make([]string, 0, len(nameSet))
	for n := range nameSet {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// LabelValues implements Store.
func (s *MemStore) LabelValues(_ context.Context, name string, mint, maxt int64, matchers ...*labels.Matcher) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	valueSet := make(map[string]struct{})
	for _, bucket := range s.series {
		for _, ms := range bucket {
			if !matchesAll(ms.lset, matchers) {
				continue
			}
			if !hasOverlap(ms.samples, mint, maxt) {
				continue
			}
			if v := ms.lset.Get(name); v != "" {
				valueSet[v] = struct{}{}
			}
		}
	}
	values := make([]string, 0, len(valueSet))
	for v := range valueSet {
		values = append(values, v)
	}
	sort.Strings(values)
	return values, nil
}

// Close implements Store.
func (s *MemStore) Close() error {
	return nil
}

// Prune removes samples older than the retention window relative to now.
func (s *MemStore) Prune(now time.Time) {
	if s.retention == 0 {
		return
	}
	cutoff := now.Add(-s.retention).UnixMilli()
	s.mu.Lock()
	defer s.mu.Unlock()
	for fp, bucket := range s.series {
		for i := len(bucket) - 1; i >= 0; i-- {
			ms := bucket[i]
			ms.samples = pruneSamples(ms.samples, cutoff)
			if len(ms.samples) == 0 {
				bucket = append(bucket[:i], bucket[i+1:]...)
			}
		}
		if len(bucket) == 0 {
			delete(s.series, fp)
		} else {
			s.series[fp] = bucket
		}
	}
}

func (s *MemStore) appendSamples(pending []pendingSample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range pending {
		fp := p.labels.Hash()
		bucket := s.series[fp]
		ms := findSeries(bucket, p.labels)
		if ms == nil {
			s.nextRef++
			ms = &memSeries{ref: s.nextRef, lset: p.labels}
			s.series[fp] = append(bucket, ms)
		}
		ms.samples = append(ms.samples, Sample{T: p.t, V: p.v})
	}
}

type pendingSample struct {
	labels labels.Labels
	t      int64
	v      float64
}

type memAppender struct {
	store   *MemStore
	pending []pendingSample
}

func (a *memAppender) Append(_ storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	a.pending = append(a.pending, pendingSample{labels: l, t: t, v: v})
	return 0, nil
}

func (a *memAppender) Commit() error {
	a.store.appendSamples(a.pending)
	a.pending = nil
	return nil
}

func (a *memAppender) Rollback() error {
	a.pending = nil
	return nil
}

// concreteSeries is a Series backed by a snapshot of labels and samples.
type concreteSeries struct {
	lset    labels.Labels
	samples []Sample
}

func (s *concreteSeries) Labels() labels.Labels { return s.lset }
func (s *concreteSeries) Samples() []Sample     { return s.samples }

// sliceSeriesSet iterates over a slice of Series.
type sliceSeriesSet struct {
	series []Series
	idx    int
}

func (s *sliceSeriesSet) Next() bool {
	s.idx++
	return s.idx < len(s.series)
}

func (s *sliceSeriesSet) At() Series { return s.series[s.idx] }
func (s *sliceSeriesSet) Err() error { return nil }

func findSeries(bucket []*memSeries, lset labels.Labels) *memSeries {
	for _, ms := range bucket {
		if labels.Equal(ms.lset, lset) {
			return ms
		}
	}
	return nil
}

func matchesAll(lset labels.Labels, matchers []*labels.Matcher) bool {
	for _, m := range matchers {
		if !m.Matches(lset.Get(m.Name)) {
			return false
		}
	}
	return true
}

func filterSamples(samples []Sample, mint, maxt int64) []Sample {
	var result []Sample
	for _, s := range samples {
		if s.T >= mint && s.T <= maxt {
			result = append(result, s)
		}
	}
	return result
}

func hasOverlap(samples []Sample, mint, maxt int64) bool {
	for _, s := range samples {
		if s.T >= mint && s.T <= maxt {
			return true
		}
	}
	return false
}

func pruneSamples(samples []Sample, cutoff int64) []Sample {
	idx := sort.Search(len(samples), func(i int) bool {
		return samples[i].T >= cutoff
	})
	if idx == 0 {
		return samples
	}
	return samples[idx:]
}
