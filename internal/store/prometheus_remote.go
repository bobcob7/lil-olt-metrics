package store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
)

// PrometheusRemoteConfig holds Prometheus remote-write backend configuration.
type PrometheusRemoteConfig struct {
	WriteURL   string
	ReadURL    string
	Timeout    time.Duration
	Username   string
	Password   string
	BatchSize  int
	MaxRetries int
}

// PrometheusRemote implements the Store interface by forwarding writes to a
// Prometheus remote-write endpoint.
type PrometheusRemote struct {
	logger *slog.Logger
	cfg    PrometheusRemoteConfig
	client *http.Client
}

// NewPrometheusRemote creates a Prometheus remote-write backend.
func NewPrometheusRemote(logger *slog.Logger, cfg PrometheusRemoteConfig) *PrometheusRemote {
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 500
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &PrometheusRemote{
		logger: logger,
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

// Appender implements Store.
func (p *PrometheusRemote) Appender(_ context.Context) Appender {
	return &promRemoteAppender{store: p}
}

// Select implements Store.
func (p *PrometheusRemote) Select(_ context.Context, _ bool, _, _ int64, _ ...*labels.Matcher) SeriesSet {
	return &sliceSeriesSet{idx: -1}
}

// LabelNames implements Store.
func (p *PrometheusRemote) LabelNames(_ context.Context, _, _ int64, _ ...*labels.Matcher) ([]string, error) {
	return nil, nil
}

// LabelValues implements Store.
func (p *PrometheusRemote) LabelValues(_ context.Context, _ string, _, _ int64, _ ...*labels.Matcher) ([]string, error) {
	return nil, nil
}

// Close implements Store.
func (p *PrometheusRemote) Close() error {
	return nil
}

func (p *PrometheusRemote) send(timeseries []prompb.TimeSeries) error {
	req := &prompb.WriteRequest{Timeseries: timeseries}
	data, err := req.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling write request: %w", err)
	}
	compressed := snappy.Encode(nil, data)
	var lastErr error
	for attempt := range p.cfg.MaxRetries {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<attempt) * 100 * time.Millisecond)
		}
		httpReq, err := http.NewRequest(http.MethodPost, p.cfg.WriteURL, bytes.NewReader(compressed))
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/x-protobuf")
		httpReq.Header.Set("Content-Encoding", "snappy")
		httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
		if p.cfg.Username != "" {
			httpReq.SetBasicAuth(p.cfg.Username, p.cfg.Password)
		}
		resp, err := p.client.Do(httpReq)
		if err != nil {
			lastErr = err
			p.logger.Warn("remote write failed, retrying", "attempt", attempt+1, "error", err)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			p.logger.Error("closing response body", "error", closeErr)
		}
		if resp.StatusCode/100 == 2 {
			return nil
		}
		if resp.StatusCode/100 == 5 {
			lastErr = fmt.Errorf("remote write: status %d", resp.StatusCode)
			p.logger.Warn("remote write server error, retrying", "attempt", attempt+1, "status", resp.StatusCode)
			continue
		}
		return fmt.Errorf("remote write: status %d", resp.StatusCode)
	}
	p.logger.Error("remote write failed after retries, dropping batch", "error", lastErr, "samples", len(timeseries))
	return nil
}

type promRemoteAppender struct {
	store   *PrometheusRemote
	mu      sync.Mutex
	pending []prompb.TimeSeries
}

func (a *promRemoteAppender) Append(_ storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	lbls := make([]prompb.Label, 0, l.Len())
	l.Range(func(lbl labels.Label) {
		lbls = append(lbls, prompb.Label{Name: lbl.Name, Value: lbl.Value})
	})
	a.pending = append(a.pending, prompb.TimeSeries{
		Labels:  lbls,
		Samples: []prompb.Sample{{Value: v, Timestamp: t}},
	})
	return 0, nil
}

func (a *promRemoteAppender) Commit() error {
	a.mu.Lock()
	pending := a.pending
	a.pending = nil
	a.mu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	batchSize := a.store.cfg.BatchSize
	for i := 0; i < len(pending); i += batchSize {
		end := i + batchSize
		if end > len(pending) {
			end = len(pending)
		}
		if err := a.store.send(pending[i:end]); err != nil {
			return err
		}
	}
	return nil
}

func (a *promRemoteAppender) Rollback() error {
	a.mu.Lock()
	a.pending = nil
	a.mu.Unlock()
	return nil
}
