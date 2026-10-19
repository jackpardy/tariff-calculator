package competitions

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// Each flight planned has an ID of its own, the same each time from the same
// seed, and keeps it through moving, swapping, redrawing and renumbering:
// it's the flight with the same gymnasts.
func TestFlightIDs(t *testing.T) {
	setup := venue()
	setup.Timings = map[string]Timings{Trampoline: {PerCompetitor: 5, Between: 10, MaxFlight: 4}}
	entries := append(people("BUCS L3", Trampoline, 10), people("BUCS L5", Trampoline, 10)...)
	s := PlanSchedule(entries, []string{"BUCS L3", "BUCS L5"}, setup, 1)
	ids := map[string]string{} // ID → its first gymnast
	for _, f := range s.Flights {
		if f.ID == "" || ids[f.ID] != "" {
			t.Fatalf("an ID of its own: %q", f.ID)
		}
		ids[f.ID] = f.Entries[0]
	}
	if again := PlanSchedule(entries, []string{"BUCS L3", "BUCS L5"}, setup, 1); again.Flights[0].ID != s.Flights[0].ID {
		t.Error("the same plan has the same IDs")
	}
	kept := func(what string) {
		t.Helper()
		if len(s.Flights) != len(ids) {
			t.Fatalf("%s: %d flights", what, len(s.Flights))
		}
		for _, f := range s.Flights {
			if first, ok := ids[f.ID]; !ok || !slices.Contains(f.Entries, first) {
				t.Errorf("%s: %s isn't the flight it was", what, f.ID)
			}
		}
	}
	last := len(s.Flights) - 1
	before := s.Flights[last].ID
	if !s.MoveFlight(last, 0, setup.Areas[0].Name) {
		t.Fatal("moved")
	}
	kept("moved")
	i := slices.IndexFunc(s.Flights, func(f ScheduledFlight) bool { return f.Number > 1 })
	if !s.Earlier(i) {
		t.Fatal("swapped")
	}
	kept("swapped and renumbered")
	s.Redraw(0, clubsOf(entries), peopleOfEntries(entries), rand.New(rand.NewPCG(2, 2)))
	s.Retime()
	kept("redrawn")
	if !slices.ContainsFunc(s.Flights, func(f ScheduledFlight) bool { return f.ID == before }) {
		t.Error("the moved flight keeps its ID")
	}

	// A flight made since gets one of its own.
	s.Flights = append(s.Flights, ScheduledFlight{Flight: Flight{Event: "New"}})
	s.Identify()
	if id := s.Flights[len(s.Flights)-1].ID; id == "" || ids[id] != "" {
		t.Errorf("a new flight's ID: %q", id)
	}
	if FlightKey(s.Flights[0]) != s.Flights[0].ID {
		t.Error("a flight is known on the day by its ID")
	}
	if f := (ScheduledFlight{Flight: Flight{Event: "L", Number: 2}, Area: "P1", Day: 1}); FlightKey(f) != "1|P1|L||2" {
		t.Errorf("one without, by its place: %s", FlightKey(f))
	}
}

// A fresh plan's flights take the IDs of the flights they were: the one of
// their event with the same gymnasts, or else the one with the same number
// that nothing has happened to yet. One that's neither never takes an old
// flight's ID, even one its own plan gave it.
func TestKeepIDs(t *testing.T) {
	flight := func(id, event string, number int, entries ...string) ScheduledFlight {
		return ScheduledFlight{ID: id, Flight: Flight{Event: event, Number: number, Entries: entries}}
	}
	old := []ScheduledFlight{
		flight("ran", "L3", 1, "ann", "bea"),
		flight("waiting", "L3", 2, "cal", "dee"),
		flight("started", "L5", 1, "eve"),
		flight("other", "L7", 1, "fay"),
	}
	started := map[string]bool{"ran": true, "started": true}
	fresh := func(id string) bool { return !started[id] }
	s := Schedule{
		Flights: []ScheduledFlight{
			flight("x1", "L3", 1, "cal", "ann"),  // neither the same gymnasts, nor fresh at its number
			flight("x2", "L3", 2, "bea", "dee"),  // the same number as one not started
			flight("ran", "L5", 1, "eve", "gus"), // its plan gave it an old ID, but it's not that flight
		},
		Unplaced: []ScheduledFlight{flight("x4", "L7", 2, "fay")}, // the same gymnasts, unplaced
	}
	s.KeepIDs(old, fresh)
	if s.Flights[1].ID != "waiting" || s.Unplaced[0].ID != "other" {
		t.Errorf("kept: %q, %q", s.Flights[1].ID, s.Unplaced[0].ID)
	}
	if s.Flights[0].ID != "x1" {
		t.Errorf("a flight that's no old one keeps its own ID: %q", s.Flights[0].ID)
	}
	if id := s.Flights[2].ID; id == "" || id == "ran" || id == "started" {
		t.Errorf("a flight that's no old one has an ID of its own: %q", id)
	}

	// The same gymnasts in another order, on another day, are the same
	// flight, started or not.
	s = Schedule{Flights: []ScheduledFlight{flight("new", "L3", 1, "bea", "ann")}}
	s.Flights[0].Day = 1
	s.KeepIDs(old, fresh)
	if s.Flights[0].ID != "ran" {
		t.Errorf("the flight that ran: %q", s.Flights[0].ID)
	}
}
