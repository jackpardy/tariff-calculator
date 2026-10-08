package requirements

import (
	"fmt"
	"math"
	"slices"
	"sync"

	"tariffCalculator/skills"
)

// Conflicts lists the set's rules that can't be met, alone or together: a
// description no skill fits, more elements than a routine can have, a skill
// that's required and banned, more of a skill than exist without repeating,
// difficulty out of reach. It catches the obvious cases, not every one; a set
// with none listed may still be impossible. The set should be valid (Validate)
// first.
func Conflicts(s Set) []string {
	c := conflicts{set: s}
	c.elementBounds()
	c.unmatchable()
	c.tooMany()
	c.everyAgainstNeeds()
	c.needsAgainstLimits()
	c.repeats()
	c.positions()
	c.difficulty()
	return c.out
}

type conflicts struct {
	set Set
	out []string
	// The routine's length as the rules allow it, and the rules that set it.
	minElements, maxElements int
	minFrom, maxFrom         int // rule index, or -1 for the Code of Points' 10
}

func (c *conflicts) add(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if !slices.Contains(c.out, msg) {
		c.out = append(c.out, msg)
	}
}

// one flags a rule that can't be met on its own.
func (c *conflicts) one(i int, format string, args ...any) {
	c.add("Rule %d (“%s”) can't be met: %s.", i+1, Describe(c.set.Rules[i]), fmt.Sprintf(format, args...))
}

// two flags two rules that can't both be met.
func (c *conflicts) two(i, j int, format string, args ...any) {
	i, j = min(i, j), max(i, j)
	c.add("Rules %d (“%s”) and %d (“%s”) can't both be met: %s.", i+1, Describe(c.set.Rules[i]), j+1, Describe(c.set.Rules[j]), fmt.Sprintf(format, args...))
}

// tooLong flags rule i needing n elements (what), more than the routine can have.
func (c *conflicts) tooLong(i, n int, what string) {
	if c.maxFrom < 0 {
		c.one(i, "it needs %d %s, but a routine has at most %d elements", n, what, skills.RoutineLength)
	} else {
		c.two(i, c.maxFrom, "%d %s are needed, but at most %d elements are allowed", n, what, c.maxElements)
	}
}

// elementBounds combines the number-of-elements rules and any set routine
// (exactly its length), and flags a minimum above a maximum.
func (c *conflicts) elementBounds() {
	c.minElements, c.maxElements, c.minFrom, c.maxFrom = 1, skills.RoutineLength, -1, -1
	for i, r := range c.set.Rules {
		lo, hi := r.Min, r.Max
		if r.Type == Sequence {
			n := float64(len(r.Sequence))
			lo, hi = &n, &n
		} else if r.Type != Elements {
			continue
		}
		if lo != nil && int(*lo) > c.minElements {
			c.minElements, c.minFrom = int(*lo), i
		}
		// A rule repeating the Code of Points' 10 is still named as the limit.
		if hi != nil && (int(*hi) < c.maxElements || int(*hi) == c.maxElements && c.maxFrom < 0) {
			c.maxElements, c.maxFrom = int(*hi), i
		}
	}
	if c.minElements > c.maxElements && c.minFrom >= 0 {
		c.tooLong(c.minFrom, c.minElements, "elements")
	}
}

// need is an element the rules require: what it must be, and the rule.
type need struct {
	m    Matcher
	rule int
	n    int // how many such elements, for a count rule
}

// needs are the elements each rule requires to be performed. An includes rule
// with several options needs none in particular, so only a single option counts.
func (c *conflicts) needs() []need {
	var out []need
	for i, r := range c.set.Rules {
		switch r.Type {
		case Count:
			if r.Min != nil && *r.Min >= 1 {
				out = append(out, need{*r.Match, i, int(*r.Min)})
			}
		case Position:
			out = append(out, need{*r.Match, i, 1})
		case Sequence:
			for _, m := range r.Sequence {
				out = append(out, need{m, i, 1})
			}
		case Separate:
			for _, m := range r.Each {
				out = append(out, need{m, i, 1})
			}
		case Includes:
			if len(r.Options) == 1 {
				for _, m := range r.Options[0] {
					out = append(out, need{m, i, 1})
				}
			}
		}
	}
	return out
}

// unmatchable flags a description that no skill fits, where the rule needs one.
func (c *conflicts) unmatchable() {
	for _, n := range c.needs() {
		if !fits(n.m) {
			c.one(n.rule, "no skill is %s", DescribeMatcher(n.m))
		}
	}
	for i, r := range c.set.Rules {
		switch r.Type {
		case Every:
			if !fits(*r.Match) {
				c.one(i, "no skill is %s", DescribeMatcher(*r.Match))
			}
		case Includes:
			if len(r.Options) > 1 && !slices.ContainsFunc(r.Options, func(o []Matcher) bool { return !slices.ContainsFunc(o, func(m Matcher) bool { return !fits(m) }) }) {
				c.one(i, "none of its options describes real skills")
			}
		}
	}
}

