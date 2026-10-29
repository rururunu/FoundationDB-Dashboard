package present

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ParseQuery reads the search box. Empty text scans the whole user keyspace.
// Any other text is a FoundationDB tuple in the same form the page displays.
// The result covers that exact key and every key whose tuple starts with it.
func ParseQuery(s string) (begin, end []byte, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return []byte{}, []byte{0xff}, nil
	}
	p := parser{s: s}
	value, err := p.parseValue()
	if err != nil {
		return nil, nil, err
	}
	p.skip()
	if p.i != len(p.s) {
		return nil, nil, errors.New("键表达式没有结束：" + strings.TrimSpace(p.s[p.i:]))
	}
	begin, end = TupleRange(packPrefix(value))
	return begin, end, nil
}

// TupleRange is the key range of a packed tuple and everything below it.
// An empty tuple covers the whole user keyspace.
func TupleRange(packed []byte) (begin, end []byte) {
	if len(packed) == 0 {
		return []byte{}, []byte{0xff}
	}
	begin = append([]byte(nil), packed...)
	end = append(append([]byte(nil), packed...), 0xff)
	return begin, end
}

type versionstamp struct {
	tv   [10]byte
	user uint16
}

type parser struct {
	s string
	i int
}

func (p *parser) skip() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *parser) parseValue() (any, error) {
	p.skip()
	if p.i >= len(p.s) {
		return nil, errors.New("还需要一个 tuple 元素")
	}
	rest := p.s[p.i:]
	switch {
	case rest[0] == '(':
		return p.parseTuple()
	case rest[0] == '"':
		return p.parseQuoted()
	case strings.HasPrefix(rest, "0x") || strings.HasPrefix(rest, "0X"):
		return p.parseBytes()
	case strings.HasPrefix(rest, "uuid(") || strings.HasPrefix(rest, "UUID("):
		return p.parseUUID()
	case strings.HasPrefix(rest, "float32("):
		return p.parseFloat32()
	case strings.HasPrefix(rest, "Versionstamp("):
		return p.parseVersionstamp()
	default:
		return p.parseAtom()
	}
}

func (p *parser) parseTuple() ([]any, error) {
	p.i++
	var items []any
	p.skip()
	if p.i < len(p.s) && p.s[p.i] == ')' {
		p.i++
		return items, nil
	}
	for {
		item, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		p.skip()
		if p.i >= len(p.s) {
			return nil, errors.New("tuple 缺少 )")
		}
		switch p.s[p.i] {
		case ',':
			p.i++
			p.skip()
			if p.i < len(p.s) && p.s[p.i] == ')' {
				p.i++
				return items, nil
			}
		case ')':
			p.i++
			return items, nil
		default:
			return nil, errors.New("tuple 元素之间需要逗号")
		}
	}
}

func (p *parser) parseQuoted() (string, error) {
	start := p.i
	p.i++
	for p.i < len(p.s) {
		if p.s[p.i] == '\\' {
			if p.i+1 >= len(p.s) {
				break
			}
			p.i += 2
			continue
		}
		if p.s[p.i] == '"' {
			p.i++
			text, err := strconv.Unquote(p.s[start:p.i])
			if err != nil {
				return "", errors.New("字符串格式不对")
			}
			return text, nil
		}
		p.i++
	}
	return "", errors.New("字符串缺少结束引号")
}

func (p *parser) parseBytes() ([]byte, error) {
	p.i += 2
	start := p.i
	for p.i < len(p.s) && isHex(p.s[p.i]) {
		p.i++
	}
	raw := p.s[start:p.i]
	if len(raw)%2 != 0 {
		return nil, errors.New("0x 后面的十六进制长度必须是偶数")
	}
	if p.i < len(p.s) && !isDelimiter(p.s[p.i]) {
		return nil, errors.New("0x 后面有无法识别的字符")
	}
	out, err := hex.DecodeString(raw)
	if err != nil {
		return nil, errors.New("0x 后面必须是十六进制")
	}
	return out, nil
}

