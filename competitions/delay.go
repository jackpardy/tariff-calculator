package competitions

import (
	"errors"
	"fmt"
	"slices"
)

// What if there's a delay: an area (or several) held up from a time for some
// minutes. Each delayed area's later flights shift back, only as far as they
// must (slack absorbs the delay) and around blocked time; a flight that would
// then run past the day's end, or put someone in two places, moves to another
// area of its discipline if one is free, no earlier than it was. Nothing is
// saved: the timetable stays as published.

// Delay is a hold-up: on a day, from a time, for some minutes, on some areas
// (none for all of the day's).
type Delay struct {
	Day     int
	Areas   []string
	From    int // minutes since midnight
	Minutes int
}

// DelayChange is a flight that starts later, or moves to another area.
type DelayChange struct {
	Flight           string
	Area, NewArea    string
	Start, NewStart  int
	End, NewEnd      int
	Moved            bool   // to another area
	PastEnd, Clashes bool   // what made it move
	After            string // the blocked time it waited for, e.g. "Lunch": it wouldn't have finished first
}

// Clash is a person needed in two flights at once.
type Clash struct {
	Person        string // their key
	First, Second string // the flights
	At            int    // when they overlap
}

// DelayReport is what a delay does to the timetable.
type DelayReport struct {
	Changes   []DelayChange
	Unplaced  []string // flights that no longer fit the day, by name
	Clashes   []Clash  // people needed in two places
	Before    []DayReport
	After     []DayReport
	ShortRest [2]int // people with less rest than wanted, before and after
}

// CheckDelay reports what's wrong with a delay for the schedule.
func (s Schedule) CheckDelay(d Delay) error {
	if d.Day < 0 || d.Day >= len(s.Setup.Days) {
		return errors.New("choose a day")
	}
	day := s.Setup.Days[d.Day]
	start, end := day.Hours()
	var errs []error
	if d.From < start || d.From >= end {
		errs = append(errs, fmt.Errorf("the delay should start between %s and %s", day.Start, day.End))
	}
	if d.Minutes < 1 || d.Minutes > 600 {
		errs = append(errs, errors.New("the delay should be from 1 to 600 minutes"))
	}
	for _, a := range d.Areas {
		if !slices.Contains(s.dayAreas(d.Day), a) {
			errs = append(errs, fmt.Errorf("%s isn't in use on %s", a, day.Name))
		}
	}
	return errors.Join(errs...)
}

// dayAreas are the areas a day uses.
func (s Schedule) dayAreas(day int) []string {
	var out []string
	for _, a := range s.Setup.Areas {
		if s.Setup.Days[day].uses(a.Name) {
			out = append(out, a.Name)
		}
	}
	return out
}

// Delayed is the schedule after a delay, and what changed. people are each
// entry's people, by entry id, as the planner keys them.
func (s Schedule) Delayed(d Delay, people map[string][]string) (Schedule, DelayReport, error) {
	if err := s.CheckDelay(d); err != nil {
		return s, DelayReport{}, err
	}
	out := s
	out.Flights = slices.Clone(s.Flights)
	areas := d.Areas
	if len(areas) == 0 {
		areas = s.dayAreas(d.Day)
	}
	_, dayEnd := s.Setup.Days[d.Day].Hours()

	// Shift each delayed area's later flights, keeping their order.
	var shifted []int
	waited := map[int]string{} // flights that waited for blocked time
	for _, area := range areas {
		var idx []int
		for i, f := range out.Flights {
			if f.Day == d.Day && f.Area == area && f.End > d.From {
				idx = append(idx, i)
			}
		}
		slices.SortFunc(idx, func(a, b int) int { return out.Flights[a].Start - out.Flights[b].Start })
		cursor := d.From + d.Minutes
		for _, i := range idx {
			f := &out.Flights[i]
			length := f.End - f.Start
			if f.Start < d.From { // under way: it finishes that much later
				f.End += d.Minutes
				cursor = f.End
				shifted = append(shifted, i)
				continue
			}
			t := out.afterBlocks(d.Day, area, max(f.Start, cursor), length)
			if t > max(f.Start, cursor) {
				waited[i] = out.blockEnding(d.Day, area, t)
			}
			if t != f.Start {
				f.Start, f.End = t, t+length
				shifted = append(shifted, i)
			}
			cursor = f.End
		}
	}

	// A shifted flight past the day's end, or putting someone in two places,
	// moves if it can.
	r := DelayReport{}
	why := map[int][2]bool{}
	slices.SortFunc(shifted, func(a, b int) int { return out.Flights[a].Start - out.Flights[b].Start })
	for _, i := range shifted {
		f := out.Flights[i]
		pastEnd, clashes := f.End > dayEnd, len(out.clashes(i, people)) > 0
		if !pastEnd && !clashes {
			continue
		}
		why[i] = [2]bool{pastEnd, clashes}
		if place, ok := out.freeSlot(i, s.Flights[i].Start, dayEnd, people, d, areas); ok {
			out.Flights[i].Area, out.Flights[i].Start, out.Flights[i].End = place.Area, place.Start, place.End
		} else if pastEnd {
			r.Unplaced = append(r.Unplaced, f.Name())
		}
	}

	for i, f := range out.Flights {
		was := s.Flights[i]
		if f.Start == was.Start && f.Area == was.Area && f.End == was.End {
			continue
		}
		r.Changes = append(r.Changes, DelayChange{
			Flight: f.Name(), Area: was.Area, NewArea: f.Area, Start: was.Start, NewStart: f.Start, End: was.End, NewEnd: f.End,
			Moved: f.Area != was.Area, PastEnd: why[i][0], Clashes: why[i][1],
		})
		if !r.Changes[len(r.Changes)-1].Moved {
			r.Changes[len(r.Changes)-1].After = waited[i]
		}
	}
	slices.SortStableFunc(r.Changes, func(a, b DelayChange) int { return a.NewStart - b.NewStart })
	seen := map[Clash]bool{}
	for i := range out.Flights {
		for _, c := range out.clashes(i, people) {
			if !seen[c] {
				seen[c] = true
				r.Clashes = append(r.Clashes, c)
			}
		}
	}
	before, after := s.Report(people), out.Report(people)
	r.Before, r.After = before.Days, after.Days
	r.ShortRest = [2]int{len(before.ShortRest), len(after.ShortRest)}
	return out, r, nil
}

