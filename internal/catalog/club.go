package catalog

import (
	"errors"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/member"
	"kaderisasi/web/internal/richtext"
)

var clubTypes = []string{"UNIT", "CLUB_KEPROFESIAN", "CLUB_BAHASA", "AVISMAN_REGIONAL"}

// ClubQuery is normalizeClubListQuery.
type ClubQuery struct {
	Page     Page
	Search   string
	ClubType *string
}

// positiveInteger is normalizePositiveInteger: Number(value) when it is an
// integer of at least 1 (capped at maximum), otherwise the fallback.
func positiveInteger(values []string, fallback, maximum float64) float64 {
	if len(values) == 0 {
		return fallback
	}
	n := js.Number(strings.Join(values, ","))
	if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) || n < 1 {
		return fallback
	}
	if maximum > 0 && n > maximum {
		return maximum
	}
	return n
}

func NormalizeClubQuery(q url.Values) ClubQuery {
	search := ""
	if v := q["search"]; len(v) == 1 {
		search = js.Trim(v[0])
		if units := utf16Length(search); units > 100 {
			search = sliceUTF16(search, 100)
		}
	}
	var clubType *string
	if v := q["club_type"]; len(v) == 1 {
		for _, t := range clubTypes {
			if v[0] == t {
				clubType = &t
			}
		}
	}
	return ClubQuery{Page: Page{Page: positiveInteger(q["page"], 1, 0), PerPage: positiveInteger(q["per_page"], 10, 50)}, Search: search, ClubType: clubType}
}

func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// sliceUTF16 is String#slice(0, n); a surrogate pair cut in half leaves its
// high half, which becomes U+FFFD when the string reaches the database.
func sliceUTF16(s string, n int) string {
	units := 0
	for i, r := range s {
		w := 1
		if r >= 0x10000 {
			w = 2
		}
		if units+w > n {
			if units < n {
				return s[:i] + string(utf8.RuneError)
			}
			return s[:i]
		}
		units += w
	}
	return s
}

var (
	safeImageKey = regexp.MustCompile(`^[a-zA-Z0-9/_-]+\.[a-zA-Z0-9]+$`)
	youtubePath  = regexp.MustCompile(`^/embed/[a-zA-Z0-9_-]+$`)
)

// publicMedia is serializePublicClubMedia.
func publicMedia(raw []byte) member.Object {
	items := []member.Object{}
	v := jsonb(raw)
	value, ok := v.(js.Value)
	if ok && value.Kind == 'o' {
		if list, ok := value.Get("items"); ok && list.Kind == 'a' {
			for _, item := range list.Items {
				if item.Kind != 'o' {
					continue
				}
				mediaURL, _ := item.Get("media_url")
				if mediaURL.Kind != 's' {
					continue
				}
				mediaType, _ := item.Get("media_type")
				source, _ := item.Get("video_source")
				switch {
				case mediaType.Kind == 's' && mediaType.String == "image" && safeClubImageKey(mediaURL.String):
					items = append(items, member.KV("media_url", mediaURL.String, "media_type", "image"))
				case mediaType.Kind == 's' && mediaType.String == "video" && source.Kind == 's' && source.String == "youtube" && safeYoutubeEmbed(mediaURL.String):
					items = append(items, member.KV("media_url", mediaURL.String, "media_type", "video", "video_source", "youtube"))
				}
			}
		}
	}
	return member.KV("items", items)
}

func safeClubImageKey(v string) bool {
	return strings.HasPrefix(v, "club/") && !strings.Contains(v, "..") && safeImageKey.MatchString(v)
}

// safeYoutubeEmbed is isSafeYoutubeEmbed under WHATWG URL parsing. Values
// it accepts are plain ASCII https URLs, which Go parses the same way;
// anything with unusual characters is rejected by the checks themselves.
func safeYoutubeEmbed(v string) bool {
	u, err := url.Parse(v)
	if err != nil || strings.ContainsAny(v, " \t\n\r\\") {
		return false
	}
	return u.Scheme == "https" && strings.ToLower(u.Host) == "www.youtube.com" && u.Host == "www.youtube.com" && u.Port() == "" &&
		youtubePath.MatchString(u.EscapedPath()) && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && !strings.Contains(v, "#") && u.User == nil
}

// publicRegistrationInfo is `{...club.registrationInfo, registration_info:
// sanitizeRichText(...)}` when registrationInfo is truthy.
func publicRegistrationInfo(raw []byte) any {
	v := jsonb(raw)
	value, ok := v.(js.Value)
	if !ok || !value.Truthy() {
		return v
	}
	out := js.Value{Kind: 'o', Fields: map[string]js.Value{}}
	add := func(k string, item js.Value) {
		if _, seen := out.Fields[k]; !seen {
			out.Keys = append(out.Keys, k)
		}
		out.Fields[k] = item
	}
	switch value.Kind {
	case 'o':
		for _, k := range value.Keys {
			add(k, value.Fields[k])
		}
	case 'a':
		for i, item := range value.Items {
			add(strconv.Itoa(i), item)
		}
	case 's':
		i := 0
		for _, r := range value.String {
			add(strconv.Itoa(i), js.Value{Kind: 's', String: string(r)})
			i++
		}
	}
	info, _ := value.Get("registration_info")
	html := ""
	switch info.Kind {
	case 's':
		html = info.String
	case 'f':
		if info.Truthy() {
			html = js.ToString(info.Number)
		}
	}
	add("registration_info", js.Value{Kind: 's', String: richtext.Sanitize(html)})
	out.Keys = js.ObjectKeyOrder(out.Keys)
	return out
}

// Leadership is serializePublicClubLeadershipRole for each role.
func Leadership(rows []dbgen.ClubLeadershipRow, zone *time.Location) ([]member.Object, error) {
	out := []member.Object{}
	for _, r := range rows {
		// Same failure order as the JavaScript: the member is read first,
		// then created_at and updated_at.
		if r.UserID == nil {
			return nil, errors.New("Cannot read properties of null (reading 'profile')")
		}
		if !r.CreatedAt.Valid || !r.UpdatedAt.Valid {
			return nil, errors.New("Cannot read properties of null (reading 'toISO')")
		}
		m := member.KV("id", *r.UserID)
		if r.HasProfile {
			m = append(m, member.KV("profile", member.KV("name", r.ProfileName, "picture", r.ProfilePicture))...)
		}
		out = append(out, member.KV("id", r.ID, "club_registration_id", r.ClubRegistrationID, "role_name", r.RoleName,
			"start_date", Date(r.StartDate), "end_date", Date(r.EndDate), "is_primary", r.IsPrimary, "sort_order", r.SortOrder,
			"created_at", member.ISO(r.CreatedAt.Time, zone), "updated_at", member.ISO(r.UpdatedAt.Time, zone),
			"registration", member.KV("id", r.ClubRegistrationID, "member", m)))
	}
	return out, nil
}
