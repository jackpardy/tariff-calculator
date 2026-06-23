package main

import (
	"testing"

	"tariffCalculator/skills"
)

func namedSkill(rotation int, twists []int, takeoff skills.BodyPosition, shape skills.Shape, backward, seat bool) skills.TrampolineSkill {
	return skills.TrampolineSkill{
		Rotation:          rotation,
		TwistDistribution: twists,
		TakeoffPosition:   takeoff,
		Shape:             shape,
		Backward:          backward,
		SeatLanding:       seat,
	}
}

// TestFindCommonSkillName covers the naming rule: append the shape whenever shape
// is relevant to the skill (including "Straight"), and omit it when it is not.
func TestFindCommonSkillName(t *testing.T) {
	cases := []struct {
		name string
		s    skills.TrampolineSkill
		want string
	}{
		// Shape relevant -> always clarified, including straight.
		{"front tuck", namedSkill(4, []int{0}, skills.Feet, skills.Tuck, false, false), "Front Tuck"},
		{"front straight", namedSkill(4, []int{0}, skills.Feet, skills.Straight, false, false), "Front Straight"},
		{"barani tuck", namedSkill(4, []int{1}, skills.Feet, skills.Tuck, false, false), "Barani Tuck"},
		{"miller straight", namedSkill(8, []int{3, 3}, skills.Feet, skills.Straight, true, false), "Miller Straight"},
		{"double back tuck", namedSkill(8, []int{0, 0}, skills.Feet, skills.Tuck, true, false), "Double Back Tuck"},
		{"triple back tuck", namedSkill(12, []int{0, 0, 0}, skills.Feet, skills.Tuck, true, false), "Triple Back Tuck"},

		// Shape NOT relevant -> never clarified.
		{"rudi", namedSkill(4, []int{3}, skills.Feet, skills.Straight, false, false), "Rudi"},
		{"full back", namedSkill(4, []int{2}, skills.Feet, skills.Straight, true, false), "Full Back"},

		// Basic jumps: the shape is the skill name itself.
		{"straight jump", namedSkill(0, []int{0}, skills.Feet, skills.Straight, false, false), "Straight Jump"},
		{"tuck jump", namedSkill(0, []int{0}, skills.Feet, skills.Tuck, false, false), "Tuck Jump"},

		// No match.
		{"custom", namedSkill(9, []int{0, 0}, skills.Feet, skills.Tuck, false, false), "Custom Skill"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := findCommonSkillName(c.s); got != c.want {
				t.Errorf("findCommonSkillName(%s) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}
