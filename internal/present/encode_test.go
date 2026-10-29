package present

import "testing"

func TestFuzzyAndMatch(t *testing.T) {
	if IsFuzzy(`"demo"`) || IsFuzzy(`("demo", "bytes")`) || IsFuzzy("uuid(3c6047fa-4f4e-466f-b349-49b72f5d6260)") {
		t.Fatal("structured queries stay prefix searches")
	}
	if !IsFuzzy("bytes") || !IsFuzzy("测试记录") || !IsFuzzy("3c6047") {
		t.Fatal("fragments are fuzzy")
	}
	if !Matches(`("demo", "bytes", 3)`, "ff00", "bytes") {
		t.Fatal("key text")
	}
	if !Matches(`("demo", "text", 0)`, "测试记录 0", "测试") {
		t.Fatal("value text")
	}
	if Matches(`("demo", "text", 0)`, "测试记录 0", "missing") {
		t.Fatal("miss")
	}
}

func TestEncodeValue(t *testing.T) {
	text, err := EncodeValue("text", "hello")
	if err != nil || string(text) != "hello" {
		t.Fatalf("text %q %v", text, err)
	}
	js, err := EncodeValue("json", `{"id":1}`)
	if err != nil || string(js) != `{"id":1}` {
		t.Fatalf("json %q %v", js, err)
	}
	if _, err := EncodeValue("json", "nope"); err == nil {
		t.Fatal("bad json")
	}
	packed, err := EncodeValue("tuple", `("row", 2, true)`)
	if err != nil || len(packed) == 0 || packed[0] == 0xff {
		t.Fatalf("tuple %x %v", packed, err)
	}
	raw, err := EncodeValue("bytes", "0a ff")
	if err != nil || len(raw) != 2 || raw[0] != 0x0a || raw[1] != 0xff {
		t.Fatalf("bytes %x %v", raw, err)
	}
	if _, err := PackExpr(`("a",`); err == nil {
		t.Fatal("open tuple")
	}
}
