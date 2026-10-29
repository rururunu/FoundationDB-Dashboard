package present

import (
	"encoding/binary"
	"strings"
	"testing"
)

func fakeTuple(data []byte) (string, bool) {
	if len(data) > 0 && data[0] == 0x33 {
		return "(Versionstamp(...),)", true
	}
	return "", false
}

func TestSummarizeKinds(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, "bytes"},
		{"tuple", append([]byte{0x33}, make([]byte, 12)...), "tuple"},
		{"json", []byte(`{"a": 1, "b": [true]}`), "json"},
		{"text", []byte("hello world"), "text"},
		{"binary", []byte{0xff, 0xfe, 0x00}, "bytes"},
	}
	for _, tc := range cases {
		if got := Summarize(tc.data, fakeTuple, false); got.Kind != tc.want {
			t.Fatalf("%s: kind %s want %s (%q)", tc.name, got.Kind, tc.want, got.Text)
		}
	}
}

func TestSummarizeJSONAndLength(t *testing.T) {
	data := []byte(`{"name": "x", "tags": ["a", "b"]}`)
	list := Summarize(data, nil, false)
	if list.Text != `{"name":"x","tags":["a","b"]}` {
		t.Fatalf("compact %q", list.Text)
	}
	full := Summarize(data, nil, true)
	if !strings.Contains(full.Text, "\n  \"name\": \"x\"") {
		t.Fatalf("indent %q", full.Text)
	}
	long := Summarize([]byte(strings.Repeat("字", 300)), nil, false)
	if len([]rune(long.Text)) != 121 || !strings.HasSuffix(long.Text, "…") {
		t.Fatalf("shorten %d", len([]rune(long.Text)))
	}
	whole := Summarize([]byte(strings.Repeat("字", 300)), nil, true)
	if len([]rune(whole.Text)) != 300 {
		t.Fatalf("full %d", len([]rune(whole.Text)))
	}
}

func TestSummarizeNotes(t *testing.T) {
	stamp := make([]byte, 13)
	stamp[0] = 0x33
	binary.BigEndian.PutUint64(stamp[1:9], 6598658498199)
	got := Summarize(stamp, fakeTuple, true)
	if got.Kind != "tuple" || got.Note != "" {
		t.Fatalf("%+v", got)
	}
	counter := make([]byte, 8)
	counter[0] = 5
	counter[7] = 0xff
	if n := Summarize(counter, nil, true).Note; !strings.Contains(n, "小端整数") {
		t.Fatalf("counter note %q", n)
	}
}
