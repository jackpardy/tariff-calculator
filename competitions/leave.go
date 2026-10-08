package competitions

import (
	"errors"
	"slices"
	"strings"
)

// What if an official has to leave: from a time on a day (for the rest of
// the day, or of the competition), each seat they held is filled again: by
// someone free, or by moving others round (someone on the panel up a role, or
// across from a panel at the same time), their seat filled in turn. A seat is
// filled for the rest of its event's run, one person for all its flights
// (ADR 0006). Two ways,
// the organiser's choice: make the gap the easiest seat to fill, the seat left
// at the end one the most free people can take (a marshal or recorder before
// an HD or execution judge, before a chair); or change as few seats as
// possible. Nothing is saved.

// What a leave prefers when filling a seat.
const (
	PreferEasiest = ""       // the gap ends at the seat the most free people can take
	PreferFewest  = "fewest" // as few seats change as can be
)

// Leave is an official leaving.
type Leave struct {
	Person  string // their key
	Day     int
	From    int  // minutes since midnight
	ForGood bool // for the rest of the competition, not just the day
	Prefer  string
}

// SeatStep is one change: Person takes the To role, leaving From ("" if they
// weren't on a panel then), on another panel at the time if Elsewhere names it.
type SeatStep struct {
	Person    string
	From, To  string
	Elsewhere string // the other flight they leave, e.g. "BUCS L5 Men on Panel 1"
	On        string // the flight they take a seat on, when not the leaver's
}

// SeatFill is how one of the leaver's seats is filled.
type SeatFill struct {
	Flight          string
	Area            string
	Day, Start, End int
	Role            string     // the seat they leave
	Steps           []SeatStep // in order; none if it's left empty
	Empty           string     // the role left empty, if no one could take it
	Others          []string   // others free who could have taken the last seat, by key
}

// CheckLeave reports what's wrong with a leave.
func (s Schedule) CheckLeave(l Leave) error {
	var errs []error
	if l.Day < 0 || l.Day >= len(s.Setup.Days) {
		errs = append(errs, errors.New("choose a day"))
	}
	if l.Person == "" {
		errs = append(errs, errors.New("choose who leaves"))
	}
	if l.Prefer != PreferEasiest && l.Prefer != PreferFewest {
		errs = append(errs, errors.New("choose easiest to fill or fewest changes"))
	}
	return errors.Join(errs...)
}

// Left is the schedule with the official gone from their seats from the
// leave on, each seat filled again as the leave prefers, and how. people are
// each entry's people, by entry id; officials are who can officiate.
func (s Schedule) Left(l Leave, people map[string][]string, officials []RotaPerson) (Schedule, []SeatFill, error) {
	if err := s.CheckLeave(l); err != nil {
		return s, nil, err
	}
	out := s
	out.Flights = slices.Clone(s.Flights)
	for i := range out.Flights {
		out.Flights[i].Officials = slices.Clone(out.Flights[i].Officials)
	}
	gone := func(f ScheduledFlight) bool {
		if l.ForGood {
			return f.Day > l.Day || f.Day == l.Day && f.End > l.From
		}
		return f.Day == l.Day && f.End > l.From
	}
	var held []int
	for i, f := range out.Flights {
		if gone(f) && slices.ContainsFunc(f.Officials, func(d Duty) bool { return d.Person == l.Person }) {
			held = append(held, i)
		}
	}
	slices.SortFunc(held, func(a, b int) int {
		fa, fb := out.Flights[a], out.Flights[b]
		if fa.Day != fb.Day {
			return fa.Day - fb.Day
		}
		return fa.Start - fb.Start
	})
	byKey := map[string]RotaPerson{}
	for _, o := range officials {
		byKey[o.Key] = o
	}
	var fills []SeatFill
	for _, i := range held {
		for seat, d := range out.Flights[i].Officials {
			if d.Person != l.Person {
				continue
			}
			st := out.stretchFrom(i, seat) // the rest of the event, one fill
			for _, j := range st.flights {
				out.Flights[j].Officials[seat].Person = ""
			}
			iv := out.span(st)
			fill := SeatFill{Flight: out.stretchName(st), Area: out.Flights[i].Area, Day: iv.day, Start: iv.start, End: iv.end, Role: d.Role}
			plan, others, ok := out.refill(st, l, people, byKey, officials)
			if !ok {
				fill.Empty = d.Role
			} else {
				fill.Steps, fill.Others = plan.steps, others
				for _, mv := range plan.moves {
					for _, j := range mv.at.flights {
						out.Flights[j].Officials[mv.at.seat].Person = mv.person
					}
				}
			}
			fills = append(fills, fill)
		}
	}
	return out, fills, nil
}

