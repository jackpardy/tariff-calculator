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
	// TooLong says of each unplaced run that no day is long enough for it,
	// e.g. "BUCS L3 takes 10h 50m in one run: the longest day is 9h 00m".
	TooLong   []string
	ShortRest []ShortRest // people with less rest than wanted between turns
	Broken    []string    // prefer rules the schedule breaks
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
	for _, l := range s.longRuns() {
		r.TooLong = append(r.TooLong, fmt.Sprintf("%s takes %s in one run: the longest day is %s", l.group, hoursWord(l.minutes), hoursWord(s.longestDay())))
	}

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
	for _, rule := range s.Setup.Rules {
		if !rule.Must && s.eventRuleBroken(rule) {
			r.Broken = append(r.Broken, rule.Describe(s.Setup.Days))
		}
	}
	return r
}

// eventRuleBroken says whether the schedule breaks one of the organiser's
// rules about events (area, day, before, apart); false for a rule about a
// person.
func (s Schedule) eventRuleBroken(rule Rule) bool {
	broken := false
	for _, f := range s.Flights {
		if f.Event != rule.Event {
			continue
		}
		switch rule.Kind {
		case RuleArea:
			broken = broken || f.Area != rule.Area
		case RuleDay:
			broken = broken || f.Day != rule.Day
		case RuleBefore, RuleApart:
			for _, g := range s.Flights {
				if g.Event != rule.Event2 {
					continue
				}
				if rule.Kind == RuleBefore {
					broken = broken || f.Day > g.Day || (f.Day == g.Day && f.End > g.Start)
				} else {
					broken = broken || overlaps(interval{f.Day, f.Start, f.End}, interval{g.Day, g.Start, g.End})
				}
			}
		}
	}
	return broken
}

// longRun is an unplaced run that no day is long enough for.
type longRun struct {
	event, group string
	minutes      int
}

// longestDay is the length of the longest day, in minutes.
func (s Schedule) longestDay() int {
	longest := 0
	for _, d := range s.Setup.Days {
		start, err1 := clock(d.Start)
		end, err2 := clock(d.End)
		if err1 == nil && err2 == nil {
			longest = max(longest, end-start)
		}
	}
	return longest
}

// longRuns are the runs left unplaced that take longer than any day's hours
// (one run, one area, one day), so no other panel or time would help, in
// the order they were left.
func (s Schedule) longRuns() []longRun {
	longest := s.longestDay()
	var out []longRun
	at := map[string]int{}
	for _, f := range s.Unplaced {
		i, ok := at[f.Group()]
		if !ok {
			i = len(out)
			at[f.Group()] = i
			out = append(out, longRun{event: f.Event, group: f.Group()})
		}
		out[i].minutes += s.Setup.duration(f)
	}
	return slices.DeleteFunc(out, func(l longRun) bool { return l.minutes <= longest })
}

// hoursWord is minutes as hours and minutes, e.g. "10h 50m".
func hoursWord(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dh %02dm", minutes/60, minutes%60)
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
// area of a discipline, fewer minutes competing per competitor or shorter
// changeovers, larger flights, rest as a prefer rather than a must, events
// running across breaks, or a cap on the entries of an event that doesn't
// fit, at as many as would; and for an event whose run is longer than any
// day, men and women in separate flights, or a longer day.
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
		tries = append(tries, Fix{Change: fmt.Sprintf("%s competing at %s per competitor, not %g", DisciplineName(d), minutesWord(less.PerCompetitor), t.PerCompetitor), Setup: c})

		if t.Between > 5 {
			c = clone()
			less := t
			less.Between = t.Between - 5
			c.Timings[d] = less
			tries = append(tries, Fix{Change: fmt.Sprintf("%s with %s changeovers, not %d", DisciplineName(d), minutesWord(float64(less.Between)), t.Between), Setup: c})
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
	if !setup.AcrossBreaks && len(setup.Blocks) > 0 {
		c := clone()
		c.AcrossBreaks = true
		tries = append(tries, Fix{Change: "events can run across breaks", Setup: c})
	}
	tries = append(tries, longRunFixes(entries, eventOrder, setup, seed)...)
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

// longRunFixes are the fixes for an event whose run is longer than any day:
// where it flies men and women together, flying them separately (each is its
// own run, so each can go on a day of its own); and a longer day, the
// longest made as long as the longest run needs, up to the end of the day.
func longRunFixes(entries []SchedEntry, eventOrder []string, setup Setup, seed uint64) []Fix {
	planned := PlanSchedule(entries, eventOrder, setup, seed)
	runs := planned.longRuns()
	if len(runs) == 0 {
		return nil
	}
	var out []Fix
	for _, ev := range eventOrder {
		if !slices.ContainsFunc(runs, func(l longRun) bool { return l.event == ev }) || setup.Separate.Splits(ev) {
			continue
		}
		c := setup
		c.Separate = Split{Mode: SplitSome, Levels: append(slices.Clone(setup.Separate.Levels), ev)}
		out = append(out, Fix{Change: "men and women in separate flights for " + ev, Setup: c})
	}
	need := 0
	for _, l := range runs {
		need = max(need, l.minutes)
	}
	longest, at := 0, -1
	for i, d := range setup.Days {
		start, err1 := clock(d.Start)
		end, err2 := clock(d.End)
		if err1 == nil && err2 == nil && end-start > longest {
			longest, at = end-start, i
		}
	}
	if at >= 0 {
		start, _ := clock(setup.Days[at].Start)
		end := min(start+(need+14)/15*15, 24*60-1)
		c := setup
		c.Days = slices.Clone(setup.Days)
		c.Days[at].End = Clock(end)
		out = append(out, Fix{Change: fmt.Sprintf("a longer day: %s to %s, not %s", setup.Days[at].Name, Clock(end), setup.Days[at].End), Setup: c})
	}
	return out
}

// caps are the fixes that cap an event that doesn't fit at the most entries
// that would, dropping its last entries. An event goes whole or not at all
// (ADR 0006), so they're found by trying a flight's worth fewer at a time.
func caps(entries []SchedEntry, eventOrder []string, setup Setup, seed uint64) []Fix {
	planned := PlanSchedule(entries, eventOrder, setup, seed)
	short := map[string]bool{}
	for _, f := range planned.Unplaced {
		short[f.Event] = true
	}
	var out []Fix
	for _, ev := range eventOrder {
		if !short[ev] {
			continue
		}
		total, discipline := 0, ""
		for _, e := range entries {
			if e.Level == ev {
				total++
				discipline = e.Discipline
			}
		}
		step := setup.TimingsFor(discipline).MaxFlight
		for n, tries := total-step, 0; n > 0 && tries < 8; n, tries = n-step, tries+1 {
			capped := capAt(entries, ev, n)
			fitted := true
			for _, f := range PlanSchedule(capped, eventOrder, setup, seed).Unplaced {
				fitted = fitted && f.Event != ev
			}
			if fitted {
				out = append(out, Fix{Change: fmt.Sprintf("%s capped at %d entries (%d fewer)", ev, n, total-n), Setup: setup, entries: capped})
				break
			}
		}
	}
	return out
}

// capAt is the entries with an event's kept to its first n.
func capAt(entries []SchedEntry, event string, n int) []SchedEntry {
	var out []SchedEntry
	kept := 0
	for _, e := range entries {
		if e.Level == event {
			if kept == n {
				continue
			}
			kept++
		}
		out = append(out, e)
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

// minutesWord is a number of minutes in words: "1 minute", "2.5 minutes".
func minutesWord(n float64) string {
	if n == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%g minutes", n)
}
