package web

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"tariffCalculator/store"
)

// roleTab is one of the role tabs on a page.
type roleTab struct {
	label, href string
	current     bool
}

// roleTabsIn are a page's role tabs, if it has them.
func roleTabsIn(page string) []roleTab {
	var out []roleTab
	for _, m := range regexp.MustCompile(`<li class="role-tab( is-active)?"><a href="([^"]*)"[^>]*>([^<]*)</a>`).FindAllStringSubmatch(page, -1) {
		out = append(out, roleTab{label: m[3], href: unescape(m[2]), current: m[1] != ""})
	}
	return out
}

// roleTabTo is the href of a page's role tab, by its label.
func roleTabTo(t *testing.T, page, label string) string {
	t.Helper()
	for _, tab := range roleTabsIn(page) {
		if tab.label == label {
			return tab.href
		}
	}
	t.Fatalf("no %s tab in: %v", label, roleTabsIn(page))
	return ""
}

// currentRole is the label of a page's current role tab.
func currentRole(t *testing.T, page string) string {
	t.Helper()
	for _, tab := range roleTabsIn(page) {
		if tab.current {
			return tab.label
		}
	}
	t.Fatalf("no current role in: %v", roleTabsIn(page))
	return ""
}

// noticeOf is the notice a redirect carries.
func noticeOf(t *testing.T, location string) string {
	t.Helper()
	u, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("notice")
}

