package skills

import (
	"slices"
	"strings"
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

// A routine ended early by a straight jump says so once for the elements
// after it, not once each; nor that one of them is past the tenth, nor that
// the tenth must land on feet (as in the screenshot of 15 problems).
func TestRoutineNotesGroupTheirCause(t *testing.T) {
	jump := TrampolineSkill{Rotation: 0, TwistDistribution: []int{0}, TakeoffPosition: Feet, Shape: Straight}
	half := TrampolineSkill{Rotation: 0, TwistDistribution: []int{1}, TakeoffPosition: Feet, Shape: Straight}
	seat := TrampolineSkill{Rotation: 0, TwistDistribution: []int{0}, TakeoffPosition: Feet, Shape: Straight, SeatLanding: true}
	routine := []TrampolineSkill{half, jump, half, half, half, half, half, half, half, seat, half}
	got := RoutineNotes(ValidateRoutine(routine).Messages)
	want := []string{
		"Element 2 is a straight jump, which ends the routine there.",
		"Elements 3 to 11 come after the routine ended, so their difficulty doesn't count.",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("notes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// Twelve elements: the two past the tenth in one sentence.
	long := []TrampolineSkill{half, seat, {Rotation: 0, TwistDistribution: []int{0}, TakeoffPosition: Seat, Shape: Straight}}
	for len(long) < 12 {
		long = append(long, long[len(long)%3])
	}
	if got := RoutineNotes(ValidateRoutine(long).Messages); !slices.Contains(got, "Elements 11 and 12 are past the tenth, so their difficulty doesn't count.") {
		t.Errorf("past the tenth: %q", got)
	}
	if got := elementsPhrase([]int{2, 4, 5, 7, 8, 9}); got != "3, 5, 6 and 8 to 10" {
		t.Errorf("elementsPhrase = %q", got)
	}
}
