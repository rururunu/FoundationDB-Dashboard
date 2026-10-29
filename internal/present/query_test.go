package present

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestParseQueryKnownTuple(t *testing.T) {
	begin, end, err := ParseQuery(`("M", uuid(3c6047fa-4f4e-466f-b349-49b72f5d6260), "l")`)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := hex.DecodeString("024d00303c6047fa4f4e466fb34949b72f5d6260026c00")
	if !bytes.Equal(begin, want) {
		t.Fatalf("begin %x", begin)
	}
	if bytes.Compare(begin, end) >= 0 {
		t.Fatalf("end %x is not after begin", end)
	}

	prefix, limit, err := ParseQuery(`"M"`)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(prefix, []byte{0x02, 'M', 0x00}) || !bytes.Equal(limit, []byte{0x02, 'M', 0x00, 0xff}) {
		t.Fatalf("prefix %x end %x", prefix, limit)
	}
	if bytes.Compare(want, prefix) < 0 || bytes.Compare(want, limit) >= 0 {
		t.Fatal("full key is outside the M prefix")
	}
}

func TestParseQueryElements(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`("only",)`, "026f6e6c7900"},
		{`true`, "27"},
		{`false`, "26"},
		{`null`, "00"},
		{`0`, "14"},
		{`1`, "1501"},
		{`-1`, "13fe"},
		{`256`, "160100"},
		{`0xff`, "01ff00"},
		{`0x0001`, "0100ff0100"},
		{`("M", ("inner", 2))`, "024d000502696e6e657200150200"},
		{`M`, "024d00"},
		{`(true, null)`, "2700"},
	}
	for _, tc := range cases {
		begin, _, err := ParseQuery(tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if hex.EncodeToString(begin) != tc.want {
			t.Fatalf("%s got %x want %s", tc.in, begin, tc.want)
		}
	}
	begin, end, err := ParseQuery("  ")
	if err != nil || len(begin) != 0 || !bytes.Equal(end, []byte{0xff}) {
		t.Fatalf("empty %#v %#v %v", begin, end, err)
	}
	if _, _, err := ParseQuery(`("M"`); err == nil {
		t.Fatal("unclosed tuple should fail")
	}
}

func TestParseQueryVersionstamp(t *testing.T) {
	begin, _, err := ParseQuery("Versionstamp(incomplete, user=4)")
	if err != nil {
		t.Fatal(err)
	}
	if len(begin) != 13 || begin[0] != 0x33 || begin[1] != 0xff || begin[11] != 0x00 || begin[12] != 0x04 {
		t.Fatalf("%x", begin)
	}
}
