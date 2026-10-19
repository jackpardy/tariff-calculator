package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/store"
	"tariffCalculator/views"

	"github.com/chromedp/chromedp"
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

// memberCompetitionPage is a member's page for the first competition their
// club is in, which they find through their list of competitions.
func memberCompetitionPage(t *testing.T, h http.Handler, member string) string {
	t.Helper()
	list := do(t, h, http.MethodGet, member, nil).Body.String()
	m := regexp.MustCompile(`href="(` + regexp.QuoteMeta(member) + `/competitions/[A-Za-z0-9_-]+)"`).FindStringSubmatch(list)
	if m == nil {
		t.Fatalf("no competition in %s's list", member)
	}
	return do(t, h, http.MethodGet, m[1], nil).Body.String()
}

// clubEntriesPage is the comp sec's page for the club's first (usually only)
// competition, its Entries, opened from the row on Competitions.
func clubEntriesPage(t *testing.T, h http.Handler, club string) string {
	t.Helper()
	return clubCompetitionTab(t, h, club, "")
}

// clubCompetitionTab is another tab of it: "/coaches", "/offers" or "/fees".
func clubCompetitionTab(t *testing.T, h http.Handler, club, tab string) string {
	t.Helper()
	m := regexp.MustCompile(`href="(` + regexp.QuoteMeta(club) + `/competitions/[^"/?]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, club, nil).Body.String())
	if m == nil {
		t.Fatalf("no competition in %s's list", club)
	}
	return do(t, h, http.MethodGet, m[1]+tab, nil).Body.String()
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
	clubLink := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/club/")

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
	join := pathIn(t, do(t, h, http.MethodGet, club+"/members", nil).Body.String(), "/clubs/join/")

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
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(dash, `<p class="title is-4">1</p><p class="heading">clubs</p>`) {
		t.Error("the organiser's dashboard counts the club")
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
	save := formAction(t, memberCompetitionPage(t, h, member), member+"/competitions/")
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
	page = clubEntriesPage(t, h, club)
	if !strings.Contains(page, chip(t, views.NotYet, "Not sent")) || !strings.Contains(page, "Send new and changed (1)") {
		t.Error("the club page shows the entry not sent")
	}
	send := formAction(t, page, club+"/competitions/")
	location = redirected(t, h, send, url.Values{"which": {"changed"}})
	if page := do(t, h, http.MethodGet, location, nil).Body.String(); !strings.Contains(page, "Sent 1 entry to Student Open.") || !strings.Contains(page, "Send new and changed (0)") ||
		!strings.Contains(page, chip(t, views.InProgress, "Sent")) {
		t.Error("sent")
	}
	if dash := do(t, h, http.MethodGet, admin+"?club=UCD", nil).Body.String(); !strings.Contains(dash, "A. Murphy") {
		t.Error("the organiser sees the club's entry")
	}
	if page := do(t, h, http.MethodGet, member, nil).Body.String(); !strings.Contains(page, "Sent by your club") {
		t.Error("the member sees it's sent")
	}
	if page := memberCompetitionPage(t, h, member); !strings.Contains(page, `<details class="comp-opens comp-opens-action"><summary><span class="button is-small comp-opens-button"><span class="comp-opens-label">Change your entry</span>`) {
		t.Error("with an entry, the form to change it starts closed")
	}

	// A change shows on both pages until the club sends again.
	entry.Set("ex1Option", "builtin:bucs-l3-option-2")
	redirected(t, h, save, entry)
	if page := do(t, h, http.MethodGet, member, nil).Body.String(); !strings.Contains(page, "Changed since your club sent it") {
		t.Error("the member sees it's changed since sent")
	}
	if page := clubEntriesPage(t, h, club); !strings.Contains(page, chip(t, views.Attention, "Changed since sent")) || !strings.Contains(page, "Send new and changed (1)") {
		t.Error("the comp sec sees it's changed since sent")
	}

	// The comp sec changes it on the member's behalf.
	page = clubEntriesPage(t, h, club)
	edit := regexp.MustCompile(`href="(` + regexp.QuoteMeta(club) + `/members/[^"]+)"`).FindStringSubmatch(page)
	if edit == nil {
		t.Fatal("no link to change the member's entry")
	}
	if page := do(t, h, http.MethodGet, edit[1], nil).Body.String(); !strings.Contains(page, "Change the entry") || !strings.Contains(page, "← Student Open") {
		t.Error("the comp sec's edit page")
	}
	location = redirected(t, h, edit[1], entry)
	if page := do(t, h, http.MethodGet, location, nil).Body.String(); !strings.Contains(page, "Saved A. Murphy&#39;s entry") {
		t.Error("saved on the member's behalf")
	}

	// A member who lost their link gets a new one, once the comp sec says
	// yes: the old one stops working.
	newLink := formAction(t, do(t, h, http.MethodGet, club+"/members", nil).Body.String(), club+"/members/")
	rec := do(t, h, http.MethodPost, newLink, url.Values{})
	if page := rec.Body.String(); rec.Code != http.StatusOK || !strings.Contains(page, "Make a new link for A. Murphy?") || !strings.Contains(page, "stops working at once") ||
		!strings.Contains(page, `name="anyway" value="1"`) || strings.Contains(page, "/clubs/member/") {
		t.Fatalf("a new member link is asked about first: %d", rec.Code)
	}
	if do(t, h, http.MethodGet, member, nil).Code != http.StatusOK {
		t.Error("asking leaves the old link working")
	}
	rec = do(t, h, http.MethodPost, newLink, url.Values{"anyway": {"1"}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "A. Murphy&#39;s new link") {
		t.Fatalf("new member link: %d", rec.Code)
	}
	fresh := pathIn(t, rec.Body.String(), "/clubs/member/")
	if do(t, h, http.MethodGet, member, nil).Code != http.StatusNotFound || do(t, h, http.MethodGet, fresh, nil).Code != http.StatusOK {
		t.Error("the old member link stops working, the new one works")
	}

	// Withdrawing: the sent copy stays until the club sends all again.
	withdraw := formAction(t, memberCompetitionPage(t, h, fresh), fresh+"/competitions/")
	if !strings.HasSuffix(withdraw, "/withdraw") {
		withdraw += "/withdraw"
	}
	redirected(t, h, withdraw, url.Values{"confirm": {"1"}})
	if page := clubEntriesPage(t, h, club); !strings.Contains(page, chip(t, views.Stopped, "Withdrawn (still sent)")) {
		t.Error("the comp sec sees the withdrawn entry is still with the competition")
	}
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(dash, `class="is-withdrawn"`) || !strings.Contains(dash, chip(t, views.Stopped, "Withdrawn")) || !strings.Contains(dash, `<p class="heading">withdrawn</p>`) {
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
	// Asked first, on Members, with no browser pop-up: nothing goes until yes.
	if page := do(t, h, http.MethodPost, remove, url.Values{}).Body.String(); !strings.Contains(page, "Their entries go with them") ||
		!strings.Contains(page, `action="`+remove+`"`) || !strings.Contains(page, `name="anyway" value="1"`) || strings.Contains(page, "confirm(") ||
		do(t, h, http.MethodGet, fresh, nil).Code != http.StatusOK {
		t.Errorf("removing a member asks first: %s", page)
	}
	redirected(t, h, remove, url.Values{"anyway": {"1"}})
	if do(t, h, http.MethodGet, fresh, nil).Code != http.StatusNotFound {
		t.Error("a removed member's link stops working")
	}

	// Deleting the club needs "delete" typed (in any case).
	if got := redirected(t, h, club+"/delete", url.Values{"confirm": {"1"}}); !strings.Contains(got, "Type+delete") || do(t, h, http.MethodGet, club, nil).Code != http.StatusOK {
		t.Errorf("the old tick box's 1 deletes nothing, and says why: %s", got)
	}
	if rec := do(t, h, http.MethodPost, club+"/delete", url.Values{"confirm": {"Delete "}}); rec.Code != http.StatusOK || do(t, h, http.MethodGet, club, nil).Code != http.StatusNotFound {
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

// The comp sec's Settings says when the club would be deleted: 13 months on
// (ADR 0011 Decision 7).
func TestClubKeptUntil(t *testing.T) {
	h, _ := competitionServerAt(t, dayBefore)
	club := aClub(t, h, created(t, h, newCompetition()), "UCD", false, "A. Murphy")
	page := strings.Join(strings.Fields(do(t, h, http.MethodGet, club.page+"/settings", nil).Body.String()), " ")
	want := "The club is kept until " + dayBefore.AddDate(0, store.ClubRetentionMonths, 0).In(local).Format("2 January 2006") + ": 13 months after anyone last opened its pages"
	if !strings.Contains(page, want) {
		t.Errorf("Settings should say %q: %s", want, page)
	}
}

// The comp sec's Competitions page searches its entries as the other lists
// do, and each entry's Change link sits under the member's name, where a
// narrow table can't cut it off.
func TestClubEntriesSearch(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore))
	admin := created(t, s.h, newCompetition())
	club := aClub(t, s.h, admin, "UCD", true, "Tadhg Ryan", "Éabha Kelly")
	doc := parsePage(t, clubEntriesPage(t, s.h, club.page))
	tools := byClass(doc, "comp-list-tools")
	if len(tools) != 1 || !strings.HasPrefix(attrOf(tools[0], "data-list-tools"), "entries-") {
		t.Fatalf("the entries have search tools: %d", len(tools))
	}
	list := byID(doc, attrOf(tools[0], "data-list-tools"))
	if list == nil {
		t.Fatal("the tools name the entries' table")
	}
	var names []string
	for _, tr := range byTag(list, "tr") {
		if attrOf(tr, "data-name") == "" {
			continue
		}
		names = append(names, attrOf(tr, "data-name"))
		cells := byTag(tr, "td")
		if links := byTag(cells[0], "a"); len(links) != 1 || textOf(links[0]) != "Change" {
			t.Errorf("Change under %s's name: %s", attrOf(tr, "data-name"), textOf(cells[0]))
		}
	}
	if strings.Join(names, ", ") != "Éabha Kelly, Tadhg Ryan" {
		t.Errorf("the rows, by name: %v", names)
	}
}

// The organiser's and comp sec's pages save their links in the browser when
// they're opened, not only when they're made; a helper's link, or a page
// opened through someone's own link, never.
func TestAdminLinksSavedWhenOpened(t *testing.T) {
	ctx := browser(t)
	s := newTestServer(t, atTime(dayBefore))
	admin := withoutQuery(created(t, s.h, newCompetition()))
	helper := madeLink(t, s.h, admin, "Mary", "everything")
	club := aClub(t, s.h, admin, "UCD", false)
	srv := httptest.NewServer(s.h)
	defer srv.Close()

	saved := func(key string) string {
		t.Helper()
		var links string
		step(t, ctx, "reading "+key, chromedp.Evaluate(`localStorage.getItem('`+key+`') || ''`, &links))
		return links
	}
	step(t, ctx, "opening the helper's link", chromedp.Navigate(srv.URL+helper), chromedp.WaitReady(`body`, chromedp.ByQuery))
	if got := saved("trampolineCompetitions"); got != "" {
		t.Errorf("a helper's link isn't saved: %s", got)
	}
	step(t, ctx, "opening the organiser's page", chromedp.Navigate(srv.URL+admin), chromedp.WaitReady(`body`, chromedp.ByQuery))
	if got := saved("trampolineCompetitions"); !strings.Contains(got, `"url":"`+srv.URL+admin+`"`) || !strings.Contains(got, `"name":"Student Open"`) {
		t.Errorf("the organiser's link is saved: %s", got)
	}
	step(t, ctx, "opening the club's page", chromedp.Navigate(srv.URL+club.page+"/members"), chromedp.WaitReady(`body`, chromedp.ByQuery))
	if got := saved("trampolineClubs"); !strings.Contains(got, `"url":"`+srv.URL+club.page+`"`) || !strings.Contains(got, `"name":"UCD"`) {
		t.Errorf("the comp sec's link is saved: %s", got)
	}
}
