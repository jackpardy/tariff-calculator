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
