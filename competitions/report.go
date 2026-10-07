package competitions

import (
	"fmt"
	"slices"
)

// What a schedule falls short on, and what would fix it (ADR 0005 Decisions 9
// and 10).

// Report is how a schedule meets the soft constraints, and what it couldn't
// place.
type Report struct {
	Days           []DayReport
	Unplaced       []string // flights that fit nowhere
	UnplacedBlocks []string
	ShortRest      []ShortRest // people with less rest than wanted between turns
	Broken         []string    // prefer rules the schedule breaks
}

// DayReport is when a day finishes against its end, and how much time is
// free after its last flight.
type DayReport struct {
	Name          string
	Finish, End   string // Finish counts blocked time, such as awards at the end
	SpareMinutes  int    // after Finish, before the end; negative can't happen (the end is a must)
	FlightsPlaced int
	FlightsEnd    string `json:",omitempty"` // when the last flight ends; "" for none
	// FreeMinutes is the time after the last flight (or from the day's
	// start, with none) to the end, less blocked time that takes the whole
	// venue: what more flights could use.
	FreeMinutes int
}

// ShortRest is a person with less rest than wanted between two turns.
type ShortRest struct {
	Person        string // their key
	Minutes       int
	First, Second string // the flights
}

// Report reports on the schedule. people are each entry's people, by entry id.
func (s Schedule) Report(people map[string][]string) Report {
	var r Report
	for i, d := range s.Setup.Days {
		end, _ := clock(d.End)
		start, _ := clock(d.Start)
		last, n := start, 0
		for _, f := range s.Flights {
			if f.Day == i {
				n++
				last = max(last, f.End)
			}
		}
		flightsEnd := last
		for _, b := range s.Blocks {
			if b.Day == i {
				last = max(last, b.End)
			}
		}
		dr := DayReport{Name: d.Name, Finish: Clock(last), End: d.End, SpareMinutes: end - last, FlightsPlaced: n, FreeMinutes: end - flightsEnd}
		if n > 0 {
			dr.FlightsEnd = Clock(flightsEnd)
		}
		for _, b := range s.Blocks {
			if b.Day == i && s.wholeVenue(d, b) {
				dr.FreeMinutes -= max(0, min(b.End, end)-max(b.Start, flightsEnd))
			}
		}
		r.Days = append(r.Days, dr)
	}
	for _, f := range s.Unplaced {
		r.Unplaced = append(r.Unplaced, f.Name())
	}
	r.UnplacedBlocks = s.UnplacedBlocks

	// Each person's turns, in order.
	turns := map[string][]ScheduledFlight{}
	for _, f := range s.Flights {
		for _, id := range f.Entries {
			for _, p := range people[id] {
				turns[p] = append(turns[p], f)
			}
		}
	}
	var keys []string
	for p := range turns {
		keys = append(keys, p)
	}
	slices.Sort(keys)
	if s.Setup.Rest > 0 {
		for _, p := range keys {
			ts := turns[p]
			slices.SortFunc(ts, func(a, b ScheduledFlight) int { return (a.Day*1440 + a.Start) - (b.Day*1440 + b.Start) })
			for i := 1; i < len(ts); i++ {
				if ts[i].Day != ts[i-1].Day {
					continue
				}
				if gap := ts[i].Start - ts[i-1].End; gap < s.Setup.Rest {
					r.ShortRest = append(r.ShortRest, ShortRest{Person: p, Minutes: gap, First: ts[i-1].Name(), Second: ts[i].Name()})
				}
			}
		}
	}

	// Prefer rules broken.
	at := map[string][]ScheduledFlight{}
	for _, f := range s.Flights {
		at[f.Level] = append(at[f.Level], f)
	}
	for _, rule := range s.Setup.Rules {
		if rule.Must {
			continue
		}
		broken := false
		for _, f := range at[rule.Event] {
			switch rule.Kind {
			case RuleArea:
				broken = broken || f.Area != rule.Area
			case RuleDay:
				broken = broken || f.Day != rule.Day
			case RuleBefore:
				for _, g := range at[rule.Event2] {
					broken = broken || f.Day > g.Day || (f.Day == g.Day && f.End > g.Start)
				}
			case RuleApart:
				for _, g := range at[rule.Event2] {
					broken = broken || overlaps(interval{f.Day, f.Start, f.End}, interval{g.Day, g.Start, g.End})
				}
			}
		}
		if broken {
			r.Broken = append(r.Broken, rule.Describe(s.Setup.Days))
		}
	}
	return r
}

// Fix is a change to the setup (or a cap on an event's entries), and whether
// everything then fits.
type Fix struct {
	Change  string
	Fits    bool
	Setup   Setup
	entries []SchedEntry // the entries, if the change caps some
}

