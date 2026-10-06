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
	if dash := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(dash, ">Women</span>") {
		t.Error("the dashboard shows each gymnast's category")
	}

	tt := admin + "/timetable"
	if page := do(t, h, http.MethodGet, tt, nil).Body.String(); !strings.Contains(page, `value="plan"`) || strings.Contains(page, "Panel 1") {
		t.Error("nothing planned yet")
	}
	plan := url.Values{"action": {"plan"}, "panels": {"2"}, "start": {"09:00"}, "perGymnast": {"5"}, "between": {"10"}, "maxFlight": {"2"}, "separate": {"all"}}
	location := redirected(t, h, tt+"/plan", plan)
	page := do(t, h, http.MethodGet, location, nil).Body.String()
	for _, want := range []string{"Planned 5 gymnasts on 2 panels.", "BUCS L3 Women · flight 1 of 2", "BUCS L3 Men", "Panel 1", "Panel 2", "Warm-up 09:00", "Not published"} {
		if !strings.Contains(page, want) {
			t.Errorf("the timetable should say %q", want)
		}
	}
	// Women (3) in two flights, 9:00–9:20 and 9:20–9:35 on one panel; men (2) on the other.
	if !strings.Contains(page, "finishes 09:35") || !strings.Contains(page, "finishes 09:20") {
		t.Error("each panel's finish")
	}

	// Mixed flights for L3, though ranked separately.
	mixed := url.Values{}
	for k, v := range plan {
		mixed[k] = v
	}
	mixed.Set("separate", "")
	mixed.Set("maxFlight", "12")
	page = do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", mixed), nil).Body.String()
	if strings.Contains(page, "BUCS L3 Women") || !strings.Contains(page, "<strong>BUCS L3</strong>") {
		t.Error("one mixed BUCS L3 flight")
	}

	// How many panels to finish by a time.
	byNine := url.Values{}
	for k, v := range plan {
		byNine[k] = v
	}
	byNine.Set("finishBy", "09:40")
	page = do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", byNine), nil).Body.String()
	if !strings.Contains(page, "To finish by 09:40 you need 2 panels.") {
		t.Error("says how many panels finish by 09:40")
	}
	byNine.Set("finishBy", "09:10")
	page = do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", byNine), nil).Body.String()
	if !strings.Contains(page, "Even 20 panels can") {
		t.Error("says when no number of panels is enough")
	}

	// Times only keeps the flights.
	before := regexp.MustCompile(`<ol class="comp-order">.*?</ol>`).FindAllString(page, -1)
	times := url.Values{"action": {"times"}, "panels": {"2"}, "start": {"10:00"}, "perGymnast": {"5"}, "between": {"10"}, "maxFlight": {"2"}, "separate": {"all"}}
	page = do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", times), nil).Body.String()
	after := regexp.MustCompile(`<ol class="comp-order">.*?</ol>`).FindAllString(page, -1)
	if !strings.Contains(page, "Warm-up 10:00") || strings.Join(before, "") != strings.Join(after, "") {
		t.Error("times move to 10:00; the flights and orders stay")
	}

	// Flights move up, down and between panels, and redraw.
	for _, action := range []url.Values{
		{"panel": {"0"}, "flight": {"1"}, "action": {"up"}},
		{"panel": {"0"}, "flight": {"0"}, "action": {"down"}},
		{"panel": {"0"}, "flight": {"0"}, "action": {"redraw"}},
		{"panel": {"0"}, "flight": {"0"}, "action": {"move"}, "to": {"1"}},
	} {
		if location := redirected(t, h, tt+"/flight", action); strings.Contains(location, "notice=") {
			t.Errorf("%v: %s", action, location)
		}
	}
	if location := redirected(t, h, tt+"/flight", url.Values{"panel": {"0"}, "flight": {"9"}, "action": {"up"}}); !strings.Contains(location, "nothing+was+changed") {
		t.Error("a flight that isn't there")
	}

	// A late entry isn't in a flight until the organiser moves it in.
	late := withoutQuery(redirected(t, h, enter, l3("Fay", "Women")))
	page = do(t, h, http.MethodGet, tt, nil).Body.String()
	if !strings.Contains(page, "1 gymnast not in a flight") {
		t.Error("the late entry is listed")
	}
	fayID := regexp.MustCompile(`name="entry" value="([^"]+)">\s*<span class="is-size-7">Fay`).FindStringSubmatch(page)
	if fayID == nil {
		i := strings.Index(page, "not in a flight")
		t.Fatal("Fay's move form: " + page[i:i+600])
	}
	redirected(t, h, tt+"/entry", url.Values{"entry": {fayID[1]}, "to": {"0:0"}})
	if page := do(t, h, http.MethodGet, tt, nil).Body.String(); strings.Contains(page, "not in a flight") {
		t.Error("Fay is in a flight now")
	}

	// Publishing shows gymnasts their flight.
	if page := do(t, h, http.MethodGet, own[0], nil).Body.String(); strings.Contains(page, "Panel 1 · warm-up") || strings.Contains(page, "Panel 2 · warm-up") {
		t.Error("nothing shows before it's published")
	}
	redirected(t, h, tt+"/publish", url.Values{"on": {"1"}})
	if page := do(t, h, http.MethodGet, late, nil).Body.String(); !strings.Contains(page, "Panel 1 · warm-up 10:00") {
		t.Error("Fay sees her flight, panel and time")
	}

	// The printed sheets.
	marshal := do(t, h, http.MethodGet, tt+"/print?sheet=marshal", nil).Body.String()
	if !strings.Contains(marshal, "Marshal") || strings.Count(marshal, `class="comp-sheet"`) != 2 || !strings.Contains(marshal, "Fay") {
		t.Error("a marshal sheet per panel")
	}
	judges := do(t, h, http.MethodGet, tt+"/print?sheet=judges", nil).Body.String()
	if !strings.Contains(judges, "Chair of judges") || !strings.Contains(judges, "BUCS L3 · option 1") || !strings.Contains(judges, "problems") {
		t.Error("the chair of judges sees each gymnast's exercises and problems")
	}

	// A bad setting changes nothing.
	bad := url.Values{"action": {"plan"}, "panels": {"0"}, "start": {"09:00"}, "perGymnast": {"5"}, "between": {"10"}, "maxFlight": {"2"}}
	if location := redirected(t, h, tt+"/plan", bad); !strings.Contains(location, "Nothing+was+changed") {
		t.Errorf("0 panels is refused: %s", location)
	}
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
	redirected(t, h, tt+"/plan", url.Values{"action": {"plan"}, "panels": {"1"}, "start": {"08:30"}, "perGymnast": {"5"}, "between": {"10"}, "maxFlight": {"12"}})
	redirected(t, h, tt+"/publish", url.Values{"on": {"1"}})
	if page := do(t, h, http.MethodGet, club, nil).Body.String(); !strings.Contains(page, "BUCS L3 · panel 1 · warm-up 08:30") {
		t.Error("the comp sec sees each member's flight")
	}
	if page := do(t, h, http.MethodGet, member, nil).Body.String(); !strings.Contains(page, "Panel 1 · warm-up 08:30") {
		t.Error("the member sees their flight")
	}
}
