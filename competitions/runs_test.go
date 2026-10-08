package competitions

import (
	"slices"
	"strings"
	"testing"
)

// runsTogether checks every event's flights run back to back on one area and
// day, numbered in the order they run.
func runsTogether(t *testing.T, s Schedule) {
	t.Helper()
	seen := map[string]bool{}
	for i, f := range s.Flights {
		if seen[f.Event()] {
			continue
		}
		seen[f.Event()] = true
		run := s.runOf(i)
		for k, j := range run {
			if s.Flights[j].Number != k+1 {
				t.Errorf("%s: flight %d runs %d%s", f.Event(), s.Flights[j].Number, k+1, map[bool]string{true: "st", false: "th"}[k == 0])
			}
		}
	}
	for _, p := range s.runProblems() {
		t.Error(p)
	}
}

func TestRuns(t *testing.T) {
	// Lunch at noon everywhere; three events of two or three flights.
	setup := venue()
	setup.Blocks = []Block{{Name: "Lunch", Minutes: 60, Day: 0, At: "12:00"}}
	entries := append(people("L1", Trampoline, 36), people("L2", Trampoline, 24)...)
	entries = append(entries, people("L3", Trampoline, 24)...)
	s := PlanSchedule(entries, []string{"L1", "L2", "L3"}, setup, 1)
	if len(s.Unplaced) != 0 {
		t.Fatalf("everything fits: %v", s.Unplaced)
	}
	check(t, s)
	runsTogether(t, s)
}

func TestRunsAcrossBreaks(t *testing.T) {
	// One panel, 09:00–14:00, lunch 11:00–11:30: L1's three flights (3½
	// hours) don't fit either side of lunch, only across it.
	setup := venue()
	setup.Areas, setup.Days[0].End = setup.Areas[:1], "14:00"
	setup.Blocks = []Block{{Name: "Lunch", Minutes: 30, Day: 0, At: "11:00"}}
	entries := people("L1", Trampoline, 36)
	if s := PlanSchedule(entries, []string{"L1"}, setup, 1); len(s.Unplaced) != 3 || len(s.Flights) != 0 {
		t.Errorf("not across lunch, L1 doesn't fit at all: %d placed, %d not", len(s.Flights), len(s.Unplaced))
	}
	fits := map[string]bool{}
	for _, f := range Fixes(entries, []string{"L1"}, setup, 1) {
		fits[f.Change] = f.Fits
	}
	if !fits["events can run across breaks"] {
		t.Errorf("allowing events across breaks fits it: %v", fits)
	}

	setup.AcrossBreaks = true
	s := PlanSchedule(entries, []string{"L1"}, setup, 1)
	if len(s.Unplaced) != 0 {
		t.Fatalf("across lunch, it fits: %v", s.Unplaced)
	}
	check(t, s)
	runsTogether(t, s)
	if starts := []int{s.Flights[0].Start, s.Flights[1].Start, s.Flights[2].Start}; !slices.Equal(starts, []int{9 * 60, 11*60 + 30, 12*60 + 40}) {
		t.Errorf("09:00, then after lunch 11:30 and 12:40: %v", starts)
	}

	// Turned off again, the timetable says L1 runs either side of lunch.
	s.Setup.AcrossBreaks = false
	if p := s.Problems(nil, nil); len(p) != 1 || p[0] != "L1 runs either side of Lunch" {
		t.Errorf("flagged: %v", p)
	}
}

func TestMoveRun(t *testing.T) {
	setup := venue()
	entries := append(people("L1", Trampoline, 24), people("L2", Trampoline, 12)...)
	s := PlanSchedule(entries, []string{"L1", "L2"}, setup, 1)
	l1, _ := s.Find("L1#0")
	to := "Panel 1"
	if s.Flights[l1].Area == to {
		to = "Panel 2"
	}
	// Moving one of L1's flights moves both, after whatever's on that panel.
	if !s.MoveFlight(l1, 0, to) {
		t.Fatal("moved")
	}
	check(t, s)
	runsTogether(t, s)
	var end int
	for _, f := range s.Flights {
		if f.Level == "L1" && f.Area != to {
			t.Errorf("all of L1 moves to %s: %s on %s", to, f.Name(), f.Area)
		}
		if f.Level != "L1" && f.Area == to {
			end = max(end, f.End)
		}
	}
	for _, f := range s.Flights {
		if f.Level == "L1" && f.Start < end {
			t.Errorf("L1 goes after what was on %s: %s at %s", to, f.Name(), Clock(f.Start))
		}
	}

	// Earlier swaps flight 2 with flight 1, keeping their gymnasts.
	l1, _ = s.Find("L1#0")
	second := s.runOf(l1)[1]
	gymnasts := slices.Clone(s.Flights[second].Entries)
	start := s.Flights[s.runOf(l1)[0]].Start
	if !s.Earlier(second) {
		t.Fatal("earlier")
	}
	l1, _ = s.Find("L1#0")
	first := s.runOf(l1)[0]
	if !slices.Equal(s.Flights[first].Entries, gymnasts) || s.Flights[first].Start != start || s.Flights[first].Number != 1 {
		t.Errorf("flight 2's gymnasts now go first, at %s: %+v", Clock(start), s.Flights[first])
	}
	runsTogether(t, s)
	if s.Earlier(first) {
		t.Error("the first flight can't go earlier")
	}

	// Moving one flight away by hand splits the event: flagged.
	s.Flights[first].Area = map[string]string{"Panel 1": "Panel 2", "Panel 2": "Panel 1"}[to]
	s.Retime()
	if p := s.Problems(nil, nil); len(p) == 0 || !strings.Contains(p[0], "L1's flights aren't back to back on one area") {
		t.Errorf("a split event is flagged: %v", p)
	}
}

