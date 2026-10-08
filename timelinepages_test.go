package main

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"

	"tariffCalculator/competitions"
	"tariffCalculator/views"
)

func TestPanelTimeline(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	officials := admin + "/officials"
	redirected(t, h, officials+"/settings", url.Values{
		"panel-trampoline-chair": {"1"}, "panel-trampoline-execution": {"2"}, "panel-trampoline-difficulty": {"0"}, "panel-trampoline-recorder": {"0"}, "panel-trampoline-marshal": {"0"},
		"panel-tumbling-chair": {"1"}, "panel-tumbling-execution": {"1"}, "panel-tumbling-difficulty": {"0"}, "panel-tumbling-recorder": {"0"}, "panel-tumbling-marshal": {"0"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Mary"}, "club": {"UCD"}, "judge-trampoline": {"1"}, "chair-trampoline": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Tom"}, "judge-trampoline": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Ann"}, "judge-tumbling": {"1"}, "chair-tumbling": {"1"}})
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, url.Values{"gymnast": {"Eve"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	redirected(t, h, enter, url.Values{"gymnast": {"Dara"}, "discipline": {"tumbling"}, "level": {"Novice"}})
	tt := admin + "/timetable"
	redirected(t, h, tt+"/setup/rules", url.Values{"add": {"1"}, "kind": {"before"}, "must": {"1"}, "event": {"Tumbling Novice"}, "event2": {"BUCS L3"}})
	redirected(t, h, tt+"/setup/blocks", url.Values{"add": {"1"}, "name": {"Lunch"}, "minutes": {"45"}, "day": {"0"}, "at": {"12:00"}})

	if loc := do(t, h, http.MethodGet, tt+"/print?sheet=timeline", nil).Header().Get("Location"); !strings.Contains(loc, "Plan+the+timetable+first") {
		t.Errorf("nothing to show before planning: %q", loc)
	}
	if loc := do(t, h, http.MethodGet, tt+"/timeline.csv", nil).Header().Get("Location"); !strings.Contains(loc, "Plan+the+timetable+first") {
		t.Errorf("no CSV before planning: %q", loc)
	}
	page := do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()
	if !strings.Contains(page, "/timetable/print?sheet=timeline") {
		t.Error("the timetable links to the timeline")
	}
	// Lunch takes both areas; the day runs to 18:00.
	if !strings.Contains(page, "<td>Day 1</td><td>09:27</td><td>12:45</td><td>18:00</td><td>7h 48m</td>") {
		t.Error("the report says when the flights end and the time free after them")
	}

	textOf := func(page string) string {
		return strings.Join(strings.Fields(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(page, " ")), " ")
	}
	// Without officials: the day's areas side by side, a row a minute from
	// 09:00 under two header rows.
	page = do(t, h, http.MethodGet, tt+"/print?sheet=timeline", nil).Body.String()
	text := textOf(page)
	for _, want := range []string{"Panel timeline", "Day 1 Panel 1 Track 1", "Tumbling Novice 09:00–09:12 · 1", "BUCS L3 09:12–09:27 · 1", "Lunch 12:00–12:45", "With officials"} {
		if !strings.Contains(text, want) {
			t.Errorf("the timeline shows %q: %s", want, text)
		}
	}
	if strings.Contains(text, "Mary") || strings.Count(page, "comp-tl-block") != 2 {
		t.Error("no officials, and lunch on both areas")
	}
	for _, want := range []string{"grid-template-rows: repeat(2, auto) repeat(540, var(--tl-min))", "grid-row: 3 / span 12; grid-column: 3 / span 1", "grid-row: 15 / span 15; grid-column: 2 / span 1"} {
		if !strings.Contains(page, want) {
			t.Errorf("the grid has %q", want)
		}
	}

	// With officials: an area to a sheet, a column for each seat, a name to a
	// cell, and the same time scale under three header rows.
	page = do(t, h, http.MethodGet, tt+"/print?sheet=timeline-officials", nil).Body.String()
	text = textOf(page)
	for _, want := range []string{"Timeline with officials", "Day 1 · Panel 1 Flight Chair E1 E2", "Day 1 · Track 1 Flight Chair E1", "BUCS L3 09:12–09:27 · 1 Mary", "Tumbling Novice 09:00–09:12 · 1 Ann —", "Without officials"} {
		if !strings.Contains(text, want) {
			t.Errorf("the officials timeline shows %q: %s", want, text)
		}
	}
	for _, want := range []string{
		`grid-row: 16 / span 15; grid-column: 3 / span 1;" title="Mary · Chair of judges · BUCS L3 · 09:12–09:27">Mary`,
		`comp-tl-seat comp-tl-empty" style="grid-row: 4 / span 12; grid-column: 4 / span 1;" title="No one · Execution judge · Tumbling Novice · 09:00–09:12">—`,
		`grid-row: 184 / span 45; grid-column: 2 / span 4`, // lunch takes Panel 1's every column
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the officials grid has %q", want)
		}
	}

	rec := do(t, h, http.MethodGet, tt+"/timeline.csv", nil)
	if rec.Header().Get("Content-Type") != "text/csv; charset=utf-8" || !strings.Contains(rec.Header().Get("Content-Disposition"), "timeline.csv") {
		t.Errorf("a CSV download: %v", rec.Header())
	}
	rows, err := csv.NewReader(rec.Body).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"Day", "Area", "Starts", "Ends", "What", "Gymnasts", "Chair of judges", "Difficulty judge", "HD judge", "Synchronisation judge", "Execution judge", "Recorder", "Marshal"},
		{"Day 1", "Track 1", "09:00", "09:12", "Tumbling Novice", "1", "Ann", "", "", "", "—", "", ""},
		{"Day 1", "Panel 1", "09:12", "09:27", "BUCS L3", "1", "Mary (UCD)", "", "", "", "Tom; —", "", ""},
		{"Day 1", "Panel 1", "12:00", "12:45", "Lunch", "", "", "", "", "", "", "", ""},
		{"Day 1", "Track 1", "12:00", "12:45", "Lunch", "", "", "", "", "", "", "", ""},
	}
	if !slices.EqualFunc(rows, want, slices.Equal) {
		t.Errorf("the CSV:\n%q\nwant\n%q", rows, want)
	}
}

func TestTimelineLayout(t *testing.T) {
	duties := []competitions.Duty{{Role: competitions.RoleChair, Person: "a"}, {Role: competitions.RoleExecution, Person: "b"}, {Role: competitions.RoleExecution}}
	s := competitions.Schedule{
		Setup: competitions.Setup{
			Areas: []competitions.Area{{Name: "Panel 1"}, {Name: "Track"}},
			Days:  []competitions.Day{{Name: "Fri", Start: "18:00", End: "20:00", Areas: []string{"Panel 1"}}, {Name: "Sat", Start: "09:00", End: "12:00"}},
		},
		Flights: []competitions.ScheduledFlight{
			{Flight: competitions.Flight{Level: "A"}, Day: 1, Area: "Panel 1", Start: 9 * 60, End: 10 * 60, Officials: duties},
			{Flight: competitions.Flight{Level: "B", Entries: []string{"x", "y"}}, Discipline: competitions.Synchro, Day: 1, Area: "Panel 1", Start: 9*60 + 30, End: 10*60 + 30}, // moved by hand onto A
			{Flight: competitions.Flight{Level: "C"}, Day: 0, Area: "Panel 1", Start: 18 * 60, End: 18*60 + 20},
		},
	}
	name := func(k string) string { return strings.ToUpper(k) }

	// Side by side, every day runs 09:00 to 20:00, the hours outside each
	// day's shaded.
	tl := timeline(s, name, false, nil)
	if len(tl.Sheets) != 2 || tl.Sheets[0].Minutes != 11*60 || tl.Header != 2 {
		t.Fatalf("a sheet a day, 660 minutes: %+v", tl)
	}
	fri, sat := tl.Sheets[0], tl.Sheets[1]
	if len(fri.Areas) != 1 || len(sat.Areas) != 2 || sat.Columns != "var(--tl-time) var(--tl-event) var(--tl-event)" {
		t.Errorf("Friday has Panel 1 only, Saturday both: %+v", sat)
	}
	if want := []views.TimelineSpan{{Row: 3, Rows: 9 * 60}}; !slices.Equal(fri.Off, want) {
		t.Errorf("Friday before 18:00 is shaded: %v", fri.Off)
	}
	if len(fri.Hours) != 11 || fri.Hours[0] != (views.TimelineHour{Row: 3, Rows: 60, Label: "09:00"}) {
		t.Errorf("the hours: %v", fri.Hours)
	}
	a, b := sat.Areas[0].Cells[0], sat.Areas[0].Cells[1]
	if a.Row != 3 || a.Rows != 60 || b.Row != 33 || !a.Overlaps || !b.Overlaps || len(sat.Areas[0].Cells) != 2 {
		t.Errorf("A and B overlap, without officials: %+v", sat.Areas[0].Cells)
	}
	if a.Pairs || !b.Pairs || b.Gymnasts != 2 {
		t.Errorf("synchro flight B counts pairs: %+v", b)
	}

	// With officials, a sheet a day's area, each its own day's minutes, a
	// column for each seat.
	tl = timeline(s, name, true, nil)
	if len(tl.Sheets) != 3 || tl.Sheets[0].Title != "Fri · Panel 1" || tl.Sheets[0].Minutes != 120 || tl.Sheets[1].Minutes != 180 || tl.Header != 3 {
		t.Fatalf("Friday's Panel 1, Saturday's Panel 1 and Track: %+v", tl.Sheets)
	}
	p1 := tl.Sheets[1].Areas[0]
	var labels []string
	for _, seat := range p1.Seats {
		labels = append(labels, seat.Label)
	}
	if !slices.Equal(labels, []string{"Chair", "E1", "E2"}) || p1.Columns != 4 {
		t.Errorf("Panel 1's seats: %v", labels)
	}
	var cells []string
	for _, c := range p1.Cells {
		cells = append(cells, fmt.Sprintf("%s %s %d/%d %d", c.Kind, c.Name, c.Row, c.Rows, c.Column))
	}
	want := []string{"flight A 4/60 2", "seat A 4/60 3", "seat B 4/60 4", "seat — 4/60 5", "flight B 34/60 2"}
	if !slices.Equal(cells, want) {
		t.Errorf("cells %q, want %q", cells, want)
	}
	if len(tl.Sheets[2].Areas[0].Seats) != 0 {
		t.Error("the track has no seats")
	}
}
