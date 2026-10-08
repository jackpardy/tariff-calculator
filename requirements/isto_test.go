package requirements

import (
	"math"
	"slices"
	"strings"
	"testing"

	"tariffCalculator/skills"
)

// Linked pairs are counted one straight after another, overlapping.
func TestLinked(t *testing.T) {
	back := skills.CommonSkills["backSomersault"]
	jump := skills.CommonSkills["shapeJump"]
	front := skills.CommonSkills["front"]
	barani := skills.CommonSkills["barani"]
	one := 1.0
	rule := Rule{Type: Linked, Match: &Matcher{Rotation: &Range{Min: ptr(3)}}, Max: &one}
	if err := rule.validate(); err != nil {
		t.Fatalf("a linked rule should be valid: %v", err)
	}
	if err := (Rule{Type: Linked, Match: &Matcher{}}).validate(); err == nil {
		t.Errorf("a linked rule needs a minimum or maximum")
	}
	if got := Describe(rule); got != "At most 1 linked pair, one straight after another: at least ¾ somersault" {
		t.Errorf("Describe = %q", got)
	}
	set := Set{Format: Format, Name: "x", Rules: []Rule{rule}}

	r := Evaluate(set, skills.ValidateRoutine([]skills.TrampolineSkill{back, jump, front, barani, jump}))[0]
	if !r.Passed || r.Detail != "found 1 pair (3–4)" || !slices.Equal(r.Elements, []int{3, 4}) {
		t.Errorf("one linked pair: got %+v", r)
	}
	// Three in a row are two pairs.
	r = Evaluate(set, skills.ValidateRoutine([]skills.TrampolineSkill{back, front, barani, jump}))[0]
	if r.Passed || r.Detail != "found 2 pairs (1–2, 2–3)" || !slices.Equal(r.Elements, []int{1, 2, 3}) {
		t.Errorf("three in a row: got %+v", r)
	}
}

// Each ISTO set routine also meets its level's voluntary requirements, which
// checks the voluntaries are written as the document means. Disability Level
// 5's set repeats a pike jump, so repeated as the voluntary (as the document
// allows) it isn't 10 different skills, and without the repeat its counted
// difficulty is 2.3, under the voluntary's 2.4.
func TestISTOSetsMeetVoluntaries(t *testing.T) {
	for _, l := range BuiltinLevels() {
		if !strings.HasPrefix(l.ID, "isto-") || l.Level.Second == nil {
			continue
		}
		vol, _ := LookupBuiltin(l.Level.Second.Options[0])
		for _, ref := range l.Level.First.Options {
			set, _ := LookupBuiltin(ref)
			routine, ok := SetRoutine(set)
			if !ok {
				t.Fatalf("%s: no set routine", ref)
			}
			for _, r := range Evaluate(vol, skills.ValidateRoutine(routine)) {
				l5 := ref == "builtin:isto-disability-l5-set"
				if !r.Passed && !(l5 && (r.Description == "10 different individual skills" || r.Detail == "is 2.3")) {
					t.Errorf("%s against %s: %s: %s", ref, vol.Name, r.Description, r.Detail)
				}
			}
		}
	}
}

// An Elite routine meeting every requirement passes; one with a twisting
// double, or no 270° to body landing, doesn't.
func TestISTOElite(t *testing.T) {
	c := func(key string, shape skills.Shape) skills.TrampolineSkill { return common(t, key, shape) }
	routine := []skills.TrampolineSkill{c("fullBack", skills.Straight), c("crashDive", skills.Straight), c("ballOut", skills.Tuck),
		c("barani", skills.Pike), c("backSomersault", skills.Pike), c("doubleBack", skills.Tuck), c("rudi", skills.Straight),
		c("backSomersault", skills.Tuck), c("front", skills.Pike), c("shapeJump", skills.Straddle)}
	checkBuiltin(t, "isto-elite", routine)

	failing := func(routine []skills.TrampolineSkill) []string {
		set, _ := LookupBuiltin("isto-elite")
		var out []string
		for _, r := range Evaluate(set, skills.ValidateRoutine(routine)) {
			if !r.Passed {
				out = append(out, r.Description)
			}
		}
		return out
	}
	twisting := slices.Clone(routine)
	twisting[5] = c("halfOut", skills.Tuck)
	if got := failing(twisting); !slices.Equal(got, []string{"A skill over 450° somersault has no twist"}) {
		t.Errorf("a half-out: failed %q", got)
	}
	noBodyLanding := slices.Clone(routine)
	noBodyLanding[1], noBodyLanding[2] = c("backSomersault", skills.Straight), c("barani", skills.Straight)
	if got := failing(noBodyLanding); !slices.Equal(got, []string{"270° somersault to a body landing, then a 450° somersault with at most 540° twist"}) {
		t.Errorf("no 270° to body landing: failed %q", got)
	}
}

// A level's Set A and Set B are the same difficulty, though it isn't scored.
func TestISTOSetsSameDifficulty(t *testing.T) {
	tariff := func(ref string) float64 {
		set, _ := LookupBuiltin(ref)
		routine, _ := SetRoutine(set)
		total := 0.0
		for _, s := range routine {
			total += s.Tariff
		}
		return total
	}
	for _, l := range BuiltinLevels() {
		if !strings.HasPrefix(l.ID, "isto-") || len(l.Level.First.Options) < 2 {
			continue
		}
		a, b := l.Level.First.Options[0], l.Level.First.Options[1]
		if ta, tb := tariff(a), tariff(b); math.Abs(ta-tb) > 1e-9 {
			t.Errorf("%s: %s is %.1f but %s is %.1f", l.ID, a, ta, b, tb)
		}
	}
}
