package present

import (
	"bytes"
	"math/big"
	"testing"
)

func TestParseHex(t *testing.T) {
	got, err := ParseHex("  0x0A0b ")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{0x0a, 0x0b}) {
		t.Fatalf("%x", got)
	}
	empty, err := ParseHex("  ")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty %#v %v", empty, err)
	}
	if _, err := ParseHex("abc"); err == nil {
		t.Fatal("odd length should fail")
	}
	if _, err := ParseHex("zz"); err == nil {
		t.Fatal("non-hex should fail")
	}
}

func TestHexPreview(t *testing.T) {
	if HexPreview(nil) != "(空)" {
		t.Fatal(HexPreview(nil))
	}
	if HexPreview([]byte{0x0a, 0xff}) != "0aff" {
		t.Fatal(HexPreview([]byte{0x0a, 0xff}))
	}
}

func TestSuccessor(t *testing.T) {
	if !bytes.Equal(Successor([]byte{0x01}), []byte{0x01, 0x00}) {
		t.Fatal(Successor([]byte{0x01}))
	}
	if !bytes.Equal(Successor(nil), []byte{0x00}) {
		t.Fatal(Successor(nil))
	}
}

func TestFormatTuple(t *testing.T) {
	type uuid [16]byte
	type versionstamp struct {
		TransactionVersion [10]byte
		UserVersion        uint16
	}
	id := uuid{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}
	nested := []any{"inner", int64(2)}
	got := Format([]any{"M", id, nested, []byte{0xff}, true, (*big.Int)(nil)})
	want := `("M", uuid(01234567-89ab-cdef-0123-456789abcdef), ("inner", 2), 0xff, true, null)`
	if got != want {
		t.Fatalf("got %s", got)
	}

	one := Format([]any{"only"})
	if one != `("only",)` {
		t.Fatalf("one %s", one)
	}

	vs := versionstamp{UserVersion: 4}
	for i := range vs.TransactionVersion {
		vs.TransactionVersion[i] = 0xff
	}
	if Format(vs) != "Versionstamp(incomplete, user=4)" {
		t.Fatalf("versionstamp %s", Format(vs))
	}
}
