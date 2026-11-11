package web

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/competitions"
)

// What a leave would break lists people in two places, which the other "what
// ifs" leave to themselves, ahead of the rest.
func TestLeaveListsWhatItWouldBreak(t *testing.T) {
	judge := func(key string) competitions.RotaPerson {
		return competitions.RotaPerson{Key: key, Name: strings.ToUpper(key), Judge: map[string]bool{"L": true}, Execution: map[string]bool{"L": true}}
	}
	seat := func(who string) []competitions.Duty {
		return []competitions.Duty{{Role: competitions.RoleExecution, Person: who}}
	}
	flight := func(area string, officials []competitions.Duty) competitions.ScheduledFlight {
		return competitions.ScheduledFlight{Flight: competitions.Flight{Event: "L"}, Area: area, Start: 600, End: 660, Officials: officials}
	}
	f := rotaFacts{oneNames: map[string]string{}, officials: []competitions.RotaPerson{judge("ann"), judge("bob")}}
	s := competitions.Schedule{Setup: competitions.Setup{Days: []competitions.Day{{Name: "Sat"}}}, Flights: []competitions.ScheduledFlight{flight("P1", seat("ann")), flight("P2", seat("bob"))}}
	if got := f.leaveBreaking(s, s); len(got) != 0 {
		t.Errorf("nothing newly broken: %v", got)
	}
	// Bob ends up on both panels at once.
	after := s.Copy()
	after.Flights[0].Officials = seat("bob")
	got := f.leaveBreaking(s, after)
	if len(got) == 0 || !strings.Contains(got[0], "BOB officiates") || !strings.Contains(got[0], "at once") {
		t.Errorf("someone in two places is listed: %v", got)
	}
}

func TestLeavePage(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	officials := admin + "/officials"
	redirected(t, h, officials+"/settings", url.Values{
		"panel-trampoline-chair": {"1"}, "panel-trampoline-execution": {"1"}, "panel-trampoline-difficulty": {"0"}, "panel-trampoline-recorder": {"1"}, "panel-trampoline-marshal": {"0"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Mary"}, "judge-trampoline": {"1"}, "chair-trampoline": {""}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Tom"}, "judge-trampoline": {"1"}, "chair-trampoline": {""}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Rory"}, "recorder": {"1"}, "judge-trampoline": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Ann"}, "recorder": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Bea"}, "recorder": {"1"}})
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, bucsEntry("Eve"))
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
	chair := regexp.MustCompile(`<option value="([^"]+)"[^>]*>` + seats[1] + `</option>`).FindStringSubmatch(form)
	if chair == nil {
		t.Fatalf("%s isn't among those who can leave", seats[1])
	}
	// The seat is filled (who the rota seated varies: unit tests cover how).
	got := text(tt + "/leave?person=" + url.QueryEscape(chair[1]) + "&day=0&from=09:00")
	for _, want := range []string{seats[1] + "'s 1 seat:", "BUCS L3 · Panel 1 · Day 1 09:00–09:15 · Chair of judges", "chair of judges"} {
		if !strings.Contains(got, want) {
			t.Errorf("shows %q: %s", want, got)
		}
	}
	if strings.Contains(got, "left empty") || !regexp.MustCompile(`(takes|moves from \w+ judge to) chair of judges`).MatchString(got) {
		t.Errorf("fills the chair: %s", got)
	}
	if strings.Contains(form, `name="prefer"`) {
		t.Error("there's one way to fill a seat, not a choice")
	}
	if got := text(tt + "/leave?person=" + url.QueryEscape(chair[1]) + "&day=0&from=soon"); !strings.Contains(got, "Give the day and the time") {
		t.Errorf("a bad time is refused: %s", got)
	}
}