// Fixes tries changes that might make everything fit, one at a time: another
// area of a discipline, fewer minutes per competitor or between flights, larger
// flights, rest as a prefer rather than a must, or a cap on the entries of an
// event that doesn't fit, at as many as did.
func Fixes(entries []SchedEntry, eventOrder []string, setup Setup, seed uint64) []Fix {
	clone := func() Setup {
		c := setup
		c.Areas = slices.Clone(setup.Areas)
		c.Timings = map[string]Timings{}
		for k, v := range setup.Timings {
			c.Timings[k] = v
		}
		return c
	}
	var tries []Fix
	disciplines := map[string]bool{}
	for _, e := range entries {
		disciplines[e.Discipline] = true
	}
	for _, d := range AllDisciplines {
		if !disciplines[d] {
			continue
		}
		area := d
		if d == Synchro {
			if disciplines[Trampoline] {
				continue // the same panels
			}
			area = Trampoline
		}
		c := clone()
		n := 1
		for _, a := range c.Areas {
			if a.Discipline == area {
				n++
			}
		}
		name := map[string]string{Trampoline: "Panel", Tumbling: "Track", DMT: "DMT"}[area]
		c.Areas = append(c.Areas, Area{Name: fmt.Sprintf("%s %d (extra)", name, n), Discipline: area})
		tries = append(tries, Fix{Change: "another " + map[string]string{Trampoline: "trampoline panel", Tumbling: "tumbling track", DMT: "DMT"}[area], Setup: c})

		t := setup.TimingsFor(d)
		c = clone()
		less := t
		less.PerCompetitor = max(0.5, t.PerCompetitor-0.5)
		c.Timings[d] = less
		tries = append(tries, Fix{Change: fmt.Sprintf("%s at %g minutes per competitor, not %g", DisciplineName(d), less.PerCompetitor, t.PerCompetitor), Setup: c})

		if t.Between > 5 {
			c = clone()
			less := t
			less.Between = t.Between - 5
			c.Timings[d] = less
			tries = append(tries, Fix{Change: fmt.Sprintf("%s with %d minutes between flights, not %d", DisciplineName(d), less.Between, t.Between), Setup: c})
		}
		c = clone()
		more := t
		more.MaxFlight = t.MaxFlight + 3
		c.Timings[d] = more
		tries = append(tries, Fix{Change: fmt.Sprintf("%s flights of up to %d, not %d", DisciplineName(d), more.MaxFlight, t.MaxFlight), Setup: c})
	}
	if setup.RestMust && setup.Rest > 0 {
		c := clone()
		c.RestMust = false
		tries = append(tries, Fix{Change: "rest between turns as a prefer, not a must", Setup: c})
	}
	tries = append(tries, caps(entries, eventOrder, setup, seed)...)
	for i := range tries {
		with := entries
		if tries[i].entries != nil {
			with = tries[i].entries
		}
		s := PlanSchedule(with, eventOrder, tries[i].Setup, seed)
		tries[i].Fits = len(s.Unplaced) == 0 && len(s.UnplacedBlocks) == 0
	}
	return tries
}

// CoachClashes are coaches with gymnasts on two areas at once. coaches are
// each entry's coaches, by entry id; names name them.
func (s Schedule) CoachClashes(coaches map[string][]string, names map[string]string) []string {
	var out []string
	seen := map[string]bool{}
	for i, a := range s.Flights {
		for _, b := range s.Flights[i+1:] {
			if a.Area == b.Area || !overlaps(interval{a.Day, a.Start, a.End}, interval{b.Day, b.Start, b.End}) {
				continue
			}
			in := map[string]bool{}
			for _, id := range a.Entries {
				for _, c := range coaches[id] {
					in[c] = true
				}
			}
			for _, id := range b.Entries {
				for _, c := range coaches[id] {
					msg := fmt.Sprintf("%s coaches in %s (%s) and %s (%s) at once", names[c], a.Name(), a.Area, b.Name(), b.Area)
					if in[c] && !seen[msg] {
						seen[msg] = true
						out = append(out, msg)
					}
				}
			}
		}
	}
	return out
}

// caps are the fixes that cap an event that doesn't fit at the entries of
// it that did, dropping its last entries.
func caps(entries []SchedEntry, eventOrder []string, setup Setup, seed uint64) []Fix {
	planned := PlanSchedule(entries, eventOrder, setup, seed)
	short := map[string]bool{}
	for _, f := range planned.Unplaced {
		short[f.Level] = true
	}
	placed := map[string]int{}
	for _, f := range planned.Flights {
		placed[f.Level] += len(f.Entries)
	}
	var out []Fix
	for _, ev := range eventOrder {
		if !short[ev] {
			continue
		}
		total := 0
		for _, e := range entries {
			if e.Level == ev {
				total++
			}
		}
		n, kept := placed[ev], 0
		if n == 0 {
			continue // none of it fits: capping won't help
		}
		var capped []SchedEntry
		for _, e := range entries {
			if e.Level == ev {
				if kept == n {
					continue
				}
				kept++
			}
			capped = append(capped, e)
		}
		out = append(out, Fix{Change: fmt.Sprintf("%s capped at %d entries (%d fewer)", ev, n, total-n), Setup: setup, entries: capped})
	}
	return out
}

// wholeVenue says whether a block takes every area a day uses.
func (s Schedule) wholeVenue(d Day, b ScheduledBlock) bool {
	for _, a := range s.Setup.Areas {
		if d.uses(a.Name) && !slices.Contains(b.Areas, a.Name) {
			return false
		}
	}
	return true
}
