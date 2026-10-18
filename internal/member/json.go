// Package member serializes members the way web-be's Lucid models do and
// holds the account workflows shared by the auth routes.
package member

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Object is a JSON object that keeps its key order, as JavaScript objects
// serialized by Lucid do (attribute insertion order).
type Object []Pair

type Pair struct {
	Key   string
	Value any
}

// KV builds an Object from alternating keys and values.
func KV(keyValues ...any) Object {
	o := make(Object, 0, len(keyValues)/2)
	for i := 0; i+1 < len(keyValues); i += 2 {
		o = append(o, Pair{keyValues[i].(string), keyValues[i+1]})
	}
	return o
}

func (o Object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, p := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(p.Key)
		b.Write(key)
		b.WriteByte(':')
		value, err := marshal(p.Value)
		if err != nil {
			return nil, err
		}
		b.Write(value)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// DateTime is a Lucid `@column.dateTime` value: Luxon's toISO() in the
// process time zone, millisecond precision, or null.
func DateTime(t pgtype.Timestamptz, zone *time.Location) any {
	if !t.Valid || t.InfinityModifier != pgtype.Finite {
		return nil
	}
	return ISO(t.Time, zone)
}

func ISO(t time.Time, zone *time.Location) string {
	return t.In(zone).Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z07:00")
}

// Date is a plain `@column()` holding a PostgreSQL date: node-postgres builds
// a local-midnight JavaScript Date, which serializes as its UTC instant.
func Date(d pgtype.Date, zone *time.Location) any {
	if !d.Valid || d.InfinityModifier != pgtype.Finite {
		return nil
	}
	local := time.Date(d.Time.Year(), d.Time.Month(), d.Time.Day(), 0, 0, 0, 0, zone)
	return local.UTC().Format("2006-01-02T15:04:05.000Z")
}

// Now is a Lucid auto timestamp: DateTime.local() with millisecond precision.
func Now() time.Time { return time.Now().Truncate(time.Millisecond) }

func Timestamp(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// jsonb returns a jsonb column the way JSON.parse would see it (compacted;
// JavaScript's reordering of integer-like keys is not reproduced).
func jsonb(raw []byte) any {
	if raw == nil {
		return nil
	}
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return nil
	}
	return json.RawMessage(b.Bytes())
}

func jsWhitespace(r rune) bool {
	switch r {
	case 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x20, 0xA0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

func trimJS(s string) string { return strings.TrimFunc(s, jsWhitespace) }

// jsNumber is JavaScript's Number(string) for an already non-empty string.
func jsNumber(s string) float64 {
	s = trimJS(s)
	if s == "" {
		return 0
	}
	for prefix, base := range map[string]int{"0x": 16, "0X": 16, "0o": 8, "0O": 8, "0b": 2, "0B": 2} {
		if strings.HasPrefix(s, prefix) {
			n, err := strconv.ParseUint(s[2:], base, 64)
			if err != nil || strings.ContainsAny(s[2:], "_+-") {
				return math.NaN()
			}
			return float64(n)
		}
	}
	switch strings.TrimLeft(s, "+-") {
	case "Infinity":
		return math.Inf(1)
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9') && !strings.ContainsRune("+-.eE", r) {
			return math.NaN()
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if errors, ok := err.(*strconv.NumError); ok && errors.Err == strconv.ErrRange {
			return n
		}
		return math.NaN()
	}
	return n
}
