package requirements

import (
	"errors"
	"slices"
	"testing"

	"tariffCalculator/skills"
)

func TestChecksFor(t *testing.T) {
	if got := ChecksFor(nil, ""); got != (Checks{ScoreDifficulty: true, FlagRepeats: true}) {
		t.Errorf("no requirements: every check, got %+v", got)
	}
	set := Set{NoDifficulty: true, RepeatsAllowed: true}
	if got := ChecksFor(&set, ""); got.ScoreDifficulty || got.FlagRepeats {
		t.Errorf("a set routine scores no difficulty and may repeat, got %+v", got)
	}
	ag3, _ := LookupBuiltin("builtin:fig-ag3-first")
	if got := ChecksFor(&ag3, ""); got.ScoredElements != 2 {
		t.Errorf("AG3's first exercise scores 2 elements, got %+v", got)
	}
	if got := ChecksFor(&ag3, `{"scored":0,"repeats":false}`); got.ScoredElements != 0 || got.FlagRepeats {
		t.Errorf("the routine's own checks win, got %+v", got)
	}
	if got := ChecksFor(&ag3, `{"scored":11}`); got.ScoredElements != 2 {
		t.Errorf("more than ten scored elements is ignored, got %+v", got)
	}
}

func TestResolve(t *testing.T) {
	if set, err := ResolveSet(" builtin:fig-ag3-first "); err != nil || set.Name == "" {
		t.Errorf("built-in set: %v", err)
	}
	if _, err := ResolveSet("builtin:nope"); err == nil {
		t.Error("a missing built-in set is an error")
	}
	if set, err := ResolveSet(`{"format":1,"name":"Mine","rules":[]}`); err != nil || set.Name != "Mine" {
		t.Errorf("custom set: %+v, %v", set, err)
	}
	if level, err := ResolveLevel("builtin-level:fig-ag3"); err != nil || level.Name == "" {
		t.Errorf("built-in level: %v", err)
	}
	if level, err := ResolveLevel("builtin-level:nope"); err == nil || level.Name != "Level" {
		t.Errorf("a missing built-in level is an error, named Level: %+v", level)
	}
}

func TestCheck(t *testing.T) {
	stale := rudi
	stale.Name, stale.Tariff = "Old name", 9
	set, _ := LookupBuiltin("builtin:fig-ag3-second")
	c := Check(Routine{Skills: []skills.TrampolineSkill{stale, rudi}, Set: &set}, nil)
	if c.Validation.Skills[0].Skill.Name == "Old name" || c.Validation.Skills[0].Skill.Tariff != 0.8 {
		t.Errorf("names and tariffs are worked out again: %+v", c.Validation.Skills[0].Skill)
	}
	if !c.HasSet || c.SetName != set.Name || len(c.Results) == 0 {
		t.Errorf("checked against its requirements: %+v", c)
	}
	if !c.Validation.Skills[1].IsDuplicate {
		t.Error("repeats are flagged")
	}

	broken := Check(Routine{SetErr: errors.New("bad"), Checks: `{"repeats":false}`}, nil)
	if !broken.HasSet || broken.SetName != "Requirements" || broken.SetErr == nil || broken.Results != nil {
		t.Errorf("requirements that can't be used are reported, not checked: %+v", broken)
	}
	if broken.Checks.FlagRepeats {
		t.Error("the routine's own checks still apply")
	}
	if none := Check(Routine{}, nil); none.HasSet {
		t.Error("no requirements")
	}
}

func TestCheckPair(t *testing.T) {
	backTuck := skill(4, []int{0}, skills.Tuck, true)
	fullBack := skill(4, []int{2}, skills.Straight, true)
	first, _ := LookupBuiltin("builtin:fig-ag3-first")
	second, _ := LookupBuiltin("builtin:fig-ag3-second")
	// The full back and Rudi score in the first exercise and carry over.
	one := Routine{Skills: []skills.TrampolineSkill{backTuck, baraniTuck, fullBack, rudi}, Set: &first}
	two := Routine{Skills: []skills.TrampolineSkill{rudi, backTuck}, Set: &second}

	p := CheckPair(one, two)
	if !slices.Equal(p.Carried, []int{3, 4}) || !slices.Equal(p.Repeated, []int{1}) {
		t.Errorf("carried %v, repeated %v", p.Carried, p.Repeated)
	}
	if got := p.Second.Validation.TotalTariff; got != 0.5 {
		t.Errorf("the repeated Rudi scores nothing: total %.2f, want 0.50", got)
	}

	// When the coach scores every element of the first exercise, nothing carries over.
	one.Checks = `{"scored":0}`
	if p := CheckPair(one, two); p.Carried != nil || p.Repeated != nil || p.Second.Validation.TotalTariff != 1.3 {
		t.Errorf("nothing carries over: %+v", p)
	}
}