// tooMany flags rules needing more elements, or a later position, than the
// routine can have.
func (c *conflicts) tooMany() {
	for i, r := range c.set.Rules {
		n, what := 0, ""
		switch r.Type {
		case Count:
			if r.Min != nil {
				n, what = int(*r.Min), "elements"
			}
		case Linked:
			if r.Min != nil && *r.Min >= 1 {
				n, what = int(*r.Min)+1, "elements in a row"
			}
		case Separate:
			n, what = len(r.Each), "different elements"
		case Position:
			n, what = r.Position, "elements to reach that position"
		case Includes:
			n = math.MaxInt
			for _, o := range r.Options {
				n = min(n, len(o))
			}
			what = "elements in a row"
		}
		if n > c.maxElements && i != c.maxFrom {
			c.tooLong(i, n, what)
		}
	}
}

// everyAgainstNeeds flags a required element that can't also be what every
// element must be.
func (c *conflicts) everyAgainstNeeds() {
	for i, r := range c.set.Rules {
		if r.Type != Every {
			continue
		}
		for _, n := range c.needs() {
			if fits(n.m) && !anyBoth(n.m, *r.Match) {
				c.two(n.rule, i, "no skill is both %s and %s", DescribeMatcher(n.m), DescribeMatcher(*r.Match))
			}
		}
		for j, other := range c.set.Rules[i+1:] {
			if other.Type == Every && fits(*r.Match) && fits(*other.Match) && !anyBoth(*r.Match, *other.Match) {
				c.two(i, i+1+j, "no skill is both %s and %s", DescribeMatcher(*r.Match), DescribeMatcher(*other.Match))
			}
		}
	}
}

// needsAgainstLimits flags more of a kind of element required than a count
// rule allows, e.g. a required double back where doubles are banned.
func (c *conflicts) needsAgainstLimits() {
	for i, r := range c.set.Rules {
		if r.Type != Count || r.Max == nil {
			continue
		}
		limit := int(*r.Max)
		// Required elements that are all of the limited kind, per rule.
		perRule := map[int]int{}
		for _, n := range c.needs() {
			if n.rule != i && fits(n.m) && subset(n.m, *r.Match) {
				perRule[n.rule] += n.n
			}
		}
		for j, n := range perRule {
			if n > limit {
				if limit == 0 {
					c.two(j, i, "rule %d needs an element that rule %d doesn't allow", j+1, i+1)
				} else {
					c.two(j, i, "rule %d needs %d such elements, but rule %d allows at most %d", j+1, n, i+1, limit)
				}
			}
		}
		for j, e := range c.set.Rules {
			if e.Type == Every && fits(*e.Match) && subset(*e.Match, *r.Match) && c.minElements > limit {
				c.two(j, i, "every element would be one allowed at most %d times", limit)
			}
		}
	}
}

// repeats flags more of a skill required than exist without repeating one,
// when repeats aren't allowed.
func (c *conflicts) repeats() {
	noRepeats := !c.set.RepeatsAllowed || slices.ContainsFunc(c.set.Rules, func(r Rule) bool { return r.Type == Different })
	if !noRepeats {
		return
	}
	for _, n := range c.needs() {
		if n.n > 1 {
			if have := distinct(n.m, n.n); have > 0 && have < n.n {
				c.one(n.rule, "it needs %d elements that are %s, but only %s, and repeats aren't allowed", n.n, DescribeMatcher(n.m), skillCount(have))
			}
		}
	}
	for i, r := range c.set.Rules {
		if r.Type == Every && c.minElements > 1 {
			if have := distinct(*r.Match, c.minElements); have > 0 && have < c.minElements {
				c.one(i, "only %s, but the routine needs at least %d elements and repeats aren't allowed", skillCount(have), c.minElements)
			}
		}
	}
}

// positions flags two rules for the same position that no skill meets both of.
func (c *conflicts) positions() {
	at := map[int][]need{}
	for i, r := range c.set.Rules {
		switch r.Type {
		case Position:
			at[r.Position] = append(at[r.Position], need{*r.Match, i, 1})
		case Sequence:
			for p, m := range r.Sequence {
				at[p+1] = append(at[p+1], need{m, i, 1})
			}
		}
	}
	for p, ns := range at {
		for a := range ns {
			for _, b := range ns[a+1:] {
				if ns[a].rule != b.rule && fits(ns[a].m) && fits(b.m) && !anyBoth(ns[a].m, b.m) {
					c.two(ns[a].rule, b.rule, "both say what element %d is, and no skill fits both", p)
				}
			}
		}
	}
}

