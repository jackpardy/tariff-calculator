package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestScoreSheets(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Ann Ryan", "Bea Kelly"} {
		redirected(t, h, enter, url.Values{"gymnast": {name}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	}
	tt := admin + "/timetable"
	redirected(t, h, tt+"/plan", url.Values{})
	if !strings.Contains(do(t, h, http.MethodGet, tt, nil).Body.String(), "print?sheet=scores") {
		t.Error("the timetable links to the score sheets")
	}
	page := do(t, h, http.MethodGet, tt+"/print?sheet=scores", nil).Body.String()
	if !strings.Contains(page, "Recorder&#39;s score sheet") && !strings.Contains(page, "Recorder's score sheet") {
		t.Fatal("a score sheet")
	}
	// The Code's panel: 6 execution judges and difficulty, for each routine.
	marks := regexp.MustCompile(`<th class="comp-score-mark[^"]*">([^<]+)</th>`).FindAllStringSubmatch(page, -1)
	var got []string
	for _, m := range marks[:len(marks)/2] {
		got = append(got, m[1])
	}
	if strings.Join(got, " ") != "E1 E2 E3 E4 E5 E6 D Pen Total" {
		t.Errorf("each routine's marks: %v", got)
	}
	if !strings.Contains(page, "Ann Ryan") || !strings.Contains(page, "Bea Kelly") || !strings.Contains(page, "size: A4 landscape") {
		t.Error("both gymnasts, landscape")
	}
	// Nothing filled in, difficulty included.
	if regexp.MustCompile(`<td class="comp-score-mark">\s*\d`).MatchString(page) {
		t.Errorf("every mark left to fill in")
	}
}
