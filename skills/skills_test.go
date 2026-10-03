package skills

import (
	"math"
	"testing"
)

func skill(rotation int, twists []int, takeoff BodyPosition, shape Shape, backward, seat bool) TrampolineSkill {
	return TrampolineSkill{
		Rotation:          rotation,
		TwistDistribution: twists,
		TakeoffPosition:   takeoff,
		Shape:             shape,
		Backward:          backward,
		SeatLanding:       seat,
	}
}

// TestSetTariff pins the difficulty values against the FIG TRA Code of Points
// 2025-2028 §17.1 (quarter/twist values, single/double/triple/quad completion,
// straight-or-pike bonus §17.1.4-.5, backward bonus §17.1.6.1, twist bonuses §17.1.6.2-.4).
func TestSetTariff(t *testing.T) {
	cases := []struct {
		name string
		s    TrampolineSkill
		want float64
	}{
		{"straight jump", skill(0, []int{0}, Feet, Straight, false, false), 0.0},
		{"tuck jump", skill(0, []int{0}, Feet, Tuck, false, false), 0.1},
		{"seat drop", skill(0, []int{0}, Feet, Straight, false, true), 0.1},
		{"half twist", skill(0, []int{1}, Feet, Straight, false, false), 0.1},
		{"full twist", skill(0, []int{2}, Feet, Straight, false, false), 0.2},
		{"crash dive (3/4 front)", skill(3, []int{0}, Feet, Straight, false, false), 0.3},
		{"front tuck", skill(4, []int{0}, Feet, Tuck, false, false), 0.5},
		{"front straight", skill(4, []int{0}, Feet, Straight, false, false), 0.6},
		{"barani", skill(4, []int{1}, Feet, Tuck, false, false), 0.6},
		{"back tuck", skill(4, []int{0}, Feet, Tuck, true, false), 0.5},
		{"full back", skill(4, []int{2}, Feet, Straight, true, false), 0.7},
		{"rudi", skill(4, []int{3}, Feet, Straight, false, false), 0.8},
		{"double back tuck", skill(8, []int{0, 0}, Feet, Tuck, true, false), 1.1},
		{"half-out", skill(8, []int{0, 1}, Feet, Tuck, false, false), 1.1},
		{"full-full", skill(8, []int{2, 2}, Feet, Straight, true, false), 1.7},
		{"full rudi", skill(8, []int{2, 3}, Feet, Straight, false, false), 1.8},
		{"miller", skill(8, []int{3, 3}, Feet, Straight, true, false), 2.1},
		{"triple back tuck", skill(12, []int{0, 0, 0}, Feet, Tuck, true, false), 1.8},
		{"trif half-out", skill(12, []int{0, 0, 1}, Feet, Tuck, false, false), 1.7},
		{"quad back tuck", skill(16, []int{0, 0, 0, 0}, Feet, Tuck, true, false), 2.5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := c.s
			got := s.SetTariff()
			if math.Abs(got-c.want) > 1e-9 {
				t.Errorf("SetTariff(%s) = %.2f, want %.2f", c.name, got, c.want)
			}
		})
	}
}

