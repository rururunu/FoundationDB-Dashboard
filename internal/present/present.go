// Package present turns FoundationDB keys into text and parses the hex form
// used by the page. It does not import the FoundationDB client.
package present

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"
)

// ParseHex accepts empty text, optional 0x, and whitespace. Empty text is the
// empty key, which is the start of a forward scan.
func ParseHex(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	if s == "" {
		return []byte{}, nil
	}
	if len(s)%2 != 0 {
		return nil, errors.New("十六进制长度必须是偶数")
	}
	out, err := hex.DecodeString(s)
	if err != nil {
		return nil, errors.New("起始键必须是十六进制")
	}
	return out, nil
}

// HexPreview is a short hex form of data for the key list.
func HexPreview(data []byte) string {
	if len(data) == 0 {
		return "(空)"
	}
	n := len(data)
	if n > 16 {
		n = 16
	}
	text := hex.EncodeToString(data[:n])
	if len(data) > 16 {
		text += "…"
	}
	return text
}

// Successor is the first key strictly greater than key.
func Successor(key []byte) []byte {
	out := make([]byte, len(key)+1)
	copy(out, key)
	return out
}

// Format renders one tuple element, or a whole tuple, as text.
func Format(v any) string {
	if v == nil {
		return "null"
	}
	switch x := v.(type) {
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float32:
		return "float32(" + strconv.FormatFloat(float64(x), 'g', -1, 32) + ")"
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case string:
		return strconv.Quote(x)
	case []byte:
		return "0x" + hex.EncodeToString(x)
	case *big.Int:
		if x == nil {
			return "null"
		}
		return x.String()
	case big.Int:
		return x.String()
	}
	return formatReflected(reflect.ValueOf(v))
}

func formatReflected(rv reflect.Value) string {
	if !rv.IsValid() {
		return "null"
	}
	switch rv.Kind() {
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			b := make([]byte, rv.Len())
			reflect.Copy(reflect.ValueOf(b), rv)
			return "0x" + hex.EncodeToString(b)
		}
		if rv.Type().Elem().Kind() == reflect.Interface {
			parts := make([]string, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				parts[i] = Format(rv.Index(i).Interface())
			}
			return formatTuple(parts)
		}
	case reflect.Array:
		if rv.Len() == 16 && rv.Type().Elem().Kind() == reflect.Uint8 {
			var u [16]byte
			reflect.Copy(reflect.ValueOf(u[:]), rv)
			return "uuid(" + formatUUID(u) + ")"
		}
	case reflect.Struct:
		if text, ok := formatVersionstamp(rv); ok {
			return text
		}
	}
	return fmt.Sprint(rv.Interface())
}

func formatTuple(parts []string) string {
	switch len(parts) {
	case 0:
		return "()"
	case 1:
		return "(" + parts[0] + ",)"
	default:
		return "(" + strings.Join(parts, ", ") + ")"
	}
}

func formatUUID(u [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:])
}

func formatVersionstamp(rv reflect.Value) (string, bool) {
	tvField, ok1 := rv.Type().FieldByName("TransactionVersion")
	userField, ok2 := rv.Type().FieldByName("UserVersion")
	if !ok1 || !ok2 || tvField.Type != reflect.TypeOf([10]byte{}) || userField.Type.Kind() != reflect.Uint16 {
		return "", false
	}
	var tv [10]byte
	reflect.Copy(reflect.ValueOf(tv[:]), rv.FieldByName("TransactionVersion"))
	user := uint16(rv.FieldByName("UserVersion").Uint())
	incomplete := true
	for _, b := range tv {
		if b != 0xff {
			incomplete = false
			break
		}
	}
	if incomplete {
		return fmt.Sprintf("Versionstamp(incomplete, user=%d)", user), true
	}
	return fmt.Sprintf("Versionstamp(%x, user=%d)", tv[:], user), true
}
