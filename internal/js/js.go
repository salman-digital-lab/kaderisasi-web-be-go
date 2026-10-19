// Package js reproduces the JavaScript built-ins web-be's behavior depends
// on: String.prototype.trim, Number(), Number#toString, parseInt and the
// JSON.parse → JSON.stringify round trip of database JSON values.
package js

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// IsWhitespace is JavaScript's WhiteSpace and LineTerminator set (\s, trim).
func IsWhitespace(r rune) bool {
	switch r {
	case 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x20, 0xA0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

// Trim is String.prototype.trim.
func Trim(s string) string { return strings.TrimFunc(s, IsWhitespace) }

// Number is Number(string).
func Number(s string) float64 {
	s = Trim(s)
	if s == "" {
		return 0
	}
	for _, p := range []struct {
		prefix string
		base   int
	}{{"0x", 16}, {"0X", 16}, {"0o", 8}, {"0O", 8}, {"0b", 2}, {"0B", 2}} {
		if strings.HasPrefix(s, p.prefix) {
			digits := s[2:]
			if digits == "" || strings.ContainsAny(digits, "_+-") {
				return math.NaN()
			}
			n := 0.0
			for _, c := range digits {
				d, err := strconv.ParseUint(string(c), p.base, 8)
				if err != nil {
					return math.NaN()
				}
				n = n*float64(p.base) + float64(d)
			}
			return n
		}
	}
	switch strings.TrimLeft(s, "+-") {
	case "Infinity":
		if strings.HasPrefix(s, "-") {
			return math.Inf(-1)
		}
		if strings.Count(s, "+")+strings.Count(s, "-") > 1 {
			return math.NaN()
		}
		return math.Inf(1)
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9') && !strings.ContainsRune("+-.eE", r) {
			return math.NaN()
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if e, ok := err.(*strconv.NumError); ok && e.Err == strconv.ErrRange {
			return n
		}
		return math.NaN()
	}
	return n
}

// ToString is Number.prototype.toString() / String(number).
func ToString(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		return "0"
	}
	abs := math.Abs(f)
	if abs >= 1e21 || abs < 1e-6 {
		s := strconv.FormatFloat(f, 'e', -1, 64)
		// Go writes e+21 / e-07; JavaScript writes e+21 / e-7.
		mantissa, exponent, _ := strings.Cut(s, "e")
		sign := exponent[0]
		exponent = strings.TrimLeft(exponent[1:], "0")
		return mantissa + "e" + string(sign) + exponent
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// ParseInt is parseInt(String(value), 10) for a number argument.
func ParseInt(f float64) float64 { return ParseIntString(ToString(f)) }

// ParseIntString is parseInt(s, 10): optional whitespace and sign, then the
// longest run of decimal digits; NaN when there is none.
func ParseIntString(s string) float64 {
	s = strings.TrimLeftFunc(s, IsWhitespace)
	negative := false
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		negative = s[0] == '-'
		s = s[1:]
	}
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return math.NaN()
	}
	n, _ := strconv.ParseFloat(s[:end], 64)
	if negative {
		n = -n
	}
	return n
}

// IsIntegerKey reports whether JavaScript treats an object key as an array
// index, which objects enumerate first, in ascending order.
func IsIntegerKey(k string) bool {
	if k == "" || len(k) > 10 || (len(k) > 1 && k[0] == '0') {
		return false
	}
	for _, c := range k {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, _ := strconv.ParseUint(k, 10, 64)
	return n < 1<<32-1
}

// Value is a parsed JSON value that re-serializes like JSON.stringify.
type Value struct {
	Kind   byte // 'n' null, 'b' bool, 'f' number, 's' string, 'a' array, 'o' object
	Bool   bool
	Number float64
	String string
	Items  []Value
	Keys   []string
	Fields map[string]Value
}

// Parse is JSON.parse into an order-preserving value.
func Parse(raw []byte) (Value, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	v, err := parseValue(d)
	if err != nil {
		return Value{}, err
	}
	if _, err := d.Token(); err == nil {
		return Value{}, &json.SyntaxError{}
	}
	return v, nil
}

func parseValue(d *json.Decoder) (Value, error) {
	t, err := d.Token()
	if err != nil {
		return Value{}, err
	}
	switch v := t.(type) {
	case nil:
		return Value{Kind: 'n'}, nil
	case bool:
		return Value{Kind: 'b', Bool: v}, nil
	case json.Number:
		f, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			if e, ok := err.(*strconv.NumError); !ok || e.Err != strconv.ErrRange {
				return Value{}, err
			}
		}
		return Value{Kind: 'f', Number: f}, nil
	case string:
		return Value{Kind: 's', String: v}, nil
	case json.Delim:
		if v == '[' {
			out := Value{Kind: 'a', Items: []Value{}}
			for d.More() {
				item, err := parseValue(d)
				if err != nil {
					return Value{}, err
				}
				out.Items = append(out.Items, item)
			}
			_, err := d.Token()
			return out, err
		}
		out := Value{Kind: 'o', Fields: map[string]Value{}}
		for d.More() {
			kt, err := d.Token()
			if err != nil {
				return Value{}, err
			}
			key := kt.(string)
			item, err := parseValue(d)
			if err != nil {
				return Value{}, err
			}
			if _, seen := out.Fields[key]; !seen {
				out.Keys = append(out.Keys, key)
			}
			out.Fields[key] = item // the last duplicate wins, like JSON.parse
		}
		_, err := d.Token()
		out.Keys = ObjectKeyOrder(out.Keys)
		return out, err
	}
	return Value{}, &json.SyntaxError{}
}

// ObjectKeyOrder applies JavaScript's own-property enumeration order.
func ObjectKeyOrder(keys []string) []string {
	var ints, rest []string
	for _, k := range keys {
		if IsIntegerKey(k) {
			ints = append(ints, k)
		} else {
			rest = append(rest, k)
		}
	}
	sort.SliceStable(ints, func(i, j int) bool {
		a, _ := strconv.ParseUint(ints[i], 10, 64)
		b, _ := strconv.ParseUint(ints[j], 10, 64)
		return a < b
	})
	return append(ints, rest...)
}

// Get returns an object field and whether it is present.
func (v Value) Get(key string) (Value, bool) {
	if v.Kind != 'o' {
		return Value{}, false
	}
	f, ok := v.Fields[key]
	return f, ok
}

// Truthy is JavaScript truthiness.
func (v Value) Truthy() bool {
	switch v.Kind {
	case 'n':
		return false
	case 'b':
		return v.Bool
	case 'f':
		return v.Number != 0 && !math.IsNaN(v.Number)
	case 's':
		return v.String != ""
	}
	return true
}

func (v Value) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	v.write(&b)
	return b.Bytes(), nil
}

