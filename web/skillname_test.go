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
