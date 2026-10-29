// Package trend keeps workload samples collected while the process is running.
package trend

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"os"
	"sync"
	"time"

	"foundationdb-dashboard/internal/cluster"
)

const keep = 24 * time.Hour

// Sample is one read/write rate at a moment.
type Sample struct {
	T int64   `json:"t"`
	R float64 `json:"r"`
	W float64 `json:"w"`
}

// Source is the cluster status the collector reads.
type Source interface {
	Overview(context.Context) (cluster.Overview, error)
}

// Buffer stores samples for the last 24 hours.
type Buffer struct {
	mu      sync.Mutex
	samples []Sample
	path    string
	dirty   int
}

// New loads any samples already saved at path. An empty path stays in memory.
func New(path string) *Buffer {
	buf := &Buffer{path: path, samples: []Sample{}}
	if path == "" {
		return buf
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return buf
	}
	var loaded []Sample
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return buf
	}
	buf.samples = loaded
	buf.trim(time.Now())
	return buf
}

// Add records one sample and drops anything older than 24 hours.
func (b *Buffer) Add(at time.Time, reads, writes float64) {
	if b == nil || math.IsNaN(reads) || math.IsNaN(writes) || math.IsInf(reads, 0) || math.IsInf(writes, 0) {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	ms := at.UnixMilli()
	if n := len(b.samples); n > 0 && ms-b.samples[n-1].T < 2000 {
		return
	}
	b.samples = append(b.samples, Sample{T: ms, R: reads, W: writes})
	b.trim(at)
	b.dirty++
	if b.dirty >= 6 {
		b.dirty = 0
		b.writeLocked()
	}
}

// After returns samples strictly newer than ms.
func (b *Buffer) After(ms int64) []Sample {
	if b == nil {
		return []Sample{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []Sample{}
	for _, sample := range b.samples {
		if sample.T > ms {
			out = append(out, sample)
		}
	}
	return out
}

// Clear drops every sample.
func (b *Buffer) Clear() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.samples = []Sample{}
	b.dirty = 0
	b.writeLocked()
}

// Flush writes pending samples.
func (b *Buffer) Flush() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dirty > 0 {
		b.dirty = 0
		b.writeLocked()
	}
}

func (b *Buffer) trim(now time.Time) {
	cutoff := now.Add(-keep).UnixMilli()
	drop := 0
	for drop < len(b.samples) && b.samples[drop].T < cutoff {
		drop++
	}
	if drop > 0 {
		b.samples = append([]Sample(nil), b.samples[drop:]...)
	}
}

func (b *Buffer) writeLocked() {
	if b.path == "" {
		return
	}
	raw, err := json.Marshal(b.samples)
	if err != nil {
		return
	}
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, b.path)
}

// Run samples the cluster until ctx is cancelled.
func Run(ctx context.Context, src Source, buf *Buffer, every time.Duration) {
	if every < time.Second {
		every = 5 * time.Second
	}
	sample(ctx, src, buf)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			buf.Flush()
			return
		case <-ticker.C:
			sample(ctx, src, buf)
		}
	}
}

func sample(ctx context.Context, src Source, buf *Buffer) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	ov, err := src.Overview(ctx)
	if err != nil {
		slog.Debug("trend sample", "err", err)
		return
	}
	buf.Add(time.Now(), ov.Workload.Reads, ov.Workload.Writes)
}
