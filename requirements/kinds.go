package requirements

import (
	"cmp"
	"slices"
	"strings"
	"sync"

	"tariffCalculator/skills"
)

// Picks are what the rule editor offers to describe the skills a rule is
// about, so a coach picks one rather than writing it in quarters and half
// twists: kinds of skill ("any single somersault", "a barani"), then the
// named skills of the catalogue. Picking one fills in its Matcher; a
// Matcher of the coach's own is described field by field instead.

// Pick is a kind of skill, or one skill, as the editor offers it.
type Pick struct {
	ID, Label string
	Group     string // the editor's heading for it: "Kinds of skill", "Jumps and drops"…
	Match     Matcher
}

// Pick groups, in the editor's order.
const (
	KindsGroup   = "Kinds of skill"
	JumpsGroup   = "Jumps and drops"
	SinglesGroup = "Single somersaults"
	DoublesGroup = "Doubles and more"
)

func exactly(n int) *Range { return &Range{Min: &n, Max: &n} }
func atLeast(n int) *Range { return &Range{Min: &n} }
func between(lo, hi int) *Range {
	return &Range{Min: &lo, Max: &hi}
}

// kinds are the kinds of skill the editor offers, most used first.
var kinds = []Pick{
	{ID: "kind:somersault", Label: "Any somersault (¾ or more)", Match: Matcher{Rotation: atLeast(3)}},
	{ID: "kind:single", Label: "Any single somersault (¾ to 1¼)", Match: Matcher{Rotation: between(3, 5)}},
	{ID: "kind:double", Label: "Any double somersault (1¾ or more)", Match: Matcher{Rotation: atLeast(7)}},
	{ID: "kind:back", Label: "A back somersault, no twist", Match: Matcher{Rotation: exactly(4), Direction: "backward", Twist: exactly(0)}},
	{ID: "kind:front", Label: "A front somersault, no twist", Match: Matcher{Rotation: exactly(4), Direction: "forward", Twist: exactly(0)}},
	{ID: "kind:barani", Label: "A barani (front with a half twist)", Match: Matcher{Rotation: exactly(4), Direction: "forward", Twist: exactly(1)}},
	{ID: "kind:twisting", Label: "Any twisting somersault", Match: Matcher{Rotation: atLeast(3), Twist: atLeast(1)}},
	{ID: "kind:three-quarter", Label: "A ¾ somersault, to front or back", Match: Matcher{Rotation: exactly(3)}},
	{ID: "kind:twist-jump", Label: "Any twist jump (no somersault)", Match: Matcher{Rotation: exactly(0), Twist: atLeast(1), Takeoff: []string{"feet"}, Landing: []string{"feet"}}},
	{ID: "kind:seat", Label: "Anything landing on the seat", Match: Matcher{Landing: []string{"seat"}}},
	{ID: "kind:front-landing", Label: "Anything landing on the front", Match: Matcher{Landing: []string{"front"}}},
	{ID: "kind:back-landing", Label: "Anything landing on the back", Match: Matcher{Landing: []string{"back"}}},
}

// Picks are every kind of skill, then every named skill of the catalogue
// grouped by how much it somersaults, each group easiest first. Shared:
// don't change them.
var Picks = sync.OnceValue(func() []Pick {
	out := slices.Clone(kinds)
	for i := range out {
		out[i].Group = KindsGroup
	}
	var named []skills.TrampolineSkill
	seen := map[string]bool{}
	for _, s := range skills.CommonSkills {
		s.NormalizePhases()
		if s.Name == "Shape Jump" {
			for _, shape := range []skills.Shape{skills.Straight, skills.Tuck, skills.Pike, skills.Straddle} {
				jump := s
				jump.Shape, jump.Name = shape, shape.String()+" Jump"
				named = append(named, jump)
			}
			continue
		}
		if !seen[s.Name] {
			seen[s.Name] = true
			named = append(named, s)
		}
	}
	slices.SortFunc(named, func(a, b skills.TrampolineSkill) int {
		return cmp.Or(cmp.Compare(a.Rotation, b.Rotation), cmp.Compare(a.TotalTwist(), b.TotalTwist()), strings.Compare(a.Name, b.Name))
	})
	for _, s := range named {
		group := JumpsGroup
		switch {
		case s.Rotation >= 7:
			group = DoublesGroup
		case s.Rotation >= 3:
			group = SinglesGroup
		}
		out = append(out, Pick{ID: "skill:" + s.Name, Label: s.Name, Group: group, Match: SkillMatch(s)})
	}
	return out
})