// seatMove puts a person in a seat on a stretch's flights ("" to empty it).
type seatMove struct {
	at     stretch
	person string
}

// stretch is a seat on an event's panel over some of its run's flights, in
// order: what one person holds when they take it.
type stretch struct {
	flights []int
	seat    int
}

// span is when a stretch runs.
func (s Schedule) span(st stretch) interval {
	first, last := s.Flights[st.flights[0]], s.Flights[st.flights[len(st.flights)-1]]
	return interval{first.Day, first.Start, last.End}
}

// stretchFrom is the seat on flight i and the flights after it in its
// event's run (back to back on its area) that the same person holds.
func (s Schedule) stretchFrom(i, seat int) stretch {
	st := stretch{flights: []int{i}, seat: seat}
	person, role := s.Flights[i].Officials[seat].Person, s.Flights[i].Officials[seat].Role
	run := s.runOf(i)
	for k := slices.Index(run, i) + 1; k < len(run); k++ {
		prev, next := s.Flights[run[k-1]], s.Flights[run[k]]
		if next.Day != prev.Day || next.Area != prev.Area || seat >= len(next.Officials) || next.Officials[seat].Person != person || next.Officials[seat].Role != role {
			break
		}
		st.flights = append(st.flights, run[k])
	}
	return st
}

// label names a stretch's flights and area, e.g. "BUCS L5 Men on Panel 1".
func (s Schedule) label(st stretch) string {
	return s.stretchName(st) + " on " + s.Flights[st.flights[0]].Area
}

// stretchName names a stretch's flights: the flight, or its event for
// several.
func (s Schedule) stretchName(st stretch) string {
	if f := s.Flights[st.flights[0]]; len(st.flights) == 1 {
		return f.Name()
	} else {
		return f.Event()
	}
}

// refillPlan is a way to fill a seat: who moves where.
type refillPlan struct {
	steps []SeatStep
	moves []seatMove // in order; a seat someone leaves is emptied, then filled by a later move
	last  string     // the role the newcomer takes
	ease  int        // how many free people could take that last seat
}

// refill finds the way to fill an empty stretch of seat that the leave
// prefers: someone free for all of it taking it, or someone who can take it
// moving into it (from another seat on the panel, or from a panel at the
// same time, for the rest of their own stretch there) and their seat filled
// in turn, at most three moves. It also gives the others free who could take
// the last seat.
func (s Schedule) refill(first stretch, l Leave, people map[string][]string, byKey map[string]RotaPerson, officials []RotaPerson) (refillPlan, []string, bool) {
	free := func(st stretch, role string, ignore *stretch) []string {
		f := s.Flights[st.flights[0]]
		var out []string
		for _, o := range officials {
			if o.Key != l.Person && o.Can(role, f.Level) && s.freeIgnoring(o.Key, st, ignore, people) {
				out = append(out, o.Key)
			}
		}
		slices.SortStableFunc(out, func(a, b string) int { return s.dutiesOn(a, f.Day) - s.dutiesOn(b, f.Day) })
		return out
	}
	var best refillPlan
	found := false
	better := func(p refillPlan) bool {
		switch {
		case !found:
			return true
		case l.Prefer == PreferFewest && len(p.steps) != len(best.steps):
			return len(p.steps) < len(best.steps)
		case p.ease != best.ease:
			return p.ease > best.ease
		case len(p.steps) != len(best.steps):
			return len(p.steps) < len(best.steps)
		}
		return roleEase(p.last) > roleEase(best.last)
	}
	var search func(st stretch, moved map[string]bool, steps []SeatStep, moves []seatMove)
	search = func(st stretch, moved map[string]bool, steps []SeatStep, moves []seatMove) {
		f := s.Flights[st.flights[0]]
		role := f.Officials[st.seat].Role
		on := ""
		if st.flights[0] != first.flights[0] {
			on = s.label(st)
		}
		// Someone free takes it.
		if candidates := free(st, role, nil); len(candidates) > 0 {
			p := refillPlan{
				steps: append(slices.Clone(steps), SeatStep{Person: candidates[0], To: role, On: on}),
				moves: append(slices.Clone(moves), seatMove{st, candidates[0]}),
				last:  role, ease: len(candidates),
			}
			if better(p) {
				best, found = p, true
			}
		}
		if len(steps) >= 2 { // three moves at most
			return
		}
		// Someone who can take it moves into it: from another seat on this
		// panel, or from a panel at the same time, their seat filled in turn.
		iv := s.span(st)
		for gj, g := range s.Flights {
			if !overlaps(iv, interval{g.Day, g.Start, g.End}) {
				continue
			}
			own := slices.Contains(st.flights, gj)
			for other, d := range g.Officials {
				if d.Person == "" || d.Person == l.Person || moved[d.Person] || (own && (other == st.seat || d.Role == role)) || !byKey[d.Person].Can(role, f.Level) {
					continue
				}
				// Their seat from the first of its flights in this time on.
				if prev := s.before(gj); prev >= 0 && overlaps(iv, interval{s.Flights[prev].Day, s.Flights[prev].Start, s.Flights[prev].End}) &&
					other < len(s.Flights[prev].Officials) && s.Flights[prev].Officials[other].Person == d.Person {
					continue
				}
				if own && gj != st.flights[0] {
					continue // the panel's seat, from its first flight here
				}
				theirs := s.stretchFrom(gj, other)
				if !own && !s.freeIgnoring(d.Person, st, &theirs, people) {
					continue
				}
				nextMoved := map[string]bool{d.Person: true}
				for k := range moved {
					nextMoved[k] = true
				}
				step := SeatStep{Person: d.Person, From: d.Role, To: role, On: on}
				if !own {
					step.Elsewhere = s.label(theirs)
				}
				if own {
					theirs = stretch{flights: st.flights, seat: other}
				}
				search(theirs, nextMoved,
					append(slices.Clone(steps), step),
					append(slices.Clone(moves), seatMove{st, d.Person}, seatMove{theirs, ""}))
			}
		}
	}
	search(first, map[string]bool{}, nil, nil)
	if !found {
		return refillPlan{}, nil, false
	}
	// The last move fills the last seat emptied; earlier empties are filled
	// by later moves, so apply them in order.
	last := best.steps[len(best.steps)-1]
	lastMove := best.moves[len(best.moves)-1]
	others := slices.DeleteFunc(free(lastMove.at, last.To, nil), func(k string) bool { return k == last.Person })
	return best, others[:min(len(others), 5)], true
}

