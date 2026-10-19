package js

import (
	"regexp"
	"strconv"
	"time"
)

// Local is the process time zone JavaScript dates use (set from TZ).
var Local = time.Local

var isoDate = regexp.MustCompile(`^([+-]\d{6}|\d{4})(?:-(\d{2})(?:-(\d{2}))?)?(?:[T ](\d{2}):(\d{2})(?::(\d{2})(?:\.(\d+))?)?)?(Z|[+-]\d{2}:\d{2})?$`)

// ParseDate is `new Date(value)` for a parsed JSON value: numbers are epoch
// milliseconds, strings use the ECMAScript date-time format (plus V8's
// space separator). ok is false for an Invalid Date. V8's other legacy
// string formats are not reproduced.
func ParseDate(v Value) (time.Time, bool) {
	switch v.Kind {
	case 'f':
		if v.Number != v.Number || v.Number > 8.64e15 || v.Number < -8.64e15 {
			return time.Time{}, false
		}
		return time.UnixMilli(int64(v.Number)), true
	case 'n':
		return time.UnixMilli(0), true
	case 'b':
		if v.Bool {
			return time.UnixMilli(1), true
		}
		return time.UnixMilli(0), true
	case 's':
		return ParseDateString(v.String)
	}
	return time.Time{}, false
}

// ParseDateString is Date.parse for the ECMAScript format.
func ParseDateString(s string) (time.Time, bool) {
	m := isoDate.FindStringSubmatch(Trim(s))
	if m == nil {
		return time.Time{}, false
	}
	n := func(x string, fallback int) int {
		if x == "" {
			return fallback
		}
		v, _ := strconv.Atoi(x)
		return v
	}
	year, month, day := n(m[1], 0), n(m[2], 1), n(m[3], 1)
	hour, minute, second := n(m[4], 0), n(m[5], 0), n(m[6], 0)
	millis := 0
	if m[7] != "" {
		frac := (m[7] + "00")[:3]
		millis, _ = strconv.Atoi(frac)
	}
	if month < 1 || month > 12 || day < 1 || day > time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day() ||
		hour > 24 || minute > 59 || second > 59 || (hour == 24 && (minute != 0 || second != 0 || millis != 0)) {
		return time.Time{}, false
	}
	location := Local
	switch {
	case m[8] == "Z":
		location = time.UTC
	case m[8] != "":
		sign := 1
		if m[8][0] == '-' {
			sign = -1
		}
		location = time.FixedZone("", sign*(n(m[8][1:3], 0)*3600+n(m[8][4:6], 0)*60))
	case m[4] == "":
		location = time.UTC // date-only forms are UTC
	}
	return time.Date(year, time.Month(month), day, hour, minute, second, millis*1e6, location), true
}
