package competitions

import (
	"testing"
	"time"
)

func TestLateness(t *testing.T) {
	loc := time.FixedZone("test", 3600)
	first := time.Date(2030, 3, 16, 0, 0, 0, 0, time.UTC)
	flight := func(level, area string, day, start, end int) ScheduledFlight {
		return ScheduledFlight{Flight: Flight{Event: level, Number: 1, Of: 1}, Area: area, Day: day, Start: start, End: end}
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

	// Warm-up over doesn't change what it's measured from, but says how late
	// competing began: B planned to compete from 10:10 (its changeover), began
	// 10:25.
	if p1 := Lateness(s, 0, first, loc, actual)["P1"]; !p1.CompetingAt.IsZero() {
		t.Errorf("no Warm-up over, nothing said of competing: %+v", p1)
	}
	actual[FlightKey(s.Flights[1])] = Actual{Started: at(9, 20), WarmUpOver: at(9, 25), Finished: at(10, 5)}
	if p1 := Lateness(s, 0, first, loc, actual)["P1"]; p1.Minutes != 5 || p1.As != "finished" || !p1.CompetingAt.Equal(at(9, 25)) || p1.Competing != 15 {
		t.Errorf("competing began 15 late: %+v", p1)
	}
}

// The day's real warm-up and competing, a competitor, from what each flight's
// taps measured: a flight with no Warm-up over measures neither, and one not
// finished only its warm-up.
func TestMeasuredTimes(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2030, 3, 16, h, m, 0, 0, time.UTC) }
	flight := func(event, discipline string, day, n int) ScheduledFlight {
		f := ScheduledFlight{Flight: Flight{Event: event, Number: 1, Of: 1}, Discipline: discipline, Area: "P1", Day: day}
		for i := range n {
			f.Entries = append(f.Entries, event+string(rune('a'+i)))
		}
		return f
	}
	s := Schedule{Flights: []ScheduledFlight{
		flight("A", Trampoline, 0, 4),
		flight("B", Trampoline, 0, 6),
		flight("C", Trampoline, 0, 5),
		flight("D", Tumbling, 0, 10),
		flight("E", Tumbling, 0, 10),
		flight("F", Trampoline, 1, 5),
	}}
	actual := map[string]Actual{
		FlightKey(s.Flights[0]): {Started: at(9, 0), WarmUpOver: at(9, 4), Finished: at(9, 24)},   // 1 and 5 a competitor
		FlightKey(s.Flights[1]): {Started: at(9, 30), WarmUpOver: at(9, 36), Finished: at(10, 0)}, // 1 and 4
		FlightKey(s.Flights[2]): {Started: at(10, 0), Finished: at(10, 40)},                       // skipped: neither
		FlightKey(s.Flights[3]): {Started: at(9, 0), WarmUpOver: at(9, 10)},                       // warm-up only
		FlightKey(s.Flights[5]): {Started: at(9, 0), WarmUpOver: at(9, 50), Finished: at(10, 40)}, // another day
	}
	got := MeasuredTimes(s, 0, actual)
	if len(got) != 2 {
		t.Fatalf("trampoline and tumbling: %+v", got)
	}
	if tr := got[0]; tr.Discipline != Trampoline || tr.WarmUp != 1 || tr.Competing != 4.4 || tr.WarmUpFlights != 2 || tr.CompetingFlights != 2 {
		t.Errorf("trampoline, from A and B: %+v", tr)
	}
	if tu := got[1]; tu.Discipline != Tumbling || tu.WarmUp != 1 || tu.CompetingFlights != 0 {
		t.Errorf("tumbling, warm-up only: %+v", tu)
	}
}

// A time recorded on another date than the flight's day doesn't count.
func TestLatenessOtherDay(t *testing.T) {
	first := time.Date(2030, 3, 16, 0, 0, 0, 0, time.UTC)
	f := ScheduledFlight{Flight: Flight{Event: "BUCS L3", Number: 1, Of: 1}, Area: "P1", Start: 9 * 60, End: 10 * 60}
	s := Schedule{Setup: Setup{Days: []Day{{Name: "Sat"}}}, Flights: []ScheduledFlight{f}}
	early := map[string]Actual{FlightKey(f): {Started: time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC)}}
	if got := Lateness(s, 0, first, time.UTC, early); len(got) != 0 {
		t.Errorf("a time from another date: %+v", got)
	}
}
