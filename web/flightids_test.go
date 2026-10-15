package web

import (
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// What happens to a flight on the day stays with it when the organiser moves
// it to another area and publishes again: its start, its panel's check-ins
// and who's on. Someone added to the flight while it runs doesn't change
// who's on either.
func TestDayMarksFollowTheFlight(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore))
	h := s.h
	get := func(path string) string { return do(t, h, http.MethodGet, path, nil).Body.String() }
	admin := created(t, h, newCompetition())
	officials := admin + "/officials"
	redirected(t, h, officials+"/settings", url.Values{
		"panel-trampoline-chair": {"1"}, "panel-trampoline-execution": {"1"}, "panel-trampoline-difficulty": {"0"}, "panel-trampoline-recorder": {"0"}, "panel-trampoline-marshal": {"0"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Mary"}, "judge-trampoline": {"1"}, "chair-trampoline": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Tom"}, "judge-trampoline": {"1"}})
	enter := pathIn(t, get(admin+"/settings"), "/competitions/enter/")
	for _, name := range []string{"Ann", "Bea", "Cal", "Dee"} {
		redirected(t, h, enter, bucsEntry(name))
	}
	tt := admin + "/timetable"
	redirected(t, h, tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "add": {"1"}})
	redirected(t, h, tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "area-1-name": {"Panel 2"}, "area-1-discipline": {"trampoline"}})
	redirected(t, h, tt+"/plan", url.Values{})
	version := func() string {
		t.Helper()
		return regexp.MustCompile(`name="version" value="([0-9a-f]+)"`).FindStringSubmatch(get(tt))[1]
	}
	// Dee joins late: out of the timetable to begin with.
	dee := checkinEntryOnTimetable(t, get(tt), "Dee")
	redirected(t, h, tt+"/entry", url.Values{"entry": {dee}, "to": {""}, "version": {version()}})
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}, "version": {version()}})

	// On the day: started, tapped through to the 1st gymnast's 2nd routine,
	// and Mary here.
	s.clock.set(onTheDay(9, 0))
	day := admin + "/day?day=0"
	before := dayPageAreas(t, get(day))["Panel 1"]
	if len(before.Flights) != 1 {
		t.Fatalf("one flight on Panel 1: %+v", before)
	}
	flight, order := before.Flights[0].Key, before.Flights[0].Order
	if len(order) != 3 {
		t.Fatalf("Ann, Bea and Cal: %v", order)
	}
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {"start"}, "day": {"0"}})
	for range 4 {
		tapUp(t, h, admin, flight, "next")
	}
	mary, key := panelSeat(t, get(day), "BUCS L3", "Mary")
	redirected(t, h, admin+"/day/officials", url.Values{"person": {mary}, "flight": {key}, "status": {"here"}, "day": {"0"}})
	screen := madeLink(t, h, admin, "Hall", "screen")
	check := func(area, when string) {
		t.Helper()
		page := get(day)
		got, ok := dayPageAreas(t, page)[area]
		if !ok || len(got.Flights) != 1 {
			t.Fatalf("%s: the flight on %s: %+v", when, area, got)
		}
		f := got.Flights[0]
		if f.Key != flight || f.Started != "09:00" || f.Up != order[0] || f.Routine != "Voluntary" {
			t.Errorf("%s: its start and who's on: %+v", when, f)
		}
		if !strings.Contains(page, "Panel · 1 of 2 here</summary>") {
			t.Errorf("%s: Mary is still here", when)
		}
		card := screenCards(t, get(screen+"/screen?day=0"))[area]
		if card.Now != "BUCS L3" || len(card.Up) == 0 || card.Up[0] != order[0]+" · Voluntary" {
			t.Errorf("%s: the screen's %s: %+v", when, area, card)
		}
	}
	check("Panel 1", "before")

	// The organiser moves it to Panel 2, and publishes again.
	redirected(t, h, tt+"/flight", url.Values{"flight": {"0"}, "action": {"move"}, "to": {"0:Panel 2"}, "version": {version()}})
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}, "version": {version()}})
	if areas := dayPageAreas(t, get(day)); len(areas["Panel 1"].Flights) != 0 {
		t.Errorf("nothing left on Panel 1: %+v", areas["Panel 1"])
	}
	check("Panel 2", "moved")

	// Dee is added to the flight while it runs, and it's published again:
	// the marshal is still on Ann's 2nd routine, and Dee's 1st comes before
	// the rest of the 2nd round.
	redirected(t, h, tt+"/entry", url.Values{"entry": {dee}, "to": {"0"}, "version": {version()}})
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}, "version": {version()}})
	check("Panel 2", "with Dee")
	if f := dayPageAreas(t, get(day))["Panel 2"].Flights[0]; !slices.Equal(f.Order, append(slices.Clone(order), "Dee")) {
		t.Errorf("Dee runs last: %v", f.Order)
	}
	tapUp(t, h, admin, flight, "next")
	if f := dayPageAreas(t, get(day))["Panel 2"].Flights[0]; f.Up != order[1] || f.Routine != "Voluntary" {
		t.Errorf("Next goes on from Ann's 2nd routine: %+v", f)
	}
}

// checkinEntryOnTimetable is a gymnast's entry id, from their move form on
// the timetable page.
func checkinEntryOnTimetable(t *testing.T, page, name string) string {
	t.Helper()
	m := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(name) + `.*?name="entry" value="([^"]+)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no move form for %s: %s", name, page)
	}
	return unescape(m[1])
}