// A member who coaches adds their coach link to their page: each link opens
// both, with a tab for each, and either side undoes it.
func TestRoleTabs(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", false, "A. Murphy")
	club, member := ucd.page, ucd.members["A. Murphy"]
	get := func(path string) string { return dayAll(t, h, path) }
	do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"A. Murphy"}})
	// A coach's link is shown once, when made.
	coach := pathIn(t, do(t, h, http.MethodPost, club+"/coaches/"+regexp.MustCompile(regexp.QuoteMeta(club) + `/coaches/([^/"]+)/new-link`).FindStringSubmatch(get(club + "/coaches"))[1]+"/new-link", url.Values{"anyway": {"1"}}).Body.String(), "/clubs/coach/")

	for _, page := range []string{member, member + "/details", coach} {
		if tabs := roleTabsIn(get(page)); len(tabs) != 0 {
			t.Errorf("%s has one role, and no tabs: %v", page, tabs)
		}
	}
	details := get(member + "/details")
	if !strings.Contains(details, "Your other links") || !strings.Contains(details, `action="`+member+`/links"`) {
		t.Fatalf("the member's details offer to add their other link: %s", details)
	}

	// Their own link, a link that isn't theirs, and a stranger's.
	for link, want := range map[string]string{
		"http://example.com" + member:                 "That's this page's own link",
		"http://example.com/clubs/join/xyz":           "a link for joining a club",
		"http://example.com/clubs/member/nothingHere": "doesn't lead anywhere",
		"not a link": "isn't a link from this site",
		"http://example.com" + member + "/details?x=1#y": "That's this page's own link",
	} {
		to := redirected(t, h, member+"/links", url.Values{"link": {link}})
		if !strings.HasPrefix(to, member+"/details?") || !strings.Contains(noticeOf(t, to), want) {
			t.Errorf("%s: %s", link, noticeOf(t, to))
		}
	}

	// Someone else's link (by first name) is asked about first: a member
	// adding their coach's would become them.
	do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Bob Byrne"}})
	bob := pathIn(t, do(t, h, http.MethodPost, club+"/coaches/"+regexp.MustCompile(`/coaches/([^/"]+)/new-link`).FindAllStringSubmatch(get(club+"/coaches"), -1)[1][1]+"/new-link", url.Values{"anyway": {"1"}}).Body.String(), "/clubs/coach/")
	if page := do(t, h, http.MethodPost, member+"/links", url.Values{"link": {"http://example.com" + bob}}).Body.String(); !strings.Contains(page, "Is Bob Byrne you?") || !strings.Contains(page, `name="sure" value="1"`) {
		t.Errorf("asked first: %s", page)
	}
	if strings.Contains(get(member+"/details"), "as Bob Byrne") {
		t.Error("asking added nothing")
	}

	to := redirected(t, h, member+"/links", url.Values{"link": {"http://example.com" + coach}})
	if !strings.Contains(noticeOf(t, to), "Added") {
		t.Fatalf("added: %s", to)
	}
	if to := redirected(t, h, coach+"/links", url.Values{"link": {"http://example.com" + member}}); !strings.Contains(noticeOf(t, to), "already on your page") {
		t.Errorf("already there, from the other side: %s", noticeOf(t, to))
	}

	// From the member's link: tabs for both, the coach's page through it.
	home := get(member)
	tabs := roleTabsIn(home)
	if len(tabs) != 2 || tabs[0].label != "Member" || !tabs[0].current || tabs[1].label != "Coach" || tabs[0].href != member {
		t.Fatalf("the member's tabs: %v", tabs)
	}
	asCoach := roleTabTo(t, home, "Coach")
	if !strings.HasPrefix(asCoach, "/clubs/coach/"+strings.TrimPrefix(member, "/clubs/member/")+"~") || strings.Contains(asCoach, strings.TrimPrefix(coach, "/clubs/coach/")) {
		t.Errorf("the coach's page is the member link's way to it, never the coach link: %s", asCoach)
	}
	page := get(asCoach)
	if currentRole(t, page) != "Coach" || !strings.Contains(page, "A. Murphy") || strings.Contains(page, "Your coach link") || strings.Contains(page, "data-coach-link") || strings.Contains(page, "Your other links") {
		t.Errorf("the coach's page, with no link of its own to keep and no other links to add there: %s", page)
	}
	// The member's details list it; the coach's details list the member.
	if !strings.Contains(get(member+"/details"), "Coach at UCD, as A. Murphy") {
		t.Error("the member's details list the coach")
	}
	if home := get(coach); strings.Contains(home, "Your other links") || !strings.Contains(home, `href="`+coach+`/details"`) {
		t.Error("the coach's Entries start with the competitions; their links are under Your details")
	}
	if rec := do(t, h, http.MethodGet, asCoach+"/details", nil); rec.Code != http.StatusNotFound || strings.Contains(page, asCoach+"/details") {
		t.Errorf("no Your details through the member's link, whose own page has them: %d", rec.Code)
	}
	coachPage := get(coach + "/details")
	if roleTabTo(t, coachPage, "Member") == member || !strings.Contains(coachPage, "Member of UCD, as A. Murphy") {
		t.Errorf("the coach's page lists the member, opened through the coach's link: %v", roleTabsIn(coachPage))
	}

	// Undone from the coach's page.
	kind, id := undoFor(t, coachPage, "Member of UCD")
	if to := redirected(t, h, coach+"/links/remove", url.Values{"kind": {kind}, "id": {id}}); !strings.Contains(noticeOf(t, to), "Taken off") {
		t.Errorf("undone: %s", to)
	}
	if tabs := roleTabsIn(get(member)); len(tabs) != 0 {
		t.Errorf("no tabs once undone: %v", tabs)
	}
	if rec := do(t, h, http.MethodGet, asCoach, nil); rec.Code != http.StatusNotFound {
		t.Errorf("the way to the coach's page ends: %d", rec.Code)
	}
}

