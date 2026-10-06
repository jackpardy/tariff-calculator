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

// DayReport is when a day finishes against its end.
type DayReport struct {
	Name          string
	Finish, End   string
	SpareMinutes  int // before the end; negative can't happen (the end is a must)
	FlightsPlaced int
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
		for _, b := range s.Blocks {
			if b.Day == i {
				last = max(last, b.End)
			}
		}
		r.Days = append(r.Days, DayReport{Name: d.Name, Finish: Clock(last), End: d.End, SpareMinutes: end - last, FlightsPlaced: n})
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

// Fix is a change to the setup, and whether everything then fits.
type Fix struct {
	Change string
	Fits   bool
	Setup  Setup
}

// Fixes tries changes that might make everything fit, one at a time: another
// area of a discipline, fewer minutes per competitor or between flights, larger
// flights, or rest as a prefer rather than a must.
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
	for i := range tries {
		s := PlanSchedule(entries, eventOrder, tries[i].Setup, seed)
		tries[i].Fits = len(s.Unplaced) == 0 && len(s.UnplacedBlocks) == 0
	}
	return tries
}
