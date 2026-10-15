package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"tariffCalculator/competitions"
)

// onTheDay is a time on the competition's first day, 16 March 2030 (as
// newCompetition has it); dayBefore is two weeks before, while entries are
// open.
func onTheDay(hour, minute int) time.Time { return inDublin(2030, time.March, 16, hour, minute) }

var dayBefore = inDublin(2030, time.March, 1, 10, 0)

// publishedWith enters each name on their own in BUCS L3, plans the
// timetable and publishes it: up to six make one flight, 09:00–09:25 on
// Panel 1 for three.
func publishedWith(t *testing.T, h http.Handler, admin string, names ...string) {
	t.Helper()
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range names {
		redirected(t, h, enter, bucsEntry(name))
	}
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})
}

// panelOne is Panel 1 on a day page, failing unless it has one flight.
func panelOne(t *testing.T, h http.Handler, path string) dayArea {
	t.Helper()
	a, ok := dayPageAreas(t, do(t, h, http.MethodGet, path, nil).Body.String())["Panel 1"]
	if !ok || len(a.Flights) != 1 {
		t.Fatalf("Panel 1 with one flight on %s: %+v", path, a)
	}
	return a
}

// firstFlightKey is the key of the first flight on a day page.
func firstFlightKey(t *testing.T, h http.Handler, path string) string {
	t.Helper()
	key := regexp.MustCompile(`name="flight" value="([^"]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, path, nil).Body.String())
	if key == nil {
		t.Fatalf("no flight to mark on %s", path)
	}
	return unescape(key[1])
}

func TestOnTheDay(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	admin := created(t, h, newCompetition())
	if rec := do(t, h, http.MethodGet, admin+"/day", nil); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Publish the timetable first") {
		t.Errorf("nothing to run until the timetable is published: %d", rec.Code)
	}
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Ann", "Bea", "Cal"} {
		redirected(t, h, enter, bucsEntry(name))
	}
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	if page := do(t, h, http.MethodGet, admin, nil).Body.String(); strings.Contains(page, ">On the day<") {
		t.Error("no On the day button before the timetable is published")
	}
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})
	if page := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(page, admin+"/day") {
		t.Error("an On the day button once it's published")
	}

	day := admin + "/day?day=0"
	area := panelOne(t, h, day)
	if f := area.Flights[0]; f.Name != "BUCS L3" || f.Planned != "09:00–09:25" || !slices.Equal(f.Buttons, []string{"Started"}) || area.Status != "Nothing started yet" {
		t.Errorf("each flight has a Started button only, and its planned time: %+v", area)
	}
	flight := firstFlightKey(t, h, day)
	mark := func(what string) string {
		t.Helper()
		return redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {what}, "day": {"0"}})
	}

	// Before the day: tried out and marked, but not measured.
	if to := mark("start"); !strings.HasPrefix(to, day) {
		t.Errorf("back to the day: %s", to)
	}
	area = panelOne(t, h, day)
	if f := area.Flights[0]; f.Started != "10:00" || f.Who != "Organiser" || !slices.Contains(f.Buttons, "Finished") || !slices.Contains(f.Buttons, "Undo") || slices.Contains(f.Buttons, "Started") {
		t.Errorf("started: Finished and Undo are offered, and who marked it: %+v", f)
	}
	if !strings.HasPrefix(area.Status, "Not measured: times marked on another date") || area.Late || area.WhatIf != "" {
		t.Errorf("marked before the day isn't measured: %+v", area)
	}
	mark("undo")

	// The morning of the day: on time, then late, then finished early.
	clock.set(onTheDay(9, 0))
	mark("start")
	area = panelOne(t, h, day)
	if area.Status != "On time" || area.Since != "measured from BUCS L3 started at 09:00" || area.Late || area.Early || area.WhatIf != "" {
		t.Errorf("started at 09:00: %+v", area)
	}
	mark("undo")
	clock.set(onTheDay(9, 20))
	mark("start")
	area = panelOne(t, h, day)
	if area.Status != "Running 20 min late" || !area.Late || area.Since != "measured from BUCS L3 started at 09:20" {
		t.Errorf("started at 09:20: %+v", area)
	}
	if u, err := url.Parse(area.WhatIf); err != nil || u.Path != admin+"/timetable/delay" ||
		u.Query().Get("minutes") != "20" || u.Query().Get("from") != "09:20" || u.Query().Get("area") != "Panel 1" {
		t.Errorf("the delay page, filled in: %q", area.WhatIf)
	}
	clock.set(onTheDay(9, 15)) // before its planned end, 09:25
	mark("finish")
	area = panelOne(t, h, day)
	if f := area.Flights[0]; area.Status != "10 min early" || !area.Early || area.Late || f.Started != "09:20" || f.Finished != "09:15" || slices.Contains(f.Buttons, "Finished") {
		t.Errorf("finished at 09:15: %+v", area)
	}
	// Doing it twice does nothing, and says so.
	if to := mark("finish"); noticeOf(t, to) == "" {
		t.Errorf("already finished: %s", to)
	}
	mark("undo")
	if f := panelOne(t, h, day).Flights[0]; f.Finished != "" || !slices.Contains(f.Buttons, "Finished") {
		t.Errorf("undo takes back the finish: %+v", f)
	}
	mark("undo")
	if area := panelOne(t, h, day); area.Status != "Nothing started yet" || slices.Contains(area.Flights[0].Buttons, "Undo") {
		t.Errorf("undo again takes back the start: %+v", area)
	}
	if rec := do(t, h, http.MethodPost, admin+"/day/flight", url.Values{"flight": {"9|Nowhere|X||1"}, "what": {"start"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("not a flight: %d", rec.Code)
	}
	clock.set(onTheDay(9, 30))
	mark("start")

	// The history says what was marked, in words, once each.
	for _, want := range []string{"Marked a flight started: BUCS L3", "Marked a flight finished: BUCS L3", "Undid a flight time: BUCS L3"} {
		if !historyHas(t, h, admin, "Organiser", want) {
			t.Errorf("the history has %q by the organiser", want)
		}
	}
	if n := strings.Count(do(t, h, http.MethodGet, admin+"/history", nil).Body.String(), "Marked a flight finished"); n != 1 {
		t.Errorf("the finish that did nothing isn't recorded again: %d", n)
	}

	// Who can: chairs and the timetable link, not the cards link.
	chair := madeLink(t, h, admin, "Chris", "chair")
	if rec := do(t, h, http.MethodGet, chair+"/day?day=0", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<h1 class=\"title is-4\">On the day</h1>") {
		t.Errorf("a chair sees the day: %d", rec.Code)
	}
	if page := do(t, h, http.MethodGet, chair, nil).Body.String(); !strings.Contains(page, chair+"/day") {
		t.Error("a chair's dashboard offers On the day")
	}
	// Running 30 min late: the organiser is offered the delay page; a chair
	// sees the lateness but isn't sent to a page they can't use.
	if area := panelOne(t, h, day); area.WhatIf == "" {
		t.Errorf("the organiser can plan for the delay: %+v", area)
	}
	if area := panelOne(t, h, chair+"/day?day=0"); area.Status != "Running 30 min late" || area.WhatIf != "" {
		t.Errorf("a chair's view: %+v", area)
	}
	clock.set(onTheDay(9, 50))
	redirected(t, h, chair+"/day/flight", url.Values{"flight": {flight}, "what": {"finish"}, "day": {"0"}})
	if !historyHas(t, h, admin, "Chris", "Marked a flight finished: BUCS L3") {
		t.Error("the history says the chair did it")
	}
	if f := panelOne(t, h, day).Flights[0]; f.Finished != "09:50" || f.Who != "Chris" {
		t.Errorf("the day page says the chair finished it: %+v", f)
	}
	if rec := do(t, h, http.MethodGet, chair+"/timetable/delay", nil); rec.Code != http.StatusForbidden {
		t.Errorf("a chair can't plan: %d", rec.Code)
	}
	timetable := madeLink(t, h, admin, "Tina", "timetable")
	if rec := do(t, h, http.MethodGet, timetable+"/day", nil); rec.Code != http.StatusOK {
		t.Errorf("the timetable link sees the day: %d", rec.Code)
	}
	cards := madeLink(t, h, admin, "Carl", "cards")
	if rec := do(t, h, http.MethodGet, cards+"/day", nil); rec.Code != http.StatusForbidden {
		t.Errorf("a cards link can't see the day: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, cards+"/day/flight", url.Values{"flight": {flight}, "what": {"start"}}); rec.Code != http.StatusForbidden {
		t.Errorf("a cards link can't mark flights: %d", rec.Code)
	}
	if page := do(t, h, http.MethodGet, cards, nil).Body.String(); strings.Contains(page, ">On the day<") {
		t.Error("a cards link isn't offered On the day")
	}
}

// An area running late or early is told to attendees, once, on the day
// only, and not for a few minutes either way.
func TestLateNotes(t *testing.T) {
	s := newTestServer(t, atTime(onTheDay(11, 0)))
	ctx := context.Background()
	comp, _, err := s.st.CreateCompetition(ctx, competitions.Competition{
		Name: "Today", Date: "2030-03-16", Deadline: onTheDay(8, 0), LiveAt: dayBefore, Individuals: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Panel 1: two flights, 10:00–11:00 and 11:00–12:00; the first only
	// just started, so it's 60 minutes late. Panel 2's flight finished at
	// 10:50, 10 minutes early. Panel 3's started at 11:00, 2 minutes late:
	// not enough to say. Panel 4 has nothing recorded.
	at := func(area, level string, start, end int) competitions.ScheduledFlight {
		return competitions.ScheduledFlight{Flight: competitions.Flight{Level: level, Number: 1, Of: 1}, Area: area, Start: start, End: end}
	}
	first, second := at("Panel 1", "BUCS L3", 600, 660), at("Panel 1", "FIG AG3", 660, 720)
	early, nearly, none := at("Panel 2", "Early", 600, 660), at("Panel 3", "Close", 658, 700), at("Panel 4", "None", 600, 660)
	comp.Published = &competitions.Schedule{
		Setup: competitions.Setup{Days: []competitions.Day{{Name: "Saturday"}},
			Areas: []competitions.Area{{Name: "Panel 1"}, {Name: "Panel 2"}, {Name: "Panel 3"}, {Name: "Panel 4"}}},
		Flights: []competitions.ScheduledFlight{first, second, early, nearly, none},
	}
	markAt := func(f competitions.ScheduledFlight, what string, hour, minute int) {
		t.Helper()
		s.clock.set(onTheDay(hour, minute))
		if err := s.st.MarkFlight(ctx, comp.ID, competitions.FlightKey(f), what, "Marshal"); err != nil {
			t.Fatal(err)
		}
	}
	markAt(early, "start", 10, 0)
	markAt(early, "finish", 10, 50)
	markAt(first, "start", 11, 0)
	markAt(nearly, "start", 11, 0)

	notes, err := s.p.lateNotes(ctx, comp, []competitions.ScheduledFlight{second, first, early, nearly, none})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Panel 1 is running about 60 min late (as of 11:00)", "Panel 2 is running about 10 min early (as of 10:50)"}
	if !slices.Equal(notes, want) {
		t.Errorf("one line for Panel 1, once, and one for Panel 2: %q", notes)
	}
	if notes, _ := s.p.lateNotes(ctx, comp, []competitions.ScheduledFlight{nearly, none}); len(notes) != 0 {
		t.Errorf("2 minutes late and nothing recorded say nothing: %q", notes)
	}
	s.clock.set(onTheDay(11, 0).AddDate(0, 0, 1))
	if notes, _ := s.p.lateNotes(ctx, comp, []competitions.ScheduledFlight{first}); len(notes) != 0 {
		t.Errorf("only on the day: %q", notes)
	}
}