func (p *parser) parseUUID() ([16]byte, error) {
	var id [16]byte
	if strings.HasPrefix(p.s[p.i:], "UUID(") {
		p.i += len("UUID(")
	} else {
		p.i += len("uuid(")
	}
	start := p.i
	for p.i < len(p.s) && p.s[p.i] != ')' {
		p.i++
	}
	if p.i >= len(p.s) {
		return id, errors.New("uuid 缺少 )")
	}
	body := strings.TrimSpace(p.s[start:p.i])
	p.i++
	parts := strings.Split(body, "-")
	if len(parts) != 5 || len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		return id, errors.New("uuid 格式应为 uuid(01234567-89ab-cdef-0123-456789abcdef)")
	}
	raw, err := hex.DecodeString(strings.Join(parts, ""))
	if err != nil || len(raw) != 16 {
		return id, errors.New("uuid 格式应为 uuid(01234567-89ab-cdef-0123-456789abcdef)")
	}
	copy(id[:], raw)
	return id, nil
}

func (p *parser) parseFloat32() (float32, error) {
	p.i += len("float32(")
	start := p.i
	for p.i < len(p.s) && p.s[p.i] != ')' {
		p.i++
	}
	if p.i >= len(p.s) {
		return 0, errors.New("float32 缺少 )")
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(p.s[start:p.i]), 32)
	p.i++
	if err != nil {
		return 0, errors.New("float32 里需要一个数字")
	}
	return float32(n), nil
}

func (p *parser) parseVersionstamp() (versionstamp, error) {
	var stamp versionstamp
	p.i += len("Versionstamp(")
	p.skip()
	if strings.HasPrefix(p.s[p.i:], "incomplete") {
		p.i += len("incomplete")
		for i := range stamp.tv {
			stamp.tv[i] = 0xff
		}
	} else {
		start := p.i
		for p.i < len(p.s) && isHex(p.s[p.i]) {
			p.i++
		}
		raw := p.s[start:p.i]
		if len(raw) != 20 {
			return stamp, errors.New("Versionstamp 的事务版本应是 20 位十六进制")
		}
		decoded, err := hex.DecodeString(raw)
		if err != nil {
			return stamp, errors.New("Versionstamp 的事务版本应是十六进制")
		}
		copy(stamp.tv[:], decoded)
	}
	p.skip()
	if p.i >= len(p.s) || p.s[p.i] != ',' {
		return stamp, errors.New("Versionstamp 需要 user= 用户版本")
	}
	p.i++
	p.skip()
	if !strings.HasPrefix(p.s[p.i:], "user=") {
		return stamp, errors.New("Versionstamp 需要 user= 用户版本")
	}
	p.i += len("user=")
	p.skip()
	start := p.i
	for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		p.i++
	}
	if start == p.i {
		return stamp, errors.New("Versionstamp 的 user 应是数字")
	}
	user, err := strconv.ParseUint(p.s[start:p.i], 10, 16)
	if err != nil {
		return stamp, errors.New("Versionstamp 的 user 应是 0 到 65535")
	}
	stamp.user = uint16(user)
	p.skip()
	if p.i >= len(p.s) || p.s[p.i] != ')' {
		return stamp, errors.New("Versionstamp 缺少 )")
	}
	p.i++
	return stamp, nil
}

func (p *parser) parseAtom() (any, error) {
	start := p.i
	if p.s[p.i] == '-' || p.s[p.i] == '+' || (p.s[p.i] >= '0' && p.s[p.i] <= '9') {
		return p.parseNumber()
	}
	for p.i < len(p.s) && !isDelimiter(p.s[p.i]) {
		_, size := utf8.DecodeRuneInString(p.s[p.i:])
		if size <= 0 {
			break
		}
		p.i += size
	}
	word := p.s[start:p.i]
	switch word {
	case "":
		return nil, errors.New("还需要一个 tuple 元素")
	case "null":
		return nil, nil
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return word, nil
	}
}

func (p *parser) parseNumber() (any, error) {
	start := p.i
	if p.s[p.i] == '-' || p.s[p.i] == '+' {
		p.i++
	}
	sawDigit := false
	sawDot := false
	sawExp := false
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c >= '0' && c <= '9':
			sawDigit = true
			p.i++
		case c == '.' && !sawDot && !sawExp:
			sawDot = true
			p.i++
		case (c == 'e' || c == 'E') && !sawExp && sawDigit:
			sawExp = true
			p.i++
			if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
				p.i++
			}
		default:
			goto done
		}
	}
done:
	token := p.s[start:p.i]
	if !sawDigit || (p.i < len(p.s) && !isDelimiter(p.s[p.i])) {
		return nil, errors.New("数字格式不对")
	}
	if sawDot || sawExp {
		n, err := strconv.ParseFloat(token, 64)
		if err != nil {
			return nil, errors.New("数字超出范围")
		}
		return n, nil
	}
	if token[0] == '-' {
		n, err := strconv.ParseInt(token, 10, 64)
		if err != nil {
			return nil, errors.New("整数超出范围")
		}
		return n, nil
	}
	digits := strings.TrimPrefix(token, "+")
	n, err := strconv.ParseInt(digits, 10, 64)
	if err == nil {
		return n, nil
	}
	u, uerr := strconv.ParseUint(digits, 10, 64)
	if uerr != nil {
		return nil, errors.New("整数超出范围")
	}
	return u, nil
}

