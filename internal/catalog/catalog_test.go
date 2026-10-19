package catalog

import (
	"encoding/json"
	"math"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"kaderisasi/web/internal/dbgen"
)

// Ports kaderisasi-web-be/tests/unit/club_service.spec.ts.

func TestNormalizeClubQuery(t *testing.T) {
	q := NormalizeClubQuery(url.Values{"page": {"0"}, "per_page": {"999"}, "search": {"  " + strings.Repeat("a", 120) + "  "}, "club_type": {"INVALID"}})
	if q.Page.Page != 1 || q.Page.PerPage != 50 || len(q.Search) != 100 || q.ClubType != nil {
		t.Fatalf("%+v", q)
	}
	for _, kind := range clubTypes {
		if got := NormalizeClubQuery(url.Values{"club_type": {kind}}).ClubType; got == nil || *got != kind {
			t.Errorf("club type %s: %v", kind, got)
		}
	}
}

func club(show, open bool, end string) dbgen.Club {
	c := dbgen.Club{IsShow: &show, IsRegistrationOpen: &open}
	if end != "" {
		d, _ := time.Parse("2006-01-02", end)
		c.RegistrationEndDate = pgtype.Date{Time: d, Valid: true}
	}
	return c
}

func TestRegistrationOpen(t *testing.T) {
	today := time.Date(2026, 7, 16, 9, 0, 0, 0, jakarta)
	cases := []struct {
		club dbgen.Club
		want bool
	}{
		{club(true, true, "2026-07-16"), true},
		{club(true, true, "2026-07-15"), false},
		{club(false, true, "2026-07-16"), false},
		{club(true, false, "2026-07-16"), false},
		{club(true, true, ""), true},
	}
	for i, c := range cases {
		if got := RegistrationOpen(c.club, today); got != c.want {
			t.Errorf("case %d: %v", i, got)
		}
	}
	// Closes at midnight Jakarta even when the server clock is UTC.
	if RegistrationOpen(club(true, true, "2026-09-10"), time.Date(2026, 9, 10, 17, 0, 0, 0, time.UTC)) {
		t.Error("still open after midnight in Jakarta")
	}
}

func TestPublicMedia(t *testing.T) {
	raw := `{"items":[{"media_url":"club/club_media_1.webp","media_type":"image"},{"media_url":"https://www.youtube.com/embed/abc_DEF-123","media_type":"video","video_source":"youtube"},
		{"media_url":"javascript:alert(1)","media_type":"video"},{"media_url":"../../secret.png","media_type":"image"},{"media_url":"https://evil.example/embed/abc","media_type":"video"}]}`
	got, _ := json.Marshal(publicMedia([]byte(raw)))
	want := `{"items":[{"media_url":"club/club_media_1.webp","media_type":"image"},{"media_url":"https://www.youtube.com/embed/abc_DEF-123","media_type":"video","video_source":"youtube"}]}`
	if string(got) != want {
		t.Fatalf("%s", got)
	}
}

func TestPaginationEdges(t *testing.T) {
	limit, offset, err := Page{Page: 3, PerPage: 10}.LimitOffset()
	if err != nil || *limit != 10 || *offset != 20 {
		t.Fatalf("%v %v %v", limit, offset, err)
	}
	if _, _, err := (Page{Page: 0, PerPage: 10}).LimitOffset(); err == nil || err.Error() != "A non-negative integer must be provided to offset." {
		t.Fatalf("negative offset: %v", err)
	}
	limit, offset, _ = Page{Page: 2, PerPage: math.NaN()}.LimitOffset()
	if limit != nil || offset != nil {
		t.Fatal("NaN per_page must leave limit and offset unset")
	}
	meta, _ := json.Marshal(Paginated(5, Page{Page: 1, PerPage: 0}, []int{}))
	if !strings.Contains(string(meta), `"last_page":null`) || !strings.Contains(string(meta), `"last_page_url":"/?page=Infinity"`) {
		t.Fatalf("%s", meta)
	}
}
