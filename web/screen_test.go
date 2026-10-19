package web

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"tariffCalculator/competitions"
)

// at is a moment on a date in March 2030 (the 16th is the first day), local.
func at(day, hour, minute int) time.Time {
	return time.Date(2030, time.March, day, hour, minute, 0, 0, local)
}

// flightAt is a one-flight event on an area, from start to end in minutes.
func flightAt(area, level string, startH, endH int) competitions.ScheduledFlight {
	return competitions.ScheduledFlight{
		Flight: competitions.Flight{Event: level, Number: 1, Of: 1}, Area: area,
		Start: startH * 60, End: endH * 60,
	}
}

func TestNowOn(t *testing.T) {
	a, b, c := flightAt("Floor 1", "A", 9, 10), flightAt("Floor 1", "B", 10, 11), flightAt("Floor 1", "C", 11, 12)
	s := competitions.Schedule{
		Flights: []competitions.ScheduledFlight{c, a, b, flightAt("Floor 2", "Z", 9, 12)}, // out of order
		Blocks: []competitions.ScheduledBlock{
			{Name: "Lunch", Areas: []string{"Floor 1"}, Start: 12 * 60, End: 13 * 60},
			{Name: "Awards", Start: 13 * 60, End: 14 * 60}, // all areas
			{Name: "Next day's lunch", Day: 1, Start: 12 * 60, End: 13 * 60},
		},
	}
	first := at(16, 0, 0)
	key := competitions.FlightKey
	name := func(f *competitions.ScheduledFlight) string {
		if f == nil {
			return "-"
		}
		return f.Event
	}
	for _, tc := range []struct {
		what    string
		now     time.Time
		actual  map[string]competitions.Actual
		area    string
		on      string
		planned bool
		brk     string
		next    string
	}{
		{what: "before the day starts", now: at(16, 8, 0), area: "Floor 1", on: "-", next: "A"},
		{what: "planned, nothing marked", now: at(16, 9, 30), area: "Floor 1", on: "A", planned: true, next: "B"},
		{what: "the planned window ends at its end", now: at(16, 10, 0), area: "Floor 1", on: "B", planned: true, next: "C"},
		{what: "another area has its own flights", now: at(16, 9, 30), area: "Floor 2", on: "Z", planned: true, next: "-"},
		{what: "started on time", now: at(16, 9, 30), area: "Floor 1", on: "A", next: "B",
			actual: map[string]competitions.Actual{key(a): {Started: at(16, 9, 2)}}},
		{what: "running late: A is still on, though B's time has come", now: at(16, 10, 10), area: "Floor 1", on: "A", next: "B",
			actual: map[string]competitions.Actual{key(a): {Started: at(16, 9, 20)}}},
		{what: "finished early: nothing on until B's time", now: at(16, 9, 50), area: "Floor 1", on: "-", next: "B",
			actual: map[string]competitions.Actual{key(a): {Started: at(16, 9, 0), Finished: at(16, 9, 45)}}},
		{what: "finished, and nothing else marked, in B's time", now: at(16, 10, 10), area: "Floor 1", on: "B", planned: true, next: "C",
			actual: map[string]competitions.Actual{key(a): {Started: at(16, 9, 0), Finished: at(16, 9, 58)}}},
		{what: "the latest started wins", now: at(16, 10, 30), area: "Floor 1", on: "B", next: "C",
			actual: map[string]competitions.Actual{key(a): {Started: at(16, 9, 0)}, key(b): {Started: at(16, 10, 5)}}},
		{what: "a gap before the next flight", now: at(16, 11, 0), area: "Floor 1", on: "C", planned: true, next: "-",
			actual: map[string]competitions.Actual{key(a): {Finished: at(16, 9, 55), Started: at(16, 9, 0)}, key(b): {Started: at(16, 10, 0), Finished: at(16, 10, 59)}}},
		{what: "a break after the last flight", now: at(16, 12, 30), area: "Floor 1", on: "-", brk: "Lunch until 13:00", next: "-",
			actual: map[string]competitions.Actual{
				key(a): {Started: at(16, 9, 0), Finished: at(16, 10, 0)}, key(b): {Started: at(16, 10, 0), Finished: at(16, 11, 0)}, key(c): {Started: at(16, 11, 0), Finished: at(16, 12, 0)}}},
		{what: "a lunch on another area isn't this area's", now: at(16, 12, 30), area: "Floor 2", on: "-", next: "-"},
		{what: "a break for every area", now: at(16, 13, 30), area: "Floor 2", on: "-", brk: "Awards until 14:00", next: "-"},
		{what: "not the day: nothing planned, the first flight is next", now: at(17, 9, 30), area: "Floor 1", on: "-", next: "A"},
		{what: "not the day: no break", now: at(17, 12, 30), area: "Floor 1", on: "-", next: "A"},
		{what: "running late: B's time has passed but it hasn't started, so it's still to come", now: at(16, 11, 30), area: "Floor 1", on: "B", planned: true, next: "C",
			actual: map[string]competitions.Actual{key(a): {Started: at(16, 9, 0), Finished: at(16, 10, 40)}}},
		{what: "running late past the end of the plan: never blank while flights are left", now: at(16, 15, 0), area: "Floor 1", on: "C", planned: true, next: "-",
			actual: map[string]competitions.Actual{key(a): {Started: at(16, 9, 0), Finished: at(16, 10, 0)}, key(b): {Started: at(16, 10, 0), Finished: at(16, 14, 50)}}},
		{what: "marked on another date counts, as on the day's pages", now: at(16, 9, 30), area: "Floor 1", on: "A", next: "B",
			actual: map[string]competitions.Actual{key(a): {Started: at(15, 9, 0)}}},
		{what: "finished on another date: the next flight is next, not the finished one", now: at(14, 9, 30), area: "Floor 1", on: "-", next: "B",
			actual: map[string]competitions.Actual{key(a): {Started: at(14, 9, 0), Finished: at(14, 9, 20)}}},
		{what: "started on another date: it's on now", now: at(14, 9, 30), area: "Floor 1", on: "B", next: "C",
			actual: map[string]competitions.Actual{key(a): {Started: at(14, 9, 0), Finished: at(14, 9, 20)}, key(b): {Started: at(14, 9, 25)}}},
	} {
		got := nowOn(s, 0, tc.area, first, tc.actual, tc.now)
		if name(got.Flight) != tc.on || got.Planned != tc.planned || got.Break != tc.brk || name(got.Next) != tc.next {
			t.Errorf("%s: on %s (planned %v), break %q, next %s; want on %s (planned %v), break %q, next %s",
				tc.what, name(got.Flight), got.Planned, got.Break, name(got.Next), tc.on, tc.planned, tc.brk, tc.next)
		}
	}
}

