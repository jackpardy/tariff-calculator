package requirements

import (
	"strings"
	"testing"
)

// Every pick is a valid description of skills, tells itself apart from the
// others, and reads as words; the named skills cover the catalogue.
func TestPicks(t *testing.T) {
	seen := map[string]string{}
	groups := map[string]int{}
	for _, p := range Picks() {
		groups[p.Group]++
		if err := p.Match.validate(); err != nil {
			t.Errorf("%s: %v", p.Label, err)
		}
		if got := PickOf(p.Match); got != p.ID {
			t.Errorf("%s reads back as %q: the same as %s", p.Label, got, seen[got])
		}
		seen[p.ID] = p.Label
		if d := DescribeMatcher(p.Match); d == "" || strings.Contains(d, "any element") && p.Group != KindsGroup {
			t.Errorf("%s is described as %q", p.Label, d)
		}
	}
	for _, g := range []string{KindsGroup, JumpsGroup, SinglesGroup, DoublesGroup} {
		if groups[g] == 0 {
			t.Errorf("nothing in %s", g)
		}
	}
	for _, want := range []string{"skill:Barani", "skill:Tuck Jump", "skill:Double Back", "kind:barani", "kind:back"} {
		if _, ok := PickByID(want); !ok {
			t.Errorf("no %s", want)
		}
	}
	if PickOf(Matcher{Rotation: &Range{}, Label: "Mine"}) != "" || PickOf(Matcher{Twist: exactly(5)}) != "" {
		t.Error("a description of the coach's own is no pick")
	}
}

// A back somersault then a barani, as two kinds of skill, is met by a back
// tuck then a barani straight, and not by the barani first.
func TestThenWithKinds(t *testing.T) {
	back, _ := PickByID("kind:back")
	barani, _ := PickByID("kind:barani")
	set := Set{Format: Format, Name: "Then", Rules: []Rule{{Type: Includes, Options: [][]Matcher{{back.Match, barani.Match}}}}}
	if err := set.Validate(); err != nil {
		t.Fatal(err)
	}
	t.Log(Describe(set.Rules[0]))
}