// undoFor is the kind and id an Undo button posts, for a role on the page.
func undoFor(t *testing.T, page, label string) (string, string) {
	t.Helper()
	m := regexp.MustCompile(`(?s)<span>` + regexp.QuoteMeta(label) + `[^<]*</span>.*?name="kind" value="([^"]+)".*?name="id" value="([^"]+)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no Undo for %s in: %s", label, page)
	}
	return m[1], m[2]
}

// The comp sec makes a member a coach too: no coach link, the member's opens
// the coach's page.
func TestMakeACoachToo(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", false, "A. Murphy")
	club, member := ucd.page, ucd.members["A. Murphy"]
	get := func(path string) string { return dayAll(t, h, path) }
	members := get(club + "/members")
	id := regexp.MustCompile(`name="member" value="([^"]+)"`).FindStringSubmatch(members)
	if id == nil || !strings.Contains(members, "Make a coach too") {
		t.Fatalf("the Members page offers it: %s", members)
	}
	to := redirected(t, h, club+"/coaches/from-member", url.Values{"member": {id[1]}})
	if !strings.HasPrefix(to, club+"/coaches?") || !strings.Contains(noticeOf(t, to), "A. Murphy is a coach too") {
		t.Errorf("back to Coaches: %s", to)
	}
	if to := redirected(t, h, club+"/coaches/from-member", url.Values{"member": {id[1]}}); !strings.Contains(noticeOf(t, to), "a coach already") {
		t.Errorf("only once: %s", noticeOf(t, to))
	}
	if page := get(club + "/coaches"); !strings.Contains(page, "A member too: opens from their member link") || strings.Contains(page, "A member who coaches…") {
		t.Errorf("the Coaches page says so, with no one left to make a coach: %s", page)
	}
	if page := get(club + "/members"); !strings.Contains(page, "Coach too") || strings.Contains(page, "Make a coach too</button>") {
		t.Errorf("the Members page says so: %s", page)
	}
	home := get(member)
	page := get(roleTabTo(t, home, "Coach"))
	if currentRole(t, page) != "Coach" || !strings.Contains(page, "UCD · coach") {
		t.Errorf("the coach's page through the member link: %s", page)
	}
}

// A shared link added to a personal page opens from it, never the other way.
func TestSharedLinkOneWay(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", false, "A. Murphy")
	club, member := ucd.page, ucd.members["A. Murphy"]
	get := func(path string) string { return dayAll(t, h, path) }
	redirected(t, h, member+"/links", url.Values{"link": {"http://example.com" + club + "/settings"}})
	home := get(member)
	asCompSec := roleTabTo(t, home, "Club admin")
	if !strings.HasPrefix(asCompSec, "/clubs/admin/"+strings.TrimPrefix(member, "/clubs/member/")+"~") {
		t.Fatalf("the comp sec's pages, through the member's link: %s", asCompSec)
	}
	page := get(asCompSec + "/members")
	if currentRole(t, page) != "Club admin" || !strings.Contains(page, "A. Murphy") {
		t.Errorf("the Members section, with tabs: %s", page)
	}
	if page := get(asCompSec + "?new=created"); strings.Contains(page, "data-club-admin") || strings.Contains(page, "http://example.com"+asCompSec) {
		t.Error("the way in is never shown as the admin link to share")
	}

	// The admin link itself opens no one's pages, and shows no one's tabs.
	adminToken := strings.TrimPrefix(club, "/clubs/admin/")
	if tabs := roleTabsIn(get(club)); len(tabs) != 0 {
		t.Errorf("the admin link has no tabs: %v", tabs)
	}
	memberID := regexp.MustCompile(`name="member" value="([^"]+)"`).FindStringSubmatch(get(club + "/members"))[1]
	for _, path := range []string{"/clubs/member/" + adminToken + "~" + memberID, "/clubs/member/" + adminToken} {
		if rec := do(t, h, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s opens: %d", path, rec.Code)
		}
	}
	// (Its /links makes the club's links with a role, ADR 0015: a pasted
	// link is no such thing.)
	if rec := do(t, h, http.MethodPost, club+"/links", url.Values{"link": {"http://example.com" + member}}); rec.Code != http.StatusSeeOther ||
		!strings.Contains(rec.Header().Get("Location"), "nothing+was+changed") {
		t.Errorf("nothing is added to an admin link: %d %s", rec.Code, rec.Header().Get("Location"))
	}

	// The comp sec sees who has it, and removes it.
	settings := get(club + "/settings")
	if !strings.Contains(settings, "Opened from people") || !strings.Contains(settings, "A. Murphy") {
		t.Fatalf("the comp sec sees who: %s", settings)
	}
	tie := regexp.MustCompile(`action="` + regexp.QuoteMeta(club) + `/ties/remove"[^>]*><input type="hidden" name="id" value="([^"]+)"`).FindStringSubmatch(settings)
	if tie == nil {
		t.Fatalf("a Remove button: %s", settings)
	}
	redirected(t, h, club+"/ties/remove", url.Values{"id": {tie[1]}})
	if rec := do(t, h, http.MethodGet, asCompSec, nil); rec.Code != http.StatusNotFound {
		t.Errorf("removed: %d", rec.Code)
	}

	// Replacing the admin link ends it too.
	redirected(t, h, member+"/links", url.Values{"link": {"http://example.com" + club}})
	if rec := do(t, h, http.MethodGet, asCompSec, nil); rec.Code != http.StatusOK {
		t.Fatalf("added again: %d", rec.Code)
	}
	redirected(t, h, club+"/replace-link", url.Values{"anyway": {"1"}})
	if rec := do(t, h, http.MethodGet, asCompSec, nil); rec.Code != http.StatusNotFound {
		t.Errorf("a replaced admin link ends it: %d", rec.Code)
	}
}

