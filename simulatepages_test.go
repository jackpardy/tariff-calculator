package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestSimulation(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	redirected(t, h, admin+"/officials/add", url.Values{"name": {"Mary"}, "judge-trampoline": {"1"}, "chair-trampoline": {"1"}, "judge-tumbling": {"1"}})
	redirected(t, h, admin+"/officials/add", url.Values{"name": {"Rory"}, "recorder": {"1"}})
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, url.Values{"gymnast": {"Eve"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	redirected(t, h, enter, url.Values{"gymnast": {"Eve"}, "discipline": {"tumbling"}, "level": {"Novice"}})
	tt := admin + "/timetable"
	sim := tt + "/simulate"

	if page := do(t, h, http.MethodGet, tt, nil).Body.String(); !strings.Contains(page, `href="`+sim+`"`) {
		t.Error("the timetable links to the simulation")
	}
	page := do(t, h, http.MethodGet, sim, nil).Body.String()
	value := func(page, name string) string {
		t.Helper()
		m := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `"[^>]*? value="([^"]*)"`).FindStringSubmatch(page)
		if m == nil {
			t.Fatalf("no %s in the form", name)
		}
		return m[1]
	}
	// Filled in from the competition: BUCS L3, FIG AG3, Tumbling Novice.
	for name, want := range map[string]string{
		"name": "Scenario 1", "entries-0": "1", "entries-1": "", "entries-2": "1", "gymnasts": "1",
		"judges-trampoline": "1", "chairs-trampoline": "1", "judges-tumbling": "1", "chairs-tumbling": "",
		"judgePeople": "1", "competing": "", "helpers": "1",
	} {
		if got := value(page, name); got != want {
			t.Errorf("%s filled in as %q, want %q", name, got, want)
		}
	}

	scenario := url.Values{"name": {"Busy"}, "entries-0": {"60"}, "entries-1": {"30"}, "entries-2": {"20"}, "gymnasts": {"100"}, "clubs": {"6"},
		"judges-trampoline": {"12"}, "chairs-trampoline": {"3"}, "judges-tumbling": {"4"}, "chairs-tumbling": {"1"}, "judgePeople": {"14"}, "competing": {"4"}, "helpers": {"4"}}
	loc := redirected(t, h, sim, scenario)
	if !strings.Contains(loc, "Busy%3A+") {
		t.Fatalf("simulated: %s", loc)
	}
	page = do(t, h, http.MethodGet, sim, nil).Body.String()
	text := strings.Join(strings.Fields(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(page, " ")), " ")
	for _, want := range []string{"Busy", "110 entries, 100 gymnasts", "Day 1 flights end", "BUCS L3: 60", "Trampoline 12 (3 can chair)", "14 people judging", "4 judges compete · 4 helpers"} {
		if !strings.Contains(text, want) {
			t.Errorf("the scenario shows %q: %s", want, text)
		}
	}
	if value(page, "name") != "Scenario 2" {
		t.Error("the next scenario's name")
	}
	if tp := do(t, h, http.MethodGet, tt, nil).Body.String(); strings.Contains(tp, "Print marshal sheets") {
		t.Error("simulating doesn't plan the real timetable")
	}

	// Too many for one day: what doesn't fit and what would.
	scenario.Set("name", "Huge")
	scenario.Set("entries-0", "400")
	scenario.Set("gymnasts", "")
	redirected(t, h, sim, scenario)
	page = do(t, h, http.MethodGet, sim, nil).Body.String()
	if !regexp.MustCompile(`<li>BUCS L3 (Men|Women): \d+ flights</li>`).MatchString(page) || !strings.Contains(page, "Would fit with") && !strings.Contains(page, "No single change tried") {
		t.Errorf("Huge doesn't fit: %s", page)
	}

	// Refused: the same name, numbers that aren't, impossible gymnasts.
	for _, bad := range []url.Values{
		{"name": {"busy"}, "entries-0": {"5"}, "clubs": {"2"}},
		{"name": {"New"}, "entries-0": {"five"}, "clubs": {"2"}},
		{"name": {"New"}, "entries-0": {"5"}, "clubs": {"2"}, "gymnasts": {"9"}},
	} {
		if loc := redirected(t, h, sim, bad); !strings.Contains(loc, "Nothing+was+simulated") {
			t.Errorf("%v should be refused: %s", bad, loc)
		}
	}

	// Change one: the form starts from its numbers.
	page = do(t, h, http.MethodGet, sim+"?from=0", nil).Body.String()
	if value(page, "entries-0") != "60" || value(page, "judgePeople") != "14" || value(page, "name") != "Scenario 3" {
		t.Error("changing a scenario starts from its numbers")
	}

	// A changed setup makes the scenarios out of date until run again.
	redirected(t, h, tt+"/setup/timings", url.Values{"per-trampoline": {"6"}, "between-trampoline": {"10"}, "max-trampoline": {"12"},
		"per-tumbling": {"2"}, "between-tumbling": {"10"}, "max-tumbling": {"15"}, "rest": {"20"}})
	if page = do(t, h, http.MethodGet, sim, nil).Body.String(); strings.Count(page, "Planned with an earlier setup") != 2 {
		t.Error("both scenarios are out of date")
	}
	redirected(t, h, sim, url.Values{"again": {"0"}})
	if page = do(t, h, http.MethodGet, sim, nil).Body.String(); strings.Count(page, "Planned with an earlier setup") != 1 {
		t.Error("run again with the new setup")
	}

	if loc := redirected(t, h, sim+"/delete", url.Values{"remove": {"1"}}); !strings.Contains(loc, "Huge+removed") {
		t.Errorf("removed: %s", loc)
	}
	for _, bad := range []string{"1", "x"} {
		if rec := do(t, h, http.MethodPost, sim+"/delete", url.Values{"remove": {bad}}); rec.Code != http.StatusBadRequest {
			t.Errorf("removing %q: %d", bad, rec.Code)
		}
		if rec := do(t, h, http.MethodPost, sim, url.Values{"again": {bad}}); rec.Code != http.StatusBadRequest {
			t.Errorf("running %q again: %d", bad, rec.Code)
		}
	}
}

func TestSimulationKeepsTheLatest(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	sim := admin + "/timetable/simulate"
	for i := range 10 {
		redirected(t, h, sim, url.Values{"name": {"Run " + string(rune('A'+i))}, "entries-0": {"5"}, "clubs": {"2"}})
	}
	page := do(t, h, http.MethodGet, sim, nil).Body.String()
	if strings.Contains(page, "<th>Run B</th>") || !strings.Contains(page, "<th>Run C</th>") || !strings.Contains(page, "<th>Run J</th>") {
		t.Error("the last 8 scenarios are kept")
	}
}
