package competitions

import (
	"fmt"
	"slices"
)

// What a timetable breaks, all of it in one place: for the "what ifs" to say
// what they'd newly break (roadmap 8), and a manual change what it causes
// (roadmap 9). The planner keeps all of it where it can; a change made by
// hand, or a "what if", may not.

// Break is one thing a schedule breaks: the line that says it, whose it is
// (a person's key, or "" for no one's), and how serious it is.
type Break struct {
	Line   string
	Person string
	Rank   int // TwoPlaces first, then MustBreak, then PreferBreak
}

// How serious a break is, most first.
const (
	TwoPlaces   = iota // someone in two places at once
	MustBreak          // past the day's end, an event's flights apart, a must rule or rest broken, a seat its holder can't have
	PreferBreak        // a preference not met
)

// Breaks is everything the schedule breaks, each in words: someone in two
// places at once (a gymnast, an official, an official on blocked time they
// staff); a flight past its day's end; an event's flights apart; the
// organiser's rules about events and about people; and, where rest is a
// must, too little rest between someone's turns. people are each entry's
// people by entry id, keyed so one person has one key; names names those
// keys. Coaches aren't checked: where a coach's gymnasts are is for the coach
// to manage.
func (s Schedule) Breaks(people map[string][]string, names map[string]string) []Break {
	return s.breaks(people, names, true)
}

// WhatIfBreaks is Breaks less what the "what ifs" report for themselves:
// flights past the end of the day, and people in two places at once.
func (s Schedule) WhatIfBreaks(people map[string][]string, names map[string]string) []Break {
	return s.breaks(people, names, false)
}

func (s Schedule) breaks(people map[string][]string, names map[string]string, all bool) []Break {
	var out []Break
	seen := map[string]bool{}
	add := func(bs ...Break) {
		for _, b := range bs {
			if !seen[b.Line] {
				seen[b.Line] = true
				out = append(out, b)
			}
		}
	}
	lines := func(rank int, ls []string) []Break {
		var bs []Break
		for _, l := range ls {
			bs = append(bs, Break{Line: l, Rank: rank})
		}
		return bs
	}
	if all {
		add(s.personClashes(people, names)...)
	}
	add(s.blockDutyClashes(names)...)
	if all {
		add(lines(MustBreak, s.pastEnd())...)
	}
	add(lines(MustBreak, s.runProblems())...)
	for _, rule := range s.Setup.Rules {
		if broken := personRule(rule.Kind) && s.personRuleBroken(rule) || !personRule(rule.Kind) && s.eventRuleBroken(rule); broken {
			b := Break{Line: "Breaks the rule: " + rule.Describe(s.Setup.Days), Person: rule.Person, Rank: PreferBreak}
			if rule.Must {
				b.Rank = MustBreak
			}
			add(b)
		}
	}
	if s.Setup.RestMust && s.Setup.Rest > 0 {
		for _, sr := range s.Report(people).ShortRest {
			add(Break{Line: fmt.Sprintf("%s has %s between %s and %s (rest is a must: %d min)", nameOr(names, sr.Person), minutesWord(float64(sr.Minutes)), sr.First, sr.Second, s.Setup.Rest), Person: sr.Person, Rank: MustBreak})
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

// NewBreaks are the breaks of after that before doesn't have: what a change
// breaks that wasn't broken already.
func NewBreaks(before, after []Break) []Break {
	var out []Break
	for _, b := range after {
		if !slices.ContainsFunc(before, func(o Break) bool { return o.Line == b.Line }) {
			out = append(out, b)
		}
	}
	return out
}

// BreakLines are breaks in words, the most serious first (people in two
// places at once), each person's breaks of one rank folded into the line of
// their first with how many more, e.g. "Ann is in L1 and L2 at once, and 3
// more times".
func BreakLines(bs []Break) []string {
	bs = slices.Clone(bs)
	slices.SortStableFunc(bs, func(a, b Break) int { return a.Rank - b.Rank })
	type whose struct {
		rank   int
		person string
	}
	more := map[whose]int{}
	var keep []Break
	for _, b := range bs {
		w := whose{b.Rank, b.Person}
		if _, seen := more[w]; seen && b.Person != "" {
			more[w]++
			continue
		}
		more[w] = 0
		keep = append(keep, b)
	}
	out := make([]string, len(keep))
	for i, b := range keep {
		out[i] = b.Line
		if n := more[whose{b.Rank, b.Person}]; b.Person != "" && n > 0 {
			out[i] += fmt.Sprintf(", and %d more time%s", n, plural(n))
		}
	}
	return out
}

// blockDutyClashes are officials seated on a flight and on blocked time that
// needs officials (a judges' meeting, say) at once.
func (s Schedule) blockDutyClashes(names map[string]string) []Break {
	var out []Break
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
					out = append(out, Break{Line: fmt.Sprintf("%s officiates %s during %s, which they're on duty for", nameOr(names, bd.Person), f.Name(), b.Name), Person: bd.Person, Rank: TwoPlaces})
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
// officiating or on blocked-time duty then, rested where rest is a must, nor
// kept off by a must rule), in the order sortForSeat gives; the seat's holder
// isn't one.
func (s Schedule) FreeForSeat(flight, seat int, people map[string][]string, officials []RotaPerson) []string {
	if flight < 0 || flight >= len(s.Flights) || seat < 0 || seat >= len(s.Flights[flight].Officials) {
		return nil
	}
	start := flight
	for prev := s.before(start); prev >= 0 && seat < len(s.Flights[prev].Officials) && s.Flights[prev].Officials[seat].same(s.Flights[flight].Officials[seat]); prev = s.before(start) {
		start = prev
	}
	st := s.stretchFrom(start, seat)
	f, d := s.Flights[flight], s.Flights[flight].Officials[seat]
	var out []string
	for _, o := range officials {
		if o.Key != d.Person && o.Can(d.Role, f.Event) && s.freeIgnoring(o.Key, st, nil, people) {
			out = append(out, o.Key)
		}
	}
	s.sortForSeat(out, d.Role, f.Day)
	return out
}
