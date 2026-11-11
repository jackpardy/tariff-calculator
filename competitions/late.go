package competitions

import (
	"fmt"
	"slices"
)

// Late changes (roadmap 2026-10-08): after the deadline, a club or an
// individual can ask to change an entry, in the ways the organiser allows,
// each with its own fee, charged if it's accepted.

// The kinds of late request (late requests, 2026-11-11: a late new entry is
// one too).
const (
	LateNew      = "new"      // a new entry, once new entries have closed
	LateLevel    = "level"    // a different level, with its routines
	LateRoutines = "routines" // new routines at the same level
)

// LateKinds are the kinds of late request, in the order pages list them.
var LateKinds = []string{LateNew, LateLevel, LateRoutines}

// LateChangeKinds are the kinds that change an entry the competition has.
var LateChangeKinds = []string{LateLevel, LateRoutines}

// LateKindName names a kind of late request.
func LateKindName(kind string) string {
	switch kind {
	case LateNew:
		return "Late entry"
	case LateRoutines:
		return "Routine change"
	}
	return "Level change"
}

// LateRule is whether a kind of late request is allowed, and its fee. A late
// entry is allowed unless turned off (Off), so competitions made before it
// existed allow it too; the other kinds only once turned on.
type LateRule struct {
	On  bool `json:"on,omitempty"`
	Off bool `json:"off,omitempty"` // a late entry's: turned off
	Fee int  `json:"fee,omitempty"` // in cents, charged when accepted
}

// LateChanges are the late requests the organiser allows, by kind, and
// whether a coach must sign a request off before the organiser can accept
// it.
type LateChanges struct {
	Rules        map[string]LateRule `json:"rules,omitempty"`
	SignoffFirst bool                `json:"signoff_first,omitempty"`
}

// Rule is a kind's rule, On saying whether it's allowed.
func (l LateChanges) Rule(kind string) LateRule {
	r := l.Rules[kind]
	if kind == LateNew {
		r.On = !r.Off
	}
	return r
}

// Allowed says whether a kind of late request can be made.
func (l LateChanges) Allowed(kind string) bool { return l.Rule(kind).On }

// Any says whether any change to an entry the competition has can be asked
// for late.
func (l LateChanges) Any() bool {
	return slices.ContainsFunc(LateChangeKinds, l.Allowed)
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
		if f.Group() != (Flight{Event: event, Category: category}).Group() || slices.Contains(f.Entries, entryID) {
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
		return fmt.Sprint(f.Event, "|", f.Category, "|", ids)
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
