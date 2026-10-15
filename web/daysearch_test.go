package web

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
	"golang.org/x/net/html"
)

// jumpLink is a "Jump to" link at the top of a page.
type jumpLink struct {
	Text, Target string // Target: the id it goes to
}

// jumpLinks are the page's "Jump to" links, in order.
func jumpLinks(t *testing.T, page string) []jumpLink {
	t.Helper()
	var out []jumpLink
	for _, box := range byClass(parsePage(t, page), "comp-jump") {
		for _, a := range byTag(box, "a") {
			out = append(out, jumpLink{textOf(a), strings.TrimPrefix(unescape(attrOf(a, "href")), "#")})
		}
	}
	return out
}

// hasAttr says whether n has an attribute, though it may be empty.
func hasAttr(n *html.Node, key string) bool {
	return slices.ContainsFunc(n.Attr, func(a html.Attribute) bool { return a.Key == key })
}

// Each jump link goes to something on the page.
func jumpsLand(t *testing.T, page string, links []jumpLink) {
	t.Helper()
	doc := parsePage(t, page)
	for _, l := range links {
		if byID(doc, l.Target) == nil {
			t.Errorf("%q jumps to #%s, which isn't on the page", l.Text, l.Target)
		}
	}
}

// On the day has "Find a name" over every check-in row, gymnasts' and
// officials', with each flight, its lists and its area a group that hides or
// opens with the search; and a link to jump to each area.
func TestDayPageSearchAndJump(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore))
	admin, _, _, _ := scratchSetupOn(t, s.h, twoFlights)
	s.clock.set(onTheDay(8, 30))
	page := do(t, s.h, http.MethodGet, admin+"/day", nil).Body.String()

	if !strings.Contains(page, `data-list-tools="day-areas" data-noun="names"`) {
		t.Fatalf("the day page has Find a name over its areas: %s", page)
	}
	rows := listRows(t, page, "day-areas")
	for _, name := range []string{"Eve", "Dara", "Finn", "Mary", "Tom", "Ann"} {
		if _, ok := rows[name]; !ok {
			t.Errorf("%s can be found: %v", name, rows)
		}
	}
	doc := parsePage(t, page)
	for _, box := range byClass(byID(doc, "day-areas"), "comp-flight") {
		if !hasAttr(box, "data-list-group") || !hasAttr(box.Parent.Parent, "data-list-group") {
			t.Errorf("a flight's box, and its area's column, are groups that hide")
		}
	}
	for _, d := range byClass(doc, "comp-checkin") {
		if !hasAttr(d, "data-list-group") {
			t.Errorf("a check-in list (%s) opens when the search finds someone in it", attrOf(d, "data-key"))
		}
	}

	areas := dayPageAreas(t, page)
	links := jumpLinks(t, page)
	if len(areas) < 2 || len(links) != len(areas) {
		t.Fatalf("a link to each of the %d areas: %+v", len(areas), links)
	}
	for _, l := range links {
		if _, ok := areas[l.Text]; !ok || l.Target != areaID(l.Text) {
			t.Errorf("the link to %s goes to its block: %+v", l.Text, l)
		}
	}
	jumpsLand(t, page, links)
}

// The timetable has "Find a name" over the flights' running orders and their
// panels' officials, and links to jump to each day and each day's areas.
func TestTimetableSearchAndJump(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore))
	_, tt, _, _ := scratchSetupOn(t, s.h, twoFlights)
	page := do(t, s.h, http.MethodGet, tt, nil).Body.String()

	if !strings.Contains(page, `data-list-tools="timetable-flights" data-noun="names"`) {
		t.Fatalf("the timetable has Find a name over its flights: %s", page)
	}
	rows := listRows(t, page, "timetable-flights")
	for _, name := range []string{"Eve", "Dara", "Finn", "Mary", "Tom", "Ann"} {
		if _, ok := rows[name]; !ok {
			t.Errorf("%s can be found: %v", name, rows)
		}
	}
	if _, ok := rows[""]; ok {
		t.Error("an empty seat isn't a row")
	}

	links := jumpLinks(t, page)
	if len(links) < 2 || links[0] != (jumpLink{"Day 1", "day-0"}) {
		t.Fatalf("the first link goes to the day: %+v", links)
	}
	if find := strings.Index(page, `id="timetable-find"`); find < 0 || find > strings.Index(page, `href="#day-0"`) {
		t.Error("Find a name is with the jumps, before them")
	}
	for _, l := range links[1:] {
		if l.Target != "day-0-"+areaID(l.Text) {
			t.Errorf("then each of its areas: %+v", l)
		}
	}
	jumpsLand(t, page, links)
}

