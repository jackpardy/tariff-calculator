package web

import (
	stdhtml "html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/views"
)

// menuCurrent is the section and the page the menu in a rendered page shows
// as current ("" for none).
func menuCurrent(html string) (section, item string) {
	html = stdhtml.UnescapeString(html)
	tabs := regexp.MustCompile(`<li class="is-active"><a href="[^"]*" aria-current>([^<]*?)\s*(?:<span|</a>)`).FindAllStringSubmatch(html, -1)
	if len(tabs) > 0 {
		section = tabs[0][1]
	}
	if len(tabs) > 1 {
		item = tabs[1][1]
	}
	return section, item
}

// menuLabels are the sections of the menu in a rendered page.
func menuLabels(html string) []string {
	start := strings.Index(html, `<nav class="comp-menu`)
	if start < 0 {
		return nil
	}
	bar := html[start:]
	if end := strings.Index(bar, "</ul>"); end >= 0 {
		bar = bar[:end]
	}
	var out []string
	for _, m := range regexp.MustCompile(`<li class="[^"]*"><a href="[^"]*"(?: aria-current)?>([^<]*?)\s*(?:<span|</a>)`).FindAllStringSubmatch(bar, -1) {
		out = append(out, m[1])
	}
	return out
}

func TestOrgMenuByAccess(t *testing.T) {
	base := "/competitions/admin/x"
	labels := func(sections []views.MenuSection) string {
		var out []string
		for _, s := range sections {
			out = append(out, s.Label)
		}
		return strings.Join(out, ", ")
	}
	items := func(sections []views.MenuSection, section string) string {
		for _, s := range sections {
			if s.Label == section {
				var out []string
				for _, it := range s.Items {
					out = append(out, it.Label)
				}
				return strings.Join(out, ", ")
			}
		}
		return "-"
	}
	all := views.Access{Organiser: true, Settings: true, Timetable: true, Sheets: true, Officials: true, Coaches: true, Day: true, Desk: true, Screen: true, People: true}
	m := menuState{Published: true, Planned: true, ApproveCoaches: true, LateWaiting: 2, CoachesWaiting: 3}
	menu := orgMenu(base, all, m, menuLate)
	if got := labels(menu); got != "Entries, Timetable, On the day, Settings" {
		t.Errorf("the organiser's sections: %s", got)
	}
	if got := items(menu, "Entries"); got != "Entries, People, Late changes, Coaches, Fees" {
		t.Errorf("entries: %s", got)
	}
	if got := items(menu, "Timetable"); got != "Timetable, Setup, Officials, Panel timeline, Simulate, What if there's a delay, What if an official leaves" {
		t.Errorf("timetable: %s", got)
	}
	if got := items(menu, "On the day"); got != "On the day, Desk messages, Venue screen" {
		t.Errorf("on the day: %s", got)
	}
	if got := items(menu, "Settings"); got != "Settings, History" {
		t.Errorf("settings: %s", got)
	}
	if !menu[0].Current || menu[0].Items[2].Badge != "2 to decide" || menu[0].Items[3].Badge != "3 waiting" {
		t.Errorf("the current section badges its pages: %+v", menu[0])
	}
	if people := orgMenu(base, all, m, menuPeople); !people[0].Current || !people[0].Items[1].Current || people[0].Items[1].Label != "People" {
		t.Errorf("People is current on its pages: %+v", people[0])
	}
	// A link that can't see the dashboard (the venue screen) has no People.
	if got := items(orgMenu(base, views.Access{Screen: true, Day: true}, m, menuScreen), "Entries"); strings.Contains(got, "People") {
		t.Errorf("the venue screen's menu: %s", got)
	}

	// Not yet published, planned or approving coaches: those pages aren't offered.
	menu = orgMenu(base, all, menuState{}, menuEntries)
	if got := labels(menu); got != "Entries, Timetable, Settings" {
		t.Errorf("before the timetable is published: %s", got)
	}
	if got := items(menu, "Entries"); got != "Entries, People, Late changes, Fees" {
		t.Errorf("entries without coach approval: %s", got)
	}
	if got := items(menu, "Timetable"); got != "Timetable, Setup, Officials, Simulate" {
		t.Errorf("timetable not planned: %s", got)
	}
	if menu[1].Badge != "" || menu[0].Badge != "" {
		t.Errorf("no badges when nothing waits")
	}
	// A section's tab carries the badge of a page inside it, when it isn't the current one.
	if menu := orgMenu(base, all, menuState{LateWaiting: 1}, menuTimetable); menu[0].Badge != "1 to decide" {
		t.Errorf("badge on the Entries tab: %+v", menu[0])
	}
	// The desk page keeps the day's section while the timetable isn't published.
	if got := labels(orgMenu(base, all, menuState{}, menuDesk)); got != "Entries, Timetable, On the day, Settings" {
		t.Errorf("on the desk page: %s", got)
	}

	// A link for checking cards sees entries and history.
	cards := views.Access{Check: true, People: true}
	if got := labels(orgMenu(base, cards, m, menuEntries)); got != "Entries, History" {
		t.Errorf("a cards link: %s", got)
	}
	// A chair of judges: the sheets are buttons on the entries page, not in the menu.
	chair := views.Access{Sheets: true, Day: true, Desk: true, Screen: true, People: true}
	menu = orgMenu(base, chair, m, menuEntries)
	if got := labels(menu); got != "Entries, On the day, History" {
		t.Errorf("a chair's link: %s", got)
	}
	// Timetable team: the timetable and officials, not settings.
	team := views.Access{Timetable: true, Sheets: true, Officials: true, Day: true, Desk: true, Screen: true, People: true}
	if got := labels(orgMenu(base, team, m, menuEntries)); got != "Entries, Timetable, On the day, History" {
		t.Errorf("a timetable link: %s", got)
	}
}

