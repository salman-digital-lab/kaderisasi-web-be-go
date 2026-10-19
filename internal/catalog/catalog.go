// Package catalog serves web-be's public reads (activities, clubs and the
// monthly leaderboard) with Lucid's serialization and pagination.
package catalog

import (
	"errors"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/member"
	"kaderisasi/web/internal/richtext"
)

// Page is Lucid's paginate(page, perPage) after Number() casting.
type Page struct{ Page, PerPage float64 }

// QueryNumber is `Number(request.qs().x ?? fallback)`; a repeated parameter
// is an array, which Number() reads through its comma-joined string.
func QueryNumber(values []string, fallback float64) float64 {
	if len(values) == 0 {
		return fallback
	}
	return js.Number(strings.Join(values, ","))
}

// LimitOffset is forPage() through knex's limit/offset: a NaN value is
// ignored (nil), a negative offset throws.
func (p Page) LimitOffset() (limit, offset *int64, err error) {
	off := 0.0
	if p.Page != 1 {
		off = p.PerPage * (p.Page - 1)
	}
	if o := js.ParseInt(off); !math.IsNaN(o) {
		if o < 0 {
			return nil, nil, errors.New("A non-negative integer must be provided to offset.")
		}
		v := int64(o)
		offset = &v
	}
	if l := js.ParseInt(p.PerPage); !math.IsNaN(l) {
		v := int64(l)
		limit = &v
	}
	return limit, offset, nil
}

func jsNumber(f float64) any {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil // JSON.stringify
	}
	return f
}

func pageURL(page float64) string {
	if page < 1 {
		page = 1
	}
	return "/?page=" + js.ToString(page)
}

// Paginated is ModelPaginator.toJSON with the snake_case naming strategy.
func Paginated(total int64, p Page, rows any) member.Object {
	t := float64(total)
	last := math.Max(math.Ceil(t/p.PerPage), 1)
	if math.IsNaN(t / p.PerPage) {
		last = math.NaN() // Math.max(NaN, 1)
	}
	var next, previous any
	if last > p.Page {
		next = pageURL(p.Page + 1)
	}
	if p.Page > 1 {
		previous = pageURL(p.Page - 1)
	}
	meta := member.KV("total", t, "per_page", jsNumber(p.PerPage), "current_page", jsNumber(p.Page), "last_page", jsNumber(last),
		"first_page", 1, "first_page_url", pageURL(1), "last_page_url", pageURL(last), "next_page_url", next, "previous_page_url", previous)
	return member.KV("meta", meta, "data", rows)
}

// Date is a Lucid `@column.date()` value: DateTime#toISODate().
func Date(d pgtype.Date) any {
	if !d.Valid || d.InfinityModifier != pgtype.Finite {
		return nil
	}
	return d.Time.Format("2006-01-02")
}

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

// Activity serializes the Activity model (badge is not a declared column).
func Activity(a dbgen.Activity, zone *time.Location) member.Object {
	return member.KV("id", a.ID, "name", a.Name, "slug", a.Slug, "description", a.Description,
		"activity_start", Date(a.ActivityStart), "activity_end", Date(a.ActivityEnd), "registration_start", Date(a.RegistrationStart),
		"registration_end", Date(a.RegistrationEnd), "selection_start", Date(a.SelectionStart), "selection_end", Date(a.SelectionEnd),
		"activity_type", a.ActivityType, "activity_category", a.ActivityCategory, "additional_config", jsonb(a.AdditionalConfig),
		"minimum_level", a.MinimumLevel, "is_published", a.IsPublished, "created_at", member.DateTime(a.CreatedAt, zone),
		"updated_at", member.DateTime(a.UpdatedAt, zone), "is_registration_open", a.IsRegistrationOpen, "club_id", a.ClubID,
		"certificate_template_id", a.CertificateTemplateID)
}

// Activities serializes a list of activities.
func Activities(rows []dbgen.Activity, zone *time.Location) []member.Object {
	out := make([]member.Object, len(rows))
	for i, a := range rows {
		out[i] = Activity(a, zone)
	}
	return out
}

// Club serializes the Club model as stored.
func Club(c dbgen.Club, zone *time.Location) member.Object {
	return member.KV("id", c.ID, "name", c.Name, "description", c.Description, "short_description", c.ShortDescription, "logo", c.Logo,
		"media", jsonb(c.Media), "start_period", Date(c.StartPeriod), "end_period", Date(c.EndPeriod), "is_show", c.IsShow,
		"created_at", member.DateTime(c.CreatedAt, zone), "updated_at", member.DateTime(c.UpdatedAt, zone),
		"registration_info", jsonb(c.RegistrationInfo), "is_registration_open", c.IsRegistrationOpen,
		"registration_end_date", Date(c.RegistrationEndDate), "club_type", c.ClubType)
}

// PublicClub is Club after clubs_controller's sanitizePublicClubContent and
// isClubRegistrationOpen.
func PublicClub(c dbgen.Club, zone *time.Location, now time.Time) member.Object {
	o := Club(c, zone)
	open := RegistrationOpen(c, now)
	description := ""
	if c.Description != nil {
		description = richtext.Sanitize(*c.Description)
	}
	for i := range o {
		switch o[i].Key {
		case "description":
			o[i].Value = description
		case "media":
			o[i].Value = publicMedia(c.Media)
		case "registration_info":
			o[i].Value = publicRegistrationInfo(c.RegistrationInfo)
		case "is_registration_open":
			o[i].Value = open
		}
	}
	return o
}

var jakarta = func() *time.Location {
	zone, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return zone
}()

// RegistrationOpen is club_service.ts isClubRegistrationOpen: visible, open,
// and the end date (inclusive) not before today in Asia/Jakarta.
func RegistrationOpen(c dbgen.Club, now time.Time) bool {
	if c.IsShow == nil || !*c.IsShow || c.IsRegistrationOpen == nil || !*c.IsRegistrationOpen {
		return false
	}
	if !c.RegistrationEndDate.Valid {
		return true
	}
	return c.RegistrationEndDate.Time.Format("2006-01-02") >= now.In(jakarta).Format("2006-01-02")
}
