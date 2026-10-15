package web

import (
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// peopleSetup is a competition with a club (UCD) whose member Mo competes at
// BUCS L3, coached by Ann (sent and approved), and judges and can chair;
// Ivy, who enters on her own and judges; Sam, who enters synchro on his own
// with Pat, a partner who hasn't confirmed; and Mary, added by the organiser.
// The timetable is planned but not published.
type peopleSetup struct {
	h             http.Handler
	admin, member string
}

func peopleCompetition(t *testing.T) peopleSetup {
	t.Helper()
	h := competitionServer(t)
	form := newCompetition()
	form.Set("signoff", "1")
	form.Set("synchroLevel", "builtin-level:bucs-l3")
	admin := created(t, h, form)
	redirected(t, h, admin+"/approve-coaches", url.Values{"on": {"1"}, "level-trampoline": {"3"}})
	settings := do(t, h, http.MethodGet, admin+"/settings", nil).Body.String()

	clubLink := pathIn(t, settings, "/competitions/club/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Ann"}})
	page := do(t, h, http.MethodGet, club+"/coaches", nil).Body.String()
	annID := regexp.MustCompile(regexp.QuoteMeta(club) + `/coaches/([^/"]+)/signs-off`).FindStringSubmatch(page)[1]
	upload(t, h, club+"/coaches/"+annID+"/qualifications", map[string]string{"qualification": "bg-trampoline-3"}, "certificate", []byte("%PDF-1.4 Ann"))
	join := pathIn(t, do(t, h, http.MethodGet, club+"/members", nil).Body.String(), "/clubs/join/")
	mo := withoutQuery(redirected(t, h, join, url.Values{"name": {"Mo"}}))
	redirected(t, h, mo+"/coach", url.Values{"coach": {annID}})
	save := formAction(t, memberCompetitionPage(t, h, mo), mo+"/competitions/")
	redirected(t, h, save, bucsEntry(""))
	redirected(t, h, save+"/offer", url.Values{"judge-trampoline": {"1"}, "chair-trampoline": {"1"}})
	comp := regexp.MustCompile(regexp.QuoteMeta(club) + `/competitions/([^/"]+)/coaches`).FindStringSubmatch(do(t, h, http.MethodGet, club, nil).Body.String())[1]
	redirected(t, h, club+"/competitions/"+comp+"/coaches", url.Values{"coach": {annID}})
	redirected(t, h, admin+"/coaches/"+annID, url.Values{"action": {"approve"}})
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, club, nil).Body.String(), club+"/competitions/"), url.Values{"which": {"all"}})

	enter := pathIn(t, settings, "/competitions/enter/")
	entry := bucsEntry("Ivy")
	ivy := withoutQuery(redirected(t, h, enter, entry))
	redirected(t, h, ivy+"/offer", url.Values{"judge-trampoline": {"1"}})
	redirected(t, h, enter, url.Values{"gymnast": {"Sam"}, "discipline": {"synchro"}, "level": {"BUCS L3"}, "partnerName": {"Pat"},
		"ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	// A name with the characters a path can't carry as they are.
	entry.Set("gymnast", "Kit O/Neil? 100% #1")
	redirected(t, h, enter, entry)
	redirected(t, h, admin+"/officials/add", url.Values{"name": {"Mary"}, "judge-trampoline": {"1"}, "chair-trampoline": {"1"}})
	return peopleSetup{h: h, admin: admin, member: mo}
}

// personKeyIn is a person's key, from the link to their page in a page.
func personKeyIn(t *testing.T, page, admin, name string) string {
	t.Helper()
	return strings.TrimPrefix(mustUnescape(t, personLink(t, page, admin, name)), admin+"/people/")
}

// mustUnescape is a path with its escapes undone.
func mustUnescape(t *testing.T, path string) string {
	t.Helper()
	p, err := url.PathUnescape(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// personLink is the link to a person's page from a page, by their name.
func personLink(t *testing.T, page, admin, name string) string {
	t.Helper()
	m := regexp.MustCompile(`href="(` + regexp.QuoteMeta(admin) + `/people/[^"]+)"[^>]*>` + regexp.QuoteMeta(name) + `</a>`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no link to %s's page in: %s", name, page)
	}
	return unescape(m[1])
}

func TestPeople(t *testing.T) {
	s := peopleCompetition(t)
	h, admin := s.h, s.admin
	list := do(t, h, http.MethodGet, admin+"/people", nil).Body.String()

	// One row per person: Mo competes and judges, and is listed once; the
	// individual, the unconfirmed partner, the added official and the coach
	// are there too.
	if !strings.Contains(list, "<strong>7 people</strong>") {
		t.Errorf("Mo, Ivy, Sam, Pat, Kit, Mary and Ann: %s", list)
	}
	for _, name := range []string{"Mo", "Ivy", "Sam", "Pat", "Mary", "Ann", "Kit O/Neil? 100% #1"} {
		if n := strings.Count(list, `data-name="`+name+`"`); n != 1 {
			t.Errorf("%s is listed %d times", name, n)
		}
	}
	row := func(name string) string {
		m := regexp.MustCompile(`(?s)<tr data-list-row="" data-name="` + name + `"[^>]*>.*?</tr>`).FindString(list)
		if m == "" {
			t.Fatalf("no row for %s", name)
		}
		return m
	}
	values := listRows(t, list, "people")
	is := func(name, key string, want ...string) bool { return slices.Equal(values[name][key], want) }
	if r := row("Mo"); !is("Mo", "club", "UCD") || !is("Mo", "does", "gymnast", "official") || !strings.Contains(r, "Trampoline BUCS L3 · judge, can chair") {
		t.Errorf("Mo's row: %s, %v", r, values["Mo"])
	}
	if r := row("Ivy"); !is("Ivy", "club", "Individual") || !strings.Contains(r, "Trampoline BUCS L3 · judge") {
		t.Errorf("Ivy's row: %s", r)
	}
	if r := row("Pat"); !strings.Contains(r, "Synchro BUCS L3") || !is("Pat", "does", "gymnast") {
		t.Errorf("the partner's row: %s", r)
	}
	if r := row("Mary"); !is("Mary", "does", "official") || strings.Contains(r, "BUCS L3 ·") {
		t.Errorf("the added official's row: %s", r)
	}
	if r := row("Ann"); !is("Ann", "does", "coach") || !strings.Contains(r, "coaches 1") || !is("Ann", "club", "UCD") {
		t.Errorf("the coach's row: %s", r)
	}
	for _, want := range []string{`data-list-tools="people"`, `data-noun="people"`, `data-list-filter="club"`, `data-list-filter="does"`,
		`<option value="gymnast">Gymnast</option><option value="official">Official</option><option value="coach">Coach</option>`, `<option value="Individual">Individual</option>`} {
		if !strings.Contains(list, want) {
			t.Errorf("the tools have %s", want)
		}
	}
	if !regexp.MustCompile(`<details class="comp-list mt-5" open><summary class="title is-5 mb-3">People <span[^>]*>\(7\)</span>`).MatchString(list) {
		t.Error("the list is a dropdown under its heading and count")
	}
	if kit := values["Kit O/Neil? 100% #1"]; !slices.Equal(kit["club"], []string{"Individual"}) {
		t.Errorf("an awkward name's values: %v", kit)
	}
	if a, b := strings.Index(list, `data-name="Ann"`), strings.Index(list, `data-name="Mo"`); a < 0 || b < a {
		t.Error("sorted by name")
	}
	if cur, item := menuCurrent(list); cur != "Entries" || item != "People" {
		t.Errorf("People is current in the menu: %s, %s", cur, item)
	}

	// The names link to the people's pages.
	mo := personLink(t, list, admin, "Mo")
	page := do(t, h, http.MethodGet, mo, nil).Body.String()
	for _, want := range []string{"<h1 class=\"title is-4\">Mo</h1>", "UCD ·", `<span class="tag is-info is-light">Judge</span>`, `<span class="tag is-info is-light">Can chair</span>`,
		"← People", `href="` + admin + `/entries/`, "BUCS L3</a>", "to sort out", "Coach: <a href=\"" + admin + "/people/c:", ">Ann</a>",
		"Judge Trampoline (can chair) · sent by their club", "Can judge:", "Judging rule: anyone, up to the level they say.", "No timetable is planned yet.",
		"The timetable isn't published yet"} {
		if !strings.Contains(page, want) {
			t.Errorf("Mo's page has %q", want)
		}
	}
	for _, not := range []string{"I&#39;m here for the rest", "Add to calendar", "Tell me about changes", "Your timetable", "score-sheets"} {
		if strings.Contains(page, not) {
			t.Errorf("Mo's page leaves out %q", not)
		}
	}
	if cur, item := menuCurrent(page); cur != "Entries" || item != "People" {
		t.Errorf("People is current on a person's page too: %s, %s", cur, item)
	}

	// Ann coaches Mo, and links to him; the partner has a page of her own.
	ann := personLink(t, list, admin, "Ann")
	page = do(t, h, http.MethodGet, ann, nil).Body.String()
	if !strings.Contains(page, ">Coaching</h2>") || !strings.Contains(page, `<a href="`+mo+`">Mo</a>`) || !strings.Contains(page, "Sent by their club: Approved ✓.") ||
		strings.Contains(page, ">Entries</h2>") || strings.Contains(page, "The timetable") {
		t.Errorf("Ann's page: %s", page)
	}
	if page := do(t, h, http.MethodGet, personLink(t, list, admin, "Pat"), nil).Body.String(); !strings.Contains(page, "Sam &amp; Pat") && !strings.Contains(page, "Sam & Pat") {
		t.Errorf("Pat's page shows the pair: %s", page)
	}
	if got := myCompetitionOf(t, do(t, h, http.MethodGet, personLink(t, list, admin, "Sam"), nil).Body.String()); got.Name != "Sam" || len(got.Events) != 1 || got.Events[0].Gymnasts != "Sam & Pat" {
		t.Errorf("Sam's page, with the pair: %+v", got)
	}

	// The links reach it: the dashboard's entry tables, the officials page and
	// the coaches page.
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(dash, `href="`+mo+`" title="Everything this person is doing">person</a>`) || !strings.Contains(dash, `>partner</a>`) {
		t.Error("the dashboard links each gymnast, and a pair's partner, to their page")
	}
	if got := personLink(t, do(t, h, http.MethodGet, admin+"/officials", nil).Body.String(), admin, "Mary"); got == "" || do(t, h, http.MethodGet, got, nil).Code != http.StatusOK {
		t.Error("the officials page links Mary")
	}
	if got := personLink(t, do(t, h, http.MethodGet, admin+"/officials", nil).Body.String(), admin, "Mo"); got != mo {
		t.Errorf("the officials page links Mo to the same page: %s", got)
	}
	if got := personLink(t, do(t, h, http.MethodGet, admin+"/coaches", nil).Body.String(), admin, "Ann"); got != ann {
		t.Errorf("the coaches page links Ann to the same page: %s", got)
	}
	// Every link on those pages is to a page that exists.
	for _, from := range []string{"", "/officials", "/coaches", "/people"} {
		for _, link := range regexp.MustCompile(`href="(`+regexp.QuoteMeta(admin)+`/people/[^"]+)"`).FindAllStringSubmatch(do(t, h, http.MethodGet, admin+from, nil).Body.String(), -1) {
			if rec := do(t, h, http.MethodGet, unescape(link[1]), nil); rec.Code != http.StatusOK {
				t.Errorf("%s links to %s: %d", from, link[1], rec.Code)
			}
		}
	}

	// A name with a slash, a question mark, a percent sign and a hash is one
	// path segment, and the page opens for it.
	kit := personLink(t, list, admin, "Kit O/Neil? 100% #1")
	if strings.Contains(strings.TrimPrefix(kit, admin+"/people/"), "/") || strings.ContainsAny(kit, "?#") {
		t.Errorf("the key is one escaped segment: %s", kit)
	}
	if page := do(t, h, http.MethodGet, kit, nil).Body.String(); !strings.Contains(page, `<h1 class="title is-4">Kit O/Neil? 100% #1</h1>`) {
		t.Errorf("Kit's page: %s", page)
	}

	// A key that's nobody's, however it's written, is not found.
	for _, key := range []string{"m:nobody", "i:nobody", "c::nobody", "x", "i:" + url.PathEscape("a/b?c%d")} {
		rec := do(t, h, http.MethodGet, personPath(admin, key), nil)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Person not found") {
			t.Errorf("%q: %d", key, rec.Code)
		}
	}
}

func TestPeoplePlannedAndPublished(t *testing.T) {
	s := peopleCompetition(t)
	h, admin := s.h, s.admin
	list := do(t, h, http.MethodGet, admin+"/people", nil).Body.String()
	mary, mo, ivy := personLink(t, list, admin, "Mary"), personLink(t, list, admin, "Mo"), personLink(t, list, admin, "Ivy")

	tt := admin + "/timetable"
	redirected(t, h, tt+"/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"09:00"}, "day-0-end": {"18:00"}})
	redirected(t, h, tt+"/plan", url.Values{})
	page := do(t, h, http.MethodGet, mary, nil).Body.String()
	if !strings.Contains(page, "(planned, not published yet)") || !strings.Contains(page, "Chair of judges") || strings.Contains(page, "Panel check-in") {
		t.Errorf("Mary's seats once planned: %s", page)
	}
	if strings.Contains(page, "The timetable</h2>") || !strings.Contains(page, "Can judge:") {
		t.Error("an official not competing has no timetable of her own, but can judge")
	}

	redirected(t, h, tt+"/publish", url.Values{"on": {"1"}})
	page = do(t, h, http.MethodGet, mary, nil).Body.String()
	if !strings.Contains(page, "(from the published timetable)") || !strings.Contains(page, "Panel check-in: <strong>Not marked yet</strong>") {
		t.Errorf("Mary's seats once published: %s", page)
	}
	page = do(t, h, http.MethodGet, mo, nil).Body.String()
	for _, want := range []string{"<strong>BUCS L3</strong> · Panel 1 · Saturday", "Warm-up <strong>09:00</strong>", "Check-in: <strong>Not marked yet</strong>", "The timetable</h2>",
		"Mo is picked out in bold", "<strong>Mo</strong>"} {
		if !strings.Contains(page, want) {
			t.Errorf("Mo's page once published has %q", want)
		}
	}
	if strings.Contains(page, "you're") || strings.Contains(page, "ask the organiser") {
		t.Error("the organiser's page doesn't speak to the person")
	}

	// Check-in, a scratch and a desk message show on the person's page.
	day := do(t, h, http.MethodGet, admin+"/day?day=0", nil).Body.String()
	entry := checkinEntry(t, day, "Ivy")
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {entry}, "status": {"here"}, "day": {"0"}})
	if page := do(t, h, http.MethodGet, ivy, nil).Body.String(); !strings.Contains(page, "Check-in: <strong>Here</strong>") {
		t.Errorf("Ivy is here: %s", page)
	}
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {entry}, "status": {"scratched"}, "day": {"0"}})
	if page := do(t, h, http.MethodGet, ivy, nil).Body.String(); !strings.Contains(page, "Check-in: <strong>Scratched</strong>") || !strings.Contains(page, "Scratched · ") {
		t.Errorf("Ivy is scratched: %s", page)
	}
	desk := do(t, h, http.MethodGet, admin+"/desk", nil).Body.String()
	redirected(t, h, admin+"/desk", url.Values{"to": {"people"}, "person": {personOnDesk(t, desk, "Ivy")}, "text": {"Come to the desk, please"}})
	if page := do(t, h, http.MethodGet, ivy, nil).Body.String(); !strings.Contains(page, "Messages from the organisers") || !strings.Contains(page, "Come to the desk, please") {
		t.Errorf("Ivy's desk message: %s", page)
	}
	if page := do(t, h, http.MethodGet, mo, nil).Body.String(); strings.Contains(page, "Come to the desk, please") {
		t.Error("Mo wasn't sent it")
	}
}

