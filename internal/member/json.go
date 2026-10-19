// Package member serializes members the way web-be's Lucid models do and
// holds the account workflows shared by the auth routes.
package member

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"kaderisasi/web/internal/js"
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

// jsonb returns a jsonb column as JSON.parse → JSON.stringify would emit it.
func jsonb(raw []byte) any {
	if raw == nil {
		return nil
	}
	v, err := js.Parse(raw)
	if err != nil {
		return nil
	}
	return v
}

func trimJS(s string) string { return js.Trim(s) }

func jsNumber(s string) float64 { return js.Number(s) }
