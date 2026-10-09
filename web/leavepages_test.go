package web

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestLeavePage(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	officials := admin + "/officials"
	redirected(t, h, officials+"/settings", url.Values{
		"panel-trampoline-chair": {"1"}, "panel-trampoline-execution": {"1"}, "panel-trampoline-difficulty": {"0"}, "panel-trampoline-recorder": {"1"}, "panel-trampoline-marshal": {"0"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Mary"}, "judge-trampoline": {"1"}, "chair-trampoline": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Tom"}, "judge-trampoline": {"1"}, "chair-trampoline": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Rory"}, "recorder": {"1"}, "judge-trampoline": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Ann"}, "recorder": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Bea"}, "recorder": {"1"}})
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, url.Values{"gymnast": {"Eve"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	tt := admin + "/timetable"
	if loc := do(t, h, http.MethodGet, tt+"/leave", nil).Header().Get("Location"); !strings.Contains(loc, "Plan+the+timetable+first") {
		t.Errorf("nothing before planning: %q", loc)
	}
	page := do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()
	if !strings.Contains(page, tt+"/leave") {
		t.Error("the timetable links to the leave page")
	}
	text := func(path string) string {
		page := do(t, h, http.MethodGet, path, nil).Body.String()
		return html.UnescapeString(strings.Join(strings.Fields(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(page, " ")), " "))
	}
	form := do(t, h, http.MethodGet, tt+"/leave", nil).Body.String()
	// Whoever chairs BUCS L3 leaves.
	seats := regexp.MustCompile(`Chair of judges: (\w+)`).FindStringSubmatch(text(tt))
	if seats == nil {
		t.Fatal("no chair on the timetable")
	}
	chair := regexp.MustCompile(`<option value="([^"]+)">` + seats[1] + `</option>`).FindStringSubmatch(form)
	if chair == nil {
		t.Fatalf("%s isn't among those who can leave", seats[1])
	}
	// The seat is filled, one way or the other (who the rota seated
	// varies: unit tests cover how).
	for _, prefer := range []string{"fewest", ""} {
		got := text(tt + "/leave?person=" + url.QueryEscape(chair[1]) + "&day=0&from=09:00&prefer=" + prefer)
		for _, want := range []string{seats[1] + "'s 1 seat:", "BUCS L3 · Panel 1 · Day 1 09:00–09:15 · Chair of judges", "chair of judges"} {
			if !strings.Contains(got, want) {
				t.Errorf("prefer %q shows %q: %s", prefer, want, got)
			}
		}
		if strings.Contains(got, "left empty") || !regexp.MustCompile(`(takes|moves from \w+ judge to) chair of judges`).MatchString(got) {
			t.Errorf("prefer %q fills the chair: %s", prefer, got)
		}
	}
	if got := text(tt + "/leave?person=" + url.QueryEscape(chair[1]) + "&day=0&from=soon"); !strings.Contains(got, "Give the day and the time") {
		t.Errorf("a bad time is refused: %s", got)
	}
}
