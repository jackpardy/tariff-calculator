package competitions

import (
	"testing"
	"time"
)

func TestLateness(t *testing.T) {
	loc := time.FixedZone("test", 3600)
	first := time.Date(2030, 3, 16, 0, 0, 0, 0, time.UTC)
	flight := func(level, area string, day, start, end int) ScheduledFlight {
		return ScheduledFlight{Flight: Flight{Level: level, Number: 1, Of: 1}, Area: area, Day: day, Start: start, End: end}
	}
	s := Schedule{Flights: []ScheduledFlight{
		flight("A", "P1", 0, 9*60, 10*60),
		flight("B", "P1", 0, 10*60, 11*60),
		flight("C", "P2", 0, 9*60, 10*60),
		flight("D", "P3", 0, 9*60, 10*60),
		flight("E", "P1", 1, 9*60, 10*60),
	}}
	// Times are UTC; the test zone is an hour ahead, so 09:15 UTC is 10:15 there.
	at := func(h, m int) time.Time { return time.Date(2030, 3, 16, h, m, 0, 0, time.UTC) }
	actual := map[string]Actual{
		FlightKey(s.Flights[0]): {Started: at(8, 10), Finished: at(9, 15)}, // A started 09:10, finished 10:15
		FlightKey(s.Flights[1]): {Started: at(9, 20)},                      // B started 10:20: 20 late, the latest on P1
		FlightKey(s.Flights[2]): {Started: at(7, 55)},                      // C started 08:55: 5 early
		FlightKey(s.Flights[4]): {Started: at(9, 0).AddDate(0, 0, 1)},      // the next day: 10:00 for 09:00
	}
	got := Lateness(s, 0, first, loc, actual)
	if len(got) != 2 {
		t.Fatalf("P1 and P2 only: %+v", got)
	}
	if p1 := got["P1"]; p1.Minutes != 20 || p1.Flight != "B" || p1.As != "started" || !p1.At.Equal(at(9, 20)) {
		t.Errorf("P1 is 20 late from B starting: %+v", p1)
	}
	if p2 := got["P2"]; p2.Minutes != -5 || p2.As != "started" {
		t.Errorf("P2 is 5 early: %+v", p2)
	}

	// Finishing beats starting for the same flight; the second day is its own.
	actual[FlightKey(s.Flights[1])] = Actual{Started: at(9, 20), Finished: at(10, 5)} // ends 11:05: 5 late
	if p1 := Lateness(s, 0, first, loc, actual)["P1"]; p1.Minutes != 5 || p1.As != "finished" {
		t.Errorf("P1 is 5 late from B finishing: %+v", p1)
	}
	if d2 := Lateness(s, 1, first, loc, actual); len(d2) != 1 || d2["P1"].Minutes != 60 || d2["P1"].Flight != "E" {
		t.Errorf("the next day is measured on its own: %+v", d2)
	}
}