func (v Value) write(b *bytes.Buffer) {
	switch v.Kind {
	case 'n', 0:
		b.WriteString("null")
	case 'b':
		b.WriteString(strconv.FormatBool(v.Bool))
	case 'f':
		if math.IsNaN(v.Number) || math.IsInf(v.Number, 0) {
			b.WriteString("null")
		} else {
			b.WriteString(ToString(v.Number))
		}
	case 's':
		b.Write(Quote(v.String))
	case 'a':
		b.WriteByte('[')
		for i, item := range v.Items {
			if i > 0 {
				b.WriteByte(',')
			}
			item.write(b)
		}
		b.WriteByte(']')
	case 'o':
		b.WriteByte('{')
		for i, k := range v.Keys {
			if i > 0 {
				b.WriteByte(',')
			}
			b.Write(Quote(k))
			b.WriteByte(':')
			v.Fields[k].write(b)
		}
		b.WriteByte('}')
	}
}

// Quote is JSON.stringify for a string: only `"`, `\` and control characters
// are escaped (no HTML escaping, U+2028/U+2029 left as they are).
func Quote(s string) []byte {
	b := make([]byte, 0, len(s)+2)
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			b = append(b, '\\', '"')
		case c == '\\':
			b = append(b, '\\', '\\')
		case c == '\b':
			b = append(b, '\\', 'b')
		case c == '\f':
			b = append(b, '\\', 'f')
		case c == '\n':
			b = append(b, '\\', 'n')
		case c == '\r':
			b = append(b, '\\', 'r')
		case c == '\t':
			b = append(b, '\\', 't')
		case c < 0x20:
			b = append(b, fmt.Sprintf("\\u%04x", c)...)
		default:
			b = append(b, c)
		}
	}
	return append(b, '"')
}
