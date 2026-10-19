package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Long pages send what's folded away inert: a folded level's rows and a
// folded day's flights wait in a template until opened, jumped to or
// searched for, so a phone lays out only what's open.

// twoDays is a competition over Saturday and Sunday: Eve's trampoline on
// Saturday, Dara's tumbling on Sunday. It gives the organiser's link.
func twoDays(t *testing.T, h http.Handler) string {
	t.Helper()
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, bucsEntry("Eve"))
	redirected(t, h, enter, url.Values{"gymnast": {"Dara"}, "discipline": {"tumbling"}, "level": {"Novice"}})
	tt := admin + "/timetable"
	redirected(t, h, tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "area-1-name": {"Track 1"}, "area-1-discipline": {"tumbling"}})
	redirected(t, h, tt+"/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"09:00"}, "day-0-end": {"18:00"}, "add": {"1"}})
	redirected(t, h, tt+"/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"09:00"}, "day-0-end": {"18:00"},
		"day-1-name": {"Sunday"}, "day-1-start": {"09:00"}, "day-1-end": {"13:00"}})
	redirected(t, h, tt+"/setup/rules", url.Values{"add": {"1"}, "event": {"Tumbling Novice"}, "kind": {"day"}, "day": {"1"}, "must": {"1"}})
	redirected(t, h, tt+"/plan", url.Values{})
	return admin
}

// The timetable opens on one day; the other's flights are in the page, but
// in a template.
func TestTimetableDaysFold(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore))
	admin := twoDays(t, s.h)
	page := do(t, s.h, http.MethodGet, admin+"/timetable", nil).Body.String()
	saturday, sunday, ok := strings.Cut(page, `id="day-1"`)
	if !ok {
		t.Fatalf("a heading for Sunday: %s", page)
	}
	if !strings.Contains(saturday, `<details class="comp-tt-day" data-list-group open`) || !strings.Contains(saturday, `data-name="Eve"`) || strings.Contains(saturday, "<template data-lazy>") {
		t.Error("Saturday is open, Eve's flight in the page")
	}
	if lazy, _, _ := strings.Cut(sunday, `data-name="Dara"`); !strings.Contains(lazy, "<template data-lazy>") {
		t.Error("Sunday is folded, Dara's flight waiting in a template")
	}
	if !strings.Contains(page, `data-day="day-0" aria-pressed="true"`) || !strings.Contains(page, `data-day="day-1" aria-pressed="false"`) {
		t.Error("a button for each day, Saturday's pressed")
	}
	if !strings.Contains(page, `data-list-tools="timetable-flights" data-noun="names" data-total="`) {
		t.Error("Find a name knows how many names there are, folded ones too")
	}
}

