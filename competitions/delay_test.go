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
