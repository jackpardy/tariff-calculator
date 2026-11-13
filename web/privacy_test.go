package web

import (
	"net/http"
	"strings"
	"testing"
)

// squash is a page's text with its spaces and line breaks made one space, so a
// phrase is found wherever the template broke the line.
func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

// What's kept and for how long is a public page, with no link, for every
// short note to point to (ADR 0004 Decision 6): its rows are the kinds of
// thing the app keeps about people, with the times the code deletes them.
func TestWhatsKeptPage(t *testing.T) {
	h := competitionServer(t)
	rec := do(t, h, http.MethodGet, "/privacy", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/privacy: %d", rec.Code)
	}
	page := squash(unescape(rec.Body.String()))
	for _, want := range []string{
		"What's kept and for how long",
		"120 days after the competition's first day",
		"13 months after anyone last used the club",
		"90 days after it's sent",
		"30 days after it's made",
		"kept for 3 years so the organiser can copy it",
		"Names on the timetable and the venue screen can be read by anyone at the venue",
		"Helper links don't see messages",
		"kept without its words for 7 days",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page should say %q", want)
		}
	}
	// It needs no storage either.
	if rec := do(t, routesWith(nil), http.MethodGet, "/privacy", nil); rec.Code != http.StatusOK {
		t.Errorf("/privacy without storage: %d", rec.Code)
	}
}

// Each page that takes names says in a few words what's kept, and links to
// the page that has the whole table.
func TestShortNotesLinkToWhatsKept(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	club := aClub(t, h, admin, "UCD", false, "A. Murphy")
	join := pathIn(t, do(t, h, http.MethodGet, club.page+"/members", nil).Body.String(), "/clubs/join/")

	for _, c := range []struct {
		name, path, want string
	}{
		{"the home page's foot", "/", ""},
		{"the hub's foot", "/competitions", ""},
		{"making a competition", "/competitions/new", "It's deleted 120 days after the competition begins"},
		{"making a club", "/clubs/new", "A club is deleted 13 months after anyone last used it"},
		{"joining a club", join, "or sooner if the comp sec removes you"},
		{"the club's Settings", club.page + "/settings", "entries already sent stay with each competition until it's deleted"},
		{"entering on your own", enter, "which anyone at the venue can read"},
	} {
		page := squash(unescape(do(t, h, http.MethodGet, c.path, nil).Body.String()))
		if !strings.Contains(page, c.want) {
			t.Errorf("%s should say %q", c.name, c.want)
		}
		if !strings.Contains(page, `<a href="/privacy">What's kept and for how long</a>`) {
			t.Errorf("%s should link to What's kept and for how long", c.name)
		}
	}
}

// The sandbox says what's true: only its practice copy is reset.
func TestSandboxBannerSaysWhatIsKept(t *testing.T) {
	s := newTestServer(t, inSandbox())
	page := squash(unescape(do(t, s.h, http.MethodGet, "/competitions/new", nil).Body.String()))
	for _, want := range []string{"Use made-up names here", "anything else you add, and any feedback, is kept as on the live site", `href="/privacy"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the sandbox banner should say %q", want)
		}
	}
	if strings.Contains(page, "nothing here is kept") {
		t.Error("the sandbox does keep what visitors add")
	}
}
