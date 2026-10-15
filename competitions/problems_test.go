package competitions

import (
	"slices"
	"testing"
)

// An entry's problems come in one order wherever they're shown: its pair's
// levels, its card, then its sign-off.
func TestEntryProblems(t *testing.T) {
	c := competition(t)
	gone := Entered{Entry: Entry{Gymnast: "Ann", Level: "No such level"}, People: []string{"m:ann"}, Signoff: "Not signed off by a coach"}
	got := c.EntryProblems(gone, nil)
	if len(got) != 1 || got[0] == gone.Signoff {
		t.Errorf("a card that can't be checked says why, and only that: %v", got)
	}
	if _, err := c.Check(gone.Entry); err == nil || got[0] != err.Error() {
		t.Errorf("why: %v", got)
	}

	// A pair far apart individually.
	pair := Entered{Entry: Entry{Gymnast: "Ann", Discipline: Synchro, Level: "BUCS L1", Partner: &Partner{Name: "Bea"}}, People: []string{"m:ann", "i:bea"}}
	all := []Entered{
		{Entry: Entry{Gymnast: "Ann", Level: "BUCS L1"}, People: []string{"m:ann"}},
		{Entry: Entry{Gymnast: "Bea", Level: "BUCS L7"}, People: []string{"i:bea"}},
	}
	if c.PairLevel(pair, all) == "" || c.PairLevel(pair, nil) != "" {
		t.Errorf("a pair's levels are checked against their individual ones: %q", c.PairLevel(pair, all))
	}
	if got := c.EntryProblems(pair, all); len(got) == 0 || got[0] != c.PairLevel(pair, all) {
		t.Errorf("the pair's problem comes first: %v", got)
	}
	// A card that can be checked.
	one := Entered{Entry: Entry{Gymnast: "Cara", Level: c.LevelNames(Trampoline)[0]}, People: []string{"m:cara"}}
	if _, err := c.Check(one.Entry); err != nil {
		t.Fatal(err)
	}
	signed := slices.Clone(c.EntryProblems(one, all))
	one.Signoff = "Not signed off by a coach"
	if got := c.EntryProblems(one, all); len(got) != len(signed)+1 || got[len(got)-1] != one.Signoff {
		t.Errorf("the sign-off's comes last: %v", got)
	}
}
