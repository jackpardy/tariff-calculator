package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"tariffCalculator/store"
)

// A gymnast moved by hand to another flight is checked in again there: their
// check-in clears when the move is published, which says so first. Others'
// stay, and so does theirs while the move is only in the draft.
func TestMovedGymnastChecksInAgain(t *testing.T) {
	s := newTestServer(t, atTime(dayBefore))
	h := s.h
	ctx := context.Background()
	get := func(path string) string { return dayAll(t, h, path) }
	admin := created(t, h, newCompetition())
	enter := pathIn(t, get(admin+"/settings"), "/competitions/enter/")
	for _, name := range []string{"Ann", "Bea", "Cal", "Dee"} {
		redirected(t, h, enter, bucsEntry(name))
	}
	tt := admin + "/timetable"
	// Flights of two, so the event has two.
	redirected(t, h, tt+"/setup/timings", url.Values{"per-trampoline": {"5"}, "between-trampoline": {"10"}, "max-trampoline": {"2"},
		"per-tumbling": {"2"}, "between-tumbling": {"10"}, "max-tumbling": {"15"}, "rest": {"20"}, "restMust": {"0"}, "separate": {"all"}})
	redirected(t, h, tt+"/plan", url.Values{})
	version := func() string {
		t.Helper()
		return regexp.MustCompile(`name="version" value="([0-9a-f]+)"`).FindStringSubmatch(get(tt))[1]
	}
	publish := func(more url.Values) *http.Response {
		t.Helper()
		form := url.Values{"action": {"publish"}, "version": {version()}}
		for k, v := range more {
			form[k] = v
		}
		return do(t, h, http.MethodPost, tt+"/publish", form).Result()
	}
	if r := publish(nil); r.StatusCode != http.StatusSeeOther {
		t.Fatalf("published: %d", r.StatusCode)
	}

	s.clock.set(onTheDay(9, 0))
	ann, bea := checkinEntryOnTimetable(t, get(tt), "Ann"), checkinEntryOnTimetable(t, get(tt), "Bea")
	for _, id := range []string{ann, bea} {
		redirected(t, h, admin+"/day/checkin", url.Values{"entry": {id}, "status": {"here"}, "day": {"0"}})
	}
	c, err := s.st.CompetitionByAdmin(ctx, strings.TrimPrefix(admin, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	from, _ := c.Timetable.Find(ann)
	to := -1
	for i, f := range c.Timetable.Flights {
		if i != from && f.Event == c.Timetable.Flights[from].Event {
			to = i
		}
	}
	if to < 0 {
		t.Fatalf("the event has a second flight: %+v", c.Timetable.Flights)
	}
	redirected(t, h, tt+"/entry", url.Values{"entry": {ann}, "to": {strconv.Itoa(to)}, "version": {version()}})
	checkins := func() map[string]store.Checkin {
		t.Helper()
		got, err := s.st.Checkins(ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if checkins()[ann].Status != store.CheckedIn {
		t.Error("moved only in the draft, Ann is still checked in")
	}

	asked := do(t, h, http.MethodPost, tt+"/publish", url.Values{"action": {"publish"}, "version": {version()}})
	if page := asked.Body.String(); asked.Code != http.StatusOK || !strings.Contains(page, "1 gymnast checked in or scratched moves to another flight, so their check-in clears to be done again there: Ann.") {
		t.Fatalf("publishing says first that Ann's check-in clears: %d %s", asked.Code, page)
	}
	if checkins()[ann].Status != store.CheckedIn {
		t.Error("asking changes nothing")
	}
	if r := publish(url.Values{"anyway": {"1"}}); r.StatusCode != http.StatusSeeOther {
		t.Fatalf("published anyway: %d", r.StatusCode)
	}
	if got := checkins(); got[ann].Status != "" || got[bea].Status != store.CheckedIn {
		t.Errorf("Ann checks in again at her new flight; Bea, not moved, stays checked in: %+v", got)
	}
}
