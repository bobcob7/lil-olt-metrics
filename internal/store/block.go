package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/prometheus/prometheus/model/labels"
)

// blockMeta holds metadata about an on-disk block.
type blockMeta struct {
	MinTime int64 `json:"minTime"`
	MaxTime int64 `json:"maxTime"`
	Samples int   `json:"samples"`
}

// block represents an immutable on-disk block of time series data.
type block struct {
	dir    string
	meta   blockMeta
	series []blockSeries
}

type blockSeries struct {
	Labels  labels.Labels `json:"labels"`
	Samples []Sample      `json:"samples"`
}

// writeBlock persists the given series data as an immutable block.
func writeBlock(dir string, series []blockSeries, mint, maxt int64) (*block, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating block dir: %w", err)
	}
	sampleCount := 0
	for _, s := range series {
		sampleCount += len(s.Samples)
	}
	meta := blockMeta{MinTime: mint, MaxTime: maxt, Samples: sampleCount}
	metaPath := filepath.Join(dir, "meta.json")
	mf, err := os.Create(metaPath)
	if err != nil {
		return nil, fmt.Errorf("creating meta file: %w", err)
	}
	if err := json.NewEncoder(mf).Encode(meta); err != nil {
		mf.Close()
		return nil, fmt.Errorf("writing meta: %w", err)
	}
	mf.Close()
	dataPath := filepath.Join(dir, "data.json")
	df, err := os.Create(dataPath)
	if err != nil {
		return nil, fmt.Errorf("creating data file: %w", err)
	}
	if err := json.NewEncoder(df).Encode(series); err != nil {
		df.Close()
		return nil, fmt.Errorf("writing data: %w", err)
	}
	df.Close()
	return &block{dir: dir, meta: meta, series: series}, nil
}

// openBlock reads an existing block from disk.
func openBlock(dir string) (*block, error) {
	metaPath := filepath.Join(dir, "meta.json")
	mf, err := os.Open(metaPath)
	if err != nil {
		return nil, fmt.Errorf("opening meta: %w", err)
	}
	defer mf.Close()
	var meta blockMeta
	if err := json.NewDecoder(mf).Decode(&meta); err != nil {
		return nil, fmt.Errorf("decoding meta: %w", err)
	}
	dataPath := filepath.Join(dir, "data.json")
	df, err := os.Open(dataPath)
	if err != nil {
		return nil, fmt.Errorf("opening data: %w", err)
	}
	defer df.Close()
	var series []blockSeries
	if err := json.NewDecoder(df).Decode(&series); err != nil {
		return nil, fmt.Errorf("decoding data: %w", err)
	}
	return &block{dir: dir, meta: meta, series: series}, nil
}

// blockDiskSize returns the total size of all files in the block directory.
func blockDiskSize(dir string) (int64, error) {
	var total int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total, nil
}

// select returns series from the block matching the matchers and time range.
func (b *block) selectSeries(sortSeries bool, mint, maxt int64, matchers ...*labels.Matcher) []Series {
	var result []Series
	for _, bs := range b.series {
		if !matchesAll(bs.Labels, matchers) {
			continue
		}
		samples := filterSamples(bs.Samples, mint, maxt)
		if len(samples) == 0 {
			continue
		}
		result = append(result, &concreteSeries{lset: bs.Labels, samples: samples})
	}
	if sortSeries {
		sort.Slice(result, func(i, j int) bool {
			return labels.Compare(result[i].Labels(), result[j].Labels()) < 0
		})
	}
	return result
}

// compactBlocks merges multiple blocks into a single new block.
func compactBlocks(targetDir string, blocks []*block) (*block, error) {
	seriesMap := make(map[uint64]*blockSeries)
	var mint, maxt int64
	first := true
	for _, b := range blocks {
		if first || b.meta.MinTime < mint {
			mint = b.meta.MinTime
		}
		if first || b.meta.MaxTime > maxt {
			maxt = b.meta.MaxTime
		}
		first = false
		for _, bs := range b.series {
			fp := bs.Labels.Hash()
			existing, ok := seriesMap[fp]
			if ok && labels.Equal(existing.Labels, bs.Labels) {
				existing.Samples = append(existing.Samples, bs.Samples...)
			} else if !ok {
				clone := blockSeries{Labels: bs.Labels, Samples: append([]Sample(nil), bs.Samples...)}
				seriesMap[fp] = &clone
			}
		}
	}
	merged := make([]blockSeries, 0, len(seriesMap))
	for _, bs := range seriesMap {
		sort.Slice(bs.Samples, func(i, j int) bool {
			return bs.Samples[i].T < bs.Samples[j].T
		})
		bs.Samples = dedup(bs.Samples)
		merged = append(merged, *bs)
	}
	return writeBlock(targetDir, merged, mint, maxt)
}

func dedup(samples []Sample) []Sample {
	if len(samples) <= 1 {
		return samples
	}
	result := samples[:1]
	for i := 1; i < len(samples); i++ {
		if samples[i].T != result[len(result)-1].T {
			result = append(result, samples[i])
		}
	}
	return result
}