func TestSettingsPage(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("signoff", "1")
	admin := created(t, h, form)
	page := do(t, h, http.MethodGet, admin+"/settings", nil).Body.String()
	for _, want := range []string{
		"<h1 class=\"title is-4\">Settings</h1>",
		`<h2 class="title is-5 mt-6 mb-3" id="links">Links</h2>`, `id="deadlines">Entries and deadlines`, `id="limits">Limits and waiting lists`,
		`id="events">Events and levels`, `id="signoff">Sign-off and coaches`, `id="video">Video`, `id="live">Going live`,
		`id="individuals">Entering on their own`, `id="admin-link">Admin link and deleting`, `href="#deadlines"`,
		`action="` + admin + `/links"`, `action="` + admin + `/deadline"`, `action="` + admin + `/later-deadlines"`, `action="` + admin + `/limits"`,
		`action="` + admin + `/events"`, `action="` + admin + `/levels/move"`, `action="` + admin + `/split"`, `action="` + admin + `/signoff"`,
		`action="` + admin + `/approve-coaches"`, `action="` + admin + `/video"`, `action="` + admin + `/individuals"`,
		`action="` + admin + `/replace-link"`, `action="` + admin + `/delete"`, "Club link", "Link for entering on their own", "Close entries now", "Make private (pause entries)",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the settings page has %q", want)
		}
	}
	if section, item := menuCurrent(page); section != "Settings" || item != "Settings" {
		t.Errorf("the menu shows the settings: %q, %q", section, item)
	}

	// The entries page keeps the entries, the menu and the way to go live; the settings have moved.
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	for _, gone := range []string{"Links and settings", `action="` + admin + `/deadline"`, `action="` + admin + `/links"`, "Club link", "← All entries", `class="button is-small is-link is-light" href="` + admin + `/timetable"`} {
		if strings.Contains(dash, gone) {
			t.Errorf("the entries page no longer has %q", gone)
		}
	}
	if section, item := menuCurrent(dash); section != "Entries" || item != "Entries" {
		t.Errorf("the menu shows the entries: %q, %q", section, item)
	}
	if got := strings.Join(menuLabels(dash), ", "); got != "Entries, Timetable, Settings" {
		t.Errorf("the organiser's menu: %s", got)
	}

	// A private competition can be taken live from either page.
	private := newCompetition()
	private.Del("opens")
	other := created(t, h, private)
	for _, path := range []string{other, other + "/settings"} {
		if page := do(t, h, http.MethodGet, path, nil).Body.String(); !strings.Contains(page, "Go live now") {
			t.Errorf("%s can go live", path)
		}
	}
	if loc := redirected(t, h, other+"/live", url.Values{"opens": {"now"}, "back": {"settings"}}); !strings.HasPrefix(loc, other+"/settings?notice=") {
		t.Errorf("going live from the settings goes back to them: %s", loc)
	}
	other2 := created(t, h, private)
	if loc := redirected(t, h, other2+"/live", url.Values{"opens": {"now"}}); !strings.HasPrefix(loc, other2+"?notice=") {
		t.Errorf("going live from the entries page goes back to it: %s", loc)
	}
}

