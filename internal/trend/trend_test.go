package trend

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"foundationdb-dashboard/internal/cluster"
)

type stub struct {
	reads float64
}

func (s stub) Overview(context.Context) (cluster.Overview, error) {
	return cluster.Overview{Workload: cluster.Workload{Reads: s.reads, Writes: 1}}, nil
}

func TestBufferKeepsADayAndCursor(t *testing.T) {
	buf := New("")
	old := time.Now().Add(-25 * time.Hour)
	buf.Add(old, 9, 9)
	now := time.Now()
	buf.Add(now, 2, 4)
	buf.Add(now.Add(3*time.Second), 3, 5)
	got := buf.After(0)
	if len(got) != 2 || got[0].R != 2 || got[0].W != 4 {
		t.Fatalf("%#v", got)
	}
	later := buf.After(got[0].T)
	if len(later) != 1 || later[0].R != 3 {
		t.Fatalf("cursor %#v", later)
	}
	buf.Clear()
	if len(buf.After(0)) != 0 {
		t.Fatal("clear")
	}
}

func TestBufferFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trend.json")
	buf := New(path)
	buf.Add(time.Now(), 5, 6)
	buf.Flush()
	again := New(path)
	got := again.After(0)
	if len(got) != 1 || got[0].R != 5 {
		t.Fatalf("%#v", got)
	}
}

func TestRunSamplesOnce(t *testing.T) {
	buf := New("")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	Run(ctx, stub{reads: 7}, buf, time.Second)
	got := buf.After(0)
	if len(got) != 1 || got[0].R != 7 {
		t.Fatalf("%#v", got)
	}
}