// screenEmpty is what the venue screen says when it has no areas to show,
// "" when it has some.
func screenEmpty(t *testing.T, body string) string {
	t.Helper()
	return textOf(firstByClass(parsePage(t, body), "screen-empty"))
}

// The venue screen through a competition day, on a fixed clock: before it,
// the morning's planned flights, flights started on time and late, finished
// early, finished for the day; areas chosen with ?area=; and who may open it.
func TestVenueScreen(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	screen := madeLink(t, h, admin, "Main hall screen", "screen")
	show := func(query string) string {
		t.Helper()
		page := do(t, h, http.MethodGet, screen+"/screen"+query, nil)
		if page.Code != http.StatusOK {
			t.Fatalf("the screen%s: %d", query, page.Code)
		}
		return page.Body.String()
	}
	cards := func() map[string]screenCard {
		t.Helper()
		return screenCards(t, show("?day=0"))
	}
	if got := screenEmpty(t, show("")); got != "The timetable isn't published yet" {
		t.Errorf("nothing to show before publishing: %q", got)
	}
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Tara", "Tom"} {
		redirected(t, h, enter, url.Values{"gymnast": {name}, "discipline": {"tumbling"}, "level": {"Novice"}})
	}
	// BUCS L3 on Panel 1 at 09:00–09:25; Tumbling Novice on Track 1,
	// 09:00–09:14.
	publishedWith(t, h, admin, "Ann", "Bea", "Cal")
	day := admin + "/day?day=0"
	if page := dayAll(t, h, day); !strings.Contains(page, `href="`+admin+`/screen?day=0"`) || !strings.Contains(page, "Venue screen") {
		t.Error("the organiser's On the day page links to the venue screen")
	}
	page := dayAll(t, h, day)
	areas := dayPageAreas(t, page)
	if len(areas["Panel 1"].Flights) != 1 || len(areas["Track 1"].Flights) != 1 {
		t.Fatalf("a flight on each area: %+v", areas)
	}
	panel, track := areas["Panel 1"].Flights[0].Key, areas["Track 1"].Flights[0].Key
	markAt := func(flight, what string, hour, minute int) {
		t.Helper()
		clock.set(onTheDay(hour, minute))
		redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {what}, "day": {"0"}})
	}

	// Before the day: nothing on, each area's flight next.
	got := cards()
	for area, next := range map[string]string{"Panel 1": "BUCS L3", "Track 1": "Tumbling Novice"} {
		if c := got[area]; c.Now != "—" || c.Next != next || c.WarmUp != "09:00" || c.Late != "" || c.Finished {
			t.Errorf("before the day, %s: %+v", area, c)
		}
	}
	// A start tried out on the day page shows, as it does there, until it's
	// taken back.
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {panel}, "what": {"start"}, "day": {"0"}})
	if c := cards()["Panel 1"]; c.Now != "BUCS L3" || c.Planned {
		t.Errorf("before the day, started on the day page: %+v", c)
	}
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {panel}, "what": {"undo-start"}, "day": {"0"}})

	// The morning, nothing marked: on by the plan.
	clock.set(onTheDay(9, 5))
	got = cards()
	if c := got["Panel 1"]; c.Now != "BUCS L3" || !c.Planned || c.NowTimes != "09:00–09:25" || !slices.Equal(c.Gymnasts, []string{"Cal", "Bea", "Ann"}) || c.Next != "—" || c.Late != "" {
		t.Errorf("Panel 1 at 09:05, by the plan: %+v", c)
	}
	if c := got["Track 1"]; c.Now != "Tumbling Novice" || !c.Planned || !slices.Equal(c.Gymnasts, []string{"Tara", "Tom"}) {
		t.Errorf("Track 1 at 09:05, by the plan: %+v", c)
	}

	// Track 1 starts on time; Panel 1 ten minutes late.
	markAt(track, "start", 9, 0)
	markAt(panel, "start", 9, 10)
	got = cards()
	if c := got["Track 1"]; c.Now != "Tumbling Novice" || c.Planned || c.Late != "On time" || c.Behind {
		t.Errorf("Track 1 started on time: %+v", c)
	}
	if c := got["Panel 1"]; c.Now != "BUCS L3" || c.Planned || c.Late != "Running 10 min late" || !c.Behind {
		t.Errorf("Panel 1 started late: %+v", c)
	}

	// Track 1 finishes: a strip, while Panel 1 keeps its card.
	markAt(track, "finish", 9, 14)
	clock.set(onTheDay(9, 15))
	got = cards()
	if c := got["Track 1"]; c.Strip != "Finished for the day" {
		t.Errorf("Track 1 is a strip once it's done: %+v", c)
	}
	if c := got["Panel 1"]; c.Now != "BUCS L3" || c.Strip != "" {
		t.Errorf("Panel 1 keeps its card: %+v", c)
	}

	// Only the areas asked for.
	if got := screenCards(t, show("?day=0&area=Panel+1")); len(got) != 1 || got["Panel 1"].Now != "BUCS L3" {
		t.Errorf("?area=Panel 1 shows only Panel 1: %+v", got)
	}
	if got := screenCards(t, show("?day=0&area=Panel+1&area=Track+1")); len(got) != 2 {
		t.Errorf("both areas asked for: %+v", got)
	}
	if body := show("?day=0&area=Nowhere"); screenEmpty(t, body) != "No flights on this day" {
		t.Errorf("an area that isn't one: %q", screenEmpty(t, body))
	}

	// Scratched gymnasts are struck through.
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {checkinEntry(t, page, "Bea")}, "status": {"scratched"}, "day": {"0"}})
	if c := cards()["Panel 1"]; !slices.Equal(c.Scratched, []string{"Bea"}) || len(c.Gymnasts) != 3 {
		t.Errorf("Bea is struck through, and still listed: %+v", c)
	}

	// Panel 1 finishes early, at 09:20 for 09:25; the evening, nothing left.
	markAt(panel, "finish", 9, 20)
	if c := cards()["Panel 1"]; c.Late != "5 min early" || c.Behind || c.Now != "—" || !c.Finished {
		t.Errorf("Panel 1 finished early: %+v", c)
	}
	clock.set(onTheDay(19, 0))
	for area, c := range cards() {
		if !c.Finished || c.Now != "—" {
			t.Errorf("the evening, %s: %+v", area, c)
		}
	}

	// A screen link shows the screen, and nothing else.
	body := show("?day=0")
	for _, want := range []string{`<meta http-equiv="refresh" content="30">`, `<meta name="robots" content="noindex">`} {
		if !strings.Contains(body, want) {
			t.Errorf("the screen has %q", want)
		}
	}
	if strings.Contains(body, "All entries") || strings.Contains(body, "<nav") {
		t.Error("no navigation on the screen")
	}
	// Any other page it opens (the link as given, say) goes to the screen.
	for _, path := range []string{"", "/day", "/day?day=0", "/cards", "/history", "/entries.csv", "/timetable"} {
		if rec := do(t, h, http.MethodGet, screen+path, nil); rec.Code != http.StatusFound || rec.Header().Get("Location") != screen+"/screen" {
			t.Errorf("a screen link opening %q goes to the screen: %d %s", path, rec.Code, rec.Header().Get("Location"))
		}
	}
	for _, path := range []string{"/concerns", "/day/flight", "/day/checkin", "/links"} {
		if rec := do(t, h, http.MethodPost, screen+path, url.Values{"text": {"x"}}); rec.Code != http.StatusForbidden {
			t.Errorf("a screen link can't POST %q: %d", path, rec.Code)
		}
	}
	// A TV left on a day that isn't one still shows something: the default.
	if got := screenCards(t, show("?day=99")); len(got) != 2 {
		t.Errorf("a day that isn't one shows the default: %+v", got)
	}

	// Others can open it too, except the cards link.
	for _, kind := range []string{"chair", "timetable", "everything"} {
		link := madeLink(t, h, admin, "Helper "+kind, kind)
		if rec := do(t, h, http.MethodGet, link+"/screen", nil); rec.Code != http.StatusOK {
			t.Errorf("a %s link opens the screen: %d", kind, rec.Code)
		}
		if page := dayAll(t, h, link+"/day"); !strings.Contains(page, link+"/screen") {
			t.Errorf("a %s link's On the day page offers the screen", kind)
		}
	}
	cardsLink := madeLink(t, h, admin, "Cards", "cards")
	if rec := do(t, h, http.MethodGet, cardsLink+"/screen", nil); rec.Code != http.StatusForbidden {
		t.Errorf("a cards link can't open the screen: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/competitions/admin/nonsense/screen", nil); rec.Code != http.StatusNotFound {
		t.Errorf("no such link: %d", rec.Code)
	}
	if page := do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(); !strings.Contains(page, `value="screen"`) {
		t.Error("the organiser can choose the kind of link")
	}
}
