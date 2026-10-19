package catalog

import (
	"regexp"
	"strconv"
	"time"

	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/member"
)

// MonthDate is `(monthParam ? DateTime.fromISO(monthParam) : DateTime.now()).toSQLDate()`
// in the process time zone. nil means an invalid DateTime, whose null SQL
// date makes Lucid query `month IS NULL`.
//
// Supported ISO 8601 forms (Luxon accepts these and a few rarer ones):
// calendar dates (YYYY, YYYY-MM, YYYY-MM-DD, YYYYMMDD), ordinal dates
// (YYYY-DDD), week dates (YYYY-Www[-D]), each optionally followed by
// THH[:mm[:ss[.fff]]] and Z or ±HH[:mm]. An offset converts the instant to
// the local zone before the date is taken, as Luxon does.
func MonthDate(values []string, now time.Time, zone *time.Location) *time.Time {
	if len(values) == 0 || values[0] == "" {
		d := now.In(zone)
		return &d
	}
	if len(values) > 1 {
		return nil
	}
	return parseISO(values[0], zone)
}

var (
	isoCalendar = regexp.MustCompile(`^([+-]\d{6}|\d{4})(?:-(\d{2})(?:-(\d{2}))?|(\d{2})(\d{2}))?$`)
	isoOrdinal  = regexp.MustCompile(`^([+-]\d{6}|\d{4})-?(\d{3})$`)
	isoWeek     = regexp.MustCompile(`^([+-]\d{6}|\d{4})-?W(\d{2})(?:-?(\d))?$`)
	isoTime     = regexp.MustCompile(`^(\d{2})(?::?(\d{2})(?::?(\d{2})(?:[.,](\d{1,30}))?)?)?(Z|[+-]\d{2}(?::?\d{2})?)?$`)
)

func parseISO(s string, zone *time.Location) *time.Time {
	datePart, timePart := s, ""
	for i := 0; i < len(s); i++ {
		if s[i] == 'T' {
			datePart, timePart = s[:i], s[i+1:]
			break
		}
	}
	var year, month, day int
	atoi := func(v string) int { n, _ := strconv.Atoi(v); return n }
	switch {
	case isoCalendar.MatchString(datePart):
		m := isoCalendar.FindStringSubmatch(datePart)
		year, month, day = atoi(m[1]), 1, 1
		if m[2] != "" {
			month = atoi(m[2])
			if m[3] != "" {
				day = atoi(m[3])
			}
		} else if m[4] != "" {
			month, day = atoi(m[4]), atoi(m[5])
		}
		if month < 1 || month > 12 || day < 1 || day > daysIn(year, month) {
			return nil
		}
	case isoOrdinal.MatchString(datePart):
		m := isoOrdinal.FindStringSubmatch(datePart)
		year = atoi(m[1])
		ordinal := atoi(m[2])
		days := 365
		if daysIn(year, 2) == 29 {
			days = 366
		}
		if ordinal < 1 || ordinal > days {
			return nil
		}
		t := time.Date(year, 1, ordinal, 0, 0, 0, 0, time.UTC)
		year, month, day = t.Year(), int(t.Month()), t.Day()
	case isoWeek.MatchString(datePart):
		m := isoWeek.FindStringSubmatch(datePart)
		year = atoi(m[1])
		week, weekday := atoi(m[2]), 1
		if m[3] != "" {
			weekday = atoi(m[3])
		}
		if week < 1 || week > weeksIn(year) || weekday < 1 || weekday > 7 {
			return nil
		}
		jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
		monday := jan4.AddDate(0, 0, -((int(jan4.Weekday()) + 6) % 7))
		t := monday.AddDate(0, 0, (week-1)*7+weekday-1)
		year, month, day = t.Year(), int(t.Month()), t.Day()
	default:
		return nil
	}
	hour, minute, second, nanos := 0, 0, 0, 0
	location := zone
	if timePart != "" {
		m := isoTime.FindStringSubmatch(timePart)
		if m == nil {
			return nil
		}
		hour = atoi(m[1])
		if m[2] != "" {
			minute = atoi(m[2])
		}
		if m[3] != "" {
			second = atoi(m[3])
		}
		if m[4] != "" {
			frac := (m[4] + "000000000")[:9]
			nanos = atoi(frac)
		}
		if hour > 23 || minute > 59 || second > 59 {
			return nil
		}
		if m[5] == "Z" {
			location = time.UTC
		} else if m[5] != "" {
			sign := 1
			if m[5][0] == '-' {
				sign = -1
			}
			digits := regexp.MustCompile(`\d+`).FindAllString(m[5], -1)
			offset := atoi(digits[0][:2]) * 3600
			if len(digits[0]) == 4 {
				offset += atoi(digits[0][2:]) * 60
			} else if len(digits) > 1 {
				offset += atoi(digits[1]) * 60
			}
			location = time.FixedZone("", sign*offset)
		}
	}
	t := time.Date(year, time.Month(month), day, hour, minute, second, nanos, location).In(zone)
	return &t
}

func daysIn(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func weeksIn(year int) int {
	_, week := time.Date(year, 12, 28, 0, 0, 0, 0, time.UTC).ISOWeek()
	return week
}

// LeaderboardEntry is a MonthlyLeaderboard with its preloaded user, the
// user's first profile and that profile's university.
type LeaderboardEntry struct {
	Row        dbgen.MonthlyLeaderboard
	User       *dbgen.PublicUser
	Profile    *dbgen.Profile
	University *dbgen.UniversitiesByIDsRow
}

func Leaderboard(entries []LeaderboardEntry, zone *time.Location) []member.Object {
	out := make([]member.Object, len(entries))
	for i, e := range entries {
		var user any
		if e.User != nil {
			var profile any
			if e.Profile != nil {
				var university any
				if e.University != nil {
					university = member.KV("id", e.University.ID, "name", e.University.Name)
				}
				profile = append(member.Profile(*e.Profile, zone), member.KV("university", university)...)
			}
			user = append(member.User(*e.User, zone), member.KV("profile", profile)...)
		}
		out[i] = member.KV("id", e.Row.ID, "user_id", e.Row.UserID, "month", Date(e.Row.Month), "score", e.Row.Score,
			"created_at", member.DateTime(e.Row.CreatedAt, zone), "updated_at", member.DateTime(e.Row.UpdatedAt, zone), "user", user)
	}
	return out
}
