package present

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

// IsFuzzy reports a search that is a text fragment. A quoted string, a tuple,
// a uuid, a versionstamp, or hex stays a prefix range.
func IsFuzzy(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	switch s[0] {
	case '(', '"':
		return false
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "uuid(") || strings.HasPrefix(s, "Versionstamp(") || strings.HasPrefix(lower, "0x") || strings.HasPrefix(s, "float32(") {
		return false
	}
	return true
}

// Matches is true when the displayed key or the value reading contains query.
func Matches(keyText, valueText, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(keyText), q) || strings.Contains(strings.ToLower(valueText), q)
}

// PackExpr packs one tuple expression into the exact key bytes.
func PackExpr(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("键是空的")
	}
	p := parser{s: s}
	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	p.skip()
	if p.i != len(p.s) {
		return nil, errors.New("键表达式没有结束：" + strings.TrimSpace(p.s[p.i:]))
	}
	packed := packPrefix(value)
	if err := UserKey(packed); err != nil {
		return nil, err
	}
	return packed, nil
}

// UserKey rejects empty keys and the system keyspace.
func UserKey(key []byte) error {
	if len(key) == 0 {
		return errors.New("键是空的")
	}
	if key[0] == 0xff {
		return errors.New("不能写入系统键")
	}
	return nil
}

// EncodeValue turns the editor contents into stored bytes.
func EncodeValue(kind, text string) ([]byte, error) {
	switch kind {
	case "text":
		return []byte(text), nil
	case "json":
		trimmed := bytes.TrimSpace([]byte(text))
		if len(trimmed) == 0 || !json.Valid(trimmed) {
			return nil, errors.New("JSON 格式不对")
		}
		return trimmed, nil
	case "tuple":
		return PackExpr(text)
	case "bytes":
		raw := strings.TrimSpace(text)
		raw = strings.TrimPrefix(strings.TrimPrefix(raw, "0x"), "0X")
		raw = strings.ReplaceAll(raw, " ", "")
		raw = strings.ReplaceAll(raw, "\n", "")
		if len(raw)%2 != 0 {
			return nil, errors.New("十六进制长度必须是偶数")
		}
		out, err := hex.DecodeString(raw)
		if err != nil {
			return nil, errors.New("值必须是十六进制")
		}
		return out, nil
	default:
		return nil, errors.New("类型必须是 text、json、tuple 或 bytes")
	}
}