// A competition's admin and helper links open from a personal page too.
func TestCompetitionLinksOnAPersonalPage(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", false, "A. Murphy")
	_, member := ucd.page, ucd.members["A. Murphy"]
	get := func(path string) string { return dayAll(t, h, path) }
	chair := madeLink(t, h, admin, "Chairs", "chair")
	redirected(t, h, member+"/links", url.Values{"link": {"http://example.com" + chair}})
	home := get(member)
	asChair := roleTabTo(t, home, "Chairs · Student Open")
	if page := get(asChair + "/day"); currentRole(t, page) != "Chairs · Student Open" {
		t.Errorf("the chair's pages: %s", page)
	}
	if rec := do(t, h, http.MethodGet, asChair+"/settings", nil); rec.Code != http.StatusForbidden {
		t.Errorf("no more than the chair's link can: %d", rec.Code)
	}
	if page := get(admin + "/settings"); !strings.Contains(page, "A. Murphy") || !strings.Contains(page, "Chairs ·") {
		t.Errorf("the organiser sees who: %s", page)
	}
	settings := get(admin + "/settings")
	for _, id := range regexp.MustCompile(`action="[^"]*/links/([^/"]+)/remove"`).FindAllStringSubmatch(settings, -1) {
		redirected(t, h, admin+"/links/"+id[1]+"/remove", url.Values{"anyway": {"1"}})
	}
	if rec := do(t, h, http.MethodGet, asChair, nil); rec.Code != http.StatusNotFound {
		t.Errorf("a removed helper link ends it: %d", rec.Code)
	}
}