func TestDelayKeepsRunsTogether(t *testing.T) {
	two := func(name, area string, start int, a, b string) ScheduledFlight {
		return ScheduledFlight{Flight: Flight{Level: name, Entries: []string{a, b}}, Area: area, Start: start, End: start + 20}
	}
	people := map[string][]string{"a": {"a"}, "b": {"b"}, "c": {"c"}, "d": {"d"}, "e": {"e"}, "f": {"f"}}
	s := Schedule{
		Setup:   Setup{Areas: []Area{{Name: "P1"}, {Name: "P2"}}, Days: []Day{{Name: "Sat", Start: "09:00", End: "12:00"}}},
		Flights: []ScheduledFlight{two("X", "P1", 9*60, "a", "b"), two("Y", "P1", 9*60+20, "c", "d"), two("Y", "P1", 9*60+40, "e", "f")},
		Blocks:  []ScheduledBlock{{Name: "Lunch", Areas: []string{"P1", "P2"}, Start: 10 * 60, End: 10*60 + 30}},
	}
	ys := func(out Schedule) (starts []int, areas []string) {
		for _, f := range out.Flights {
			if f.Level == "Y" {
				starts, areas = append(starts, f.Start), append(areas, f.Area)
			}
		}
		return starts, areas
	}
	// 15 minutes on P1 from 09:10: Y's second flight would overlap lunch, so
	// both wait for it.
	d := Delay{Areas: []string{"P1"}, From: 9*60 + 10, Minutes: 15}
	out, _ := delayed(t, s, d, people)
	if got, _ := ys(out); !slices.Equal(got, []int{10*60 + 30, 10*60 + 50}) {
		t.Errorf("Y after lunch, together: %v", got)
	}
	// Across breaks, its first flight goes before lunch.
	s.Setup.AcrossBreaks = true
	out, _ = delayed(t, s, d, people)
	if got, _ := ys(out); !slices.Equal(got, []int{9*60 + 35, 10*60 + 30}) {
		t.Errorf("Y either side of lunch: %v", got)
	}
	s.Setup.AcrossBreaks = false

	// The day ends at 11:00 and P1 is held up 70 minutes: Y's second flight
	// would run past the end, so all of Y moves to P2.
	s.Setup.Days[0].End = "11:00"
	out, r := delayed(t, s, Delay{Areas: []string{"P1"}, From: 9*60 + 10, Minutes: 70}, people)
	if starts, areas := ys(out); !slices.Equal(areas, []string{"P2", "P2"}) || starts[1] != starts[0]+20 {
		t.Errorf("all of Y moves to P2, back to back: %v %v (%+v)", starts, areas, r.Changes)
	}

	// Z's first flight has already run, and its second is under way, when P1
	// is held up: they stay as they were. Only the flights still to come,
	// which the delay pushes past the end, move.
	z := func(start int, a string) ScheduledFlight {
		return ScheduledFlight{Flight: Flight{Level: "Z", Entries: []string{a}}, Area: "P1", Start: start, End: start + 20}
	}
	people["g"], people["h"], people["i"], people["j"] = []string{"g"}, []string{"h"}, []string{"i"}, []string{"j"}
	s = Schedule{
		Setup:   Setup{Areas: []Area{{Name: "P1"}, {Name: "P2"}}, Days: []Day{{Name: "Sat", Start: "09:00", End: "10:40"}}},
		Flights: []ScheduledFlight{z(9*60, "g"), z(9*60+20, "h"), z(9*60+40, "i"), z(10*60, "j")},
	}
	out, r = delayed(t, s, Delay{Areas: []string{"P1"}, From: 9*60 + 30, Minutes: 30}, people)
	for _, f := range out.Flights {
		switch f.Entries[0] {
		case "g":
			if f.Area != "P1" || f.Start != 9*60 || f.End != 9*60+20 {
				t.Errorf("the flight already run stays: %+v", f)
			}
		case "h":
			if f.Area != "P1" || f.Start != 9*60+20 || f.End != 10*60+10 {
				t.Errorf("the flight under way runs late where it is: %+v", f)
			}
		default:
			if f.Area != "P2" {
				t.Errorf("the flights still to come move together: %+v (%+v)", f, r.Changes)
			}
		}
	}
	for _, c := range r.Changes {
		if c.Moved && c.Start < 9*60+30 {
			t.Errorf("a flight that started before the delay moved: %+v", c)
		}
	}
}