// SkillMatch is a Matcher for one skill: its somersault, twist, direction
// (where it somersaults), take-off and landing. Any shape does, but where
// the shape is the skill (a tuck jump). Skills that differ only in where
// their twist is (a full-in and a full-out) are one.
func SkillMatch(s skills.TrampolineSkill) Matcher {
	m := Matcher{
		Rotation: exactly(s.Rotation), Twist: exactly(s.TotalTwist()),
		Takeoff: []string{strings.ToLower(s.TakeoffPosition.String())},
		Landing: []string{strings.ToLower(s.LandingPosition().String())},
	}
	if s.Rotation > 0 {
		m.Direction = "forward"
		if s.Backward {
			m.Direction = "backward"
		}
	}
	if s.IsBasicJump() {
		m.Shapes = []string{strings.ToLower(s.Shape.String())}
	}
	return m
}

// PickOf is the pick a Matcher is, by ID; "" for a Matcher of the coach's
// own (or none). Its wording doesn't count.
func PickOf(m Matcher) string {
	m.Label = ""
	for _, p := range Picks() {
		if sameMatcher(p.Match, m) {
			return p.ID
		}
	}
	return ""
}

// PickByID is a pick, by its ID.
func PickByID(id string) (Pick, bool) {
	i := slices.IndexFunc(Picks(), func(p Pick) bool { return p.ID == id })
	if i < 0 {
		return Pick{}, false
	}
	return Picks()[i], true
}

// sameMatcher says whether two matchers describe the same skills, field by
// field (a missing range and an empty one alike).
func sameMatcher(a, b Matcher) bool {
	sameRange := func(x, y *Range) bool {
		v := func(p *int) int {
			if p == nil {
				return -1
			}
			return *p
		}
		if x == nil {
			x = &Range{}
		}
		if y == nil {
			y = &Range{}
		}
		return v(x.Min) == v(y.Min) && v(x.Max) == v(y.Max)
	}
	sameTariff := func(x, y *TariffRange) bool {
		return (x == nil || x.Min == nil && x.Max == nil) && (y == nil || y.Min == nil && y.Max == nil)
	}
	return sameRange(a.Rotation, b.Rotation) && sameRange(a.Twist, b.Twist) && a.Direction == b.Direction &&
		slices.Equal(a.Shapes, b.Shapes) && slices.Equal(a.Takeoff, b.Takeoff) && slices.Equal(a.Landing, b.Landing) &&
		a.FIG == b.FIG && sameTariff(a.Tariff, b.Tariff)
}

// The rule editor names kinds of rule by what they mean, not by rule type:
// a count of at least one is "Must include", of none "Must not include".
// They're stored as the rule types, so saved requirements don't change.

// RuleKind is a kind of rule as the editor offers it.
type RuleKind struct{ ID, Label string }

// RuleKinds are the kinds of rule the editor offers, most used first. A set
// routine isn't one: it's built in the Routine Builder.
var RuleKinds = []RuleKind{
	{"include", "Must include"},
	{"exclude", "Must not include"},
	{"count", "How many of a kind (at least, at most)"},
	{"then", "This, then this (skills in a row)"},
	{Linked, "Linked pairs of a kind"},
	{Different, "No repeats"},
	{Elements, "Number of skills"},
	{Difficulty, "Difficulty between"},
	{Position, "A skill at a position"},
	{Every, "Every skill must be"},
	{Separate, "Special requirements (each met by a different skill)"},
}

// SetRoutineKind is a set routine's kind, offered only for one that is.
var SetRoutineKind = RuleKind{Sequence, "Set routine"}

// KindOfRule is the kind of rule a rule is, by ID.
func KindOfRule(r Rule) string {
	switch r.Type {
	case Count:
		switch {
		case r.Max != nil && *r.Max == 0 && (r.Min == nil || *r.Min == 0):
			return "exclude"
		case r.Min != nil && *r.Min == 1 && r.Max == nil:
			return "include"
		}
		return "count"
	case Includes:
		return "then"
	}
	return r.Type
}

// KindType is the rule type a kind of rule is stored as. A rule type is its
// own kind.
func KindType(kind string) string {
	switch kind {
	case "include", "exclude", "count":
		return Count
	case "then":
		return Includes
	}
	return kind
}

// Clone is the matcher with nothing shared, for a pick put into a rule.
func (m Matcher) Clone() Matcher {
	out := m
	copyRange := func(r *Range) *Range {
		if r == nil {
			return nil
		}
		c := Range{}
		if r.Min != nil {
			v := *r.Min
			c.Min = &v
		}
		if r.Max != nil {
			v := *r.Max
			c.Max = &v
		}
		return &c
	}
	out.Rotation, out.Twist = copyRange(m.Rotation), copyRange(m.Twist)
	if m.Tariff != nil {
		t := *m.Tariff
		out.Tariff = &t
	}
	out.Shapes, out.Takeoff, out.Landing = slices.Clone(m.Shapes), slices.Clone(m.Takeoff), slices.Clone(m.Landing)
	return out
}