func TestShapeIsRelevant(t *testing.T) {
	cases := []struct {
		name string
		s    TrampolineSkill
		want bool
	}{
		{"straight jump", skill(0, []int{0}, Feet, Straight, false, false), true},
		{"seat drop", skill(0, []int{0}, Feet, Straight, false, true), false},
		{"half twist", skill(0, []int{1}, Feet, Straight, false, false), false},
		{"front drop", skill(1, []int{0}, Feet, Straight, false, false), false},
		{"crash dive", skill(3, []int{0}, Feet, Straight, false, false), true},
		{"front", skill(4, []int{0}, Feet, Tuck, false, false), true},
		{"barani (half twist)", skill(4, []int{1}, Feet, Tuck, false, false), true},
		{"full back (full twist)", skill(4, []int{2}, Feet, Straight, true, false), false},
		{"rudi (1.5 twist)", skill(4, []int{3}, Feet, Straight, false, false), false},
		{"double back", skill(8, []int{0, 0}, Feet, Tuck, true, false), true},
		{"miller", skill(8, []int{3, 3}, Feet, Straight, true, false), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := c.s
			if got := s.ShapeIsRelevant(); got != c.want {
				t.Errorf("ShapeIsRelevant(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

func TestFIGNotation(t *testing.T) {
	cases := []struct {
		name string
		s    TrampolineSkill
		want string
	}{
		{"straight jump", skill(0, []int{0}, Feet, Straight, false, false), ""},
		{"tuck jump", skill(0, []int{0}, Feet, Tuck, false, false), "(o)"},
		{"full twist (shape omitted)", skill(0, []int{2}, Feet, Straight, false, false), "(0 2)"},
		{"front tuck", skill(4, []int{0}, Feet, Tuck, false, false), "(4 - o)"},
		{"rudi (shape omitted)", skill(4, []int{3}, Feet, Straight, false, false), "(4 3)"},
		{"double back tuck", skill(8, []int{0, 0}, Feet, Tuck, true, false), "(8 - - o)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := c.s
			if got := s.FIGNotation(); got != c.want {
				t.Errorf("FIGNotation(%s) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

func TestLandingPosition(t *testing.T) {
	cases := []struct {
		name string
		s    TrampolineSkill
		want BodyPosition
	}{
		{"front tuck lands feet", skill(4, []int{0}, Feet, Tuck, false, false), Feet},
		{"crash dive lands back", skill(3, []int{0}, Feet, Straight, false, false), Back},
		{"barani lands feet", skill(4, []int{1}, Feet, Tuck, false, false), Feet},
		{"front drop lands front", skill(1, []int{0}, Feet, Straight, false, false), Front},
		{"seat drop lands seat", skill(0, []int{0}, Feet, Straight, false, true), Seat},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := c.s
			if got := s.LandingPosition(); got != c.want {
				t.Errorf("LandingPosition(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

// TestFindCommonSkillName covers the naming rule: append the shape whenever shape
// is relevant (including "Straight"), and omit it when it is not.
func TestFindCommonSkillName(t *testing.T) {
	cases := []struct {
		name string
		s    TrampolineSkill
		want string
	}{
		// Shape relevant -> always clarified, including straight.
		{"front tuck", skill(4, []int{0}, Feet, Tuck, false, false), "Front Tuck"},
		{"front straight", skill(4, []int{0}, Feet, Straight, false, false), "Front Straight"},
		{"barani tuck", skill(4, []int{1}, Feet, Tuck, false, false), "Barani Tuck"},
		{"miller straight", skill(8, []int{3, 3}, Feet, Straight, true, false), "Miller Straight"},
		{"double back tuck", skill(8, []int{0, 0}, Feet, Tuck, true, false), "Double Back Tuck"},
		{"triple back tuck", skill(12, []int{0, 0, 0}, Feet, Tuck, true, false), "Triple Back Tuck"},
		// Shape NOT relevant -> never clarified.
		{"rudi", skill(4, []int{3}, Feet, Straight, false, false), "Rudi"},
		{"full back", skill(4, []int{2}, Feet, Straight, true, false), "Full Back"},
		// Basic jumps: the shape is the skill name itself.
		{"straight jump", skill(0, []int{0}, Feet, Straight, false, false), "Straight Jump"},
		{"tuck jump", skill(0, []int{0}, Feet, Tuck, false, false), "Tuck Jump"},
		// No match.
		{"custom", skill(9, []int{0, 0}, Feet, Tuck, false, false), "Custom Skill"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FindCommonSkillName(c.s); got != c.want {
				t.Errorf("FindCommonSkillName(%s) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

func TestValidateRoutine(t *testing.T) {
	frontTuck := skill(4, []int{0}, Feet, Tuck, false, false) // lands Feet, 0.5

	t.Run("duplicate counts once", func(t *testing.T) {
		rv := ValidateRoutine([]TrampolineSkill{frontTuck, frontTuck})
		if !rv.HasDuplicates {
			t.Fatal("expected HasDuplicates")
		}
		if !rv.Skills[0].IsDuplicate || !rv.Skills[1].IsDuplicate {
			t.Errorf("both occurrences should be flagged duplicate")
		}
		if rv.TotalTariff != 0.5 {
			t.Errorf("TotalTariff = %.2f, want 0.50 (counts once)", rv.TotalTariff)
		}
		if rv.RawTariff != 1.0 {
			t.Errorf("RawTariff = %.2f, want 1.00 (counts both)", rv.RawTariff)
		}
		if rv.Messages[0] != "Duplicate (Counts Once)" || rv.Messages[1] != "Duplicate" {
			t.Errorf("messages = %q / %q", rv.Messages[0], rv.Messages[1])
		}
	})

	t.Run("bad transition", func(t *testing.T) {
		takeoffBack := skill(1, []int{0}, Back, Straight, false, false) // takes off Back
		rv := ValidateRoutine([]TrampolineSkill{frontTuck, takeoffBack})
		if !rv.HasInvalidTransitions || !rv.Skills[1].InvalidTransition {
			t.Errorf("expected invalid transition Feet -> Back on skill 2")
		}
	})

	t.Run("10th must land feet", func(t *testing.T) {
		crashDive := skill(3, []int{0}, Feet, Straight, false, false) // lands Back
		routine := make([]TrampolineSkill, 10)
		for i := range routine {
			routine[i] = crashDive
		}
		rv := ValidateRoutine(routine)
		if !rv.TenthSkillWarning {
			t.Errorf("expected TenthSkillWarning when 10th skill does not land on feet")
		}
		if rv.RoutineTooLong {
			t.Errorf("10 skills should not be RoutineTooLong")
		}
	})

	t.Run("more than 10 skills is too long", func(t *testing.T) {
		routine := make([]TrampolineSkill, 11)
		for i := range routine {
			routine[i] = frontTuck
		}
		if rv := ValidateRoutine(routine); !rv.RoutineTooLong {
			t.Errorf("expected RoutineTooLong for 11 skills")
		}
	})

	t.Run("11th skill never counts, even after a duplicate", func(t *testing.T) {
		// Twisting jumps of 1, 1 (duplicate), 2, ..., 9 and then 10 half twists as the 11th.
		twists := []int{1, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		routine := make([]TrampolineSkill, len(twists))
		for i, tw := range twists {
			routine[i] = skill(0, []int{tw}, Feet, Straight, false, false)
		}
		rv := ValidateRoutine(routine)
		// 0.1 for the first half twist + 0.2..0.9; the repeat and the 11th are not counted.
		if math.Abs(rv.TotalTariff-4.5) > 1e-9 {
			t.Errorf("TotalTariff = %.2f, want 4.50", rv.TotalTariff)
		}
		if rv.Messages[10] != "Skill >10 (No Tariff)" {
			t.Errorf("11th message = %q", rv.Messages[10])
		}
	})

	t.Run("first skill must take off from feet", func(t *testing.T) {
		backToFeet := skill(1, []int{0}, Back, Straight, false, false)
		rv := ValidateRoutine([]TrampolineSkill{backToFeet})
		if !rv.HasInvalidTransitions || !rv.Skills[0].InvalidTransition {
			t.Errorf("expected invalid start for a first skill taking off from back")
		}
		if rv.Messages[0] != "Must Start From Feet" {
			t.Errorf("message = %q", rv.Messages[0])
		}
		if rv := ValidateRoutine([]TrampolineSkill{frontTuck}); rv.HasInvalidTransitions {
			t.Errorf("a first skill from feet should be a valid start")
		}
	})

	t.Run("straight jump interrupts the routine", func(t *testing.T) {
		straightJump := skill(0, []int{0}, Feet, Straight, false, false)
		tuckJump := skill(0, []int{0}, Feet, Tuck, false, false)
		rv := ValidateRoutine([]TrampolineSkill{frontTuck, straightJump, tuckJump})
		if !rv.HasIntermediateJumps || !rv.Skills[1].IntermediateJump {
			t.Errorf("expected the straight jump to be flagged as an intermediate jump")
		}
		if rv.Skills[2].IntermediateJump {
			t.Errorf("a tuck jump is an element, not an intermediate jump")
		}
		if rv.Messages[1] != "Straight Jump Interrupts Routine" {
			t.Errorf("message = %q", rv.Messages[1])
		}
	})
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		s       TrampolineSkill
		wantErr bool
	}{
		{"front tuck", skill(4, []int{0}, Feet, Tuck, false, false), false},
		{"quad back", skill(16, []int{0, 0, 0, 0}, Feet, Tuck, true, false), false},
		{"negative rotation", skill(-4, []int{0}, Feet, Tuck, false, false), true},
		{"rotation beyond a quad", skill(17, []int{0, 0, 0, 0}, Feet, Tuck, false, false), true},
		{"negative twist", skill(4, []int{-3}, Feet, Straight, false, false), true},
		{"wrong phase count", skill(8, []int{0}, Feet, Tuck, false, false), true},
		{"invalid take-off", skill(4, []int{0}, Invalid, Tuck, false, false), true},
		{"invalid shape", skill(4, []int{0}, Feet, InvalidShape, false, false), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := c.s
			if err := s.Validate(); (err != nil) != c.wantErr {
				t.Errorf("Validate(%s) error = %v, wantErr %v", c.name, err, c.wantErr)
			}
		})
	}

	if got := ShapeFromString("banana"); got != InvalidShape {
		t.Errorf("ShapeFromString(unknown) = %v, want InvalidShape", got)
	}
}