// Someone entering on their own gets one page: a second discipline joins it,
// from it or by giving its link, and their entries are one person's.
func TestIndividualPage(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	form.Set("dmt", "Open")
	admin := created(t, h, form)
	get := func(path string) string { return dayAll(t, h, path) }
	enter := pathIn(t, get(admin+"/settings"), "/competitions/enter/")
	if page := get(enter); !strings.Contains(page, `name="as"`) || !strings.Contains(page, "data-saved-as") {
		t.Errorf("the entry form asks for their page's link: %s", page)
	}
	trampoline := bucsEntry("Róisín O'Neill")
	saved := redirected(t, h, enter, trampoline)
	first := withoutQuery(saved)
	page := get(saved)
	link := pathIn(t, page, "/competitions/gymnast/")
	if !strings.Contains(page, "data-gymnast-link") || !strings.Contains(page, "opens this entry and every other") || !strings.HasPrefix(first, "/competitions/entry/"+strings.TrimPrefix(link, "/competitions/gymnast/")+"~") {
		t.Fatalf("the entry, on the person's page, with their personal link to keep: %s %s", first, page)
	}
	if cur, _ := menuCurrent(page); cur != "Competitions" || !strings.Contains(page, `href="`+link+`/details"`) {
		t.Errorf("the entry is in their Competitions: %s", page)
	}

	// Their page lists the competition; its page has the discipline.
	home := get(link)
	comp := regexp.MustCompile(`href="(` + regexp.QuoteMeta(link) + `/competitions/[^"]+)"`).FindStringSubmatch(home)
	if comp == nil || !strings.Contains(home, "1 entry") {
		t.Fatalf("their competitions: %s", home)
	}
	page = get(comp[1])
	if !strings.Contains(page, "Trampoline") || !strings.Contains(page, `action="`+first+`"`) || !strings.Contains(page, "Enter another discipline") {
		t.Fatalf("a row per discipline: %s", page)
	}
	// Another discipline opens on one not entered, and a second trampoline
	// entry is refused.
	if !strings.Contains(page, `href="`+comp[1]+`/enter?discipline=tumbling"`) {
		t.Error("Enter another discipline opens on one not entered")
	}
	if rec := do(t, h, http.MethodPost, comp[1]+"/enter", bucsEntry("Róisín O'Neill")); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "already entered Trampoline") {
		t.Errorf("one entry a discipline: %d", rec.Code)
	}
	// Another discipline from their page, under the same link.
	tumbling := url.Values{"gymnast": {"Róisín O'Neill"}, "discipline": {"tumbling"}, "level": {"Novice"}}
	second := withoutQuery(redirected(t, h, comp[1]+"/enter", tumbling))
	if !strings.HasPrefix(second, "/competitions/entry/"+strings.TrimPrefix(link, "/competitions/gymnast/")+"~") {
		t.Errorf("under the same link: %s", second)
	}
	if page := get(comp[1]); strings.Count(page, `class="box comp-event-row"`) != 2 || !strings.Contains(page, "Tumbling") {
		t.Errorf("both on the page: %s", page)
	}

	// And a third, through the competition's entry link, as the person.
	dmt := url.Values{"gymnast": {"Róisín O'Neill"}, "discipline": {"dmt"}, "level": {"Open"}, "as": {"http://example.com" + link + "/details"}}
	if third := withoutQuery(redirected(t, h, enter, dmt)); !strings.HasPrefix(third, "/competitions/entry/"+strings.TrimPrefix(link, "/competitions/gymnast/")+"~") {
		t.Errorf("entered as the person whose link was given: %s", third)
	}
	if page := get(link); !strings.Contains(page, "3 entries") {
		t.Errorf("all three on their page: %s", page)
	}
	if rec := do(t, h, http.MethodPost, enter, url.Values{"gymnast": {"X"}, "discipline": {"tumbling"}, "level": {"Novice"}, "as": {"http://example.com/clubs/join/abc"}}); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "the link of your own page") {
		t.Errorf("a link that isn't someone's page: %d", rec.Code)
	}

	// One person to the organiser: one row in All people.
	people := get(admin + "/people")
	if n := strings.Count(people, `data-name="Róisín O&#39;Neill"`); n != 1 {
		t.Errorf("one person, listed %d times: %s", n, people)
	}
}