func TestPeopleAccess(t *testing.T) {
	s := peopleCompetition(t)
	h, admin := s.h, s.admin
	mo := personLink(t, do(t, h, http.MethodGet, admin+"/people", nil).Body.String(), admin, "Mo")
	path := strings.TrimPrefix(mo, admin)

	screen := madeLink(t, h, admin, "Lobby", "screen")
	for _, p := range []string{"/people", path} {
		if got := do(t, h, http.MethodGet, screen+p, nil).Code; got != http.StatusForbidden {
			t.Errorf("the venue screen link opens %s: %d", p, got)
		}
	}
	// Every link that sees the dashboard sees the people, and the menu offers them.
	for _, kind := range []string{"cards", "chair", "timetable", "everything"} {
		link := madeLink(t, h, admin, kind+" link", kind)
		for _, p := range []string{"/people", path} {
			if got := do(t, h, http.MethodGet, link+p, nil).Code; got != http.StatusOK {
				t.Errorf("the %s link opens %s: %d", kind, p, got)
			}
		}
		if page := do(t, h, http.MethodGet, link, nil).Body.String(); !strings.Contains(page, `href="`+link+`/people"`) {
			t.Errorf("the %s link's menu offers People", kind)
		}
	}
	if page := do(t, h, http.MethodGet, screen, nil).Body.String(); strings.Contains(page, "/people") {
		t.Error("the venue screen offers no People")
	}
	// Nothing can be changed from the pages: they only read.
	if rec := do(t, h, http.MethodPost, admin+"/people", url.Values{}); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("posting to the list: %d", rec.Code)
	}
}

