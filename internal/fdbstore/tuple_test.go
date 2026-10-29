//go:build linux && cgo

package fdbstore

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
)

func TestDescribeRoundTrip(t *testing.T) {
	raw := tuple.Tuple{"alpha", int64(7)}.Pack()
	text, ok := describe(raw)
	if !ok {
		t.Fatal("expected a tuple")
	}
	if text != `("alpha", 7)` {
		t.Fatalf("text %s", text)
	}
}

func TestSummarizeVersionstamp(t *testing.T) {
	raw, err := hex.DecodeString("33000005fe72cb04ae00000000")
	if err != nil {
		t.Fatal(err)
	}
	got := summarizeValue(raw, len(raw), true)
	if got.Kind != "tuple" || got.Text != "(Versionstamp(000005fe72cb04ae0000, user=0),)" || got.Note != "" {
		t.Fatalf("%+v", got)
	}
}

func TestSummarizePrefersTextOverTupleCodes(t *testing.T) {
	// 0x30 is the UUID type code; 17 printable bytes must still read as text.
	raw := []byte("0123456789abcdef0")
	got := summarizeValue(raw, len(raw), false)
	if got.Kind != "text" {
		t.Fatalf("%+v", got)
	}
}

func TestChildOfWalksTuple(t *testing.T) {
	id := tuple.UUID{0x3c, 0x60, 0x47, 0xfa, 0x4f, 0x4e, 0x46, 0x6f, 0xb3, 0x49, 0x49, 0xb7, 0x2f, 0x5d, 0x62, 0x60}
	key := tuple.Tuple{"M", id, "l"}.Pack()

	child, label, ok := childOf(key, 0)
	if !ok || label != `"M"` || !bytes.Equal(child, tuple.Tuple{"M"}.Pack()) {
		t.Fatalf("level 0: %x %s %v", child, label, ok)
	}
	child, label, ok = childOf(key, 1)
	if !ok || label != "uuid(3c6047fa-4f4e-466f-b349-49b72f5d6260)" || !bytes.Equal(child, tuple.Tuple{"M", id}.Pack()) {
		t.Fatalf("level 1: %x %s %v", child, label, ok)
	}
	child, label, ok = childOf(key, 2)
	if !ok || label != `"l"` || !bytes.Equal(child, key) {
		t.Fatalf("level 2: %x %s %v", child, label, ok)
	}
	if _, _, ok := childOf(key, 3); ok {
		t.Fatal("a tuple of three elements has no fourth")
	}
	if _, _, ok := childOf([]byte{0xff, 0x01}, 0); ok {
		t.Fatal("garbage is not a tuple")
	}
}

func TestElementCount(t *testing.T) {
	if n, ok := elementCount(nil); !ok || n != 0 {
		t.Fatalf("empty %d %v", n, ok)
	}
	if n, ok := elementCount(tuple.Tuple{"M", int64(1)}.Pack()); !ok || n != 2 {
		t.Fatalf("pair %d %v", n, ok)
	}
	if _, ok := elementCount([]byte{0xff}); ok {
		t.Fatal("garbage must not count")
	}
}

func TestDescribeRejectsGarbage(t *testing.T) {
	text, ok := describe([]byte{0xff, 0x01, 0x02})
	if ok || text != "" {
		t.Fatalf("garbage decoded as %q ok=%v", text, ok)
	}
}
