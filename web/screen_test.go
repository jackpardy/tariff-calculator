package web

import (
	"net/http"
	"net/url"
	"regexp"
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
		Flight: competitions.Flight{Level: level, Number: 1, Of: 1}, Area: area,
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
		return f.Level
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
		{what: "a break after the last flight", now: at(16, 12, 30), area: "Floor 1", on: "-", brk: "Lunch", next: "-",
			actual: map[string]competitions.Actual{
				key(a): {Started: at(16, 9, 0), Finished: at(16, 10, 0)}, key(b): {Started: at(16, 10, 0), Finished: at(16, 11, 0)}, key(c): {Started: at(16, 11, 0), Finished: at(16, 12, 0)}}},
		{what: "a lunch on another area isn't this area's", now: at(16, 12, 30), area: "Floor 2", on: "-", next: "-"},
		{what: "a break for every area", now: at(16, 13, 30), area: "Floor 2", on: "-", brk: "Awards", next: "-"},
		{what: "not the day: nothing planned, the first flight is next", now: at(17, 9, 30), area: "Floor 1", on: "-", next: "A"},
		{what: "not the day: no break", now: at(17, 12, 30), area: "Floor 1", on: "-", next: "A"},
		{what: "marked on another date doesn't count", now: at(16, 9, 30), area: "Floor 1", on: "A", planned: true, next: "B",
			actual: map[string]competitions.Actual{key(a): {Started: at(15, 9, 0)}}},
	} {
		got := nowOn(s, 0, tc.area, first, tc.actual, tc.now)
		if name(got.Flight) != tc.on || got.Planned != tc.planned || got.Break != tc.brk || name(got.Next) != tc.next {
			t.Errorf("%s: on %s (planned %v), break %q, next %s; want on %s (planned %v), break %q, next %s",
				tc.what, name(got.Flight), got.Planned, got.Break, name(got.Next), tc.on, tc.planned, tc.brk, tc.next)
		}
	}
}

func TestVenueScreen(t *testing.T) {
	h := competitionServer(t)
	// Today's competition, so flights marked now count for its first day.
	form := newCompetition()
	today := time.Now().In(local)
	if today.Hour() == 23 && today.Minute() >= 50 {
		t.Skip("too close to midnight for a competition today")
	}
	form.Set("date", today.Format("2006-01-02"))
	form.Set("deadlineDate", today.Format("2006-01-02"))
	form.Set("deadlineTime", "23:59")
	admin := created(t, h, form)
	screen := madeLink(t, h, admin, "Main hall screen", "screen")
	if rec := do(t, h, http.MethodGet, screen+"/screen", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "The timetable isn&#39;t published yet") && !strings.Contains(rec.Body.String(), "The timetable isn't published yet") {
		t.Errorf("nothing to show before publishing: %d %s", rec.Code, rec.Body.String())
	}
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Ann", "Bea", "Cal"} {
		redirected(t, h, enter, url.Values{"gymnast": {name}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	}
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})

	day := do(t, h, http.MethodGet, admin+"/day?day=0", nil).Body.String()
	areas := regexp.MustCompile(`<h2 class="title is-5 mb-1">([^<]+)</h2>`).FindAllStringSubmatch(day, -1)
	if len(areas) == 0 {
		t.Fatalf("no areas on the day page: %s", day)
	}
	if !strings.Contains(day, `href="`+admin+`/screen?day=0"`) || !strings.Contains(day, "Venue screen") {
		t.Error("the organiser's On the day page links to the venue screen")
	}

	// A screen link shows the screen, and nothing else.
	page := do(t, h, http.MethodGet, screen+"/screen?day=0", nil)
	if page.Code != http.StatusOK {
		t.Fatalf("the screen: %d", page.Code)
	}
	body := page.Body.String()
	for _, a := range areas {
		if !strings.Contains(body, ">"+a[1]+"<") {
			t.Errorf("the screen has a card for %s", a[1])
		}
	}
	for _, want := range []string{">Next<", ">Now<", `<meta http-equiv="refresh" content="30">`, `<meta name="robots" content="noindex">`} {
		if !strings.Contains(body, want) {
			t.Errorf("the screen has %q", want)
		}
	}
	if strings.Contains(body, "All entries") || strings.Contains(body, "<nav") {
		t.Error("no navigation on the screen")
	}
	for _, path := range []string{"", "/day", "/day?day=0", "/cards", "/history", "/entries.csv", "/timetable"} {
		if rec := do(t, h, http.MethodGet, screen+path, nil); rec.Code != http.StatusForbidden {
			t.Errorf("a screen link can't GET %q: %d", path, rec.Code)
		}
	}
	for _, path := range []string{"/concerns", "/day/flight", "/day/checkin", "/links"} {
		if rec := do(t, h, http.MethodPost, screen+path, url.Values{"text": {"x"}}); rec.Code != http.StatusForbidden {
			t.Errorf("a screen link can't POST %q: %d", path, rec.Code)
		}
	}
	if rec := do(t, h, http.MethodGet, screen+"/screen?day=99", nil); rec.Code != http.StatusOK {
		t.Errorf("a day that isn't one shows the default: %d", rec.Code)
	}

	// Others can open it too, except the cards link.
	for _, kind := range []string{"chair", "timetable", "everything"} {
		link := madeLink(t, h, admin, "Helper "+kind, kind)
		if rec := do(t, h, http.MethodGet, link+"/screen", nil); rec.Code != http.StatusOK {
			t.Errorf("a %s link opens the screen: %d", kind, rec.Code)
		}
		if page := do(t, h, http.MethodGet, link+"/day", nil).Body.String(); !strings.Contains(page, link+"/screen") {
			t.Errorf("a %s link's On the day page offers the screen", kind)
		}
	}
	cards := madeLink(t, h, admin, "Cards", "cards")
	if rec := do(t, h, http.MethodGet, cards+"/screen", nil); rec.Code != http.StatusForbidden {
		t.Errorf("a cards link can't open the screen: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/competitions/admin/nonsense/screen", nil); rec.Code != http.StatusNotFound {
		t.Errorf("no such link: %d", rec.Code)
	}
	if page := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(page, "Venue screen") {
		t.Error("the organiser can choose the kind of link")
	}

	// Marking the first flight started puts it under Now, with its gymnasts.
	key := regexp.MustCompile(`name="flight" value="([^"]+)"`).FindStringSubmatch(day)
	entry := regexp.MustCompile(`name="entry" value="([^"]+)"`).FindStringSubmatch(day)
	if key == nil || entry == nil {
		t.Fatal("no flight to mark")
	}
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {html2text(key[1])}, "what": {"start"}, "day": {"0"}})
	body = do(t, h, http.MethodGet, screen+"/screen?day=0", nil).Body.String()
	if !regexp.MustCompile(`Now</p>\s*<p class="screen-now">\s*BUCS L3`).MatchString(body) || strings.Contains(body, "(planned)") {
		t.Errorf("the started flight is on now: %s", body)
	}
	for _, name := range []string{"Ann", "Bea", "Cal"} {
		if !strings.Contains(body, ">"+name+"<") {
			t.Errorf("the flight's gymnasts are named: %s", name)
		}
	}
	if strings.Contains(body, "comp-scratched") {
		t.Error("nobody is scratched yet")
	}
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {html2text(entry[1])}, "status": {"scratched"}, "day": {"0"}})
	if body = do(t, h, http.MethodGet, screen+"/screen?day=0", nil).Body.String(); !strings.Contains(body, `class="comp-scratched"`) {
		t.Errorf("a scratched gymnast is struck through: %s", body)
	}
}
