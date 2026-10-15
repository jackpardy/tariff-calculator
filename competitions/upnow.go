package competitions

import (
	"slices"

	"tariffCalculator/requirements"
)

// Who's up now (roadmap item 7): as each gymnast competes, the marshal taps
// on to the next, so the venue screen can say who's on and who's next. Each
// gymnast competes twice: everyone's 1st routine in running order, then
// everyone's 2nd in the same order. The scratched, and those no longer
// entered, are skipped. Where the marshal has got is a Routine, the gymnast
// and which of their routines, not a place in that order, so it stays with
// them when someone joins or leaves the flight while it runs.

// Routine is one of a gymnast's routines in a flight: their entry, and Round
// 1 or 2. The zero Routine is before the marshal's first tap.
type Routine struct {
	Entry string
	Round int
}

// UpOrder is a flight's routines in the order they're done.
type UpOrder struct {
	Entries []string        // the flight's running order, by entry id
	open    map[string]bool // by entry: whether they compete (entered, not scratched)
}

// UpOrderOf is a flight's routines in order; competing says whether an
// entry competes (it's still entered, and not scratched).
func UpOrderOf(f ScheduledFlight, competing func(entryID string) bool) UpOrder {
	o := UpOrder{Entries: f.Entries, open: map[string]bool{}}
	for _, id := range f.Entries {
		o.open[id] = competing(id)
	}
	return o
}

// at is the routine at a place in the order, 0 to 2n-1.
func (o UpOrder) at(i int) Routine {
	n := len(o.Entries)
	return Routine{Entry: o.Entries[i%n], Round: i/n + 1}
}

// place is a routine's place in the order; -1 for one the flight doesn't
// have.
func (o UpOrder) place(r Routine) int {
	i := slices.Index(o.Entries, r.Entry)
	if i < 0 || r.Round < 1 || r.Round > 2 {
		return -1
	}
	return (r.Round-1)*len(o.Entries) + i
}

// Has says whether the flight has a routine: its gymnast is in it.
func (o UpOrder) Has(r Routine) bool { return o.place(r) >= 0 }

// Open says whether anyone competes at a routine: its gymnast is in the
// flight, still entered and not scratched.
func (o UpOrder) Open(r Routine) bool { return o.Has(r) && o.open[r.Entry] }

// No is a routine's running number: its gymnast's place in the flight (1
// first), the same in both rounds.
func (o UpOrder) No(r Routine) int { return slices.Index(o.Entries, r.Entry) + 1 }

// First is the first routine anyone competes at; false if everyone is
// scratched.
func (o UpOrder) First() (Routine, bool) {
	return o.Next(Routine{})
}

// Next is the next routine anyone competes at after from; from the zero
// Routine, the first. From a gymnast no longer in the flight (moved out
// while it ran), it's the first open routine in their round, as where they
// were is gone. false if there's none.
func (o UpOrder) Next(from Routine) (Routine, bool) {
	i := o.place(from)
	switch {
	case from == (Routine{}):
		i = -1
	case i < 0:
		i = (max(from.Round, 1)-1)*len(o.Entries) - 1
	}
	return o.step(i, 1)
}

// Back is the routine anyone competes at before from; false if there's none,
// or from isn't in the flight.
func (o UpOrder) Back(from Routine) (Routine, bool) {
	i := o.place(from)
	if i < 0 {
		return Routine{}, false
	}
	return o.step(i, -1)
}

// step is the nearest open routine from the place i, a step (1 or -1) at a
// time.
func (o UpOrder) step(i, step int) (Routine, bool) {
	for i += step; i >= 0 && i < 2*len(o.Entries); i += step {
		if r := o.at(i); o.open[r.Entry] {
			return r, true
		}
	}
	return Routine{}, false
}

// After are up to k routines anyone competes at after r, in order.
func (o UpOrder) After(r Routine, k int) []Routine {
	var out []Routine
	for len(out) < k {
		next, ok := o.Next(r)
		if !ok {
			break
		}
		out, r = append(out, next), next
	}
	return out
}

// RoutineNames are what an entry's two routines are called: the level's own
// names where its exercises have one ("Set" for set routines, "Voluntary"
// for the rest, when the two differ), else "1st routine" and "2nd routine"
// ("pass" for tumbling and DMT).
func RoutineNames(c Competition, e Entry) [2]string {
	switch e.Discipline {
	case Tumbling, DMT:
		return [2]string{"1st pass", "2nd pass"}
	}
	numbered := [2]string{"1st routine", "2nd routine"}
	if e.Discipline == Synchro {
		return numbered // a pair's routines aren't the level's two exercises
	}
	l, level, ok := c.EntryLevel(e)
	if !ok {
		return numbered
	}
	var out [2]string
	for i := range out {
		for _, ref := range level.Exercise(i + 1).Options {
			set, err := l.Set(ref)
			if err != nil {
				return numbered
			}
			name := "Voluntary"
			if _, ok := requirements.SetRoutine(set); ok {
				name = "Set"
			}
			if out[i] != "" && out[i] != name {
				return numbered // some options are set routines, some not
			}
			out[i] = name
		}
	}
	if out[0] == "" || out[0] == out[1] {
		return numbered
	}
	return out
}
