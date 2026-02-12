package store

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/prometheus/model/labels"
)

// WAL is a write-ahead log that persists samples to disk for crash recovery.
// Samples are appended to the current segment. When a segment reaches the
// configured max size, it is rotated.
type WAL struct {
	dir         string
	maxSegSize  int64
	mu          sync.Mutex
	segment     *os.File
	segmentSeq  int
	segmentSize int64
}

// NewWAL opens or creates a WAL in the given directory.
func NewWAL(dir string, maxSegmentSize int64) (*WAL, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating WAL dir: %w", err)
	}
	w := &WAL{dir: dir, maxSegSize: maxSegmentSize}
	seq, err := w.latestSegmentSeq()
	if err != nil {
		return nil, err
	}
	w.segmentSeq = seq
	f, err := os.OpenFile(w.segmentPath(seq), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening WAL segment: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	w.segment = f
	w.segmentSize = info.Size()
	return w, nil
}

// walRecord is the on-disk format for a WAL entry.
// Format: [labelCount uint16][labels...][timestamp int64][value float64]
// Each label: [keyLen uint16][key][valLen uint16][val]
type walRecord struct {
	Labels labels.Labels
	T      int64
	V      float64
}

// Log appends samples to the WAL.
func (w *WAL) Log(records []walRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, rec := range records {
		data := encodeWALRecord(rec)
		n, err := w.segment.Write(data)
		if err != nil {
			return fmt.Errorf("writing WAL record: %w", err)
		}
		w.segmentSize += int64(n)
		if w.segmentSize >= w.maxSegSize {
			if err := w.rotate(); err != nil {
				return fmt.Errorf("rotating WAL segment: %w", err)
			}
		}
	}
	return nil
}

// Replay reads all WAL segments in order and calls fn for each record.
func (w *WAL) Replay(fn func(walRecord)) error {
	segments, err := w.listSegments()
	if err != nil {
		return err
	}
	for _, seg := range segments {
		if err := replaySegment(seg, fn); err != nil {
			return fmt.Errorf("replaying segment %s: %w", seg, err)
		}
	}
	return nil
}

// Truncate removes all WAL segments with sequence number less than the given value.
func (w *WAL) Truncate(beforeSeq int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		seq, parseErr := parseSegmentSeq(e.Name())
		if parseErr != nil {
			continue
		}
		if seq < beforeSeq {
			if err := os.Remove(filepath.Join(w.dir, e.Name())); err != nil {
				return fmt.Errorf("removing old segment: %w", err)
			}
		}
	}
	return nil
}

// Close syncs and closes the current segment.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.segment == nil {
		return nil
	}
	if err := w.segment.Sync(); err != nil {
		return err
	}
	return w.segment.Close()
}

// SegmentSeq returns the current segment sequence number.
func (w *WAL) SegmentSeq() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.segmentSeq
}

func (w *WAL) rotate() error {
	if err := w.segment.Sync(); err != nil {
		return err
	}
	if err := w.segment.Close(); err != nil {
		return err
	}
	w.segmentSeq++
	f, err := os.OpenFile(w.segmentPath(w.segmentSeq), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w.segment = f
	w.segmentSize = 0
	return nil
}

func (w *WAL) segmentPath(seq int) string {
	return filepath.Join(w.dir, fmt.Sprintf("%08d", seq))
}

func (w *WAL) latestSegmentSeq() (int, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return 0, nil
	}
	maxSeq := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		seq, parseErr := parseSegmentSeq(e.Name())
		if parseErr != nil {
			continue
		}
		if seq > maxSeq {
			maxSeq = seq
		}
	}
	return maxSeq, nil
}

func (w *WAL) listSegments() ([]string, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, parseErr := parseSegmentSeq(e.Name()); parseErr == nil {
			paths = append(paths, filepath.Join(w.dir, e.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func parseSegmentSeq(name string) (int, error) {
	name = strings.TrimLeft(name, "0")
	if name == "" {
		return 0, nil
	}
	return strconv.Atoi(name)
}

func encodeWALRecord(rec walRecord) []byte {
	var buf []byte
	lbls := make([]labels.Label, 0, rec.Labels.Len())
	rec.Labels.Range(func(l labels.Label) {
		lbls = append(lbls, l)
	})
	buf = binary.BigEndian.AppendUint16(buf, uint16(len(lbls)))
	for _, l := range lbls {
		buf = binary.BigEndian.AppendUint16(buf, uint16(len(l.Name)))
		buf = append(buf, l.Name...)
		buf = binary.BigEndian.AppendUint16(buf, uint16(len(l.Value)))
		buf = append(buf, l.Value...)
	}
	buf = binary.BigEndian.AppendUint64(buf, uint64(rec.T))
	buf = binary.BigEndian.AppendUint64(buf, math.Float64bits(rec.V))
	size := make([]byte, 4)
	binary.BigEndian.PutUint32(size, uint32(len(buf)))
	return append(size, buf...)
}

func replaySegment(path string, fn func(walRecord)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for {
		rec, err := decodeWALRecord(f)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		fn(rec)
	}
}

func decodeWALRecord(r io.Reader) (walRecord, error) {
	var sizeBuf [4]byte
	if _, err := io.ReadFull(r, sizeBuf[:]); err != nil {
		return walRecord{}, err
	}
	size := binary.BigEndian.Uint32(sizeBuf[:])
	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return walRecord{}, err
	}
	offset := 0
	labelCount := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	b := labels.NewScratchBuilder(labelCount)
	for range labelCount {
		keyLen := int(binary.BigEndian.Uint16(data[offset:]))
		offset += 2
		key := string(data[offset : offset+keyLen])
		offset += keyLen
		valLen := int(binary.BigEndian.Uint16(data[offset:]))
		offset += 2
		val := string(data[offset : offset+valLen])
		offset += valLen
		b.Add(key, val)
	}
	t := int64(binary.BigEndian.Uint64(data[offset:]))
	offset += 8
	v := math.Float64frombits(binary.BigEndian.Uint64(data[offset:]))
	return walRecord{Labels: b.Labels(), T: t, V: v}, nil
}