// In a browser: Find a name finds someone on a folded day, a day's button
// shows it (beside the other on a wide screen, in its place on a phone), and
// a flight's Move a gymnast fills from its running order.
func TestTimetableDaysFoldInABrowser(t *testing.T) {
	ctx := browser(t)
	s := newTestServer(t, atTime(dayBefore))
	admin := twoDays(t, s.h)
	srv := httptest.NewServer(s.h)
	defer srv.Close()
	tt := srv.URL + admin + "/timetable"

	type view struct {
		Eve, Dara, SundayOpen bool
		Count, Order          string
	}
	const read = `({
		Eve: !!document.querySelector('[data-name="Eve"]:not([hidden])'),
		Dara: !!document.querySelector('[data-name="Dara"]:not([hidden])'),
		SundayOpen: document.getElementById('day-1').parentElement.open,
		Count: document.querySelector('[data-list-count]').textContent,
		Order: [...document.querySelectorAll('select[data-fill="order"] option')].map((o) => o.textContent).join('|'),
	})`
	var start, found, jumped view
	step(t, ctx, "opening the timetable",
		chromedp.Navigate(tt),
		chromedp.WaitVisible(`[data-list-search]`, chromedp.ByQuery),
		chromedp.Evaluate(read, &start),
	)
	if !start.Eve || start.Dara || start.SundayOpen || start.Count != "2 names" {
		t.Fatalf("Saturday open with Eve, Sunday folded, both names counted: %+v", start)
	}
	step(t, ctx, "finding Dara",
		chromedp.Evaluate(`{const s = document.querySelector('[data-list-search]'); s.value = 'dara'; s.dispatchEvent(new Event('input', {bubbles: true}))}`, nil),
		chromedp.Evaluate(read, &found),
	)
	if !found.Dara || found.Eve || !found.SundayOpen {
		t.Fatalf("the search finds Dara on Sunday, opening it: %+v", found)
	}
	step(t, ctx, "jumping to Sunday",
		chromedp.Evaluate(`sessionStorage.clear()`, nil),
		chromedp.Navigate(tt),
		chromedp.WaitVisible(`[data-list-search]`, chromedp.ByQuery),
		chromedp.EmulateViewport(1280, 900),
		chromedp.Click(`.comp-tt-daypick [data-day="day-1"]`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('[data-name="Eve"]').closest('.comp-flight').querySelector('details.comp-opens').open = true`, nil),
		chromedp.Sleep(100*time.Millisecond),
		chromedp.Evaluate(read, &jumped),
	)
	if !jumped.SundayOpen || !jumped.Dara || !jumped.Eve || !strings.Contains(jumped.Order, "1. Eve") {
		t.Fatalf("on a wide screen Sunday's button shows it beside Saturday; Move a gymnast lists the running order: %+v", jumped)
	}
	// Too wide to fit: › moves the days along, an area at a time, and the
	// row fades at the edge with more.
	type along struct {
		Hidden       bool
		Moved        int
		Class        string
		LeftDisabled bool
	}
	var moved along
	step(t, ctx, "moving along with ›",
		chromedp.EmulateViewport(800, 900),
		// Two small days fit at 800 px: the row is made narrower than them.
		chromedp.Evaluate(`{const row = document.querySelector('.comp-tt-dayrow'); row.style.width = '300px'; row.scrollLeft = 0; window.dispatchEvent(new Event('resize'))}`, nil),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Click(`.comp-tt-scroll [data-scroll="1"]`, chromedp.ByQuery),
		chromedp.Sleep(800*time.Millisecond),
		chromedp.Evaluate(`({
			Hidden: document.querySelector('.comp-tt-scroll').hidden,
			Moved: Math.round(document.querySelector('.comp-tt-dayrow').scrollLeft),
			Class: document.querySelector('.comp-tt-dayrow').className,
			LeftDisabled: document.querySelector('.comp-tt-scroll [data-scroll="-1"]').disabled,
		})`, &moved),
	)
	if moved.Hidden || moved.Moved < 200 || !strings.Contains(moved.Class, "has-more-left") || moved.LeftDisabled {
		t.Errorf("› moves the days along an area, the row fading at the edge behind: %+v", moved)
	}
	var phone view
	var saturdayOpen bool
	step(t, ctx, "a day's tab on a phone",
		chromedp.EmulateViewport(390, 844),
		chromedp.Evaluate(`localStorage.clear()`, nil),
		chromedp.Navigate(tt),
		chromedp.WaitVisible(`.comp-tt-daypick`, chromedp.ByQuery),
		chromedp.Click(`.comp-tt-daypick [data-day="day-1"]`, chromedp.ByQuery),
		chromedp.Evaluate(read, &phone),
		chromedp.Evaluate(`document.getElementById('day-0').parentElement.open`, &saturdayOpen),
	)
	if !phone.SundayOpen || saturdayOpen {
		t.Errorf("on a phone Sunday's tab shows Sunday alone: %+v, Saturday open %v", phone, saturdayOpen)
	}
}

// The entries page's folded levels have their rows in a template, in the
// page when the level opens or a search needs them.
func TestDashboardLevelsLazyInABrowser(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore))
	admin := twoDays(t, s.h)
	if page := do(t, s.h, http.MethodGet, admin, nil).Body.String(); strings.Count(page, "<template data-lazy>") != 2 {
		t.Errorf("both levels folded, their rows in templates: %s", page)
	}
	if page := do(t, s.h, http.MethodGet, admin+"?club=individual", nil).Body.String(); strings.Contains(page, "<template data-lazy>") {
		t.Error("filtered, the levels with rows are open and in the page")
	}
	ctx := browser(t)
	srv := httptest.NewServer(s.h)
	defer srv.Close()
	rows := func() string { return `document.querySelectorAll('tr[data-list-row]').length` }
	var before, opened, searched int
	step(t, ctx, "opening a level, then searching",
		chromedp.Navigate(srv.URL+admin),
		chromedp.WaitVisible(`#entries`, chromedp.ByQuery),
		chromedp.Evaluate(rows(), &before),
		chromedp.Evaluate(`document.querySelector('details.comp-dash-level').open = true`, nil),
		chromedp.Sleep(100*time.Millisecond),
		chromedp.Evaluate(rows(), &opened),
		chromedp.Evaluate(`{const s = document.querySelector('[data-list-search]'); s.value = 'dara'; s.dispatchEvent(new Event('input', {bubbles: true}))}`, nil),
		chromedp.Evaluate(rows(), &searched),
	)
	if before != 0 || opened != 1 || searched != 2 {
		t.Errorf("rows in the page: %d folded, %d with one level open, %d searched", before, opened, searched)
	}
}

// More cards than fit on a phone's page, over several levels, list the
// levels to print one at a time (a level with too many, by category), with
// every level on one page still there.
func TestCardsByLevel(t *testing.T) {
	defer func(n int) { cardsAtOnce = n }(cardsAtOnce)
	cardsAtOnce = 3
	h := competitionServer(t)
	form := newCompetition()
	form["level"] = append(form["level"], "builtin-level:bucs-l4")
	form.Set("split", "all")
	admin := created(t, h, form)
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for name, category := range map[string]string{"Ann": "Women", "Bea": "Men"} {
		v := bucsEntry(name)
		v.Set("category", category)
		redirected(t, h, enter, v)
	}
	redirected(t, h, enter, url.Values{"gymnast": {"Cal"}, "category": {"Men"}, "level": {"BUCS L4"}, "ex1Option": {"builtin:bucs-l4-option-1"}, "ex2Option": {"builtin:bucs-l4-second"}, "ex2Skills": {voluntary}})

	page := do(t, h, http.MethodGet, admin+"/cards", nil).Body.String()
	for _, want := range []string{`<a href="?level=BUCS+L3&amp;part=Men">BUCS L3 · Men</a> · 2 cards`, `<a href="?level=BUCS+L3&amp;part=Women">BUCS L3 · Women</a> · 2 cards`,
		`<a href="?level=BUCS+L4">BUCS L4</a> · 2 cards`, `<a href="?all=1">Every level on one page</a> · 6 cards`} {
		if !strings.Contains(page, want) {
			t.Errorf("the levels to print have %q: %s", want, page)
		}
	}
	if strings.Contains(page, `class="card-page"`) {
		t.Error("the list of levels prints no cards")
	}
	men := do(t, h, http.MethodGet, admin+"/cards?level=BUCS+L3&part=Men", nil).Body.String()
	if strings.Count(men, `class="card-page"`) != 2 || !strings.Contains(men, "<strong>Bea</strong>") || !strings.Contains(men, `<a href="?">Other levels</a>`) ||
		!strings.Contains(men, `<a href="?level=BUCS+L3&amp;part=Women">Next: BUCS L3 · Women →</a>`) {
		t.Errorf("a part's cards, and the way on: %s", men)
	}
	if last := do(t, h, http.MethodGet, admin+"/cards?level=BUCS+L3&part=Women", nil).Body.String(); strings.Count(last, `class="card-page"`) != 2 || strings.Contains(last, "Next:") {
		t.Error("the last level's cards, with no next")
	}
	if all := do(t, h, http.MethodGet, admin+"/cards?all=1", nil).Body.String(); strings.Count(all, `class="card-page"`) != 6 {
		t.Error("every level on one page")
	}
	// A filter that leaves few prints them at once.
	if few := do(t, h, http.MethodGet, admin+"/cards?q=cal", nil).Body.String(); strings.Count(few, `class="card-page"`) != 2 || strings.Contains(few, "Other levels") {
		t.Errorf("a search's cards, at once: %s", few)
	}
}

// My competition shows the person's own flights, with the whole timetable a
// link away.
func TestMyCompetitionOwnFlights(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore))
	admin := twoDays(t, s.h)
	redirected(t, s.h, admin+"/timetable/publish", url.Values{"action": {"publish"}})
	list := do(t, s.h, http.MethodGet, admin+"/people", nil).Body.String()
	eve := personLink(t, list, admin, "Eve")
	page := do(t, s.h, http.MethodGet, eve, nil).Body.String()
	if !strings.Contains(page, "Their flights and panels") || !strings.Contains(page, "<strong>Eve</strong>") || strings.Contains(page, "Dara") {
		t.Errorf("Eve's flight, not Dara's: %s", page)
	}
	if whole := do(t, s.h, http.MethodGet, eve+"?timetable=all", nil).Body.String(); !strings.Contains(whole, "Dara") {
		t.Error("the whole timetable has Dara's flight too")
	}
}