// Two people of one name, each entering on their own, are two people.
func TestSameNameTwoPeople(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	get := func(path string) string { return dayAll(t, h, path) }
	enter := pathIn(t, get(admin+"/settings"), "/competitions/enter/")
	name := "Sam O'Brien / Jr?"
	one := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {name}, "discipline": {"tumbling"}, "level": {"Novice"}}))
	two := withoutQuery(redirected(t, h, enter, bucsEntry(name)))
	people := get(admin + "/people")
	if n := strings.Count(people, `data-name="Sam O&#39;Brien / Jr?"`); n != 2 {
		t.Errorf("two people, listed %d times: %s", n, people)
	}
	pages := regexp.MustCompile(`href="(`+regexp.QuoteMeta(admin)+`/people/[^"]+)"[^>]*>Sam O&#39;Brien / Jr\?</a>`).FindAllStringSubmatch(people, -1)
	if len(pages) != 2 || pages[0][1] == pages[1][1] {
		t.Fatalf("a page each: %v", pages)
	}
	for _, person := range pages {
		if rec := do(t, h, http.MethodGet, unescape(person[1]), nil); rec.Code != http.StatusOK {
			t.Errorf("their page opens, its key escaped: %s %d", person[1], rec.Code)
		}
	}
	for _, link := range []string{one, two} {
		personal, _, _ := strings.Cut(strings.TrimPrefix(link, "/competitions/entry/"), "~")
		if page := get("/competitions/gymnast/" + personal); !strings.Contains(page, "1 entry") {
			t.Errorf("each has only their own: %s", page)
		}
	}
	// One's link never opens the other's entry.
	oneLink, _, _ := strings.Cut(strings.TrimPrefix(one, "/competitions/entry/"), "~")
	_, twoID, _ := strings.Cut(two, "~")
	if rec := do(t, h, http.MethodGet, "/competitions/entry/"+oneLink+"~"+twoID, nil); rec.Code != http.StatusNotFound {
		t.Errorf("someone else's entry: %d", rec.Code)
	}
}