func TestSettingsLimitedLinks(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	cards := madeLink(t, h, admin, "Difficulty judges", "cards")
	chair := madeLink(t, h, admin, "Chairs", "chair")
	team := madeLink(t, h, admin, "Timetable team", "timetable")
	co := madeLink(t, h, admin, "Mary", "everything")

	for _, link := range []string{cards, chair, team} {
		if rec := do(t, h, http.MethodGet, link+"/settings", nil); rec.Code != http.StatusForbidden {
			t.Errorf("%s opens the settings: %d", link, rec.Code)
		}
		page := do(t, h, http.MethodGet, link, nil).Body.String()
		for _, label := range menuLabels(page) {
			if label == "Settings" {
				t.Errorf("%s is offered the settings", link)
			}
		}
		if !strings.Contains(strings.Join(menuLabels(page), ","), "History") {
			t.Errorf("%s is offered the history: %v", link, menuLabels(page))
		}
		if rec := do(t, h, http.MethodGet, link+"/history", nil); rec.Code != http.StatusOK {
			t.Errorf("%s opens the history: %d", link, rec.Code)
		}
	}
	if page := do(t, h, http.MethodGet, chair, nil).Body.String(); !strings.Contains(page, ">Score sheets</a>") || !strings.Contains(page, ">Panel timeline</a>") {
		t.Error("a chair of judges has the sheets on the entries page")
	}
	if got := strings.Join(menuLabels(do(t, h, http.MethodGet, team, nil).Body.String()), ", "); got != "Entries, Timetable, History" {
		t.Errorf("the timetable team's menu: %s", got)
	}

	// A co-organiser opens the settings, without making links, replacing the admin link or deleting.
	rec := do(t, h, http.MethodGet, co+"/settings", nil)
	page := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(page, `action="`+co+`/deadline"`) {
		t.Fatalf("a co-organiser's settings: %d", rec.Code)
	}
	for _, gone := range []string{"/links\"", "/replace-link", "/delete", "Admin link and deleting"} {
		if strings.Contains(page, gone) {
			t.Errorf("a co-organiser isn't offered %q", gone)
		}
	}
	if !strings.Contains(page, "Club link") {
		t.Error("a co-organiser still has the club link")
	}
}

