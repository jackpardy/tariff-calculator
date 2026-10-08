package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestTimetable(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("split", "all")
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	if page := do(t, h, http.MethodGet, enter, nil).Body.String(); !strings.Contains(page, `name="category" value="Women"`) {
		t.Error("gymnasts at a split level choose men or women")
	}
	l3 := func(name, category string) url.Values {
		return url.Values{"gymnast": {name}, "level": {"BUCS L3"}, "category": {category}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}
	}
	if rec := do(t, h, http.MethodPost, enter, l3("No category", "")); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "split into men and women") {
		t.Errorf("a split level needs men or women: %d", rec.Code)
	}
	var own []string
	for _, g := range []struct{ name, category string }{{"Ann", "Women"}, {"Bea", "Women"}, {"Cal", "Men"}, {"Dan", "Men"}, {"Eve", "Women"}} {
		own = append(own, withoutQuery(redirected(t, h, enter, l3(g.name, g.category))))
	}
	redirected(t, h, enter, url.Values{"gymnast": {"Tia"}, "discipline": {"tumbling"}, "level": {"Novice"}, "category": {"Women"}})

	tt := admin + "/timetable"
	page := do(t, h, http.MethodGet, tt, nil).Body.String()
	for _, want := range []string{`value="Day 1"`, `value="Panel 1"`, `value="Track 1"`, "Places every event", `name="per-tumbling" value="2"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the default setup should have %q", want)
		}
	}

	// Set up: the day, a second panel, a second (half) day, lunch, a rule.
	redirected(t, h, tt+"/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"09:00"}, "day-0-end": {"18:00"}, "add": {"1"}})
	redirected(t, h, tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "area-1-name": {"Track 1"}, "area-1-discipline": {"tumbling"}, "add": {"1"}})
	redirected(t, h, tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "area-1-name": {"Track 1"}, "area-1-discipline": {"tumbling"}, "area-2-name": {"Panel 2"}, "area-2-discipline": {"trampoline"}})
	redirected(t, h, tt+"/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"09:00"}, "day-0-end": {"18:00"},
		"day-1-name": {"Sunday"}, "day-1-start": {"09:00"}, "day-1-end": {"13:00"}, "day-1-area": {"Panel 1"}})
	redirected(t, h, tt+"/setup/blocks", url.Values{"add": {"1"}, "name": {"Lunch"}, "minutes": {"45"}, "day": {"0"}, "from": {"12:00"}, "to": {"14:00"}})
	location := redirected(t, h, tt+"/setup/rules", url.Values{"add": {"1"}, "event": {"Tumbling Novice"}, "kind": {"day"}, "day": {"1"}, "must": {"0"}})
	if !strings.Contains(location, "Rule+added") {
		t.Errorf("rule added: %s", location)
	}
	page = do(t, h, http.MethodGet, tt, nil).Body.String()
	for _, want := range []string{`value="Sunday"`, `value="Panel 2"`, "Lunch, 45 minutes, Saturday between 12:00 and 14:00, every area", "Tumbling Novice on Sunday (prefer)"} {
		if !strings.Contains(page, want) {
			t.Errorf("the setup should show %q", want)
		}
	}
	if location := redirected(t, h, tt+"/setup/rules", url.Values{"add": {"1"}, "event": {"BUCS L3"}, "kind": {"before"}, "event2": {"BUCS L3"}}); !strings.Contains(location, "Nothing+was+changed") {
		t.Error("a rule an event can't keep is refused")
	}

	// Plan.
	location = redirected(t, h, tt+"/plan", url.Values{})
	page = do(t, h, http.MethodGet, location, nil).Body.String()
	for _, want := range []string{"Planned 6 gymnasts.", "Everything fits.", "BUCS L3 Women", "BUCS L3 Men", "Tumbling Novice", "Not published"} {
		if !strings.Contains(page, want) {
			t.Errorf("the plan should say %q", want)
		}
	}
	// Tumbling preferred Sunday, but Sunday has no track: the preference is broken, and said so.
	if !strings.Contains(page, "Preferences not met") {
		t.Error("the broken preference is reported")
	}

	// A setup change marks the plan stale.
	redirected(t, h, tt+"/setup/timings", url.Values{"per-trampoline": {"5"}, "between-trampoline": {"10"}, "max-trampoline": {"2"}, "per-tumbling": {"2"}, "between-tumbling": {"10"}, "max-tumbling": {"15"}, "rest": {"20"}, "restMust": {"0"}, "separate": {"all"}})
	if page := do(t, h, http.MethodGet, tt, nil).Body.String(); !strings.Contains(page, "The setup has changed since this was planned.") {
		t.Error("stale")
	}
	page = do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()
	if !strings.Contains(page, "BUCS L3 Women · flight 1 of 2") {
		t.Error("flights of at most 2: the three women in two flights")
	}
	// The women's two flights go together: moving one moves the event, and
	// the second can go first.
	if !strings.Contains(page, "Move event") {
		t.Error("an event of two flights moves as one")
	}
	second := regexp.MustCompile(`(?s)<strong>BUCS L3 Women · flight 2 of 2</strong>.*?<input type="hidden" name="flight" value="(\d+)"`).FindStringSubmatch(page)
	if second == nil || !strings.Contains(page, `value="earlier"`) {
		t.Fatal("the second flight can go earlier")
	}
	if location := redirected(t, h, tt+"/flight", url.Values{"flight": {second[1]}, "action": {"earlier"}}); strings.Contains(location, "notice=") {
		t.Errorf("swapped: %s", location)
	}

	// Events can run across breaks, if the organiser says so.
	if location := redirected(t, h, tt+"/setup/blocks", url.Values{"breaks": {"1"}, "across": {"1"}}); !strings.Contains(location, "events+can+run+across+breaks") {
		t.Errorf("saved: %s", location)
	}
	if page := do(t, h, http.MethodGet, tt, nil).Body.String(); !strings.Contains(page, `name="across" value="1" checked`) {
		t.Error("the setting shows")
	}
	redirected(t, h, tt+"/setup/blocks", url.Values{"breaks": {"1"}})

	// Too little time: what doesn't fit, and what would fix it.
	// Flights of two take 20 minutes: Saturday fits one per panel by 09:20,
	// Sunday none by 09:15, so one of the three trampoline flights is left over.
	redirected(t, h, tt+"/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"09:00"}, "day-0-end": {"09:20"},
		"day-1-name": {"Sunday"}, "day-1-start": {"09:00"}, "day-1-end": {"09:15"}, "day-1-area": {"Panel 1"}})
	page = do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()
	if !strings.Contains(page, "Doesn&#39;t fit:") && !strings.Contains(page, "Doesn't fit:") {
		t.Error("says what doesn't fit")
	}
	if !strings.Contains(page, "another trampoline panel") || !strings.Contains(page, "Trampoline flights of up to 5, not 2") {
		t.Error("says what each change would do")
	}
	redirected(t, h, tt+"/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"09:00"}, "day-0-end": {"18:00"},
		"day-1-name": {"Sunday"}, "day-1-start": {"09:00"}, "day-1-end": {"13:00"}, "day-1-area": {"Panel 1"}})
	page = do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()

	// Hand changes: move a flight, redraw it, move a gymnast.
	flight := regexp.MustCompile(`name="flight" value="(\d+)"`).FindStringSubmatch(page)[1]
	if location := redirected(t, h, tt+"/flight", url.Values{"flight": {flight}, "action": {"redraw"}}); strings.Contains(location, "notice=") {
		t.Errorf("redrawn: %s", location)
	}
	if location := redirected(t, h, tt+"/flight", url.Values{"flight": {flight}, "action": {"move"}, "to": {"0:Track 1"}}); !strings.Contains(location, "go+there") {
		t.Error("trampoline can't move to the track")
	}
	if location := redirected(t, h, tt+"/flight", url.Values{"flight": {flight}, "action": {"move"}, "to": {"1:Panel 1"}}); strings.Contains(location, "notice=") {
		t.Errorf("moved to Sunday: %s", location)
	}

	// A late entry isn't in a flight until the organiser moves it in.
	late := withoutQuery(redirected(t, h, enter, l3("Fay", "Women")))
	page = do(t, h, http.MethodGet, tt, nil).Body.String()
	if !strings.Contains(page, "1 gymnast not in a flight") {
		t.Fatal("the late entry is listed")
	}
	fayID := regexp.MustCompile(`name="entry" value="([^"]+)">\s*<span class="is-size-7">Fay`).FindStringSubmatch(page)
	if fayID == nil {
		t.Fatal("Fay's move form")
	}
	redirected(t, h, tt+"/entry", url.Values{"entry": {fayID[1]}, "to": {"0"}})
	if page := do(t, h, http.MethodGet, tt, nil).Body.String(); strings.Contains(page, "not in a flight") {
		t.Error("Fay is in a flight now")
	}

	// Publishing shows gymnasts their flight.
	if page := do(t, h, http.MethodGet, late, nil).Body.String(); strings.Contains(page, "warm-up") {
		t.Error("nothing shows before it's published")
	}
	redirected(t, h, tt+"/publish", url.Values{"on": {"1"}})
	if page := do(t, h, http.MethodGet, late, nil).Body.String(); !regexp.MustCompile(`· (Panel \d) · warm-up (Saturday|Sunday) \d\d:\d\d`).MatchString(page) {
		t.Error("Fay sees her flight, area and time")
	}

	// The printed sheets, one area's day to a page.
	marshal := do(t, h, http.MethodGet, tt+"/print?sheet=marshal", nil).Body.String()
	if !strings.Contains(marshal, "Marshal") || !strings.Contains(marshal, "Fay") || !strings.Contains(marshal, "Track 1 · Saturday") {
		t.Error("marshal sheets per area and day")
	}
	judges := do(t, h, http.MethodGet, tt+"/print?sheet=judges", nil).Body.String()
	if !strings.Contains(judges, "Chair of judges") || !strings.Contains(judges, "BUCS L3 · option 1") || !strings.Contains(judges, "not checked") {
		t.Error("the chair of judges sees each gymnast's exercises; tumbling isn't checked")
	}
	_ = own
}