func isDelimiter(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', ',', '(', ')':
		return true
	default:
		return false
	}
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func packPrefix(value any) []byte {
	if items, ok := value.([]any); ok {
		return packTuple(items, false)
	}
	return packTuple([]any{value}, false)
}

func packTuple(items []any, nested bool) []byte {
	buf := make([]byte, 0, 32)
	if nested {
		buf = append(buf, 0x05)
	}
	for _, item := range items {
		buf = packElement(buf, item, nested)
	}
	if nested {
		buf = append(buf, 0x00)
	}
	return buf
}

func packElement(buf []byte, item any, nested bool) []byte {
	switch value := item.(type) {
	case nil:
		buf = append(buf, 0x00)
		if nested {
			buf = append(buf, 0xff)
		}
		return buf
	case bool:
		if value {
			return append(buf, 0x27)
		}
		return append(buf, 0x26)
	case int64:
		return appendInt(buf, value)
	case uint64:
		return appendUint(buf, value)
	case float32:
		return appendFloat32(buf, value)
	case float64:
		return appendFloat64(buf, value)
	case string:
		return appendCoded(buf, 0x02, []byte(value))
	case []byte:
		return appendCoded(buf, 0x01, value)
	case [16]byte:
		buf = append(buf, 0x30)
		return append(buf, value[:]...)
	case versionstamp:
		buf = append(buf, 0x33)
		buf = append(buf, value.tv[:]...)
		var user [2]byte
		binary.BigEndian.PutUint16(user[:], value.user)
		return append(buf, user[:]...)
	case []any:
		return append(buf, packTuple(value, true)...)
	default:
		return buf
	}
}

func appendCoded(buf []byte, code byte, data []byte) []byte {
	buf = append(buf, code)
	for len(data) > 0 {
		i := indexByte(data, 0x00)
		if i < 0 {
			buf = append(buf, data...)
			break
		}
		buf = append(buf, data[:i+1]...)
		buf = append(buf, 0xff)
		data = data[i+1:]
	}
	return append(buf, 0x00)
}

func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

func sizeLimit(n int) uint64 {
	if n <= 0 {
		return 0
	}
	if n >= 8 {
		return ^uint64(0)
	}
	return 1<<uint(n*8) - 1
}

func byteCount(i uint64) int {
	if i == 0 {
		return 0
	}
	n := 0
	for sizeLimit(n) < i {
		n++
	}
	return n
}

func appendUint(buf []byte, i uint64) []byte {
	if i == 0 {
		return append(buf, 0x14)
	}
	n := byteCount(i)
	buf = append(buf, byte(0x14+n))
	var scratch [8]byte
	binary.BigEndian.PutUint64(scratch[:], i)
	return append(buf, scratch[8-n:]...)
}

func appendInt(buf []byte, i int64) []byte {
	if i >= 0 {
		return appendUint(buf, uint64(i))
	}
	var mag uint64
	if i == math.MinInt64 {
		mag = 1 << 63
	} else {
		mag = uint64(-i)
	}
	n := byteCount(mag)
	buf = append(buf, byte(0x14-n))
	var scratch [8]byte
	binary.BigEndian.PutUint64(scratch[:], sizeLimit(n)-mag)
	return append(buf, scratch[8-n:]...)
}

func appendFloat32(buf []byte, f float32) []byte {
	var scratch [4]byte
	binary.BigEndian.PutUint32(scratch[:], math.Float32bits(f))
	adjustFloatBytes(scratch[:])
	buf = append(buf, 0x20)
	return append(buf, scratch[:]...)
}

func appendFloat64(buf []byte, f float64) []byte {
	var scratch [8]byte
	binary.BigEndian.PutUint64(scratch[:], math.Float64bits(f))
	adjustFloatBytes(scratch[:])
	buf = append(buf, 0x21)
	return append(buf, scratch[:]...)
}

func adjustFloatBytes(b []byte) {
	if b[0]&0x80 != 0 {
		for i := range b {
			b[i] ^= 0xff
		}
		return
	}
	b[0] ^= 0x80
}