// difficulty flags a minimum above a maximum, or more difficulty than the
// routine could reach with its longest length and hardest allowed elements.
func (c *conflicts) difficulty() {
	var lo, hi *float64
	var loFrom, hiFrom int
	cap := math.Inf(1)
	for i, r := range c.set.Rules {
		if r.Type != Difficulty {
			continue
		}
		if r.Min != nil && (lo == nil || *r.Min > *lo) {
			lo, loFrom = r.Min, i
		}
		if r.Max != nil && (hi == nil || *r.Max < *hi) {
			hi, hiFrom = r.Max, i
		}
		if r.Cap != nil {
			cap = min(cap, *r.Cap)
		}
	}
	if lo == nil {
		return
	}
	if hi != nil && *lo > *hi+1e-9 {
		c.two(loFrom, hiFrom, "difficulty of at least %.1f, but at most %.1f", *lo, *hi)
		return
	}
	best := math.Inf(1)
	for _, r := range c.set.Rules {
		if r.Type == Every {
			best = min(best, hardest(*r.Match))
		}
	}
	if math.IsInf(best, 1) {
		best = hardest(Matcher{})
	}
	reach := float64(c.maxElements) * min(best, cap)
	if *lo > reach+1e-9 {
		c.one(loFrom, "%d elements counting at most %.1f each reach only %.1f", c.maxElements, min(best, cap), reach)
	}
}

// skillCount says how many different skills fit, e.g. "only 1 skill is".
func skillCount(n int) string {
	if n == 1 {
		return "1 skill fits"
	}
	return fmt.Sprintf("%d different skills fit", n)
}

// universe is a broad sample of real skills, for asking what a description
// can match: every rotation, twists up to 4 (in the first or last somersault),
// each take-off, shape and direction, landing on feet or seat.
var universe = sync.OnceValue(func() []skills.TrampolineSkill {
	var out []skills.TrampolineSkill
	shapes := []skills.Shape{skills.Straight, skills.Tuck, skills.Pike, skills.Straddle}
	takeoffs := []skills.BodyPosition{skills.Feet, skills.Front, skills.Back, skills.Seat}
	for rot := 0; rot <= skills.MaxRotation; rot++ {
		phases := skills.CalculatePhases(rot)
		for tw := 0; tw <= 8; tw++ {
			places := []int{phases - 1}
			if phases > 1 && tw > 0 {
				places = append(places, 0)
			}
			for _, place := range places {
				for _, takeoff := range takeoffs {
					for _, shape := range shapes {
						for _, backward := range []bool{false, true} {
							for _, seat := range []bool{false, true} {
								s := skills.TrampolineSkill{Rotation: rot, TakeoffPosition: takeoff, Shape: shape, Backward: backward, SeatLanding: seat}
								s.TwistDistribution = make([]int, phases)
								s.TwistDistribution[place] = tw
								if s.LandingPosition() == skills.Invalid {
									continue
								}
								s.SetTariff()
								out = append(out, s)
							}
						}
					}
				}
			}
		}
	}
	return out
})

// candidates are the skills to test a matcher against: the universe, plus the
// matcher's own example (which follows its FIG notation's twist by phase).
func candidates(m Matcher) []skills.TrampolineSkill {
	u := universe()
	if s, ok := m.Example(); ok {
		return append(u[:len(u):len(u)], s)
	}
	return u
}

// fits reports whether some skill fits the matcher.
func fits(m Matcher) bool {
	return slices.ContainsFunc(candidates(m), m.Matches)
}

// anyBoth reports whether some skill fits both matchers.
func anyBoth(a, b Matcher) bool {
	both := func(s skills.TrampolineSkill) bool { return a.Matches(s) && b.Matches(s) }
	return slices.ContainsFunc(candidates(a), both) || slices.ContainsFunc(candidates(b), both)
}

// subset reports whether every skill fitting a also fits b.
func subset(a, b Matcher) bool {
	return !slices.ContainsFunc(candidates(a), func(s skills.TrampolineSkill) bool { return a.Matches(s) && !b.Matches(s) })
}

// distinct counts the different skills (as repeats are judged) fitting the
// matcher, stopping at enough.
func distinct(m Matcher, enough int) int {
	var found []skills.TrampolineSkill
	for _, s := range candidates(m) {
		if !m.Matches(s) || slices.ContainsFunc(found, func(f skills.TrampolineSkill) bool { return f.Equal(&s) }) {
			continue
		}
		if found = append(found, s); len(found) >= enough {
			break
		}
	}
	return len(found)
}

// hardest is the highest tariff of a skill fitting the matcher.
func hardest(m Matcher) float64 {
	best := 0.0
	for _, s := range candidates(m) {
		if m.Matches(s) {
			best = max(best, s.Tariff)
		}
	}
	return best
}
