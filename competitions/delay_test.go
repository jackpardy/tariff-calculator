package competitions

import (
	"strings"
	"testing"
)

func delayed(t *testing.T, s Schedule, d Delay, people map[string][]string) (Schedule, DelayReport) {
	t.Helper()
	out, r, err := s.Delayed(d, people)
	if err != nil {
		t.Fatal(err)
	}
	return out, r
}

func at(s Schedule, name string) ScheduledFlight {
	for _, f := range s.Flights {
		if f.Level == name {
			return f
		}
	}
	return ScheduledFlight{}
}

func TestDelay(t *testing.T) {
	flight := func(name, area string, start, end int, entries ...string) ScheduledFlight {
		return ScheduledFlight{Flight: Flight{Level: name, Entries: entries}, Area: area, Start: start, End: end}
	}
	people := map[string][]string{"a": {"pa"}, "b": {"pb"}, "c": {"pc"}, "d": {"pd"}, "e": {"pb"}}
	s := Schedule{
		Setup: Setup{Areas: []Area{{Name: "P1"}, {Name: "P2"}}, Days: []Day{{Name: "Sat", Start: "09:00", End: "12:00"}}},
		Flights: []ScheduledFlight{
			flight("A", "P1", 9*60, 10*60, "a"),
			flight("B", "P1", 10*60, 10*60+30, "b"),
			flight("C", "P1", 11*60, 11*60+30, "c"), // after half an hour's slack
			flight("D", "P2", 9*60, 9*60+30, "d"),
		},
	}

	// 45 minutes on P1 from 09:30: A, under way, runs late; B follows it;
	// the slack before C takes up half an hour of it.
	out, r := delayed(t, s, Delay{Areas: []string{"P1"}, From: 9*60 + 30, Minutes: 45}, people)
	if a, b, c := at(out, "A"), at(out, "B"), at(out, "C"); a.Start != 9*60 || a.End != 10*60+45 || b.Start != 10*60+45 || c.Start != 11*60+15 {
		t.Errorf("shifted: A %d–%d, B %d, C %d", a.Start, a.End, b.Start, c.Start)
	}
	if len(r.Changes) != 3 || r.Changes[0].Flight != "A" || r.Changes[0].Moved || len(r.Unplaced) != 0 || len(r.Clashes) != 0 {
		t.Errorf("three later, none moved: %+v", r)
	}
	if r.Before[0].FlightsEnd != "11:30" || r.After[0].FlightsEnd != "11:45" {
		t.Errorf("flights end 11:30, then 11:45: %+v %+v", r.Before[0], r.After[0])
	}

	// 90 minutes: C would run past 12:00, so it moves to P2.
	out, r = delayed(t, s, Delay{Areas: []string{"P1"}, From: 9*60 + 30, Minutes: 90}, people)
	if c := at(out, "C"); c.Area != "P2" || c.Start != 11*60 {
		t.Errorf("C moves to P2 at 11:00: %+v", c)
	}
	var moved DelayChange
	for _, c := range r.Changes {
		if c.Flight == "C" {
			moved = c
		}
	}
	if !moved.Moved || !moved.PastEnd || moved.NewArea != "P2" {
		t.Errorf("C's change: %+v", moved)
	}

	// B's gymnast also competes on P2 at 10:45: shifted there, B would
	// clash, so it moves to P2 at 10:00, which is free.
	clash := s
	clash.Flights = append(clash.Flights, flight("E", "P2", 10*60+45, 11*60+15, "e"))
	out, r = delayed(t, clash, Delay{Areas: []string{"P1"}, From: 9*60 + 30, Minutes: 45}, people)
	if b := at(out, "B"); b.Area != "P2" || b.Start != 10*60 || len(r.Clashes) != 0 {
		t.Errorf("B moves to P2 at 10:00, clear of its gymnast's other flight: %+v, %v", b, r.Clashes)
	}

	// Nowhere to go: the delay runs to the end, so C doesn't fit; a clash
	// that can't be avoided is reported.
	full := s
	full.Flights = append(full.Flights, flight("F", "P2", 9*60+30, 12*60, "c"))
	_, r = delayed(t, full, Delay{From: 9*60 + 30, Minutes: 150}, people)
	if len(r.Unplaced) == 0 || len(r.Clashes) == 0 {
		t.Errorf("a whole-venue delay to the end leaves flights out: %+v", r)
	}

	// Flights go round blocked time.
	lunch := s
	lunch.Blocks = []ScheduledBlock{{Name: "Lunch", Areas: []string{"P1", "P2"}, Start: 11 * 60, End: 11*60 + 30}}
	out, r = delayed(t, lunch, Delay{Areas: []string{"P1"}, From: 9*60 + 30, Minutes: 45}, people)
	if b := at(out, "B"); b.Start != 11*60+30 {
		t.Errorf("B goes after lunch: %d", b.Start)
	}
	for _, c := range r.Changes {
		if c.Flight == "B" && c.After != "Lunch" {
			t.Errorf("B says it waited for lunch: %+v", c)
		}
	}

	for want, bad := range map[string]Delay{
		"choose a day": {Day: 3, From: 9 * 60, Minutes: 10},
		"between":      {From: 8 * 60, Minutes: 10},
		"1 to 600":     {From: 9 * 60, Minutes: 0},
		"isn't in use": {Areas: []string{"P9"}, From: 9 * 60, Minutes: 10},
	} {
		if _, _, err := s.Delayed(bad, people); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", want, err)
		}
	}
}

