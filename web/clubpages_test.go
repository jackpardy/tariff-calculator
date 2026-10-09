package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// redirected posts a form and returns where it redirects, failing otherwise.
func redirected(t *testing.T, h http.Handler, path string, form url.Values) string {
	t.Helper()
	rec := do(t, h, http.MethodPost, path, form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST %s: %d %s", path, rec.Code, rec.Body.String())
	}
	return rec.Header().Get("Location")
}

// withoutQuery is a path without its query.
func withoutQuery(path string) string {
	p, _, _ := strings.Cut(path, "?")
	return p
}

// formAction finds a form's action in a page by its path prefix.
func formAction(t *testing.T, html, prefix string) string {
	t.Helper()
	m := regexp.MustCompile(`action="(` + regexp.QuoteMeta(prefix) + `[^"]*)"`).FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("no form posting to %s...", prefix)
	}
	return m[1]
}

func TestClubs(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	clubLink := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/club/")

	// A comp sec creates a club.
	if rec := do(t, h, http.MethodPost, "/clubs", url.Values{"name": {" "}}); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "The club needs a name.") {
		t.Errorf("a club needs a name: %d", rec.Code)
	}
	location := redirected(t, h, "/clubs", url.Values{"name": {"UCD"}})
	club := withoutQuery(location)
	page := do(t, h, http.MethodGet, location, nil).Body.String()
	if !strings.Contains(page, "Club created.") || !strings.Contains(page, "http://example.com"+club) || !strings.Contains(page, "Not entered in any yet") {
		t.Error("the new club's page shows its admin link once")
	}
	join := pathIn(t, page, "/clubs/join/")

	// They enter it in the competition through its club link.
	if page := do(t, h, http.MethodGet, clubLink, nil).Body.String(); !strings.Contains(page, "Enter your club in Student Open") {
		t.Error("the club link page")
	}
	if rec := do(t, h, http.MethodPost, clubLink, url.Values{"clubAdmin": {"nonsense"}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a wrong club admin link: %d", rec.Code)
	}
	location = redirected(t, h, clubLink, url.Values{"clubAdmin": {"http://example.com" + club + "?new=created"}})
	if page := do(t, h, http.MethodGet, location, nil).Body.String(); !strings.Contains(page, "UCD is entered in Student Open") || !strings.Contains(page, "No member has entered yet") {
		t.Error("entered: the club page says so and lists the competition")
	}
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(dash, `<option value="UCD">UCD</option>`) {
		t.Error("the organiser's dashboard lists the club")
	}

	// A member joins and enters.
	if rec := do(t, h, http.MethodPost, join, url.Values{"name": {""}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a member needs a name: %d", rec.Code)
	}
	location = redirected(t, h, join, url.Values{"name": {"A. Murphy"}})
	member := withoutQuery(location)
	page = do(t, h, http.MethodGet, location, nil).Body.String()
	if !strings.Contains(page, "You've joined.") || !strings.Contains(page, "Student Open") || !strings.Contains(page, "Not entered") || strings.Contains(page, `name="gymnast"`) {
		t.Error("the member's page lists the competition to enter, under their own name")
	}
	save := formAction(t, page, member+"/competitions/")
	entry := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}}
	if rec := do(t, h, http.MethodPost, save, entry); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Choose a routine for the second exercise.") {
		t.Errorf("problems are shown: %d", rec.Code)
	}
	entry.Set("ex2Skills", voluntary)
	location = redirected(t, h, save, entry)
	if page := do(t, h, http.MethodGet, location, nil).Body.String(); !strings.Contains(page, "Saved, not sent yet") || !strings.Contains(page, "A. Murphy") {
		t.Error("saved, not sent")
	}
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); strings.Contains(dash, "A. Murphy") {
		t.Error("the organiser sees nothing until the club sends")
	}

	// The comp sec sends it.
	page = do(t, h, http.MethodGet, club, nil).Body.String()
	if !strings.Contains(page, "Not sent") || !strings.Contains(page, "Send new and changed (1)") {
		t.Error("the club page shows the entry not sent")
	}
	send := formAction(t, page, club+"/competitions/")
	location = redirected(t, h, send, url.Values{"which": {"changed"}})
	if page := do(t, h, http.MethodGet, location, nil).Body.String(); !strings.Contains(page, "Sent 1 entry to Student Open.") || !strings.Contains(page, "Send new and changed (0)") {
		t.Error("sent")
	}
	if dash := do(t, h, http.MethodGet, admin+"?club=UCD", nil).Body.String(); !strings.Contains(dash, "A. Murphy") {
		t.Error("the organiser sees the club's entry")
	}
	if page := do(t, h, http.MethodGet, member, nil).Body.String(); !strings.Contains(page, "Sent by your club") {
		t.Error("the member sees it's sent")
	}
	if page := do(t, h, http.MethodGet, member, nil).Body.String(); !strings.Contains(page, `<details class="box mt-4 comp-change"><summary><strong>Change your entry</strong>`) {
		t.Error("with an entry, the form to change it starts closed")
	}

	// A change shows on both pages until the club sends again.
	entry.Set("ex1Option", "builtin:bucs-l3-option-2")
	redirected(t, h, save, entry)
	if page := do(t, h, http.MethodGet, member, nil).Body.String(); !strings.Contains(page, "Changed since your club sent it") {
		t.Error("the member sees it's changed since sent")
	}
	if page := do(t, h, http.MethodGet, club, nil).Body.String(); !strings.Contains(page, "Changed since sent") || !strings.Contains(page, "Send new and changed (1)") {
		t.Error("the comp sec sees it's changed since sent")
	}

	// The comp sec changes it on the member's behalf.
	page = do(t, h, http.MethodGet, club, nil).Body.String()
	edit := regexp.MustCompile(`href="(` + regexp.QuoteMeta(club) + `/members/[^"]+)"`).FindStringSubmatch(page)
	if edit == nil {
		t.Fatal("no link to change the member's entry")
	}
	if page := do(t, h, http.MethodGet, edit[1], nil).Body.String(); !strings.Contains(page, "Change the entry") || !strings.Contains(page, "← UCD") {
		t.Error("the comp sec's edit page")
	}
	location = redirected(t, h, edit[1], entry)
	if page := do(t, h, http.MethodGet, location, nil).Body.String(); !strings.Contains(page, "Saved A. Murphy&#39;s entry") {
		t.Error("saved on the member's behalf")
	}

	// A member who lost their link gets a new one.
	newLink := formAction(t, page, club+"/members/")
	rec := do(t, h, http.MethodPost, newLink, url.Values{})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "A. Murphy&#39;s new link") {
		t.Fatalf("new member link: %d", rec.Code)
	}
	fresh := pathIn(t, rec.Body.String(), "/clubs/member/")
	if do(t, h, http.MethodGet, member, nil).Code != http.StatusNotFound || do(t, h, http.MethodGet, fresh, nil).Code != http.StatusOK {
		t.Error("the old member link stops working, the new one works")
	}

	// Withdrawing: the sent copy stays until the club sends all again.
	withdraw := formAction(t, do(t, h, http.MethodGet, fresh, nil).Body.String(), fresh+"/competitions/")
	if !strings.HasSuffix(withdraw, "/withdraw") {
		withdraw += "/withdraw"
	}
	redirected(t, h, withdraw, url.Values{"confirm": {"1"}})
	if page := do(t, h, http.MethodGet, club, nil).Body.String(); !strings.Contains(page, "Withdrawn (still sent)") {
		t.Error("the comp sec sees the withdrawn entry is still with the competition")
	}
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(dash, `class="is-withdrawn"`) || !strings.Contains(dash, ">Withdrawn</span>") || !strings.Contains(dash, `<p class="heading">withdrawn</p>`) {
		t.Error("the organiser sees the entry marked withdrawn until the club sends again")
	}
	if !strings.Contains(dash, `<p class="title is-4">0</p><p class="heading">entries</p>`) {
		t.Error("a withdrawn entry isn't counted")
	}
	if cards := do(t, h, http.MethodGet, admin+"/cards", nil).Body.String(); !strings.Contains(cards, "No entries to print.") {
		t.Error("a withdrawn entry isn't printed")
	}
	redirected(t, h, send, url.Values{"which": {"all"}})
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); strings.Contains(dash, "A. Murphy") {
		t.Error("sending all takes it back")
	}

	// Another club's comp sec can't reach this club's members.
	other := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"DCU"}}))
	if rec := do(t, h, http.MethodGet, strings.Replace(edit[1], club, other, 1), nil); rec.Code != http.StatusNotFound {
		t.Errorf("another club's member: %d", rec.Code)
	}

	// Removing a member ends their link.
	remove := strings.TrimSuffix(newLink, "/new-link") + "/remove"
	redirected(t, h, remove, url.Values{})
	if do(t, h, http.MethodGet, fresh, nil).Code != http.StatusNotFound {
		t.Error("a removed member's link stops working")
	}

	// Deleting the club.
	if rec := do(t, h, http.MethodPost, club+"/delete", url.Values{"confirm": {"1"}}); rec.Code != http.StatusOK || do(t, h, http.MethodGet, club, nil).Code != http.StatusNotFound {
		t.Errorf("deleted: %d", rec.Code)
	}
}

func TestClubRateLimit(t *testing.T) {
	h := competitionServer(t)
	for i := range maxClubsPerHour {
		if rec := do(t, h, http.MethodPost, "/clubs", url.Values{"name": {"Club"}}); rec.Code != http.StatusSeeOther {
			t.Fatalf("club %d: %d", i+1, rec.Code)
		}
	}
	if rec := do(t, h, http.MethodPost, "/clubs", url.Values{"name": {"Club"}}); rec.Code != http.StatusTooManyRequests {
		t.Errorf("one address can create %d clubs an hour: %d", maxClubsPerHour, rec.Code)
	}
}

func TestHub(t *testing.T) {
	h := competitionServer(t)
	page := do(t, h, http.MethodGet, "/competitions", nil).Body.String()
	for _, want := range []string{`data-hub-list="trampolineCompetitions"`, `data-hub-list="trampolineMemberLinks"`, `href="/clubs/new"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the hub should have %q", want)
		}
	}
}
