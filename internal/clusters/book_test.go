package clusters

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clusters.json")
	book, err := Open(path, "docker", "docker:docker@foundationdb:4500")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := book.Add("", "docker:docker@foundationdb:4500"); err == nil {
		t.Fatal("empty name accepted")
	}
	if _, err := book.Add("lab", "not a connection"); err == nil {
		t.Fatal("bad connection accepted")
	}
	if _, err := book.Add("same", "docker:docker@foundationdb:4500"); err == nil {
		t.Fatal("duplicate accepted")
	}
	added, err := book.Add("lab", "lab:lab@127.0.0.1:4501")
	if err != nil {
		t.Fatal(err)
	}
	if err := book.Remove(BuiltinID); err == nil {
		t.Fatal("builtin removed")
	}
	if err := book.Activate(added.ID); err != nil {
		t.Fatal(err)
	}
	if err := book.Remove(added.ID); err == nil {
		t.Fatal("active removed")
	}
	if err := book.Activate(BuiltinID); err != nil {
		t.Fatal(err)
	}
	if err := book.Remove(added.ID); err != nil {
		t.Fatal(err)
	}

	again, err := Open(path, "docker", "docker:docker@foundationdb:4500")
	if err != nil {
		t.Fatal(err)
	}
	items, active := again.List()
	if active != BuiltinID || len(items) != 1 {
		t.Fatalf("reloaded %#v %s", items, active)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestValidConnectionPorts(t *testing.T) {
	if !ValidConnection("docker:docker@foundationdb:4500") {
		t.Fatal("expected valid")
	}
	if ValidConnection("docker:docker@foundationdb:70000") || ValidConnection("docker:docker@host:0") {
		t.Fatal("port accepted")
	}
}
