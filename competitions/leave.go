package competitions

import (
	"errors"
	"slices"
	"strings"
)

// What if an official has to leave: from a time on a day (for the rest of
// the day, or of the competition), each seat they held is filled again: by
// someone free, or by moving others round (someone on the panel up a role, or
// across from a panel at the same time), their seat filled in turn. Two ways,
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
	var seats []int
	for i, f := range out.Flights {
		if gone(f) && slices.ContainsFunc(f.Officials, func(d Duty) bool { return d.Person == l.Person }) {
			seats = append(seats, i)
		}
	}
	slices.SortFunc(seats, func(a, b int) int {
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
	for _, i := range seats {
		for seat, d := range out.Flights[i].Officials {
			if d.Person != l.Person {
				continue
			}
			out.Flights[i].Officials[seat].Person = ""
			f := out.Flights[i]
			fill := SeatFill{Flight: f.Name(), Area: f.Area, Day: f.Day, Start: f.Start, End: f.End, Role: d.Role}
			plan, others, ok := out.refill(i, seat, l, people, byKey, officials)
			if !ok {
				fill.Empty = d.Role
			} else {
				fill.Steps, fill.Others = plan.steps, others
				for _, mv := range plan.moves {
					out.Flights[mv.flight].Officials[mv.seat].Person = mv.person
				}
			}
			fills = append(fills, fill)
		}
	}
	return out, fills, nil
}

// seatMove puts a person in a seat on a flight's panel ("" to empty it).
type seatMove struct {
	flight, seat int
	person       string
}

// refillPlan is a way to fill a seat: who moves where.
type refillPlan struct {
	steps []SeatStep
	moves []seatMove // in order; a seat someone leaves is emptied, then filled by a later move
	last  string     // the role the newcomer takes
	ease  int        // how many free people could take that last seat
}

// refill finds the way to fill a flight's empty seat that the leave prefers:
// someone free taking it, or someone who can take it moving into it (from
// another seat on the panel, or from a panel at the same time) and their seat
// filled in turn, at most three moves. It also gives the others free who
// could take the last seat.
func (s Schedule) refill(i, seat int, l Leave, people map[string][]string, byKey map[string]RotaPerson, officials []RotaPerson) (refillPlan, []string, bool) {
	free := func(fi int, role string, ignore int) []string {
		f := s.Flights[fi]
		var out []string
		for _, o := range officials {
			if o.Key != l.Person && o.Can(role, f.Level) && s.freeIgnoring(o.Key, fi, ignore, people) {
				out = append(out, o.Key)
			}
		}
		slices.SortStableFunc(out, func(a, b string) int { return s.dutiesOn(a, f.Day) - s.dutiesOn(b, f.Day) })
		return out
	}
	label := func(fi int) string { return s.Flights[fi].Name() + " on " + s.Flights[fi].Area }
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
	var search func(fi, seat int, moved map[string]bool, steps []SeatStep, moves []seatMove)
	search = func(fi, seat int, moved map[string]bool, steps []SeatStep, moves []seatMove) {
		f := s.Flights[fi]
		role := f.Officials[seat].Role
		on := ""
		if fi != i {
			on = label(fi)
		}
		// Someone free takes it.
		if candidates := free(fi, role, -1); len(candidates) > 0 {
			p := refillPlan{
				steps: append(slices.Clone(steps), SeatStep{Person: candidates[0], To: role, On: on}),
				moves: append(slices.Clone(moves), seatMove{fi, seat, candidates[0]}),
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
		for gj, g := range s.Flights {
			if g.Day != f.Day || !(f.Start < g.End && g.Start < f.End) {
				continue
			}
			for other, d := range g.Officials {
				if d.Person == "" || d.Person == l.Person || moved[d.Person] || (gj == fi && (other == seat || d.Role == role)) || !byKey[d.Person].Can(role, f.Level) {
					continue
				}
				if gj != fi && !s.freeIgnoring(d.Person, fi, gj, people) {
					continue
				}
				nextMoved := map[string]bool{d.Person: true}
				for k := range moved {
					nextMoved[k] = true
				}
				step := SeatStep{Person: d.Person, From: d.Role, To: role, On: on}
				if gj != fi {
					step.Elsewhere = label(gj)
				}
				search(gj, other, nextMoved,
					append(slices.Clone(steps), step),
					append(slices.Clone(moves), seatMove{fi, seat, d.Person}, seatMove{gj, other, ""}))
			}
		}
	}
	search(i, seat, map[string]bool{}, nil, nil)
	if !found {
		return refillPlan{}, nil, false
	}
	// The last move fills the last seat emptied; earlier empties are filled
	// by later moves, so apply them in order.
	last := best.steps[len(best.steps)-1]
	lastMove := best.moves[len(best.moves)-1]
	others := slices.DeleteFunc(free(lastMove.flight, last.To, -1), func(k string) bool { return k == last.Person })
	return best, others[:min(len(others), 5)], true
}

// roleEase orders roles by how easy they usually are to fill: marshal and
// recorder (anyone helping), then HD and synchronisation, execution,
// difficulty, then chair.
func roleEase(role string) int {
	return map[string]int{RoleMarshal: 6, RoleRecorder: 6, RoleHD: 4, RoleSync: 4, RoleExecution: 3, RoleDifficulty: 2, RoleChair: 1}[role]
}

// freeIgnoring says whether a person is free for flight i: not competing or
// officiating in a flight at the same time (other than ignore, a flight they'd
// leave; -1 for none), nor kept off it by a must rule.
func (s Schedule) freeIgnoring(key string, i, ignore int, people map[string][]string) bool {
	f := s.Flights[i]
	for j, g := range s.Flights {
		if j == ignore || g.Day != f.Day || !(f.Start < g.End && g.Start < f.End) {
			continue
		}
		if j != i && slices.ContainsFunc(g.Officials, func(d Duty) bool { return d.Person == key }) {
			return false
		}
		if j == i && slices.ContainsFunc(g.Officials, func(d Duty) bool { return d.Person == key }) {
			return false // already on this panel
		}
		for _, id := range g.Entries {
			if slices.Contains(people[id], key) {
				return false
			}
		}
	}
	for _, rule := range s.Setup.Rules {
		if rule.Person == key && rule.Must {
			if broken, _ := s.breaksPersonRule(rule, f, key); broken {
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
