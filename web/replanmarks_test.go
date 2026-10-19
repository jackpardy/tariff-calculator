package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

// Nothing marked on the day is lost without a word. Planning again on the
// day asks first; a fresh plan keeps the flight that ran, with its times,
// who's up and its panel's check-ins, while it keeps its gymnasts, and the
// gymnasts keep theirs: On the day, the area's page and the venue screen
// show it all after publishing again. When a fresh plan does change the
// flight, Publish changes says what would be dropped before it is.
func TestReplanKeepsDayMarks(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore))
	h := s.h
	get := func(path string) string { return dayAll(t, h, path) }
	admin := created(t, h, newCompetition())
	officials := admin + "/officials"
	redirected(t, h, officials+"/settings", url.Values{
		"panel-trampoline-chair": {"1"}, "panel-trampoline-execution": {"1"}, "panel-trampoline-difficulty": {"0"}, "panel-trampoline-recorder": {"0"}, "panel-trampoline-marshal": {"0"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Mary"}, "judge-trampoline": {"1"}, "chair-trampoline": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Tom"}, "judge-trampoline": {"1"}})
	enter := pathIn(t, get(admin+"/settings"), "/competitions/enter/")
	for _, name := range []string{"Ann", "Bea", "Cal"} {
		redirected(t, h, enter, bucsEntry(name))
	}
	tt := admin + "/timetable"
	redirected(t, h, tt+"/plan", url.Values{})
	version := func() string {
		t.Helper()
		return regexp.MustCompile(`name="version" value="([0-9a-f]+)"`).FindStringSubmatch(get(tt))[1]
	}
	publish := func() *httptest.ResponseRecorder {
		t.Helper()
		return do(t, h, http.MethodPost, tt+"/publish", url.Values{"action": {"publish"}, "version": {version()}})
	}
	if r := publish(); r.Code != http.StatusSeeOther {
		t.Fatalf("published before the day: %d %s", r.Code, r.Body)
	}

	// On the day: Ann here and Cal scratched, Mary here on the panel, the
	// flight started, tapped on to Bea and finished.
	s.clock.set(onTheDay(9, 0))
	day := admin + "/day?day=0"
	before := dayPageAreas(t, get(day))["Panel 1"]
	if len(before.Flights) != 1 {
		t.Fatalf("one flight on Panel 1: %+v", before)
	}
	flight := before.Flights[0].Key
	ann, cal := checkinEntryOnTimetable(t, get(tt), "Ann"), checkinEntryOnTimetable(t, get(tt), "Cal")
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {ann}, "status": {"here"}, "day": {"0"}})
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {cal}, "status": {"scratched"}, "day": {"0"}})
	mary, key := panelSeat(t, get(day), "BUCS L3", "Mary")
	redirected(t, h, admin+"/day/officials", url.Values{"person": {mary}, "flight": {key}, "status": {"here"}, "day": {"0"}})
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {"start"}, "day": {"0"}})
	tapUp(t, h, admin, flight, "next")
	tapUp(t, h, admin, flight, "next")
	screen := madeLink(t, h, admin, "Hall", "screen")
	upNow := func() (dayFlight, screenCard) {
		t.Helper()
		return dayPageAreas(t, get(day))["Panel 1"].Flights[0], screenCards(t, get(screen+"/screen?day=0"))["Panel 1"]
	}
	wantUp, wantCard := upNow()
	if wantUp.Up == "" || len(wantCard.Up) == 0 || wantCard.Up[0] != wantUp.Up+" · "+wantUp.Routine {
		t.Fatalf("who's up: %+v, on the screen %+v", wantUp, wantCard)
	}
	s.clock.set(onTheDay(9, 40))
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {"finish"}, "day": {"0"}})
	want := dayPageAreas(t, get(day))["Panel 1"].Flights[0]
	if want.Started != "09:00" || want.Finished != "09:40" || want.Gymnasts["Ann"] != "here" || want.Gymnasts["Cal"] != "scratched" || !strings.Contains(want.Panel, "1 of 2 here") {
		t.Fatalf("marked on the day: %+v", want)
	}
	wantScreen := screenCards(t, get(screen+"/screen?day=0"))["Panel 1"]
	if wantScreen.Late == "" {
		t.Fatalf("the screen says how Panel 1 is running: %+v", wantScreen)
	}
	check := func(when string) {
		t.Helper()
		for _, page := range []string{day, day + "&area=" + url.QueryEscape("Panel 1")} {
			got := dayPageAreas(t, get(page))["Panel 1"]
			if len(got.Flights) != 1 {
				t.Fatalf("%s: one flight on %s: %+v", when, page, got)
			}
			f := got.Flights[0]
			if f.Key != flight || f.Started != want.Started || f.Finished != want.Finished ||
				f.Gymnasts["Ann"] != "here" || f.Gymnasts["Cal"] != "scratched" || f.Panel != want.Panel {
				t.Errorf("%s: %s keeps what was marked: %+v, was %+v", when, page, f, want)
			}
		}
		if card := screenCards(t, get(screen+"/screen?day=0"))["Panel 1"]; card.Late != wantScreen.Late {
			t.Errorf("%s: the screen measures Panel 1 from the flight: %+v, was %+v", when, card, wantScreen)
		}
	}
	check("before")

	// Plan again asks first now the day has begun; nothing changes, and
	// nothing goes in the history, until the organiser says so.
	asked := do(t, h, http.MethodPost, tt+"/plan", url.Values{})
	page := asked.Body.String()
	if asked.Code != http.StatusOK || !strings.Contains(page, "The competition has begun") {
		t.Fatalf("Plan again asks first: %d %s", asked.Code, page)
	}
	for _, line := range []string{"1 flight has started or finished.", "1 official is checked in or marked missing on a panel.", "2 gymnasts are checked in or scratched.", "Plan again anyway"} {
		if !strings.Contains(page, line) {
			t.Errorf("the question says %q: %s", line, page)
		}
	}
	if strings.Count(get(admin+"/history"), "Planned the timetable") != 1 {
		t.Error("asking isn't recorded as planning")
	}
	s.clock.set(onTheDay(10, 0)) // another seed: a plan of its own
	redirected(t, h, tt+"/plan", url.Values{"anyway": {"1"}})
	if r := publish(); r.Code != http.StatusSeeOther {
		t.Fatalf("nothing to drop, so it publishes straight away: %d %s", r.Code, r.Body)
	}
	check("planned again")
	// Who's up is kept too: taking back Finished shows where the marshal was.
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {"undo-finish"}, "day": {"0"}})
	if got, card := upNow(); got.Up != wantUp.Up || got.Routine != wantUp.Routine || len(card.Up) == 0 || card.Up[0] != wantCard.Up[0] {
		t.Errorf("planned again: who's up %+v, on the screen %+v; was %+v, %+v", got, card, wantUp, wantCard)
	}

	// Flights are made smaller and the timetable planned again: BUCS L3 is
	// now two flights, neither the one that ran. Publishing says first that
	// its times and who's up go, and so does Mary's check-in on its panel;
	// Ann's and Cal's stay with them.
	redirected(t, h, tt+"/setup/timings", url.Values{"per-trampoline": {"5"}, "between-trampoline": {"10"}, "max-trampoline": {"2"},
		"per-tumbling": {"2"}, "between-tumbling": {"10"}, "max-tumbling": {"15"}, "rest": {"20"}, "restMust": {"0"}, "separate": {"all"}})
	redirected(t, h, tt+"/plan", url.Values{"anyway": {"1"}})
	asked = publish()
	page = asked.Body.String()
	if asked.Code != http.StatusOK || !strings.Contains(page, "Publishing drops what was marked on the day") {
		t.Fatalf("Publish changes asks first: %d %s", asked.Code, page)
	}
	for _, line := range []string{
		"1 flight that has started or finished isn&#39;t in the plan any more: its times and who&#39;s up are dropped (BUCS L3).",
		"1 panel check-in is for seats no longer in the plan.", "Publish anyway",
	} {
		if !strings.Contains(page, line) {
			t.Errorf("the question says %q: %s", line, page)
		}
	}
	if strings.Contains(page, "gymnast checked in") {
		t.Errorf("gymnasts' check-ins aren't dropped: %s", page)
	}
	if got := dayPageAreas(t, get(day))["Panel 1"].Flights; len(got) != 1 || got[0].Key != flight || got[0].Started != "09:00" {
		t.Errorf("nothing published until the organiser says: %+v", got)
	}
	if strings.Count(get(admin+"/history"), "Published the timetable") != 2 {
		t.Error("asking isn't recorded as publishing")
	}
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}, "version": {version()}, "anyway": {"1"}})
	gymnasts := map[string]string{}
	flights := dayFlights(t, get(day))
	for _, f := range flights {
		if f.Key == flight || f.Started != "" || f.Finished != "" {
			t.Errorf("published anyway: new flights, not started: %+v", f)
		}
		for name, mark := range f.Gymnasts {
			gymnasts[name] = mark
		}
	}
	if len(flights) != 2 || gymnasts["Ann"] != "here" || gymnasts["Bea"] != "" || gymnasts["Cal"] != "scratched" {
		t.Errorf("published anyway: two flights, the gymnasts' check-ins kept: %+v", flights)
	}
}

