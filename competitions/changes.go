package competitions

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// What a draft changes from the published timetable, for the organiser to
// see before publishing: flights placed, moved or taken out, gymnasts moved
// between flights, panels' seats given to someone else, and blocked time
// moved. Running orders redrawn count as one line a flight.

// Kinds of change, in the order they're shown.
const (
	ChangeFlights  = "Flights"
	ChangeGymnasts = "Gymnasts"
	ChangePanels   = "Panels"
	ChangeBreaks   = "Breaks"
	ChangeSetup    = "Setup"
)

// ChangeKinds are the kinds of change, in the order they're shown.
var ChangeKinds = []string{ChangeFlights, ChangeGymnasts, ChangePanels, ChangeBreaks, ChangeSetup}

// Change is one way a draft differs from what's published.
type Change struct {
	Kind string // one of ChangeKinds
	Line string // e.g. "BUCS L3 Men · flight 2 of 3: now Saturday 10:20, Panel 2 (was Saturday 09:40, Panel 1)"
}

// ChangesFrom are what s changes from published, kind by kind. gymnasts
// names an entry by its id, people a person by their key; "" for one they
// don't know.
func (s Schedule) ChangesFrom(published Schedule, gymnasts, people func(string) string) []Change {
	var out []Change
	add := func(kind, format string, args ...any) {
		out = append(out, Change{Kind: kind, Line: fmt.Sprintf(format, args...)})
	}
	who := func(key string) string {
		if key == "" {
			return "no one"
		}
		if n := people(key); n != "" {
			return n
		}
		return "someone no longer listed"
	}

	// Flights, by their ID (or, without one, their name).
	key := func(f ScheduledFlight) string {
		if f.ID != "" {
			return f.ID
		}
		return "name:" + f.Name()
	}
	was := map[string]ScheduledFlight{}
	for _, f := range published.Flights {
		was[key(f)] = f
	}
	now := map[string]bool{}
	for _, f := range s.Flights {
		now[key(f)] = true
		b, ok := was[key(f)]
		switch {
		case !ok:
			add(ChangeFlights, "%s: added, %s, %s", f.Name(), s.when(f.Day, f.Start), f.Area)
		case f.Day != b.Day || f.Start != b.Start || f.Area != b.Area:
			add(ChangeFlights, "%s: now %s, %s (was %s, %s)", f.Name(), s.when(f.Day, f.Start), f.Area, published.when(b.Day, b.Start), b.Area)
		case f.End != b.End:
			add(ChangeFlights, "%s: now finishes about %s (was %s)", f.Name(), Clock(f.End), Clock(b.End))
		}
		if ok && !slices.Equal(f.Entries, b.Entries) && sameSet(f.Entries, b.Entries) {
			add(ChangeFlights, "%s: running order redrawn", f.Name())
		}
	}
	for _, f := range published.Flights {
		if !now[key(f)] {
			add(ChangeFlights, "%s: taken out", f.Name())
		}
	}

	// Gymnasts in another flight than they were, or newly in or out.
	in := func(t Schedule) map[string]ScheduledFlight {
		m := map[string]ScheduledFlight{}
		for _, f := range t.Flights {
			for _, id := range f.Entries {
				m[id] = f
			}
		}
		return m
	}
	inNow, inWas := in(s), in(published)
	var ids []string
	for id := range inNow {
		ids = append(ids, id)
	}
	for id := range inWas {
		if _, ok := inNow[id]; !ok {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	var moved []Change
	for _, id := range ids {
		name := gymnasts(id)
		if name == "" {
			continue // withdrawn since: not anyone's to see
		}
		f, isNow := inNow[id]
		b, wasIn := inWas[id]
		switch {
		case !wasIn:
			moved = append(moved, Change{ChangeGymnasts, fmt.Sprintf("%s: added to %s", name, f.Name())})
		case !isNow:
			moved = append(moved, Change{ChangeGymnasts, fmt.Sprintf("%s: taken out of %s", name, b.Name())})
		case key(f) != key(b):
			moved = append(moved, Change{ChangeGymnasts, fmt.Sprintf("%s: now in %s (was %s)", name, f.Name(), b.Name())})
		}
	}
	slices.SortStableFunc(moved, func(a, b Change) int { return strings.Compare(a.Line, b.Line) })
	out = append(out, moved...)

	// Panels: a seat is its event's, so a change shows once for the event
	// (its group), whichever of its flights it's on.
	seen := map[string]bool{}
	seats := func(name string, now, was []Duty) {
		for i, d := range now {
			if i >= len(was) || was[i].Role != d.Role {
				continue // the panel itself changed: its flight's line says so
			}
			if d.Person != was[i].Person {
				line := fmt.Sprintf("%s · %s: %s (was %s)", name, RoleName(d.Role), who(d.Person), who(was[i].Person))
				if !seen[line] {
					seen[line] = true
					add(ChangePanels, "%s", line)
				}
			}
		}
	}
	for _, f := range s.Flights {
		if b, ok := was[key(f)]; ok {
			seats(f.Group(), f.Officials, b.Officials)
		}
	}
	blocks := map[string]ScheduledBlock{}
	for k, b := range blockKeys(published.Blocks) {
		blocks[k] = b
	}
	for k, b := range blockKeys(s.Blocks) {
		if w, ok := blocks[k]; ok {
			seats(b.Name, b.Officials, w.Officials)
		}
	}

	// Blocked time, by its name, day and areas (each day's lunch on each
	// area is a block of its own).
	kept := map[string]bool{}
	for k, b := range blockKeys(s.Blocks) {
		kept[k] = true
		w, ok := blocks[k]
		switch {
		case !ok:
			add(ChangeBreaks, "%s: added, %s–%s, %s", b.Name, s.when(b.Day, b.Start), Clock(b.End), strings.Join(b.Areas, ", "))
		case b.Day != w.Day || b.Start != w.Start || b.End != w.End || !slices.Equal(b.Areas, w.Areas):
			add(ChangeBreaks, "%s: now %s–%s, %s (was %s–%s, %s)", b.Name, s.when(b.Day, b.Start), Clock(b.End), strings.Join(b.Areas, ", "),
				published.when(w.Day, w.Start), Clock(w.End), strings.Join(w.Areas, ", "))
		}
	}
	for k, b := range blockKeys(published.Blocks) {
		if !kept[k] {
			add(ChangeBreaks, "%s: taken out, %s, %s", b.Name, published.when(b.Day, b.Start), strings.Join(b.Areas, ", "))
		}
	}

	// The setup, as one line: what it changes shows above.
	a, _ := json.Marshal(s.Setup)
	b, _ := json.Marshal(published.Setup)
	if string(a) != string(b) {
		add(ChangeSetup, "The setup changed (days, areas, timings, breaks or rules)")
	}

	slices.SortStableFunc(out, func(a, b Change) int { return slices.Index(ChangeKinds, a.Kind) - slices.Index(ChangeKinds, b.Kind) })
	return out
}

// blockKeys are blocks in order, each with a key that finds it in another
// copy of the timetable: its name, day and areas, and which of those it is.
func blockKeys(blocks []ScheduledBlock) func(func(string, ScheduledBlock) bool) {
	return func(yield func(string, ScheduledBlock) bool) {
		seen := map[string]int{}
		for _, b := range blocks {
			k := fmt.Sprintf("%s|%d|%s", b.Name, b.Day, strings.Join(b.Areas, ","))
			seen[k]++
			if !yield(fmt.Sprintf("%s|%d", k, seen[k]), b) {
				return
			}
		}
	}
}

// when is a time on one of the timetable's days, e.g. "Saturday 09:40".
func (s Schedule) when(day, minute int) string {
	if day >= 0 && day < len(s.Setup.Days) && s.Setup.Days[day].Name != "" {
		return s.Setup.Days[day].Name + " " + Clock(minute)
	}
	return Clock(minute)
}

// sameSet says whether two lists hold the same items, in any order.
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}
