package competitions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
)

// Manual changes to a schedule: the organiser moves gymnasts and flights; times
// are worked out again, and anything the edits break is flagged rather than
// refused.

// Version identifies the timetable as it is: any change to it changes its
// version, so a form can say which timetable it was made from, and one made
// from another is refused rather than moving the wrong flight or seat.
func (s Schedule) Version() string {
	b, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

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

// MoveFlight moves a flight's event, its whole run, to after everything else
// on an area and day, keeping its flights' order (ADR 0006 Decision 8).
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
	run := s.runOf(flight)
	start, _ := clock(s.Setup.Days[day].Start)
	length := 0
	for _, i := range run {
		length += s.Setup.duration(s.Flights[i])
	}
	for i, f := range s.Flights {
		if !slices.Contains(run, i) && f.Day == day && f.Area == area {
			start = max(start, f.End)
		}
	}
	if !s.Setup.AcrossBreaks {
		start = s.afterBlocks(day, area, start, length) // clear of breaks, the whole run
	}
	for _, i := range run {
		f := &s.Flights[i]
		f.Day, f.Area, f.Start = day, area, start
		start += s.Setup.duration(*f)
	}
	s.Retime()
	return true
}

// Earlier swaps a flight with the one before it in its event's run.
func (s *Schedule) Earlier(flight int) bool {
	if flight < 0 || flight >= len(s.Flights) {
		return false
	}
	run := s.runOf(flight)
	k := slices.Index(run, flight)
	if k < 1 {
		return false
	}
	a, b := &s.Flights[run[k-1]], &s.Flights[flight]
	if a.Day == b.Day && a.Area == b.Area {
		b.Start, a.Start = a.Start, a.Start+s.Setup.duration(*b)
	} else {
		a.Day, a.Area, a.Start, b.Day, b.Area, b.Start = b.Day, b.Area, b.Start, a.Day, a.Area, a.Start
	}
	s.Retime()
	return true
}

// runOf is the flights of a flight's event, by index, in the order they run.
func (s Schedule) runOf(flight int) []int {
	var out []int
	for i, f := range s.Flights {
		if f.Group() == s.Flights[flight].Group() {
			out = append(out, i)
		}
	}
	slices.SortStableFunc(out, func(a, b int) int {
		fa, fb := s.Flights[a], s.Flights[b]
		return (fa.Day*1440 + fa.Start) - (fb.Day*1440 + fb.Start)
	})
	return out
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
	s.number()
}

// number numbers each event's flights in the order they run (ADR 0006
// Decision 5): flight 1 starts first.
func (s *Schedule) number() {
	byEvent := map[string][]int{}
	for i, f := range s.Flights {
		byEvent[f.Group()] = append(byEvent[f.Group()], i)
	}
	for _, idx := range byEvent {
		slices.SortStableFunc(idx, func(a, b int) int {
			fa, fb := s.Flights[a], s.Flights[b]
			return (fa.Day*1440 + fa.Start) - (fb.Day*1440 + fb.Start)
		})
		for n, i := range idx {
			s.Flights[i].Number = n + 1
		}
	}
}

// Problems are what manual changes have broken: a flight past its day's end, an
// event's flights not back to back on one area (or across a break, where
// that isn't allowed), a person in two places at once, or a coach competing
// while their gymnasts compete elsewhere. people and coaches are each entry's
// people and coaches, by entry id, keyed so that one person has one key.
func (s Schedule) Problems(people, coaches map[string][]string, names map[string]string) []string {
	out := s.pastEnd()
	out = append(out, s.runProblems()...)
	out = append(out, s.personClashes(people, names)...)
	return append(out, s.coachCompeting(people, coaches, names)...)
}

// pastEnd are flights past their day's end.
func (s Schedule) pastEnd() []string {
	var out []string
	for _, f := range s.Flights {
		if f.Day < len(s.Setup.Days) {
			if end, _ := clock(s.Setup.Days[f.Day].End); f.End > end {
				out = append(out, fmt.Sprintf("%s runs past the end of %s (%s)", f.Name(), s.Setup.Days[f.Day].Name, s.Setup.Days[f.Day].End))
			}
		}
	}
	return out
}

// personClashes are people competing in two flights at once.
func (s Schedule) personClashes(people map[string][]string, names map[string]string) []string {
	// Each person's flights, by index, people in the order they first come:
	// pairs only within a person, not every turn against every other.
	var order []string
	turns := map[string][]int{}
	for i, f := range s.Flights {
		for _, id := range f.Entries {
			for _, p := range people[id] {
				if len(turns[p]) == 0 {
					order = append(order, p)
				}
				if !slices.Contains(turns[p], i) {
					turns[p] = append(turns[p], i)
				}
			}
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, p := range order {
		ts := turns[p]
		for x, i := range ts {
			a := s.Flights[i]
			for _, j := range ts[x+1:] {
				b := s.Flights[j]
				if overlaps(interval{a.Day, a.Start, a.End}, interval{b.Day, b.Start, b.End}) {
					msg := fmt.Sprintf("%s is in %s and %s at once", names[p], a.Name(), b.Name())
					if !seen[msg] {
						seen[msg] = true
						out = append(out, msg)
					}
				}
			}
		}
	}
	return out
}

// coachCompeting are coaches competing while gymnasts they coach compete
// elsewhere.
func (s Schedule) coachCompeting(people, coaches map[string][]string, names map[string]string) []string {
	var out []string
	seen := map[string]bool{}
	for i, a := range s.Flights {
		coaching := map[string]bool{}
		for _, id := range a.Entries {
			for _, c := range coaches[id] {
				coaching[c] = true
			}
		}
		for j, b := range s.Flights {
			if i == j || !overlaps(interval{a.Day, a.Start, a.End}, interval{b.Day, b.Start, b.End}) {
				continue
			}
			for _, id := range b.Entries {
				for _, p := range people[id] {
					msg := fmt.Sprintf("%s coaches in %s while competing in %s", names[p], a.Name(), b.Name())
					if coaching[p] && !seen[msg] {
						seen[msg] = true
						out = append(out, msg)
					}
				}
			}
		}
	}
	return out
}

// runProblems are events whose flights one panel can't judge together: not
// back to back on one area and day, or (unless the organiser allows it)
// either side of blocked time.
func (s Schedule) runProblems() []string {
	var out []string
	seen := map[string]bool{}
	for i, f := range s.Flights {
		if seen[f.Group()] {
			continue
		}
		seen[f.Group()] = true
		run := s.runOf(i)
		split, across := false, ""
		for k := 1; k < len(run); k++ {
			a, b := s.Flights[run[k-1]], s.Flights[run[k]]
			if a.Day != b.Day || a.Area != b.Area {
				split = true
				break
			}
			for j, g := range s.Flights {
				split = split || (!slices.Contains(run, j) && g.Day == a.Day && g.Area == a.Area && g.Start >= a.End && g.Start < b.Start)
			}
			for _, bl := range s.Blocks {
				if across == "" && bl.Day == a.Day && slices.Contains(bl.Areas, a.Area) && bl.Start >= a.End && bl.End <= b.Start {
					across = bl.Name
				}
			}
		}
		switch {
		case split:
			out = append(out, fmt.Sprintf("%s's flights aren't back to back on one area, so one panel can't judge them all", f.Group()))
		case across != "" && !s.Setup.AcrossBreaks:
			out = append(out, fmt.Sprintf("%s runs either side of %s", f.Group(), across))
		}
	}
	return out
}
