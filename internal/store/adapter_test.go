package store

import (
	"context"
	"sort"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryableSelect(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	stubSeries := []Series{
		&stubSeriesImpl{
			labels:  labels.FromStrings("__name__", "http_requests_total", "method", "GET"),
			samples: []Sample{{T: 1000, V: 10}, {T: 2000, V: 20}},
		},
		&stubSeriesImpl{
			labels:  labels.FromStrings("__name__", "http_requests_total", "method", "POST"),
			samples: []Sample{{T: 1000, V: 5}, {T: 2000, V: 15}},
		},
	}
	mock := &StoreMock{
		SelectFunc: func(_ context.Context, _ bool, _, _ int64, _ ...*labels.Matcher) SeriesSet {
			return &sliceSeriesSet{series: stubSeries, idx: -1}
		},
	}
	q := NewQueryable(mock)
	querier, err := q.Querier(0, 10000)
	require.NoError(t, err)
	defer func() {
		if err := querier.Close(); err != nil {
			t.Log("closing querier:", err)
		}
	}()
	matcher := labels.MustNewMatcher(labels.MatchEqual, "__name__", "http_requests_total")
	ss := querier.Select(ctx, false, nil, matcher)
	var collected []storage.Series
	for ss.Next() {
		collected = append(collected, ss.At())
	}
	require.NoError(t, ss.Err())
	assert.Nil(t, ss.Warnings())
	require.Len(t, collected, 2)
	assert.Equal(t, "GET", collected[0].Labels().Get("method"))
	assert.Equal(t, "POST", collected[1].Labels().Get("method"))
	it := collected[0].Iterator(nil)
	require.Equal(t, chunkenc.ValFloat, it.Next())
	ts, v := it.At()
	assert.Equal(t, int64(1000), ts)
	assert.Equal(t, float64(10), v)
	require.Equal(t, chunkenc.ValFloat, it.Next())
	ts, v = it.At()
	assert.Equal(t, int64(2000), ts)
	assert.Equal(t, float64(20), v)
	assert.Equal(t, chunkenc.ValNone, it.Next())
	assert.NoError(t, it.Err())
}

func TestQueryableSelectWithHints(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	var capturedMint, capturedMaxt int64
	mock := &StoreMock{
		SelectFunc: func(_ context.Context, _ bool, mint, maxt int64, _ ...*labels.Matcher) SeriesSet {
			capturedMint = mint
			capturedMaxt = maxt
			return &sliceSeriesSet{idx: -1}
		},
	}
	q := NewQueryable(mock)
	querier, err := q.Querier(0, 10000)
	require.NoError(t, err)
	defer func() {
		if err := querier.Close(); err != nil {
			t.Log("closing querier:", err)
		}
	}()
	hints := &storage.SelectHints{Start: 500, End: 5000}
	querier.Select(ctx, false, hints)
	assert.Equal(t, int64(500), capturedMint)
	assert.Equal(t, int64(5000), capturedMaxt)
}

func TestQueryableLabelNames(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	expected := []string{"__name__", "instance", "job"}
	mock := &StoreMock{
		LabelNamesFunc: func(_ context.Context, _, _ int64, _ ...*labels.Matcher) ([]string, error) {
			return expected, nil
		},
	}
	q := NewQueryable(mock)
	querier, err := q.Querier(0, 10000)
	require.NoError(t, err)
	defer func() {
		if err := querier.Close(); err != nil {
			t.Log("closing querier:", err)
		}
	}()
	names, warnings, err := querier.LabelNames(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, warnings)
	assert.Equal(t, expected, names)
}

func TestQueryableLabelValues(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	expected := []string{"http_requests_total", "process_cpu_seconds_total"}
	mock := &StoreMock{
		LabelValuesFunc: func(_ context.Context, name string, _, _ int64, _ ...*labels.Matcher) ([]string, error) {
			if name == "__name__" {
				return expected, nil
			}
			return nil, nil
		},
	}
	q := NewQueryable(mock)
	querier, err := q.Querier(0, 10000)
	require.NoError(t, err)
	defer func() {
		if err := querier.Close(); err != nil {
			t.Log("closing querier:", err)
		}
	}()
	vals, warnings, err := querier.LabelValues(ctx, "__name__", nil)
	require.NoError(t, err)
	assert.Nil(t, warnings)
	assert.Equal(t, expected, vals)
}

func TestSampleIteratorSeek(t *testing.T) {
	t.Parallel()
	samples := []Sample{{T: 100, V: 1}, {T: 200, V: 2}, {T: 300, V: 3}, {T: 400, V: 4}}
	it := &sampleIterator{samples: samples, idx: -1}
	vt := it.Seek(250)
	require.Equal(t, chunkenc.ValFloat, vt)
	ts, v := it.At()
	assert.Equal(t, int64(300), ts)
	assert.Equal(t, float64(3), v)
}

func TestSampleIteratorSeekPastEnd(t *testing.T) {
	t.Parallel()
	samples := []Sample{{T: 100, V: 1}, {T: 200, V: 2}}
	it := &sampleIterator{samples: samples, idx: -1}
	vt := it.Seek(500)
	assert.Equal(t, chunkenc.ValNone, vt)
}

func TestEmptySeriesSet(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	mock := &StoreMock{
		SelectFunc: func(_ context.Context, _ bool, _, _ int64, _ ...*labels.Matcher) SeriesSet {
			return &sliceSeriesSet{idx: -1}
		},
	}
	q := NewQueryable(mock)
	querier, err := q.Querier(0, 10000)
	require.NoError(t, err)
	defer func() {
		if err := querier.Close(); err != nil {
			t.Log("closing querier:", err)
		}
	}()
	ss := querier.Select(ctx, false, nil)
	assert.False(t, ss.Next())
	assert.NoError(t, ss.Err())
}

func TestSeriesSetSorted(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	series := []Series{
		&stubSeriesImpl{labels: labels.FromStrings("__name__", "b_metric"), samples: []Sample{{T: 1, V: 1}}},
		&stubSeriesImpl{labels: labels.FromStrings("__name__", "a_metric"), samples: []Sample{{T: 1, V: 1}}},
	}
	mock := &StoreMock{
		SelectFunc: func(_ context.Context, sortSeries bool, _, _ int64, _ ...*labels.Matcher) SeriesSet {
			if sortSeries {
				sort.Slice(series, func(i, j int) bool {
					return labels.Compare(series[i].Labels(), series[j].Labels()) < 0
				})
			}
			return &sliceSeriesSet{series: series, idx: -1}
		},
	}
	q := NewQueryable(mock)
	querier, err := q.Querier(0, 10000)
	require.NoError(t, err)
	defer func() {
		if err := querier.Close(); err != nil {
			t.Log("closing querier:", err)
		}
	}()
	ss := querier.Select(ctx, true, nil)
	require.True(t, ss.Next())
	assert.Equal(t, "a_metric", ss.At().Labels().Get("__name__"))
	require.True(t, ss.Next())
	assert.Equal(t, "b_metric", ss.At().Labels().Get("__name__"))
	assert.False(t, ss.Next())
}

// stubSeriesImpl is a simple Series implementation for testing.
type stubSeriesImpl struct {
	labels  labels.Labels
	samples []Sample
}

func (s *stubSeriesImpl) Labels() labels.Labels { return s.labels }
func (s *stubSeriesImpl) Samples() []Sample     { return s.samples }
