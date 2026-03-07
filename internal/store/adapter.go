package store

import (
	"context"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

// NewQueryable wraps a Store into a Prometheus storage.Queryable that can
// be consumed by the PromQL engine.
func NewQueryable(s Store) storage.Queryable {
	return &queryable{store: s}
}

type queryable struct {
	store Store
}

func (q *queryable) Querier(mint, maxt int64) (storage.Querier, error) {
	return &querier{store: q.store, mint: mint, maxt: maxt}, nil
}

type querier struct {
	store Store
	mint  int64
	maxt  int64
}

func (q *querier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	mint, maxt := q.mint, q.maxt
	if hints != nil {
		if hints.Start != 0 {
			mint = hints.Start
		}
		if hints.End != 0 {
			maxt = hints.End
		}
	}
	ss := q.store.Select(ctx, sortSeries, mint, maxt, matchers...)
	return &promSeriesSet{inner: ss}
}

func (q *querier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	vals, err := q.store.LabelValues(ctx, name, q.mint, q.maxt, matchers...)
	return vals, nil, err
}

func (q *querier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	names, err := q.store.LabelNames(ctx, q.mint, q.maxt, matchers...)
	return names, nil, err
}

func (q *querier) Close() error {
	return nil
}

// promSeriesSet bridges our SeriesSet to Prometheus storage.SeriesSet.
type promSeriesSet struct {
	inner SeriesSet
}

func (s *promSeriesSet) Next() bool                        { return s.inner.Next() }
func (s *promSeriesSet) Err() error                        { return s.inner.Err() }
func (s *promSeriesSet) Warnings() annotations.Annotations { return nil }
func (s *promSeriesSet) At() storage.Series                { return &promSeries{inner: s.inner.At()} }

// promSeries bridges our Series to Prometheus storage.Series.
type promSeries struct {
	inner Series
}

func (s *promSeries) Labels() labels.Labels {
	return s.inner.Labels()
}

func (s *promSeries) Iterator(it chunkenc.Iterator) chunkenc.Iterator {
	samples := s.inner.Samples()
	return &sampleIterator{samples: samples, idx: -1}
}

// sampleIterator implements chunkenc.Iterator over a slice of Samples.
type sampleIterator struct {
	samples []Sample
	idx     int
}

func (it *sampleIterator) Next() chunkenc.ValueType {
	it.idx++
	if it.idx >= len(it.samples) {
		return chunkenc.ValNone
	}
	return chunkenc.ValFloat
}

func (it *sampleIterator) Seek(t int64) chunkenc.ValueType {
	if it.idx < 0 {
		it.idx = 0
	}
	for it.idx < len(it.samples) {
		if it.samples[it.idx].T >= t {
			return chunkenc.ValFloat
		}
		it.idx++
	}
	return chunkenc.ValNone
}

func (it *sampleIterator) At() (int64, float64) {
	s := it.samples[it.idx]
	return s.T, s.V
}

func (it *sampleIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

func (it *sampleIterator) AtFloatHistogram(fh *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

func (it *sampleIterator) AtT() int64 {
	if it.idx < 0 || it.idx >= len(it.samples) {
		return 0
	}
	return it.samples[it.idx].T
}

func (it *sampleIterator) Err() error {
	return nil
}