func TestClubsSeeTheirFlights(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	clubLink := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/club/")
	club := withoutQuery(redirected(t, h, "/clubs", url.Values{"name": {"UCD"}}))
	redirected(t, h, clubLink, url.Values{"clubAdmin": {club}})
	join := pathIn(t, do(t, h, http.MethodGet, club, nil).Body.String(), "/clubs/join/")
	member := withoutQuery(redirected(t, h, join, url.Values{"name": {"M"}}))
	save := formAction(t, do(t, h, http.MethodGet, member, nil).Body.String(), member+"/competitions/")
	redirected(t, h, save, url.Values{"level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	redirected(t, h, formAction(t, do(t, h, http.MethodGet, club, nil).Body.String(), club+"/competitions/"), url.Values{"which": {"all"}})

	tt := admin + "/timetable"
	redirected(t, h, tt+"/setup/days", url.Values{"day-0-name": {"Saturday"}, "day-0-start": {"08:30"}, "day-0-end": {"18:00"}})
	redirected(t, h, tt+"/plan", url.Values{})
	redirected(t, h, tt+"/publish", url.Values{"on": {"1"}})
	if page := do(t, h, http.MethodGet, club, nil).Body.String(); !strings.Contains(page, "BUCS L3 · Panel 1 · warm-up Saturday 08:30") {
		t.Error("the comp sec sees each member's flight")
	}
	if page := do(t, h, http.MethodGet, member, nil).Body.String(); !strings.Contains(page, "Panel 1 · warm-up Saturday 08:30") {
		t.Error("the member sees their flight")
	}
}

func TestIndividualsMatchedByName(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	// Dara enters trampoline and, separately, tumbling: the same person.
	redirected(t, h, enter, url.Values{"gymnast": {"Dara"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	redirected(t, h, enter, url.Values{"gymnast": {" dara "}, "discipline": {"tumbling"}, "level": {"Novice"}})
	tt := admin + "/timetable"
	page := do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()
	times := regexp.MustCompile(`Warm-up (\d\d:\d\d) · finishes about (\d\d:\d\d)`).FindAllStringSubmatch(page, -1)
	if len(times) != 2 {
		t.Fatalf("two flights: %v", times)
	}
	if !(times[0][2] <= times[1][1] || times[1][2] <= times[0][1]) {
		t.Errorf("Dara's two flights don't overlap: %v", times)
	}
}

func TestLookAlikeNames(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, url.Values{"gymnast": {"Dara O'Neill"}, "discipline": {"tumbling"}, "level": {"Novice"}})
	redirected(t, h, enter, url.Values{"gymnast": {"Dara ONeill"}, "discipline": {"tumbling"}, "level": {"Novice"}})
	page := do(t, h, http.MethodGet, redirected(t, h, admin+"/timetable/plan", url.Values{}), nil).Body.String()
	if !strings.Contains(page, "Dara O&#39;Neill and Dara ONeill") {
		t.Error("names that look alike are flagged")
	}
}
