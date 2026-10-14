package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// personLink is the link to a named person's page in a page that has one.
func personLink(t *testing.T, page, name string) string {
	t.Helper()
	m := regexp.MustCompile(`href="([^"]*/people/[^"]+)">` + regexp.QuoteMeta(name) + `</a>`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no link to %s in %s", name, page)
	}
	return m[1]
}

func TestPeople(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form["synchroLevel"] = []string{"builtin-level:bucs-l3"}
	admin := created(t, h, form)
	settings := do(t, h, http.MethodGet, admin+"/settings", nil).Body.String()

	// A club with a coach, Ann, and a member, Mo, who competes and judges.
	clubLink := pathIn(t, settings, "/competitions/club/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Ann"}})
	ids := regexp.MustCompile(regexp.QuoteMeta(club) + `/coaches/([^/"]+)/signs-off`).FindStringSubmatch(do(t, h, http.MethodGet, club+"/coaches", nil).Body.String())
	join := pathIn(t, do(t, h, http.MethodGet, club+"/members", nil).Body.String(), "/clubs/join/")
	mo := withoutQuery(redirected(t, h, join, url.Values{"name": {"Mo"}}))
	redirected(t, h, mo+"/coach", url.Values{"coach": {ids[1]}})
	entry := url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}
	redirected(t, h, formAction(t, memberCompetitionPage(t, h, mo), mo+"/competitions/"), entry)
	page := memberCompetitionPage(t, h, mo)
	offer := regexp.MustCompile(`action="(` + regexp.QuoteMeta(mo) + `/competitions/[^"]+/offer)"`).FindStringSubmatch(page)[1]
	redirected(t, h, offer, url.Values{"judge-trampoline": {"1"}, "chair-trampoline": {"1"}})
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, club, nil).Body.String(), club+"/competitions/"), url.Values{"which": {"all"}})

	// An individual, Ivy, who also enters a synchro pair with Pat, who has no entry of her own.
	enter := pathIn(t, settings, "/competitions/enter/")
	redirected(t, h, enter, url.Values{"gymnast": {"Ivy"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	redirected(t, h, enter, url.Values{"gymnast": {"Ivy"}, "discipline": {"synchro"}, "level": {"BUCS L3"}, "partnerName": {"Pat O'Neil"},
		"ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	// An official the organiser added.
	redirected(t, h, admin+"/officials/add", url.Values{"name": {"Judge J"}, "judge-trampoline": {"1"}})

	list := do(t, h, http.MethodGet, admin+"/people", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("the list: %d", list.Code)
	}
	body := list.Body.String()
	if !strings.Contains(body, "<strong>5</strong> people") {
		t.Errorf("Mo, Ann, Ivy, Pat and Judge J, each once: %s", body)
	}
	for _, name := range []string{"Mo", "Ann", "Ivy", "Pat O&#39;Neil", "Judge J"} {
		if n := strings.Count(body, `">`+name+`</a></td>`); n != 1 {
			t.Errorf("%s is in the list %d times", name, n)
		}
	}
	for _, want := range []string{`data-name="Mo" data-club="UCD" data-roles="compete official"`, `data-name="Ann" data-club="UCD" data-roles="coach"`,
		`data-name="Ivy" data-club="individual" data-roles="compete"`, `data-name="Judge J" data-club="" data-roles="official"`,
		"BUCS L3 · judge, can chair", "coaches 1 entry", "BUCS L3, Synchro BUCS L3", `<div class="table-container">`} {
		if !strings.Contains(body, want) {
			t.Errorf("the list has %q", want)
		}
	}
	if section, item := menuCurrent(body); section != "Entries" || item != "People" {
		t.Errorf("the menu shows People: %q, %q", section, item)
	}
	if strings.Index(body, "Ann</a>") > strings.Index(body, "Ivy</a>") || strings.Index(body, "Ivy</a>") > strings.Index(body, "Judge J</a>") {
		t.Error("sorted by name")
	}

	// Mo competes and judges, and is Ann's gymnast.
	moPage := do(t, h, http.MethodGet, personLink(t, body, "Mo"), nil)
	for _, want := range []string{"← People", "UCD", ">Competes<", ">Official<", "Offered: Judge Trampoline (can chair)", "Open the entry", `href="` + admin + `/entries/`, "Entries"} {
		if !strings.Contains(moPage.Body.String(), want) {
			t.Errorf("Mo's page has %q", want)
		}
	}
	if section, item := menuCurrent(moPage.Body.String()); section != "Entries" || item != "People" {
		t.Errorf("the menu on a person's page: %q, %q", section, item)
	}
	for _, not := range []string{"I'm here for the rest", "Tell me about changes", "Add to calendar", "Your timetable"} {
		if strings.Contains(moPage.Body.String(), not) {
			t.Errorf("Mo's page for the organiser has %q", not)
		}
	}

	ann := do(t, h, http.MethodGet, personLink(t, body, "Ann"), nil).Body.String()
	if !strings.Contains(ann, "Coaching") || !strings.Contains(ann, `>Mo</a>`) || !strings.Contains(ann, ">Coach<") {
		t.Errorf("Ann's page lists Mo: %s", ann)
	}
	if !strings.Contains(ann, personLink(t, body, "Mo")) {
		t.Error("Ann's gymnast links to their page")
	}
	if ivy := do(t, h, http.MethodGet, personLink(t, body, "Ivy"), nil).Body.String(); !strings.Contains(ivy, "Individual") || !strings.Contains(ivy, "Synchro BUCS L3") || strings.Count(ivy, "Open the entry") != 2 {
		t.Error("Ivy's page has both her entries")
	}
	patLink := personLink(t, body, "Pat O&#39;Neil")
	if pat := do(t, h, http.MethodGet, patLink, nil).Body.String(); !strings.Contains(pat, "Synchro BUCS L3") || !strings.Contains(pat, "Ivy &amp; Pat") {
		t.Errorf("the partner's page (%s): %s", patLink, pat)
	}
	if j := do(t, h, http.MethodGet, personLink(t, body, "Judge J"), nil).Body.String(); !strings.Contains(j, "No club") || !strings.Contains(j, "Judge Trampoline") || !strings.Contains(j, "No seats yet") {
		t.Errorf("the added official's page: %s", j)
	}

	// Seats, once the timetable is planned.
	tt := admin + "/timetable"
	redirected(t, h, tt+"/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"09:00"}, "day-0-end": {"18:00"}})
	redirected(t, h, tt+"/plan", url.Values{})
	if j := do(t, h, http.MethodGet, personLink(t, body, "Judge J"), nil).Body.String(); !strings.Contains(j, "planned, not published yet") {
		t.Errorf("planned seats: %s", j)
	}
	redirected(t, h, tt+"/publish", url.Values{"on": {"1"}})
	j := do(t, h, http.MethodGet, personLink(t, body, "Judge J"), nil).Body.String()
	if !strings.Contains(j, "Saturday") || !strings.Contains(j, "Panel 1") || strings.Contains(j, "planned, not published yet") || !strings.Contains(j, "Panel check-in: here for 0 of") {
		t.Errorf("published seats: %s", j)
	}
	if moNow := do(t, h, http.MethodGet, personLink(t, body, "Mo"), nil).Body.String(); !strings.Contains(moNow, "Not checked in yet.") || !strings.Contains(moNow, "They&#39;re <strong>") && !strings.Contains(moNow, "they&#39;re <strong>") {
		t.Errorf("Mo's flight and check-in: %s", moNow)
	}

	// The links reach the pages.
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(dash, `href="`+personLink(t, body, "Mo")+`">person</a>`) {
		t.Error("the dashboard's rows link to the person")
	}
	if !strings.Contains(do(t, h, http.MethodGet, admin+"/officials", nil).Body.String(), `href="`+personLink(t, body, "Judge J")+`">Judge J</a>`) {
		t.Error("the Officials page links to the person")
	}
	if section, item := menuCurrent(do(t, h, http.MethodGet, tt, nil).Body.String()); section != "Timetable" || item != "Timetable" {
		t.Errorf("People doesn't change the other sections: %q, %q", section, item)
	}

	// An unknown key is not found; so is a withdrawn-only name.
	if rec := do(t, h, http.MethodGet, admin+"/people/i:nobody", nil); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown key: %d", rec.Code)
	}

	// The member's own page is the member's: no organiser's parts.
	day := regexp.MustCompile(`href="(` + regexp.QuoteMeta(mo) + `/competitions/[^"/]+/day)"`).FindStringSubmatch(memberCompetitionPage(t, h, mo))[1]
	own := do(t, h, http.MethodGet, day, nil).Body.String()
	for _, want := range []string{"← Your entries", "Your timetable", "Add to calendar", "you&#39;re <strong>", "You&#39;re picked out in bold"} {
		if !strings.Contains(own, want) {
			t.Errorf("Mo's own page has %q", want)
		}
	}
	for _, not := range []string{"Open the entry", "People", "Offered:"} {
		if strings.Contains(own, not) {
			t.Errorf("Mo's own page has %q", not)
		}
	}
}

func TestPeopleAccess(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, url.Values{"gymnast": {"Ann Ryan"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	link := personLink(t, do(t, h, http.MethodGet, admin+"/people", nil).Body.String(), "Ann Ryan")
	if link != admin+"/people/i:ann%20ryan" {
		t.Errorf("the key is one escaped path segment: %s", link)
	}
	for _, kind := range []string{"cards", "chair", "timetable", "everything"} {
		l := madeLink(t, h, admin, kind, kind)
		if rec := do(t, h, http.MethodGet, l+"/people", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Ann Ryan") {
			t.Errorf("%s link sees the list: %d", kind, rec.Code)
		}
		if rec := do(t, h, http.MethodGet, l+"/people/i:ann%20ryan", nil); rec.Code != http.StatusOK {
			t.Errorf("%s link sees a person: %d", kind, rec.Code)
		}
		if !strings.Contains(strings.Join(menuLabels(do(t, h, http.MethodGet, l, nil).Body.String()), ","), "Entries") {
			t.Errorf("%s link has the menu", kind)
		}
	}
	screen := madeLink(t, h, admin, "Venue", "screen")
	for _, path := range []string{"/people", "/people/i:ann%20ryan"} {
		if rec := do(t, h, http.MethodGet, screen+path, nil); rec.Code != http.StatusForbidden {
			t.Errorf("the venue screen link opens %s: %d", path, rec.Code)
		}
	}
	if !allowed("cards", "GET "+adminPrefix+"/people/{key}") || allowed("screen", "GET "+adminPrefix+"/people") {
		t.Error("who may see People")
	}
}
