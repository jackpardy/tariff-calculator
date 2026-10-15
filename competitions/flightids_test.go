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
