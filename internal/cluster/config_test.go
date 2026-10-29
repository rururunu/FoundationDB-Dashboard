package cluster

import "testing"

func TestDiffConfig(t *testing.T) {
	cur := Config{Redundancy: "single", StorageEngine: "ssd-2", CommitProxies: 1, GrvProxies: 1, Logs: 1, Resolvers: 1, Processes: 1}
	same, err := DiffConfig(cur, cur)
	if err != nil || len(same) != 0 {
		t.Fatalf("same %#v %v", same, err)
	}
	next := cur
	next.CommitProxies = 2
	changes, err := DiffConfig(cur, next)
	if err != nil || changes["commit_proxies"] != "2" || len(changes) != 1 {
		t.Fatalf("%#v %v", changes, err)
	}
	next = cur
	next.Redundancy = "double"
	if _, err := DiffConfig(cur, next); err == nil {
		t.Fatal("one process cannot become double")
	}
	next = cur
	next.StorageEngine = "tape"
	if _, err := DiffConfig(cur, next); err == nil {
		t.Fatal("unknown engine")
	}
}
