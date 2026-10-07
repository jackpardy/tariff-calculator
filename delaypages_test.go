package main

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestDelayPage(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, url.Values{"gymnast": {"Eve"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	redirected(t, h, enter, url.Values{"gymnast": {"Dara"}, "discipline": {"tumbling"}, "level": {"Novice"}})
	tt := admin + "/timetable"
	if loc := do(t, h, http.MethodGet, tt+"/delay", nil).Header().Get("Location"); !strings.Contains(loc, "Plan+the+timetable+first") {
		t.Errorf("nothing to delay before planning: %q", loc)
	}
	page := do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()
	if !strings.Contains(page, tt+"/delay") {
		t.Error("the timetable links to the delay page")
	}
	text := func(path string) string {
		page := do(t, h, http.MethodGet, path, nil).Body.String()
		return html.UnescapeString(strings.Join(strings.Fields(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(page, " ")), " "))
	}
	got := text(tt + "/delay")
	if !strings.Contains(got, "What if there's a delay?") || strings.Contains(got, "Flights that change") {
		t.Error("the form, and no result until asked")
	}

	// Panel 1 held up from 09:05 for 20 minutes: BUCS L3, under way, runs late.
	got = text(tt + "/delay?day=0&from=09:05&minutes=20&area=Panel+1")
	for _, want := range []string{"Day 1 09:15 → 09:35", "Everything still fits.", "Flights that change (1)", "BUCS L3 Panel 1 09:00–09:15 Panel 1 09:00–09:35 under way: finishes 20 min late"} {
		if !strings.Contains(got, want) {
			t.Errorf("the delay shows %q: %s", want, got)
		}
	}
	// Eased: no time between flights and quicker turns, compared with the
	// delay alone.
	got = text(tt + "/delay?day=0&from=09:05&minutes=20&area=Panel+1&between=0&quicker=50&breaks=shorten&shorten=10")
	for _, want := range []string{"Easing it: breaks on the held-up areas can be up to 10 min shorter, 0 min between flights, turns 50% quicker.", "Without that: Day 1's flights end 09:35"} {
		if !strings.Contains(got, want) {
			t.Errorf("the eased delay shows %q: %s", want, got)
		}
	}
	if got := text(tt + "/delay?day=0&from=09:05&minutes=20&quicker=fast"); !strings.Contains(got, "Quicker turns should be a whole number") {
		t.Errorf("a bad easing is refused: %s", got)
	}

	// The timetable itself is unchanged.
	if !strings.Contains(text(tt), "Warm-up 09:00 · finishes about 09:15") {
		t.Error("the timetable isn't changed")
	}
	for bad, want := range map[string]string{
		"day=0&from=8:00&minutes=20":              "should start between 09:00 and 18:00",
		"day=0&from=soon&minutes=20":              "Give the day",
		"day=0&from=10:00&minutes=0":              "from 1 to 600 minutes",
		"day=0&from=10:00&minutes=5&area=Panel+9": "Panel 9 isn't in use",
	} {
		if got := text(tt + "/delay?" + bad); strings.Contains(got, "Flights that change") || !strings.Contains(got, want) {
			t.Errorf("%s should be refused with %q: %s", bad, want, got)
		}
	}
}