// afterBlocks is the earliest start from t that a flight of length minutes
// can have on an area without overlapping its blocked time.
func (s Schedule) afterBlocks(day int, area string, t, length int) int {
	for moved := true; moved; {
		moved = false
		for _, b := range s.Blocks {
			if b.Day == day && slices.Contains(b.Areas, area) && t < b.End && b.Start < t+length {
				t, moved = b.End, true
			}
		}
	}
	return t
}

// blockEnding names the blocked time on an area ending at t.
func (s Schedule) blockEnding(day int, area string, t int) string {
	for _, b := range s.Blocks {
		if b.Day == day && b.End == t && slices.Contains(b.Areas, area) {
			return b.Name
		}
	}
	return ""
}

// who are a flight's people: its gymnasts and officials.
func (s Schedule) who(i int, people map[string][]string) []string {
	var out []string
	for _, id := range s.Flights[i].Entries {
		out = append(out, people[id]...)
	}
	for _, d := range s.Flights[i].Officials {
		if d.Person != "" {
			out = append(out, d.Person)
		}
	}
	return out
}

// clashes are the people a flight shares with another at the same time.
func (s Schedule) clashes(i int, people map[string][]string) []Clash {
	f := s.Flights[i]
	mine := s.who(i, people)
	var out []Clash
	for j, g := range s.Flights {
		if j == i || g.Day != f.Day || !(f.Start < g.End && g.Start < f.End) {
			continue
		}
		for _, p := range s.who(j, people) {
			if slices.Contains(mine, p) {
				first, second := f, g
				if j < i {
					first, second = g, f
				}
				out = append(out, Clash{Person: p, First: first.Name(), Second: second.Name(), At: max(f.Start, g.Start)})
			}
		}
	}
	return out
}

// freeSlot is the earliest place for a flight, no earlier than it was
// published, on an area of its discipline that day, ending by the day's end,
// clear of other flights, blocked time and the delay on the area and of its
// people's other flights. Its own area wins a tie.
func (s Schedule) freeSlot(i, notBefore, dayEnd int, people map[string][]string, d Delay, delayed []string) (ScheduledFlight, bool) {
	f := s.Flights[i]
	length := f.End - f.Start
	mine := s.who(i, people)
	areas := []string{f.Area}
	for _, a := range s.Setup.Areas {
		if a.Name != f.Area && a.runs(f.Discipline) && s.Setup.Days[f.Day].uses(a.Name) {
			areas = append(areas, a.Name)
		}
	}
	best, found := f, false
	for _, area := range areas {
		for t := notBefore; t+length <= dayEnd; {
			t = s.afterBlocks(f.Day, area, t, length)
			if t+length > dayEnd {
				break
			}
			blocked := -1 // the end of whatever's in the way
			if slices.Contains(delayed, area) && t < d.From+d.Minutes && d.From < t+length {
				blocked = d.From + d.Minutes
			}
			for j, g := range s.Flights {
				if j == i || g.Day != f.Day || !(t < g.End && g.Start < t+length) {
					continue
				}
				if g.Area == area || slices.ContainsFunc(s.who(j, people), func(p string) bool { return slices.Contains(mine, p) }) {
					blocked = max(blocked, g.End)
				}
			}
			if blocked < 0 {
				if !found || t < best.Start {
					best.Area, best.Start, best.End, found = area, t, t+length, true
				}
				break
			}
			t = blocked
		}
	}
	return best, found
}
