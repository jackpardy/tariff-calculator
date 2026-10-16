package competitions

import (
	"fmt"
	"slices"
)

// What a timetable breaks, all of it in one place: for the "what ifs" to say
// what they'd newly break (roadmap 8), and a manual change what it causes
// (roadmap 9). The planner keeps all of it where it can; a change made by
// hand, or a "what if", may not.

// Breaks is everything the schedule breaks, each in words: a flight past its
// day's end; an event's flights apart; someone in two places at once (a
// gymnast, an official, an official on blocked time they staff); a coach with
// gymnasts on two areas at once, or competing while they coach; the
// organiser's rules about events and about people; and, where rest is a
// must, too little rest between someone's turns. people and coaches are each
// entry's people and coaches by entry id, keyed so one person has one key;
// names names those keys.
func (s Schedule) Breaks(people, coaches map[string][]string, names map[string]string) []string {
	return s.breaks(people, coaches, names, true)
}

// WhatIfBreaks is Breaks less what the "what ifs" report for themselves:
// flights past the end of the day, and people in two places at once.
func (s Schedule) WhatIfBreaks(people, coaches map[string][]string, names map[string]string) []string {
	return s.breaks(people, coaches, names, false)
}

func (s Schedule) breaks(people, coaches map[string][]string, names map[string]string, all bool) []string {
	var out []string
	seen := map[string]bool{}
	add := func(lines ...string) {
		for _, l := range lines {
			if !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	if all {
		add(s.pastEnd()...)
	}
	add(s.runProblems()...)
	if all {
		add(s.personClashes(people, names)...)
	}
	add(s.coachCompeting(people, coaches, names)...)
	add(s.CoachClashes(coaches, names)...)
	add(s.blockDutyClashes(names)...)
	for _, rule := range s.Setup.Rules {
		if broken := personRule(rule.Kind) && s.personRuleBroken(rule) || !personRule(rule.Kind) && s.eventRuleBroken(rule); broken {
			add("Breaks the rule: " + rule.Describe(s.Setup.Days))
		}
	}
	if s.Setup.RestMust && s.Setup.Rest > 0 {
		for _, sr := range s.Report(people).ShortRest {
			add(fmt.Sprintf("%s has %s between %s and %s (rest is a must: %d min)", nameOr(names, sr.Person), minutesWord(float64(sr.Minutes)), sr.First, sr.Second, s.Setup.Rest))
		}
	}
	return out
}

// Copy is a copy of the schedule to change, leaving this one as it is: its
// flights, blocks and their seats are its own.
func (s Schedule) Copy() Schedule {
	out := s
	copyFlights := func(in []ScheduledFlight) []ScheduledFlight {
		fs := slices.Clone(in)
		for i := range fs {
			fs[i].Entries, fs[i].Officials = slices.Clone(fs[i].Entries), slices.Clone(fs[i].Officials)
		}
		return fs
	}
	out.Flights, out.Unplaced = copyFlights(s.Flights), copyFlights(s.Unplaced)
	out.Blocks = slices.Clone(s.Blocks)
	for i := range out.Blocks {
		out.Blocks[i].Areas, out.Blocks[i].Officials = slices.Clone(out.Blocks[i].Areas), slices.Clone(out.Blocks[i].Officials)
	}
	out.UnplacedBlocks = slices.Clone(s.UnplacedBlocks)
	return out
}

// NewBreaks are the lines of after that before doesn't have: what a change
// breaks that wasn't broken already.
func NewBreaks(before, after []string) []string {
	var out []string
	for _, l := range after {
		if !slices.Contains(before, l) {
			out = append(out, l)
		}
	}
	return out
}

// blockDutyClashes are officials seated on a flight and on blocked time that
// needs officials (a judges' meeting, say) at once.
func (s Schedule) blockDutyClashes(names map[string]string) []string {
	var out []string
	for _, b := range s.Blocks {
		for _, bd := range b.Officials {
			if bd.Person == "" {
				continue
			}
			for _, f := range s.Flights {
				if !overlaps(interval{b.Day, b.Start, b.End}, interval{f.Day, f.Start, f.End}) {
					continue
				}
				if slices.ContainsFunc(f.Officials, func(d Duty) bool { return d.Person == bd.Person }) {
					out = append(out, fmt.Sprintf("%s officiates %s during %s, which they're on duty for", nameOr(names, bd.Person), f.Name(), b.Name))
				}
			}
		}
	}
	return out
}

// nameOr is a person's name, or "someone" for a key with none.
func nameOr(names map[string]string, key string) string {
	if n := names[key]; n != "" {
		return n
	}
	return "someone"
}

// FreeForSeat are the officials who could take a flight's seat, for the
// stretch of its event's run that seat covers, as an official leaving is
// replaced: allowed the role there, and free for all of it (not competing,
// officiating or on blocked-time duty then, not coaching someone competing
// elsewhere, rested where rest is a must, nor kept off by a must rule).
// Those with fewest duties that day come first; the seat's holder isn't one.
func (s Schedule) FreeForSeat(flight, seat int, people, coaches map[string][]string, officials []RotaPerson) []string {
	if flight < 0 || flight >= len(s.Flights) || seat < 0 || seat >= len(s.Flights[flight].Officials) {
		return nil
	}
	start := flight
	for prev := s.before(start); prev >= 0 && seat < len(s.Flights[prev].Officials) && s.Flights[prev].Officials[seat] == s.Flights[flight].Officials[seat]; prev = s.before(start) {
		start = prev
	}
	st := s.stretchFrom(start, seat)
	f, d := s.Flights[flight], s.Flights[flight].Officials[seat]
	var out []string
	for _, o := range officials {
		if o.Key != d.Person && o.Can(d.Role, f.Event) && s.freeIgnoring(o.Key, st, nil, people, coaches) {
			out = append(out, o.Key)
		}
	}
	slices.SortStableFunc(out, func(a, b string) int { return s.dutiesOn(a, f.Day) - s.dutiesOn(b, f.Day) })
	return out
}