func TestDelayEasing(t *testing.T) {
	two := func(name string, start int, a, b string) ScheduledFlight {
		// Two gymnasts at trampoline's defaults: 10 minutes between and 5 each.
		return ScheduledFlight{Flight: Flight{Level: name, Entries: []string{a, b}}, Area: "P1", Start: start, End: start + 20}
	}
	people := map[string][]string{"a": {"a"}, "b": {"b"}, "c": {"c"}, "d": {"d"}, "e": {"e"}, "f": {"f"}}
	// Y's flights are either side of lunch, so events run across breaks.
	s := Schedule{
		Setup:   Setup{Areas: []Area{{Name: "P1"}, {Name: "P2"}}, Days: []Day{{Name: "Sat", Start: "09:00", End: "12:00"}}, AcrossBreaks: true},
		Flights: []ScheduledFlight{two("X", 9*60, "a", "b"), two("Y", 9*60+20, "c", "d"), two("Y", 10*60+30, "e", "f")},
		Blocks:  []ScheduledBlock{{Name: "Lunch", Areas: []string{"P1", "P2"}, Start: 10 * 60, End: 10*60 + 30}},
	}
	// 30 minutes on P1 from 09:10: X runs to 09:50, and Y (09:50–10:10)
	// can't finish before lunch.
	base := Delay{Areas: []string{"P1"}, From: 9*60 + 10, Minutes: 30}
	ys := func(out Schedule) []int {
		var starts []int
		for _, f := range out.Flights {
			if f.Level == "Y" {
				starts = append(starts, f.Start)
			}
		}
		return starts
	}
	out, r := delayed(t, s, base, people)
	if got := ys(out); got[0] != 10*60+30 || got[1] != 10*60+50 {
		t.Errorf("waiting for lunch: Y at %v", got)
	}

	shorten := base
	shorten.Breaks, shorten.Shorten = BreaksShorten, 15
	out, r = delayed(t, s, shorten, people)
	if got := ys(out); got[0] != 9*60+50 || got[1] != 10*60+30 || len(r.Breaks) != 1 || !strings.Contains(r.Breaks[0], "Lunch on P1 starts at 10:10, 10 min shorter") {
		t.Errorf("lunch 10 min shorter on P1: Y at %v, %v", got, r.Breaks)
	}
	if p2 := out.Blocks[0]; p2.Start != 10*60 || len(p2.Areas) != 1 || p2.Areas[0] != "P2" {
		t.Errorf("P2's lunch is as it was: %+v", p2)
	}

	move := base
	move.Breaks = BreaksMove
	out, r = delayed(t, s, move, people)
	if got := ys(out); got[0] != 9*60+50 || got[1] != 10*60+40 || !strings.Contains(r.Breaks[0], "Lunch on P1 moves to 10:10–10:40") {
		t.Errorf("lunch moved later on P1: Y at %v, %v", got, r.Breaks)
	}

	through := base
	through.Breaks = BreaksThrough
	out, r = delayed(t, s, through, people)
	if got := ys(out); got[0] != 9*60+50 || got[1] != 10*60+30 || !strings.Contains(r.Breaks[0], "P1 works through Lunch") {
		t.Errorf("P1 works through lunch: Y at %v, %v", got, r.Breaks)
	}

	// 5 minutes between flights and turns half as long: Y takes 10 minutes
	// and finishes by lunch.
	quick := base
	five := 5
	quick.Between, quick.Quicker = &five, 50
	out, _ = delayed(t, s, quick, people)
	if y := out.Flights[1]; y.Start != 9*60+50 || y.End != 10*60 {
		t.Errorf("a quicker Y: %d–%d", y.Start, y.End)
	}

	// Flights of up to 4: the two Ys merge, saving a changeover.
	merge := base
	merge.MaxFlight = 4
	out, r = delayed(t, s, merge, people)
	if len(out.Flights) != 2 || len(r.Merged) != 1 || len(out.Flights[1].Entries) != 4 || out.Flights[1].Start != 10*60+30 || out.Flights[1].End != 11*60 {
		t.Errorf("the Ys merged after lunch, 10:30–11:00: %+v %v", out.Flights, r.Merged)
	}
	if base.Eased() || !merge.Eased() {
		t.Error("whether a delay is eased")
	}
}

func TestDelayOverrun(t *testing.T) {
	people := map[string][]string{"a": {"pa"}, "b": {"pb"}, "c": {"pc"}}
	s := Schedule{
		Setup: Setup{Areas: []Area{{Name: "P1"}, {Name: "P2"}}, Days: []Day{{Name: "Sat", Start: "09:00", End: "12:00"}}},
		Flights: []ScheduledFlight{
			{Flight: Flight{Level: "A", Entries: []string{"a"}}, Area: "P1", Start: 9 * 60, End: 10 * 60},
			{Flight: Flight{Level: "C", Entries: []string{"c"}}, Area: "P1", Start: 11 * 60, End: 11*60 + 30},
			{Flight: Flight{Level: "P2 all day", Entries: []string{"b"}}, Area: "P2", Start: 9 * 60, End: 12 * 60},
		},
	}
	d := Delay{Areas: []string{"P1"}, From: 9*60 + 30, Minutes: 120}
	_, r := delayed(t, s, d, people)
	if len(r.Unplaced) != 1 {
		t.Errorf("C no longer fits: %v", r.Unplaced)
	}
	d.Overrun = 30
	out, r := delayed(t, s, d, people)
	if c := at(out, "C"); len(r.Unplaced) != 0 || c.Area != "P1" || c.End != 12*60+30 {
		t.Errorf("30 minutes over is allowed: %+v, %v", c, r.Unplaced)
	}
}