// What publishing would drop: a flight under way that's gone, a panel
// check-in whose seat is gone (the flight's, or the person's on it), and a
// gymnast checked in who's gone. A seat still there, and a flight not
// started, drop nothing.
func TestDayMarksLost(t *testing.T) {
	seat := func(person string) []competitions.Duty {
		return []competitions.Duty{{Role: competitions.RoleChair, Person: person}}
	}
	published := competitions.Schedule{Flights: []competitions.ScheduledFlight{
		{ID: "run", Flight: competitions.Flight{Event: "L3", Entries: []string{"ann", "bea"}}, Officials: seat("mary")},
		{ID: "kept", Flight: competitions.Flight{Event: "L5", Entries: []string{"cal"}}, Officials: seat("tom")},
		{ID: "waiting", Flight: competitions.Flight{Event: "L7", Entries: []string{"dee"}}, Officials: seat("sue")},
	}}
	next := competitions.Schedule{Flights: []competitions.ScheduledFlight{
		{ID: "kept", Flight: competitions.Flight{Event: "L5", Entries: []string{"cal"}}, Officials: seat("tom")},
		{ID: "new", Flight: competitions.Flight{Event: "L3", Entries: []string{"ann"}}, Officials: seat("mary")},
		{ID: "waiting", Flight: competitions.Flight{Event: "L7", Entries: []string{"dee"}}, Officials: seat("max")},
	}}
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	m := dayMarks{
		times: map[string]store.FlightTime{"run": {Started: at}, "kept": {Started: at, Finished: at}},
		up:    map[string]store.Up{},
		official: map[string]map[string]store.Checkin{
			"run": {"mary": {Status: store.OfficialHere}}, "kept": {"tom": {Status: store.OfficialHere}},
			"waiting": {"sue": {Status: store.OfficialMissing}},
		},
		checkins: map[string]store.Checkin{"bea": {Status: store.CheckedIn}, "ann": {Status: store.Scratched}, "cal": {Status: store.CheckedIn}},
	}
	got := m.lost(published, next, strings.ToUpper)
	want := []string{
		"1 flight that has started or finished isn't in the plan any more: its times and who's up are dropped (L3).",
		"2 panel check-ins are for seats no longer in the plan.",
		"1 gymnast checked in or scratched isn't in the plan any more: BEA.",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("lost:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := m.lost(published, published, strings.ToUpper); len(got) != 0 {
		t.Errorf("publishing the same drops nothing: %v", got)
	}
	if got := (dayMarks{}).begun(published); len(got) != 0 {
		t.Errorf("nothing marked, nothing begun: %v", got)
	}
}