func TestSettingsFormsReturnToSettings(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("signoff", "1")
	admin := created(t, h, form)
	for _, tc := range []struct {
		path string
		form url.Values
	}{
		{"/deadline", url.Values{"deadlineDate": {"2030-03-08"}, "deadlineTime": {"12:00"}}},
		{"/deadline", url.Values{"close": {"1"}}},
		{"/later-deadlines", url.Values{"changesDate": {"2030-03-12"}, "changesTime": {"12:00"}}},
		{"/limits", url.Values{"limit-0": {"10"}}},
		{"/limits", url.Values{"limit-0": {"lots"}}},
		{"/events", url.Values{"tumbling": {"Novice"}}},
		{"/levels/move", url.Values{"move": {":1:-1"}}},
		{"/split", url.Values{"split": {"all"}}},
		{"/signoff", url.Values{"on": {"0"}}},
		{"/approve-coaches", url.Values{"on": {"1"}}},
		{"/video", url.Values{"video": {"routine"}}},
		{"/individuals", url.Values{"on": {"0"}}},
		{"/no-wait", url.Values{"on": {"1"}}},
		{"/live", url.Values{"opens": {"private"}, "back": {"settings"}}},
		{"/links", url.Values{"name": {""}, "kind": {"cards"}}},
	} {
		loc := redirected(t, h, admin+tc.path, tc.form)
		if !strings.HasPrefix(loc, admin+"/settings") {
			t.Errorf("POST %s goes to %s, not back to the settings", tc.path, loc)
		}
	}
	// The notice shows on the settings page.
	loc := redirected(t, h, admin+"/deadline", url.Values{"close": {"1"}})
	if page := do(t, h, http.MethodGet, loc, nil).Body.String(); !strings.Contains(page, "Entries are closed.") {
		t.Error("the settings page says what happened")
	}
	// The history still records the changes.
	history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String()
	for _, want := range []string{"Changed when entries close", "Closed entries", "Changed the limits", "Made it private", "Changed when changes and sign-offs close"} {
		if !strings.Contains(history, want) {
			t.Errorf("the history has %q", want)
		}
	}

	// Replacing the admin link shows the new one on the settings.
	loc = redirected(t, h, admin+"/replace-link", url.Values{})
	if !strings.HasSuffix(loc, "/settings?new=replaced") {
		t.Errorf("replacing the link: %s", loc)
	}
}

func TestMenuOnOrganiserPages(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, bucsEntry("Ann Ryan"))
	entry := entryLink(t, h, admin)
	redirected(t, h, admin+"/timetable/plan", url.Values{})

	for _, tc := range []struct {
		path, section, item string
	}{
		{"", "Entries", "Entries"},
		{strings.TrimPrefix(entry, admin), "Entries", "Entries"},
		{"/late", "Entries", "Late changes"},
		{"/fees", "Entries", "Fees"},
		{"/timetable", "Timetable", "Timetable"},
		{"/officials", "Timetable", "Officials"},
		{"/timetable/simulate", "Timetable", "Simulate"},
		{"/timetable/delay", "Timetable", "What if there's a delay"},
		{"/timetable/leave", "Timetable", "What if an official leaves"},
		{"/desk", "On the day", "Desk messages"},
		{"/settings", "Settings", "Settings"},
		{"/history", "Settings", "History"},
	} {
		rec := do(t, h, http.MethodGet, admin+tc.path, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: %d", tc.path, rec.Code)
			continue
		}
		page := rec.Body.String()
		if section, item := menuCurrent(page); section != tc.section || item != tc.item {
			t.Errorf("%s: the menu shows %q, %q; want %q, %q", tc.path, section, item, tc.section, tc.item)
		}
		if strings.Contains(page, "← All entries") {
			t.Errorf("%s still has a back link to the entries", tc.path)
		}
	}

	// The coaches page, once coaches are approved; the day, once published.
	redirected(t, h, admin+"/signoff", url.Values{"on": {"1"}})
	redirected(t, h, admin+"/approve-coaches", url.Values{"on": {"1"}})
	if section, item := menuCurrent(do(t, h, http.MethodGet, admin+"/coaches", nil).Body.String()); section != "Entries" || item != "Coaches" {
		t.Errorf("coaches: %q, %q", section, item)
	}
	redirected(t, h, admin+"/timetable/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"09:00"}, "day-0-end": {"18:00"}})
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"on": {"1"}})
	if section, item := menuCurrent(dayAll(t, h, admin+"/day")); section != "On the day" || item != "On the day" {
		t.Errorf("the day: %q, %q", section, item)
	}
	if got := strings.Join(menuLabels(do(t, h, http.MethodGet, admin, nil).Body.String()), ", "); got != "Entries, Timetable, On the day, Settings" {
		t.Errorf("the published competition's menu: %s", got)
	}

	// Print pages, CSV and the venue screen carry no menu.
	for _, path := range []string{"/cards", "/timetable/print?sheet=judges", "/screen"} {
		if page := do(t, h, http.MethodGet, admin+path, nil).Body.String(); strings.Contains(page, `class="comp-menu`) {
			t.Errorf("%s has the menu", path)
		}
	}
}
