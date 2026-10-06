package requirements

import (
	"strings"
	"testing"
)

func TestConflicts(t *testing.T) {
	doubles := Matcher{Rotation: &Range{Min: ptr(8), Max: ptr(11)}}
	doubleBack := Matcher{Label: "Double back", Rotation: &Range{Min: ptr(8), Max: ptr(8)}, Twist: &Range{Max: ptr(0)}, Direction: "backward"}
	tuckBack := Matcher{Label: "Tuck back", FIG: "4 - o", Direction: "backward", Takeoff: []string{"feet"}, Landing: []string{"feet"}}
	cases := []struct {
		name  string
		set   Set
		wants []string // a word or two from each expected message; none: no conflicts
	}{
		{"too many elements", Set{Rules: []Rule{{Type: Elements, Min: ptr(12.0)}}}, []string{"Rule 1 (“At least 12 elements”) can't be met: it needs 12 elements, but a routine has at most 10 elements."}},
		{"elements min over max", Set{Rules: []Rule{{Type: Elements, Min: ptr(8.0)}, {Type: Elements, Max: ptr(6.0)}}}, []string{"Rules 1 (“At least 8 elements”) and 2 (“At most 6 elements”) can't both be met: 8 elements are needed, but at most 6 elements are allowed."}},
		{"required and banned", Set{Rules: []Rule{
			{Type: Separate, Each: []Matcher{doubleBack}},
			{Type: Count, Max: ptr(0.0), Match: &doubles},
		}}, []string{"rule 1 needs an element that rule 2 doesn't allow"}},
		{"every against a need", Set{Rules: []Rule{
			{Type: Every, Match: &Matcher{Twist: &Range{Max: ptr(0)}}},
			{Type: Count, Min: ptr(1.0), Match: &Matcher{Twist: &Range{Min: ptr(2)}}},
		}}, []string{"Rules 1 (“Every element: no twist”) and 2", "no skill is both at least 1 twist and no twist"}},
		{"no such skill", Set{Rules: []Rule{
			{Type: Count, Min: ptr(1.0), Match: &Matcher{Rotation: &Range{Max: ptr(0)}, Twist: &Range{Max: ptr(0)}, Takeoff: []string{"feet"}, Landing: []string{"back"}}},
		}}, []string{"can't be met: no skill is at most 0 somersaults, no twist, from feet, landing on back."}},
		{"repeats", Set{Rules: []Rule{{Type: Count, Min: ptr(2.0), Match: &tuckBack}}}, []string{"it needs 2 elements that are Tuck back, but only 1 skill fits, and repeats aren't allowed"}},
		{"repeats allowed", Set{RepeatsAllowed: true, Rules: []Rule{{Type: Count, Min: ptr(2.0), Match: &tuckBack}}}, nil},
		{"position past the end", Set{Rules: []Rule{{Type: Position, Position: 11, Match: &Matcher{}}}}, []string{"needs 11 elements to reach that position"}},
		{"two rules for one position", Set{Rules: []Rule{
			{Type: Position, Position: 1, Match: &Matcher{Direction: "forward", Rotation: &Range{Min: ptr(4)}}},
			{Type: Position, Position: 1, Match: &Matcher{Direction: "backward"}},
		}}, []string{"Rules 1", "and 2", "both say what element 1 is"}},
		{"difficulty min over max", Set{Rules: []Rule{{Type: Difficulty, Min: ptr(5.0)}, {Type: Difficulty, Max: ptr(4.0)}}}, []string{"difficulty of at least 5.0, but at most 4.0"}},
		{"difficulty out of reach", Set{Rules: []Rule{{Type: Difficulty, Min: ptr(6.0), Cap: ptr(0.5)}}}, []string{"10 elements counting at most 0.5 each reach only 5.0"}},
		{"special requirements past the limit", Set{Rules: []Rule{
			{Type: Elements, Max: ptr(2.0)},
			{Type: Separate, Each: []Matcher{{}, {}, {}}},
		}}, []string{"3 different elements are needed, but at most 2 elements are allowed"}},
		{"fine", Set{Rules: []Rule{
			{Type: Elements, Min: ptr(10.0), Max: ptr(10.0)},
			{Type: Separate, Each: []Matcher{doubleBack, {Twist: &Range{Min: ptr(2)}}}},
			{Type: Count, Max: ptr(1.0), Match: &doubles},
			{Type: Different},
			{Type: Difficulty, Min: ptr(4.0)},
		}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.set.Format, tc.set.Name = Format, tc.name
			if err := tc.set.Validate(); err != nil {
				t.Fatalf("invalid test set: %v", err)
			}
			got := Conflicts(tc.set)
			all := strings.Join(got, "\n")
			if tc.wants == nil && len(got) > 0 {
				t.Errorf("want no conflicts, got:\n%s", all)
			}
			if tc.wants != nil && len(got) == 0 {
				t.Errorf("want a conflict, got none")
			}
			for _, w := range tc.wants {
				if !strings.Contains(all, w) {
					t.Errorf("missing %q in:\n%s", w, all)
				}
			}
		})
	}
}

// Every built-in set can be met, so none should be flagged.
func TestBuiltinsHaveNoConflicts(t *testing.T) {
	for _, g := range BuiltinGroups() {
		for _, b := range append(g.Requirements(), g.SetRoutines()...) {
			if got := Conflicts(b.Set); len(got) > 0 {
				t.Errorf("%s: %s", b.Set.Name, strings.Join(got, "; "))
			}
		}
	}
}
