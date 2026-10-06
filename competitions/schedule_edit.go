package competitions

import (
	"fmt"
	"math/rand/v2"
	"slices"
)

// Hand edits to a schedule: the organiser moves gymnasts and flights; times
// are worked out again, and anything the edits break is flagged rather than
// refused.

// Find is the flight an entry is in, by index, if it's in one.
func (s Schedule) Find(entryID string) (int, bool) {
	for i, f := range s.Flights {
		if slices.Contains(f.Entries, entryID) {
			return i, true
		}
	}
	return 0, false
}

// RemoveEntry takes an entry out of its flight.
func (s *Schedule) RemoveEntry(entryID string) {
	if i, ok := s.Find(entryID); ok {
		f := &s.Flights[i]
		f.Entries = slices.DeleteFunc(f.Entries, func(id string) bool { return id == entryID })
	}
}

// MoveEntry moves an entry to the end of a flight's running order.
func (s *Schedule) MoveEntry(entryID string, flight int) bool {
	if flight < 0 || flight >= len(s.Flights) {
		return false
	}
	s.RemoveEntry(entryID)
	s.Flights[flight].Entries = append(s.Flights[flight].Entries, entryID)
	s.Retime()
	return true
}

// MoveFlight moves a flight to after everything else on an area and day.
func (s *Schedule) MoveFlight(flight, day int, area string) bool {
	if flight < 0 || flight >= len(s.Flights) || day < 0 || day >= len(s.Setup.Days) {
		return false
	}
	ok := false
	for _, a := range s.Setup.Areas {
		ok = ok || (a.Name == area && a.runs(s.Flights[flight].Discipline))
	}
	if !ok {
		return false
	}
	start, _ := clock(s.Setup.Days[day].Start)
	for i, f := range s.Flights {
		if i != flight && f.Day == day && f.Area == area {
			start = max(start, f.End)
		}
	}
	for _, b := range s.Blocks {
		if b.Day == day && slices.Contains(b.Areas, area) && b.End > start && b.Start <= start {
			start = b.End
		}
	}
	f := &s.Flights[flight]
	f.Day, f.Area, f.Start = day, area, start
	s.Retime()
	return true
}

// Redraw draws a flight's running order again, then puts people with little
// rest early or late in it (OrderForRest).
func (s *Schedule) Redraw(flight int, clubs map[string]string, people map[string][]string, rng *rand.Rand) bool {
	if flight < 0 || flight >= len(s.Flights) {
		return false
	}
	f := &s.Flights[flight]
	var entries []PlanEntry
	for _, id := range f.Entries {
		entries = append(entries, PlanEntry{ID: id, Club: clubs[id]})
	}
	f.Entries = f.Entries[:0]
	for _, e := range Draw(entries, rng) {
		f.Entries = append(f.Entries, e.ID)
	}
	if p, ok := s.restPulls(people)[flight]; ok {
		f.Entries = orderForRest(f.Entries, p)
	}
	return true
}

// Retime works out each flight's end from its size again, and pushes later
// any flight that would then overlap the one before it, or a block, on its
// area. Flights keep their order.
func (s *Schedule) Retime() {
	sortFlights(s.Flights)
	for i := range s.Flights {
		f := &s.Flights[i]
		if i > 0 {
			prev := s.Flights[i-1]
			if prev.Day == f.Day && prev.Area == f.Area && f.Start < prev.End {
				f.Start = prev.End
			}
		}
		for _, b := range s.Blocks {
			if b.Day == f.Day && slices.Contains(b.Areas, f.Area) && f.Start < b.End && b.Start < f.Start+s.Setup.duration(*f) {
				f.Start = b.End
			}
		}
		f.End = f.Start + s.Setup.duration(*f)
	}
}

// Problems are what hand edits have broken: a flight past its day's end, or a
// person in two places at once. people are each entry's people, by entry id.
func (s Schedule) Problems(people map[string][]string, names map[string]string) []string {
	var out []string
	for _, f := range s.Flights {
		if f.Day < len(s.Setup.Days) {
			if end, _ := clock(s.Setup.Days[f.Day].End); f.End > end {
				out = append(out, fmt.Sprintf("%s runs past the end of %s (%s)", f.Name(), s.Setup.Days[f.Day].Name, s.Setup.Days[f.Day].End))
			}
		}
	}
	type turn struct {
		f      ScheduledFlight
		person string
	}
	var turns []turn
	for _, f := range s.Flights {
		for _, id := range f.Entries {
			for _, p := range people[id] {
				turns = append(turns, turn{f, p})
			}
		}
	}
	seen := map[string]bool{}
	for i, a := range turns {
		for _, b := range turns[i+1:] {
			if a.person != b.person || (a.f.Name() == b.f.Name() && a.f.Area == b.f.Area && a.f.Start == b.f.Start) {
				continue
			}
			if overlaps(interval{a.f.Day, a.f.Start, a.f.End}, interval{b.f.Day, b.f.Start, b.f.End}) {
				msg := fmt.Sprintf("%s is in %s and %s at once", names[a.person], a.f.Name(), b.f.Name())
				if !seen[msg] {
					seen[msg] = true
					out = append(out, msg)
				}
			}
		}
	}
	return out
}
