package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

// VMRemoteConfig holds VictoriaMetrics remote backend configuration.
type VMRemoteConfig struct {
	WriteURL   string
	ReadURL    string
	Timeout    time.Duration
	Username   string
	Password   string
	BatchSize  int
	MaxRetries int
}

// VMRemote implements the Store interface using VictoriaMetrics import/export APIs.
type VMRemote struct {
	logger *slog.Logger
	cfg    VMRemoteConfig
	client *http.Client
}

// NewVMRemote creates a VictoriaMetrics remote backend.
func NewVMRemote(logger *slog.Logger, cfg VMRemoteConfig) *VMRemote {
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 500
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &VMRemote{
		logger: logger,
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

// vmImportRow is the JSON format for VictoriaMetrics /api/v1/import.
type vmImportRow struct {
	Metric     map[string]string `json:"metric"`
	Values     []float64         `json:"values"`
	Timestamps []int64           `json:"timestamps"`
}

// Appender implements Store.
func (v *VMRemote) Appender(_ context.Context) Appender {
	return &vmAppender{store: v}
}

// Select implements Store.
func (v *VMRemote) Select(ctx context.Context, sortSeries bool, mint, maxt int64, matchers ...*labels.Matcher) SeriesSet {
	if v.cfg.ReadURL == "" {
		return &sliceSeriesSet{idx: -1}
	}
	_ = ctx
	query := buildMatcherQuery(matchers)
	u := fmt.Sprintf("%s/api/v1/export?match[]=%s&start=%d&end=%d",
		strings.TrimRight(v.cfg.ReadURL, "/"),
		url.QueryEscape(query),
		mint, maxt)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return &sliceSeriesSet{idx: -1}
	}
	if v.cfg.Username != "" {
		req.SetBasicAuth(v.cfg.Username, v.cfg.Password)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		v.logger.Warn("VM read failed", "error", err)
		return &sliceSeriesSet{idx: -1}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return &sliceSeriesSet{idx: -1}
	}
	var result []Series
	dec := json.NewDecoder(resp.Body)
	for dec.More() {
		var row vmImportRow
		if err := dec.Decode(&row); err != nil {
			break
		}
		b := labels.NewBuilder(labels.EmptyLabels())
		for k, val := range row.Metric {
			b.Set(k, val)
		}
		lset := b.Labels()
		samples := make([]Sample, len(row.Values))
		for i := range row.Values {
			samples[i] = Sample{T: row.Timestamps[i], V: row.Values[i]}
		}
		result = append(result, &concreteSeries{lset: lset, samples: samples})
	}
	if sortSeries {
		sort.Slice(result, func(i, j int) bool {
			return labels.Compare(result[i].Labels(), result[j].Labels()) < 0
		})
	}
	return &sliceSeriesSet{series: result, idx: -1}
}

// LabelNames implements Store.
func (v *VMRemote) LabelNames(_ context.Context, _, _ int64, _ ...*labels.Matcher) ([]string, error) {
	return nil, nil
}

// LabelValues implements Store.
func (v *VMRemote) LabelValues(_ context.Context, _ string, _, _ int64, _ ...*labels.Matcher) ([]string, error) {
	return nil, nil
}

// Close implements Store.
func (v *VMRemote) Close() error {
	return nil
}

func (v *VMRemote) send(rows []vmImportRow) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, row := range rows {
		if err := enc.Encode(row); err != nil {
			return fmt.Errorf("encoding import row: %w", err)
		}
	}
	writeURL := strings.TrimRight(v.cfg.WriteURL, "/") + "/api/v1/import"
	var lastErr error
	for attempt := range v.cfg.MaxRetries {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<attempt) * 100 * time.Millisecond)
		}
		req, err := http.NewRequest(http.MethodPost, writeURL, bytes.NewReader(buf.Bytes()))
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if v.cfg.Username != "" {
			req.SetBasicAuth(v.cfg.Username, v.cfg.Password)
		}
		resp, err := v.client.Do(req)
		if err != nil {
			lastErr = err
			v.logger.Warn("VM write failed, retrying", "attempt", attempt+1, "error", err)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode/100 == 2 {
			return nil
		}
		if resp.StatusCode/100 == 5 {
			lastErr = fmt.Errorf("VM write: status %d", resp.StatusCode)
			v.logger.Warn("VM write server error, retrying", "attempt", attempt+1, "status", resp.StatusCode)
			continue
		}
		return fmt.Errorf("VM write: status %d", resp.StatusCode)
	}
	v.logger.Error("VM write failed after retries, dropping batch", "error", lastErr, "rows", len(rows))
	return nil
}

type vmAppender struct {
	store   *VMRemote
	mu      sync.Mutex
	pending map[uint64]*vmPendingSeries
}

type vmPendingSeries struct {
	lset    labels.Labels
	samples []Sample
}

func (a *vmAppender) Append(_ storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pending == nil {
		a.pending = make(map[uint64]*vmPendingSeries)
	}
	fp := l.Hash()
	ps, ok := a.pending[fp]
	if !ok {
		ps = &vmPendingSeries{lset: l}
		a.pending[fp] = ps
	}
	ps.samples = append(ps.samples, Sample{T: t, V: v})
	return 0, nil
}

func (a *vmAppender) Commit() error {
	a.mu.Lock()
	pending := a.pending
	a.pending = nil
	a.mu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	var rows []vmImportRow
	for _, ps := range pending {
		metric := make(map[string]string)
		ps.lset.Range(func(l labels.Label) {
			metric[l.Name] = l.Value
		})
		values := make([]float64, len(ps.samples))
		timestamps := make([]int64, len(ps.samples))
		for i, s := range ps.samples {
			values[i] = s.V
			timestamps[i] = s.T
		}
		rows = append(rows, vmImportRow{Metric: metric, Values: values, Timestamps: timestamps})
	}
	batchSize := a.store.cfg.BatchSize
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		if err := a.store.send(rows[i:end]); err != nil {
			return err
		}
	}
	return nil
}

func (a *vmAppender) Rollback() error {
	a.mu.Lock()
	a.pending = nil
	a.mu.Unlock()
	return nil
}

func buildMatcherQuery(matchers []*labels.Matcher) string {
	if len(matchers) == 0 {
		return "{__name__=~\".+\"}"
	}
	var parts []string
	for _, m := range matchers {
		switch m.Type {
		case labels.MatchEqual:
			parts = append(parts, fmt.Sprintf("%s=%q", m.Name, m.Value))
		case labels.MatchNotEqual:
			parts = append(parts, fmt.Sprintf("%s!=%q", m.Name, m.Value))
		case labels.MatchRegexp:
			parts = append(parts, fmt.Sprintf("%s=~%q", m.Name, m.Value))
		case labels.MatchNotRegexp:
			parts = append(parts, fmt.Sprintf("%s!~%q", m.Name, m.Value))
		}
	}
	return "{" + strings.Join(parts, ",") + "}"
}