// A timetable flight card's controls are labelled buttons: Move for each
// gymnast (its flights still filled in by fill.js), and a labelled dropdown
// to move the flight.
func TestTimetableFlightCardButtons(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore))
	_, tt, _, _ := scratchSetupOn(t, s.h, twoFlights)
	doc := parsePage(t, do(t, s.h, http.MethodGet, tt, nil).Body.String())
	labels := map[string]string{}
	for _, l := range byTag(doc, "label") {
		labels[attrOf(l, "for")] = textOf(l)
	}
	cards := byClass(doc, "comp-flight")
	if len(cards) < 2 {
		t.Fatalf("flights to look at: %d", len(cards))
	}
	for _, card := range cards {
		for _, m := range byClass(card, "comp-move") {
			summary := firstChildTag(m, "summary")
			if !hasClass(summary, "button") || textOf(summary) != "Move" || attrOf(firstByClass(m, "comp-move-form"), "class") == "" {
				t.Errorf("each gymnast has a Move button: %s", renderNode(m))
			}
			if sel := byTag(m, "select"); len(sel) != 1 || attrOf(sel[0], "data-fill") != "move-options" {
				t.Errorf("whose flights fill from the page's list")
			}
		}
		actions := firstByClass(card, "comp-tt-actions")
		for _, b := range byTag(actions, "button") {
			if hasClass(b, "is-text") {
				t.Errorf("%q is a button, not a link", textOf(b))
			}
		}
		to := byTag(actions, "select")
		if len(to) != 1 || !strings.HasPrefix(labels[attrOf(to[0], "id")], "Move") {
			t.Errorf("the flight's dropdown is labelled: %q", labels[attrOf(to[0], "id")])
		}
	}
}

// In a real browser, "Find a name" on the day page shows only the flights
// with someone it finds, with their check-in list open; and a tap on Here,
// which updates the page in place, keeps the search, the list it opened and
// the rest hidden. Clearing the search closes the list again.
func TestDaySearchInABrowser(t *testing.T) {
	ctx := browser(t)
	s := newTestServer(t, atTime(dayBefore))
	admin, _, _, _ := scratchSetupOn(t, s.h, twoFlights)
	s.clock.set(onTheDay(8, 30))
	srv := httptest.NewServer(s.h)
	defer srv.Close()

	const eve = `[data-list-row][data-name="Eve"]`
	type view struct {
		Shown   string // the rows shown
		Flights int    // the flights shown
		Open    bool   // Eve's check-in list
		Opened  bool   // marked as opened by the search
		Search  string
		Here    bool // Eve's Here tag
		Same    bool // not reloaded
	}
	read := `({
		Shown: [...document.querySelectorAll('#day-areas [data-list-row]')].filter((r) => !r.hidden).map((r) => r.dataset.name).join(', '),
		Flights: [...document.querySelectorAll('#day-areas .comp-flight')].filter((f) => !f.hidden && !f.closest('[hidden]')).length,
		Open: document.querySelector('` + eve + `').closest('details').open,
		Opened: 'listOpened' in document.querySelector('` + eve + `').closest('details').dataset,
		Search: document.querySelector('[data-list-search]').value,
		Here: !!document.querySelector('` + eve + ` .chip-state-done'),
		Same: window.sameDocument === true,
	})`
	var before, after, cleared view
	step(t, ctx, "searching the day page for Eve",
		chromedp.Navigate(srv.URL+admin+"/day?day=0"),
		chromedp.WaitVisible(`[data-list-search]`, chromedp.ByQuery),
		chromedp.Evaluate(`window.sameDocument = true; {const s = document.querySelector('[data-list-search]'); s.value = 'eve'; s.dispatchEvent(new Event('input', {bubbles: true}))}`, nil),
		chromedp.Evaluate(read, &before),
	)
	if before.Shown != "Eve" || before.Flights != 1 || !before.Open || !before.Opened {
		t.Fatalf("the search shows Eve's flight alone, her list opened: %+v", before)
	}
	step(t, ctx, "tapping Here for Eve",
		chromedp.Click(eve+` button[value="here"]`, chromedp.ByQuery),
		chromedp.WaitVisible(eve+` .chip-state-done`, chromedp.ByQuery),
		chromedp.Evaluate(read, &after),
	)
	if after.Shown != "Eve" || after.Flights != 1 || !after.Open || !after.Opened || after.Search != "eve" || !after.Here || !after.Same {
		t.Errorf("the tap keeps the search, its list open and the rest hidden: %+v", after)
	}
	step(t, ctx, "clearing the search",
		chromedp.Click(`[data-list-clear]`, chromedp.ByQuery),
		chromedp.Evaluate(read, &cleared),
	)
	if cleared.Open || cleared.Flights < 2 || cleared.Search != "" {
		t.Errorf("clearing shows every flight and closes the list it opened: %+v", cleared)
	}
}

// On the day doesn't repeat its menu: the Venue screen and the desk are in
// the menu above it, not again as buttons under it.
func TestDayPageHeader(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore))
	admin, _, _, _ := scratchSetupOn(t, s.h, twoFlights)
	page := do(t, s.h, http.MethodGet, admin+"/day", nil).Body.String()
	for _, path := range []string{"/screen", "/desk"} {
		if n := strings.Count(page, `href="`+admin+path); n != 1 {
			t.Errorf("%s is linked %d times, want once, from the menu", path, n)
		}
	}
	if firstByClass(parsePage(t, page), "comp-day-links") != nil {
		t.Error("no buttons repeating the menu")
	}
}
