package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

// A synchro pair does one routine: the marshal steps through each pair once
// and then the flight is done, the venue screen shows the pair on and the
// next two without a second round, and a pair's own page gives one time. A
// position kept from when every flight had two rounds doesn't crash or point
// past the end.
func TestSynchroOneRoutine(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	form := eventsCompetition()
	form.Set("level", "builtin-level:isto-novice")
	form.Set("synchroLevel", "builtin-level:isto-novice")
	admin := created(t, h, form)
	screen := madeLink(t, h, admin, "Main hall screen", "screen")
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	var pairLink string
	for _, name := range []string{"Ann", "Bea", "Cal"} {
		link := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {name}, "discipline": {"synchro"}, "level": {"ISTO Novice"}, "partnerName": {name + " P"},
			"ex1Option": {"builtin:isto-novice-set-a"}, "ex2Option": {"builtin:isto-novice-vol"}, "ex2Skills": {voluntary}}))
		if pairLink == "" {
			pairLink = link
		}
	}
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})
	clock.set(onTheDay(9, 0))

	day := admin + "/day?day=0"
	get := func() string { return dayAll(t, h, day) }
	page := get()
	key := regexp.MustCompile(`name="flight" value="([^"]+)"`).FindStringSubmatch(page)
	if key == nil {
		t.Fatalf("no flight: %s", page)
	}
	flight := unescape(key[1])
	var order []string
	for _, m := range regexp.MustCompile(`comp-checkin-name">\s*(?:<span class="comp-run-no"[^>]*>\d+</span>\s*)?<(?:span|a)[^>]*>([^<]+)</(?:span|a)>`).FindAllStringSubmatch(page, -1) {
		order = append(order, unescape(m[1]))
	}
	if len(order) != 3 {
		t.Fatalf("one flight of three pairs: %v", order)
	}
	// The rows give the phone one routine name a round: one round.
	if !strings.Contains(page, `data-routines="Routine"`) || strings.Contains(page, `data-routines="Routine|`) {
		t.Errorf("the rows say one routine: %s", page)
	}
	tap := func(what string) string {
		t.Helper()
		form := url.Values{"flight": {flight}, "what": {what}, "day": {"0"}}
		up := regexp.MustCompile(`(?s)<form[^>]*/day/up"[^>]*>.*?</form>`).FindString(get())
		for _, field := range []string{"entry", "round"} {
			if m := regexp.MustCompile(`name="` + field + `" value="([^"]*)"`).FindStringSubmatch(up); m != nil {
				form.Set(field, unescape(m[1]))
			}
		}
		return redirected(t, h, admin+"/day/up", form)
	}
	on := func(name string) {
		t.Helper()
		if gotName, routine := upOn(get()); gotName != name || routine != "Routine" {
			t.Errorf("on now: %s (%s), want %s (Routine)", gotName, routine, name)
		}
	}
	shown := func() string { return strings.Join(screenUpIn(dayAll(t, h, screen+"/screen?day=0")), " / ") }

	redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {"start"}, "day": {"0"}})
	on(order[0])
	if to := tap("next"); noticeOf(t, to) != "" {
		t.Errorf("a tap that moves says nothing: %s", to)
	}
	on(order[0])
	if got, want := shown(), order[0]+" · Routine / "+order[1]+" · Routine / "+order[2]+" · Routine"; got != want {
		t.Errorf("the screen: %s, want %s", got, want)
	}
	if strings.Contains(get(), "Next gymnast") == false {
		t.Error("Next gymnast on the first pair")
	}
	tap("next")
	on(order[1])
	tap("next")
	on(order[2])
	// The last pair: no second round, so Finished, and nobody next.
	page = get()
	if !strings.Contains(page, "last routine") || !strings.Contains(page, ">Finished</button>") || strings.Contains(page, ">Next gymnast</button>") {
		t.Errorf("the last pair says to finish the flight: %s", page)
	}
	if got, want := shown(), order[2]+" · Routine"; got != want {
		t.Errorf("nobody next after the last pair: %s, want %s", got, want)
	}
	if to := tap("next"); !strings.Contains(noticeOf(t, to), "That was the last routine") {
		t.Errorf("next after the last pair: %s", to)
	}
	on(order[2])
	tap("back")
	on(order[1])

	// A pair's own page gives one time.
	if page := dayAll(t, h, pairLink+"/day"); !strings.Contains(page, "Routine at about 09:") || strings.Contains(page, "Routines at about") {
		t.Errorf("a pair's page gives one time: %s", page)
	}
}

// A flight's position kept from when every flight had two rounds: a synchro
// pair's 2nd round counts as their one, so it neither crashes nor points
// past the end; trampoline still has its 2nd round.
func TestSynchroStaleUpPosition(t *testing.T) {
	c := competitions.Competition{Levels: []competitions.Level{{Ref: "builtin-level:isto-novice"}}}
	byID := map[string]store.Entry{
		"a": {ID: "a", Entry: competitions.Entry{Gymnast: "Ann", Discipline: competitions.Synchro, Level: "ISTO Novice"}},
		"b": {ID: "b", Entry: competitions.Entry{Gymnast: "Bea", Discipline: competitions.Synchro, Level: "ISTO Novice"}},
	}
	f := competitions.ScheduledFlight{Flight: competitions.Flight{Entries: []string{"a", "b"}}, Discipline: competitions.Synchro}
	for _, at := range []competitions.Routine{{Entry: "a", Round: 2}, {Entry: "b", Round: 2}} {
		u := screenUp(c, f, at, byID, nil)
		if u == nil || u.On.Routine != "Routine" {
			t.Fatalf("stale %v on the screen: %+v", at, u)
		}
		d := dayUp(c, f, byID, nil, store.Up{Routine: at}, true)
		if d.Name == "" || d.Routine != "Routine" {
			t.Errorf("stale %v on the day page: %+v", at, d)
		}
	}
	if u := screenUp(c, f, competitions.Routine{Entry: "a", Round: 2}, byID, nil); len(u.Next) != 1 || u.Next[0].Name != "Bea" {
		t.Errorf("who's next from a stale first pair: %+v", u)
	}
	if u := screenUp(c, f, competitions.Routine{Entry: "b", Round: 2}, byID, nil); len(u.Next) != 0 {
		t.Errorf("nobody after the last pair: %+v", u)
	}
	if d := dayUp(c, f, byID, nil, store.Up{Routine: competitions.Routine{Entry: "b", Round: 2}}, true); !d.Last {
		t.Error("the last pair is last")
	}
}