// before is the flight before i in its event's run, back to back on its
// area, or -1.
func (s Schedule) before(i int) int {
	run := s.runOf(i)
	k := slices.Index(run, i)
	if k < 1 || s.Flights[run[k-1]].Day != s.Flights[i].Day || s.Flights[run[k-1]].Area != s.Flights[i].Area {
		return -1
	}
	return run[k-1]
}

// roleEase orders roles by how easy they usually are to fill: marshal and
// recorder (anyone helping), then HD and synchronisation, execution,
// difficulty, then chair.
func roleEase(role string) int {
	return map[string]int{RoleMarshal: 6, RoleRecorder: 6, RoleHD: 4, RoleSync: 4, RoleExecution: 3, RoleDifficulty: 2, RoleChair: 1}[role]
}

// freeIgnoring says whether a person is free for a stretch: not competing
// or officiating in a flight at the same time (other than in ignore, a
// stretch they'd leave), nor already on its panel, nor kept off any of its
// flights by a must rule.
func (s Schedule) freeIgnoring(key string, st stretch, ignore *stretch, people map[string][]string) bool {
	iv := s.span(st)
	for j, g := range s.Flights {
		if !overlaps(iv, interval{g.Day, g.Start, g.End}) {
			continue
		}
		if (ignore == nil || !slices.Contains(ignore.flights, j)) && slices.ContainsFunc(g.Officials, func(d Duty) bool { return d.Person == key }) {
			return false
		}
		for _, id := range g.Entries {
			if slices.Contains(people[id], key) {
				return false
			}
		}
	}
	for _, rule := range s.Setup.Rules {
		if rule.Person != key || !rule.Must {
			continue
		}
		for _, i := range st.flights {
			if broken, _ := s.breaksPersonRule(rule, s.Flights[i], key); broken {
				return false
			}
		}
	}
	return true
}

// dutiesOn counts a person's seats on a day.
func (s Schedule) dutiesOn(key string, day int) int {
	n := 0
	for _, f := range s.Flights {
		if f.Day == day {
			for _, d := range f.Officials {
				if d.Person == key {
					n++
				}
			}
		}
	}
	return n
}

// Describe says a step, e.g. "Mary moves from execution judge to chair of
// judges", naming people with name.
func (st SeatStep) Describe(name func(string) string) string {
	to := strings.ToLower(RoleName(st.To))
	if st.On != "" {
		to += " on " + st.On
	}
	switch {
	case st.From == "":
		return name(st.Person) + " takes " + to
	case st.Elsewhere != "":
		return name(st.Person) + " moves from " + strings.ToLower(RoleName(st.From)) + " on " + st.Elsewhere + " to " + to
	}
	return name(st.Person) + " moves from " + strings.ToLower(RoleName(st.From)) + " to " + to
}
