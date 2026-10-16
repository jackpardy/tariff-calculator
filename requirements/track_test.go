package requirements

import (
	"strings"
	"testing"

	"tariffCalculator/skills"
)

// One of a club's own skills to track (Track) needs the skill, and is never
// checked against a routine: no result, and no conflict with other rules.
func TestTrackRules(t *testing.T) {
	one := 1.0
	set := Set{Format: Format, Name: "Grade 1", Rules: []Rule{
		{Type: Track, Item: "k1"},
		{Type: Count, Match: &Matcher{Rotation: &Range{Min: ptr(4), Max: ptr(4)}}, Min: &one},
		{Type: Track, Item: "k2", Label: "Handstand"},
	}}
	if err := set.Validate(); err != nil {
		t.Fatalf("valid: %v", err)
	}
	if err := (Set{Format: Format, Name: "x", Rules: []Rule{{Type: Track}}}).Validate(); err == nil {
		t.Error("a track rule needs the club's skill")
	}
	if got := Describe(set.Rules[0]); got != "One of the club's own skills" {
		t.Errorf("described: %q", got)
	}
	if got := Describe(set.Rules[2]); got != "Handstand" {
		t.Errorf("its wording: %q", got)
	}
	back := skills.TrampolineSkill{Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: skills.Feet, Backward: true}
	results := Evaluate(set, skills.ValidateRoutineWith([]skills.TrampolineSkill{back}, skills.ValidateOptions{}))
	if len(results) != 1 || results[0].Rule != 1 || !results[0].Passed {
		t.Errorf("only the somersault rule is checked, by its index: %+v", results)
	}
	if c := Conflicts(set); len(c) != 0 {
		t.Errorf("no conflicts: %v", c)
	}
}

// Elements may be repeated, and a rule that none may: the editor says one
// has to go.
func TestRepeatsAllowedAndNoRepeats(t *testing.T) {
	set := Set{Format: Format, Name: "x", RepeatsAllowed: true, Rules: []Rule{{Type: Different}}}
	if c := Conflicts(set); len(c) != 1 || !strings.Contains(c[0], "elements may be") {
		t.Errorf("flagged: %v", c)
	}
	set.RepeatsAllowed = false
	if c := Conflicts(set); len(c) != 0 {
		t.Errorf("no repeats either way: %v", c)
	}
}

// Skills that count needn't fit one description: a skill counts if it fits
// the matcher or any of its alternatives (Or).
func TestMatcherAlternatives(t *testing.T) {
	front, _ := PickByID("kind:front")
	barani, _ := PickByID("kind:barani")
	m := front.Match.Clone()
	m.Or = []Matcher{barani.Match.Clone()}
	frontSomersault := skills.TrampolineSkill{Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: skills.Feet}
	baraniSkill := skills.TrampolineSkill{Rotation: 4, TwistDistribution: []int{1}, TakeoffPosition: skills.Feet}
	back := skills.TrampolineSkill{Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: skills.Feet, Backward: true}
	if !m.Matches(frontSomersault) || !m.Matches(baraniSkill) || m.Matches(back) {
		t.Error("a front or a barani, not a back")
	}
	if got := DescribeMatcher(m); got != "a front somersault, no twist, or a barani (front with a half twist)" {
		t.Errorf("described: %q", got)
	}
	if PickOf(m) != "" {
		t.Error("with alternatives, it isn't one pick")
	}
	two := 2.0
	set := Set{Format: Format, Name: "x", Rules: []Rule{{Type: Count, Match: &m, Min: &two}}}
	if err := set.Validate(); err != nil {
		t.Fatal(err)
	}
	res := Evaluate(set, skills.ValidateRoutineWith([]skills.TrampolineSkill{frontSomersault, back, baraniSkill}, skills.ValidateOptions{}))
	if len(res) != 1 || !res[0].Passed || len(res[0].Elements) != 2 {
		t.Errorf("elements 1 and 3 count: %+v", res)
	}
	nested := m
	nested.Or = []Matcher{{Or: []Matcher{{}}}}
	if (Set{Format: Format, Name: "x", Rules: []Rule{{Type: Count, Match: &nested, Min: &two}}}).Validate() == nil {
		t.Error("an alternative has no alternatives of its own")
	}
}
