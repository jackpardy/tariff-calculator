package main

import (
	"encoding/csv"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"tariffCalculator/competitions"
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

	page = do(t, h, http.MethodGet, tt+"/print?sheet=timeline", nil).Body.String()
	text := strings.Join(strings.Fields(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(page, " ")), " ")
	for _, want := range []string{
		"Panel timeline", "Day 1 Panel 1 Track 1",
		"Tumbling Novice 09:00–09:12 · 1 gymnast Chair: Ann Execution: —",
		"BUCS L3 09:12–09:27 · 1 gymnast Chair: Mary Execution: Tom, —",
		"Lunch 12:00–12:45",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the timeline shows %q: %s", want, text)
		}
	}
	if n := strings.Count(page, "comp-timeline-block"); n != 2 {
		t.Errorf("lunch is on both areas: %d", n)
	}
	// Each row has a cell for every column not covered from above: the
	// table lines up.
	if !rowsLineUp(page, 2) {
		t.Error("the timeline's rows don't line up")
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
		{"Day", "Area", "Starts", "Ends", "What", "Gymnasts", "Chair of judges", "Difficulty judge", "HD judge", "Execution judge", "Recorder", "Marshal"},
		{"Day 1", "Track 1", "09:00", "09:12", "Tumbling Novice", "1", "Ann", "", "", "—", "", ""},
		{"Day 1", "Panel 1", "09:12", "09:27", "BUCS L3", "1", "Mary (UCD)", "", "", "Tom; —", "", ""},
		{"Day 1", "Panel 1", "12:00", "12:45", "Lunch", "", "", "", "", "", "", ""},
		{"Day 1", "Track 1", "12:00", "12:45", "Lunch", "", "", "", "", "", "", ""},
	}
	if !slices.EqualFunc(rows, want, slices.Equal) {
		t.Errorf("the CSV:\n%q\nwant\n%q", rows, want)
	}
}

// rowsLineUp checks a timeline table: every body row's cells, with those
// spanning down from rows above, fill all its columns exactly.
func rowsLineUp(page string, columns int) bool {
	body := page[strings.Index(page, "<tbody>"):strings.Index(page, "</tbody>")]
	covered := make([]int, columns) // rows each column is still covered for
	for _, row := range strings.Split(body, "<tr")[1:] {
		col := 0
		for _, span := range regexp.MustCompile(`<td rowspan="(\d+)"`).FindAllStringSubmatch(row, -1) {
			for col < columns && covered[col] > 0 {
				col++
			}
			if col == columns {
				return false
			}
			covered[col], _ = strconv.Atoi(span[1])
			col++
		}
		for i := range covered {
			if covered[i] == 0 {
				return false
			}
			covered[i]--
		}
	}
	return slices.Max(covered) == 0
}

func TestTimelineOverlaps(t *testing.T) {
	s := competitions.Schedule{
		Setup: competitions.Setup{Areas: []competitions.Area{{Name: "Panel 1"}}, Days: []competitions.Day{{Name: "Sat", Start: "09:00", End: "12:00"}}},
		Flights: []competitions.ScheduledFlight{
			{Flight: competitions.Flight{Level: "A"}, Area: "Panel 1", Start: 9 * 60, End: 10 * 60},
			{Flight: competitions.Flight{Level: "B"}, Area: "Panel 1", Start: 9*60 + 30, End: 10*60 + 30}, // moved by hand onto A
			{Flight: competitions.Flight{Level: "C"}, Area: "Panel 1", Start: 11 * 60, End: 11*60 + 20},
		},
	}
	_, rows := timeline(s, func(string) string { return "" })
	var times, cells []string
	for _, r := range rows {
		times = append(times, r.Time)
		for _, c := range r.Cells {
			var names []string
			for _, it := range c.Items {
				names = append(names, it.Name)
			}
			cells = append(cells, r.Time+" "+c.Kind+" "+strings.Join(names, "+")+" ×"+strconv.Itoa(c.Span))
		}
	}
	if want := []string{"09:00", "09:30", "10:00", "10:30", "11:00", "11:20"}; !slices.Equal(times, want) {
		t.Errorf("rows at %v, want %v", times, want)
	}
	if want := []string{"09:00 flight A+B ×3", "10:30   ×1", "11:00 flight C ×1", "11:20   ×1"}; !slices.Equal(cells, want) {
		t.Errorf("cells %q, want %q", cells, want)
	}
}
