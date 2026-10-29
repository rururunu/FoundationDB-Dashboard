package present

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Summary is a human reading of stored bytes.
// Kind is one of tuple, json, text, bytes.
type Summary struct {
	Text string
	Kind string
	Note string
}

// Summarize reads data the way a person would. JSON and readable text come
// first, because a few printable bytes are also tuple type codes. Then a
// tuple. Everything else is bytes with a short hex preview.
// tupleText may be nil. With full set, text is not shortened and JSON is
// indented. Without it, text is one line of at most 120 characters.
func Summarize(data []byte, tupleText func([]byte) (string, bool), full bool) Summary {
	if len(data) == 0 {
		return Summary{Text: "(空)", Kind: "bytes"}
	}
	note := noteFor(data)
	if utf8.Valid(data) {
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid(trimmed) {
			var out bytes.Buffer
			if full {
				if err := json.Indent(&out, trimmed, "", "  "); err == nil {
					return Summary{Text: out.String(), Kind: "json", Note: note}
				}
			} else if err := json.Compact(&out, trimmed); err == nil {
				return Summary{Text: shorten(out.String(), false), Kind: "json", Note: note}
			}
		}
		if readable(data) {
			return Summary{Text: shorten(string(data), full), Kind: "text", Note: note}
		}
	}
	if tupleText != nil {
		if text, ok := tupleText(data); ok {
			return Summary{Text: shorten(text, full), Kind: "tuple", Note: note}
		}
	}
	return Summary{Text: HexPreview(data), Kind: "bytes", Note: note}
}

func readable(data []byte) bool {
	for _, r := range string(data) {
		if r == '\n' || r == '\t' || r == '\r' {
			continue
		}
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func shorten(text string, full bool) string {
	if full {
		return text
	}
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > 120 {
		return string(runes[:120]) + "…"
	}
	return text
}

func noteFor(data []byte) string {
	switch {
	case len(data) == 8 && !readable(data):
		return fmt.Sprintf("按小端整数读取：%d", int64(binary.LittleEndian.Uint64(data)))
	case len(data) == 4 && !readable(data):
		return fmt.Sprintf("按小端整数读取：%d", int32(binary.LittleEndian.Uint32(data)))
	}
	return ""
}