// An entry link sent before individuals had one page still opens the entry,
// on its person's page; adding another such link joins them.
func TestOldEntryLinks(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	h := routesWith(st)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	created(t, h, form)

	// Two entries as the migration leaves them: each its own individual, with
	// its entry link and its key by name.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "tariff.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var competition string
	if err := db.QueryRow(`SELECT id FROM competitions`).Scan(&competition); err != nil {
		t.Fatal(err)
	}
	hash := func(token string) string { s := sha256.Sum256([]byte(token)); return hex.EncodeToString(s[:]) }
	for i, e := range []struct{ token, discipline, level string }{{"oldLinkOne", "", "BUCS L3"}, {"oldLinkTwo", "tumbling", "Novice"}} {
		ind, entry := "ind"+e.token, "entry"+e.token
		if _, err := db.Exec(`INSERT INTO individuals (id, person_id, name, created_at) VALUES ($1, $1, 'Una Ní Bhriain', '2027-01-01T00:00:00.000000Z')`, ind); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO entries (id, competition_id, club_name, individual, gymnast, token_hash, entry, sent_at, entered_at, discipline, individual_id, person_key)
			VALUES ($1, $2, '', TRUE, 'Una Ní Bhriain', $3, $4, '2027-01-02T00:00:00.000000Z', '2027-01-02T00:00:00.000000Z', $5, $6, 'i:una ní bhriain')`,
			entry, competition, hash(e.token), `{"gymnast":"Una Ní Bhriain","level":"`+e.level+`","discipline":"`+e.discipline+`"}`, e.discipline, ind); err != nil {
			t.Fatal(i, err)
		}
	}
	get := func(path string) string { return dayAll(t, h, path) }

	page := get("/competitions/entry/oldLinkOne")
	if !strings.Contains(page, "Una Ní Bhriain") || !strings.Contains(page, `data-gymnast-link="http://example.com/competitions/gymnast/oldLinkOne"`) || !strings.Contains(page, `action="/competitions/entry/oldLinkOne~entryoldLinkOne"`) {
		t.Fatalf("the old link opens its person's page at its entry, as their link to keep: %s", page)
	}
	if cur, _ := menuCurrent(page); cur != "Competitions" || !strings.Contains(page, `href="/competitions/gymnast/oldLinkOne/details"`) {
		t.Errorf("on its person's page: %s", page)
	}
	if page := get("/competitions/gymnast/oldLinkOne"); !strings.Contains(page, "1 entry") {
		t.Errorf("its person's page has the one entry: %s", page)
	}

	// Adding the other: one person, one page, one tab.
	to := redirected(t, h, "/competitions/gymnast/oldLinkOne/links", url.Values{"link": {"https://tariff.example/competitions/entry/oldLinkTwo"}})
	if !strings.Contains(noticeOf(t, to), "Added") {
		t.Fatalf("added: %s", to)
	}
	if page := get("/competitions/gymnast/oldLinkTwo"); !strings.Contains(page, "2 entries") || len(roleTabsIn(page)) != 0 {
		t.Errorf("both entries, from either link, entering on their own being one role: %s", page)
	}
	if rec := do(t, h, http.MethodGet, "/competitions/entry/oldLinkOne~entryoldLinkTwo", nil); rec.Code != http.StatusOK {
		t.Errorf("one link opens the other's entry: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/competitions/entry/oldLinkTwo", nil); rec.Code != http.StatusOK {
		t.Errorf("the old links still work: %d", rec.Code)
	}
}

// All people: a member who coaches too is one row and one page.
func TestAllPeopleJoinsRoles(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	get := func(path string) string { return dayAll(t, h, path) }
	name := "Ann O'Neill / Jr?"
	ucd := aClub(t, h, admin, "UCD", true, name, "Mo")
	club, members := ucd.page, ucd.members
	memberID := func(who string) string {
		t.Helper()
		page := get(club + "/members")
		m := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(strings.ReplaceAll(who, "'", "&#39;")) + `.*?name="member" value="([^"]+)"`).FindStringSubmatch(page)
		if m == nil {
			t.Fatalf("no %s in: %s", who, page)
		}
		return m[1]
	}
	annID := memberID(name)
	redirected(t, h, club+"/coaches/from-member", url.Values{"member": {annID}})
	coachID := regexp.MustCompile(regexp.QuoteMeta(club) + `/coaches/([^/"]+)/signs-off`).FindStringSubmatch(get(club + "/coaches"))[1]
	redirected(t, h, members["Mo"]+"/coach", url.Values{"coach": {coachID}})
	redirected(t, h, formAction(t, clubEntriesPage(t, h, club), club+"/competitions/"), url.Values{"which": {"all"}})

	people := get(admin + "/people")
	if n := strings.Count(people, `data-name="Ann O&#39;Neill / Jr?"`); n != 1 {
		t.Fatalf("one row for Ann, listed %d times: %s", n, people)
	}
	row := regexp.MustCompile(`(?s)<tr data-list-row="" data-name="Ann O&#39;Neill / Jr\?"[^>]*>.*?</tr>`).FindString(people)
	if !slices.Equal(listRows(t, people, "people")[name]["does"], []string{"gymnast", "coach"}) || !strings.Contains(row, "coaches 1") {
		t.Errorf("competing and coaching: %s", row)
	}
	// Her page, from the list or by her coach's key, is one page.
	page := get(personLink(t, people, admin, "Ann O&#39;Neill / Jr?"))
	if got := myCompetitionOf(t, page); !slices.Equal(got.Roles, []string{"Gymnast", "Coach"}) || !slices.Equal(got.Coaching, []string{"Mo · Trampoline BUCS L3"}) || len(got.Events) != 1 {
		t.Errorf("everything she does: %+v", got)
	}
	mo := get(personLink(t, people, admin, "Mo"))
	coach := regexp.MustCompile(`href="(` + regexp.QuoteMeta(admin) + `/people/c[^"]+)"`).FindStringSubmatch(mo)
	if coach == nil {
		t.Fatalf("Mo's page links to his coach: %s", mo)
	}
	if rec := do(t, h, http.MethodGet, unescape(coach[1]), nil); rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "/people/m:") {
		t.Errorf("her coach's key goes to her page: %d %s", rec.Code, rec.Header().Get("Location"))
	}
}
