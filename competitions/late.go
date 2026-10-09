package competitions

import (
	"fmt"
	"slices"
)

// Late changes (roadmap 2026-10-08): after the deadline, a club or an
// individual can ask to change an entry, in the ways the organiser allows,
// each with its own fee, charged if it's accepted.

// The kinds of late change.
const (
	LateLevel    = "level"    // a different level, with its routines
	LateRoutines = "routines" // new routines at the same level
)

// LateKinds are the kinds of late change, in the order pages list them.
var LateKinds = []string{LateLevel, LateRoutines}

// LateKindName names a kind of late change.
func LateKindName(kind string) string {
	if kind == LateRoutines {
		return "Routine change"
	}
	return "Level change"
}

// LateRule is whether a kind of late change is allowed, and its fee.
type LateRule struct {
	On  bool `json:"on,omitempty"`
	Fee int  `json:"fee,omitempty"` // in cents, charged when accepted
}

// LateChanges are the late changes the organiser allows, by kind, and
// whether a coach must sign off a request before it reaches the organiser.
type LateChanges struct {
	Rules        map[string]LateRule `json:"rules,omitempty"`
	SignoffFirst bool                `json:"signoff_first,omitempty"`
}

// Rule is a kind's rule.
func (l LateChanges) Rule(kind string) LateRule { return l.Rules[kind] }

// Allowed says whether a kind of late change can be asked for.
func (l LateChanges) Allowed(kind string) bool { return l.Rules[kind].On }

// Any says whether any late change can be asked for.
func (l LateChanges) Any() bool {
	return slices.ContainsFunc(LateKinds, l.Allowed)
}

// LateKind is the kind of late change from one entry to another: a level
// change if the event changes, routines otherwise.
func LateKind(from, to Entry) string {
	if from.Event() != to.Event() || from.Category != to.Category {
		return LateLevel
	}
	return LateRoutines
}

// ChangePlace is where an entry changing event would go in a timetable,
// and what that does.
type ChangePlace struct {
	Flight      int      // its new flight, by index; -1 for none
	Name        string   // e.g. "BUCS L5 · flight 2 of 3"
	Area, Start string   // where and when its warm-up starts
	Later       int      // minutes its new flight finishes later
	Moves       int      // later flights the change moves
	Problems    []string // what it would break that isn't broken now
}

// PlaceChange is where an entry would go, changing to an event (and
// category): the event's flight with the fewest entries, at the end of its
// running order; the timetable with it there; and what that changes. With
// no flight of the event, Flight is -1 and the timetable is as it was, less
// the entry.
func (s Schedule) PlaceChange(entryID, event, category string, people map[string][]string, names map[string]string) (Schedule, ChangePlace) {
	before := s.Problems(people, names)
	out := s
	out.Flights = make([]ScheduledFlight, len(s.Flights))
	for i, f := range s.Flights {
		f.Entries = slices.Clone(f.Entries)
		out.Flights[i] = f
	}
	best := -1
	for i, f := range out.Flights {
		if f.Event() != flightEvent(event, category) || slices.Contains(f.Entries, entryID) {
			continue
		}
		if best < 0 || len(f.Entries) < len(out.Flights[best].Entries) {
			best = i
		}
	}
	if best < 0 {
		out.RemoveEntry(entryID)
		out.Retime()
		return out, ChangePlace{Flight: -1}
	}
	target := out.Flights[best]
	wasEnd := target.End
	out.MoveEntry(entryID, best)
	place := ChangePlace{Flight: -1}
	for i, f := range out.Flights {
		if slices.Contains(f.Entries, entryID) {
			place.Flight, place.Name, place.Area = i, f.Name(), f.Area
			place.Start = Clock(f.Start)
			if f.Day < len(out.Setup.Days) {
				place.Start = out.Setup.Days[f.Day].Name + " " + place.Start
			}
			place.Later = f.End - wasEnd
		}
	}
	// A flight is the same flight before and after by who's in it (the
	// entry aside); those that start later have moved.
	key := func(f ScheduledFlight) string {
		ids := slices.DeleteFunc(slices.Clone(f.Entries), func(id string) bool { return id == entryID })
		return fmt.Sprint(f.Level, "|", f.Category, "|", ids)
	}
	starts := map[string]int{}
	for _, f := range s.Flights {
		starts[key(f)] = f.Day*1440 + f.Start
	}
	for i, f := range out.Flights {
		if was, ok := starts[key(f)]; ok && i != place.Flight && f.Day*1440+f.Start > was {
			place.Moves++
		}
	}
	for _, p := range out.Problems(people, names) {
		if !slices.Contains(before, p) {
			place.Problems = append(place.Problems, p)
		}
	}
	return out, place
}

// flightEvent is an event's name on its flights: the event, and its
// category where men and women fly separately.
func flightEvent(event, category string) string {
	if category == "" {
		return event
	}
	return fmt.Sprintf("%s %s", event, category)
}
