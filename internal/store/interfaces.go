package store

import (
	"context"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

//go:generate moq -out moq_test.go . Store Appender

// Store is the internal storage interface consumed by both the ingestion
// and query subsystems.
type Store interface {
	// Appender returns a new Appender for batched writes.
	Appender(ctx context.Context) Appender
	// Select returns series matching the given matchers within the time range.
	Select(ctx context.Context, sortSeries bool, mint, maxt int64, matchers ...*labels.Matcher) SeriesSet
	// LabelNames returns all label names within the time range.
	LabelNames(ctx context.Context, mint, maxt int64, matchers ...*labels.Matcher) ([]string, error)
	// LabelValues returns values for a label name within the time range.
	LabelValues(ctx context.Context, name string, mint, maxt int64, matchers ...*labels.Matcher) ([]string, error)
	// Close flushes pending data and releases resources.
	Close() error
}

// Appender provides batched appends against a store. Must be completed with
// Commit or Rollback and must not be reused afterwards.
type Appender interface {
	// Append adds a sample for the given series. Returns a reference for
	// subsequent appends to the same series.
	Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error)
	// Commit submits collected samples.
	Commit() error
	// Rollback discards collected samples.
	Rollback() error
}

// SeriesSet iterates over a set of time series.
type SeriesSet interface {
	Next() bool
	At() Series
	Err() error
}

// Series represents a single time series with its label set and samples.
type Series interface {
	Labels() labels.Labels
	Samples() []Sample
}

// Sample is a single timestamped float64 value.
type Sample struct {
	T int64
	V float64
}