// The member's own page is the page it was: nothing of the organiser's view
// leaks onto it.
func TestMemberPageUnchangedByPeople(t *testing.T) {
	s := peopleCompetition(t)
	h, admin := s.h, s.admin
	tt := admin + "/timetable"
	redirected(t, h, tt+"/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"09:00"}, "day-0-end": {"18:00"}})
	redirected(t, h, tt+"/plan", url.Values{})
	redirected(t, h, tt+"/publish", url.Values{"on": {"1"}})
	day := regexp.MustCompile(`href="(` + regexp.QuoteMeta(s.member) + `/competitions/[^"/]+/day)"`).FindStringSubmatch(memberCompetitionPage(t, h, s.member))
	if day == nil {
		t.Fatal("the member page links to their competition")
	}
	page := do(t, h, http.MethodGet, day[1], nil).Body.String()
	for _, want := range []string{"← Your entries", "<h1 class=\"title is-4\">Student Open</h1>", "<p class=\"subtitle is-6\">Mo · ", "you're <strong>", "You're picked out in bold", "Your timetable", "Add to calendar"} {
		if !strings.Contains(page, want) {
			t.Errorf("the member's page has %q", want)
		}
	}
	for _, not := range []string{"← People", "Judging rule", "Check-in:", "/people/", `class="comp-menu`, "Coach:"} {
		if strings.Contains(page, not) {
			t.Errorf("the member's page has %q", not)
		}
	}
}
