package skills

import (
	"slices"
	"testing"
)

// The validation's labels read as sentences on entry pages.
func TestElementNotes(t *testing.T) {
	for _, c := range []struct {
		i    int
		msg  string
		want []string
	}{
		{9, "10th Must Land Feet", []string{"Element 10 must land on feet."}},
		{10, "Skill >10 (No Tariff)", []string{"Element 11 is past the tenth, so its difficulty doesn't count."}},
		{3, "Bad Transition: Back -> Front / Duplicate", []string{
			"Element 4 takes off from front, but element 3 lands on back.",
			"Element 4 repeats an earlier element, so its difficulty doesn't count again.",
		}},
		{2, "", nil},
	} {
		if got := ElementNotes(c.i, c.msg); !slices.Equal(got, c.want) {
			t.Errorf("%d %q: %q, want %q", c.i, c.msg, got, c.want)
		}
	}
	if got := ElementNote(9, "10th Must Land Feet / Straight Jump"); got != "Must land on feet. Is a straight jump." {
		t.Errorf("one line for the element's row: %q", got)
	}
}
