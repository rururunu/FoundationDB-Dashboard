//go:build linux && cgo

package fdbstore

import (
	"bytes"

	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"

	"foundationdb-dashboard/internal/present"
)

// describe renders key as a tuple when pack(unpack(key)) is the same bytes.
// A panic from the tuple codec means the key is not a tuple.
func describe(key []byte) (text string, ok bool) {
	defer func() {
		if recover() != nil {
			text = ""
			ok = false
		}
	}()
	unpacked, err := tuple.Unpack(key)
	if err != nil {
		return "", false
	}
	if !bytes.Equal(unpacked.Pack(), key) {
		return "", false
	}
	return present.Format(unpacked), true
}

func tupleText(data []byte) (string, bool) { return describe(data) }

// summarizeValue reads a stored value for the key list. data may be shorter
// than length when the value was too large to copy; then only bytes are shown.
func summarizeValue(data []byte, length int, full bool) present.Summary {
	if len(data) != length {
		return present.Summary{Text: present.HexPreview(data), Kind: "bytes"}
	}
	return present.Summarize(data, tupleText, full)
}

// childOf returns the packed tuple of the first n+1 elements of key, and the
// text of element n. ok is false when key is not a complete tuple that is
// longer than n.
func childOf(key []byte, n int) (child []byte, label string, ok bool) {
	defer func() {
		if recover() != nil {
			child, label, ok = nil, "", false
		}
	}()
	unpacked, err := tuple.Unpack(key)
	if err != nil || len(unpacked) <= n {
		return nil, "", false
	}
	if !bytes.Equal(unpacked.Pack(), key) {
		return nil, "", false
	}
	child = unpacked[:n+1].Pack()
	if !bytes.HasPrefix(key, child) {
		return nil, "", false
	}
	return child, present.Format(unpacked[n]), true
}

// elementCount is the number of elements in a packed tuple prefix.
func elementCount(prefix []byte) (n int, ok bool) {
	defer func() {
		if recover() != nil {
			n, ok = 0, false
		}
	}()
	if len(prefix) == 0 {
		return 0, true
	}
	unpacked, err := tuple.Unpack(prefix)
	if err != nil || !bytes.Equal(unpacked.Pack(), prefix) {
		return 0, false
	}
	return len(unpacked), true
}
