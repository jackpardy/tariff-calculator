package web

import (
	"testing"

	"tariffCalculator/catalog"
)

// Skills a search finds by notation that have no name are told apart by where
// they take off and land and which way they turn.
func TestSkillNamesApart(t *testing.T) {
	for _, q := range []string{"41/", "6 0 /", "41o"} {
		seen := map[string]bool{}
		for _, r := range catalog.Search(q) {
			name := skillName(r.Skill)
			if seen[name] {
				t.Errorf("%s: two results called %q", q, name)
			}
			seen[name] = true
		}
	}
}

// A skill with no notation (a seat drop) keeps its name whole beside where
// its notation would be.
func TestSkillNameBesideWithoutNotation(t *testing.T) {
	for _, r := range catalog.Search("seat drop") {
		if r.Skill.FIGNotation() == "" {
			if got := skillNameBeside(r.Skill); got != skillName(r.Skill) {
				t.Errorf("%q, not %q", got, skillName(r.Skill))
			}
			return
		}
	}
	t.Skip("no seat drop without notation in the catalog")
}
