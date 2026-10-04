package skills

import (
	"encoding/json"
	"math"
	"slices"
	"strings"
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

func withCustomName(s TrampolineSkill, name string) TrampolineSkill {
	s.CustomName = name
	return s
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
		// From feet, no twist.
		{"front tuck lands feet", skill(4, []int{0}, Feet, Tuck, false, false), Feet},
		{"front drop lands front", skill(1, []int{0}, Feet, Straight, false, false), Front},
		{"back drop lands back", skill(1, []int{0}, Feet, Straight, true, false), Back},
		{"crash dive lands back", skill(3, []int{0}, Feet, Straight, false, false), Back},
		{"lazy back lands front", skill(3, []int{0}, Feet, Straight, true, false), Front},
		{"2 3/4 front lands back", skill(11, []int{0, 0, 0}, Feet, Tuck, false, false), Back},
		{"half rotation lands invalid", skill(2, []int{0}, Feet, Straight, false, false), Invalid},
		// An odd number of half twists swaps front and back.
		{"half twist jump lands feet", skill(0, []int{1}, Feet, Straight, false, false), Feet},
		{"barani lands feet", skill(4, []int{1}, Feet, Tuck, false, false), Feet},
		{"1/2 twist to back", skill(1, []int{1}, Feet, Straight, false, false), Back},
		{"1/2 twist to front", skill(1, []int{1}, Feet, Straight, true, false), Front},
		{"barani to front", skill(3, []int{1}, Feet, Tuck, false, false), Front},
		{"full twist to front", skill(1, []int{2}, Feet, Straight, false, false), Front},
		{"half-out lands feet", skill(8, []int{0, 1}, Feet, Tuck, false, false), Feet},
		// From front, back and seat.
		{"front to feet", skill(1, []int{0}, Front, Straight, true, false), Feet},
		{"back to feet", skill(1, []int{0}, Back, Straight, false, false), Feet},
		{"ball-out lands feet", skill(5, []int{0}, Back, Tuck, false, false), Feet},
		{"cody lands feet", skill(5, []int{0}, Front, Tuck, true, false), Feet},
		{"seat half twist to front", skill(1, []int{1}, Seat, Straight, true, false), Front},
		// Seat landings are only possible from an upright finish.
		{"seat drop lands seat", skill(0, []int{0}, Feet, Straight, false, true), Seat},
		{"front to seat lands seat", skill(4, []int{0}, Feet, Tuck, false, true), Seat},
		{"front drop to seat is invalid", skill(1, []int{0}, Feet, Straight, false, true), Invalid},
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

func TestNormalizePhases(t *testing.T) {
	cases := []struct {
		name string
		s    TrampolineSkill
		want []int
	}{
		{"pads a double", skill(8, []int{2}, Feet, Tuck, true, false), []int{2, 0}},
		{"trims a single", skill(4, []int{1, 3}, Feet, Tuck, false, false), []int{1}},
		{"fills nil", skill(12, nil, Feet, Tuck, true, false), []int{0, 0, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := c.s
			s.NormalizePhases()
			if !slices.Equal(s.TwistDistribution, c.want) {
				t.Errorf("TwistDistribution = %v, want %v", s.TwistDistribution, c.want)
			}
		})
	}

	// It must not write through to a shared backing array.
	shared := make([]int, 1, 4)
	original := TrampolineSkill{Rotation: 8, TwistDistribution: shared}
	copied := original
	copied.NormalizePhases()
	if got := shared[:2]; got[1] != 0 || len(original.TwistDistribution) != 1 {
		t.Errorf("NormalizePhases modified the original's slice: %v", got)
	}
	copied.TwistDistribution[1] = 5
	if shared[:2][1] != 0 {
		t.Errorf("normalised slice still shares the original's backing array")
	}
}

// TestEqual pins repetition detection (CoP §14): which pairs of elements count as
// the same element. Each case is checked in both directions.
func TestEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b TrampolineSkill
		want bool
	}{
		// §14.2: basic jumps in different shapes are different elements; twisting jumps have no shape.
		{"tuck jump vs tuck jump", skill(0, []int{0}, Feet, Tuck, false, false), skill(0, []int{0}, Feet, Tuck, false, false), true},
		{"tuck jump vs pike jump", skill(0, []int{0}, Feet, Tuck, false, false), skill(0, []int{0}, Feet, Pike, false, false), false},
		{"full twist jump in any shape", skill(0, []int{2}, Feet, Straight, false, false), skill(0, []int{2}, Feet, Tuck, false, false), true},

		// §14.5.1: up to a half twist, three shapes need 270° or more of somersault.
		{"front drop in any shape", skill(1, []int{0}, Feet, Tuck, false, false), skill(1, []int{0}, Feet, Pike, false, false), true},
		{"1/2 twist to feet in any shape", skill(1, []int{1}, Back, Tuck, false, false), skill(1, []int{1}, Back, Straight, false, false), true},
		{"3/4 back tuck vs pike", skill(3, []int{0}, Feet, Tuck, true, false), skill(3, []int{0}, Feet, Pike, true, false), false},
		{"front tuck vs front pike", skill(4, []int{0}, Feet, Tuck, false, false), skill(4, []int{0}, Feet, Pike, false, false), false},
		{"barani tuck vs pike", skill(4, []int{1}, Feet, Tuck, false, false), skill(4, []int{1}, Feet, Pike, false, false), false},
		{"barani ball-out tuck vs pike", skill(5, []int{1}, Back, Tuck, false, false), skill(5, []int{1}, Back, Pike, false, false), false},

		// §14.5.2: from a full twist, three shapes need more than 450° of somersault.
		{"full back in any shape", skill(4, []int{2}, Feet, Tuck, true, false), skill(4, []int{2}, Feet, Straight, true, false), true},
		{"rudy ball-out in any shape", skill(5, []int{3}, Back, Tuck, false, false), skill(5, []int{3}, Back, Straight, false, false), true},
		{"1 1/2 with full twist tuck vs straight", skill(6, []int{2}, Feet, Tuck, false, false), skill(6, []int{2}, Feet, Straight, false, false), false},
		{"full-in full-out tuck vs straight", skill(8, []int{2, 2}, Feet, Tuck, true, false), skill(8, []int{2, 2}, Feet, Straight, true, false), false},

		// §14.3, §14.5.4: in multiple somersaults the same twist in different phases is a different element.
		{"double back vs double back", skill(8, []int{0, 0}, Feet, Tuck, true, false), skill(8, []int{0, 0}, Feet, Tuck, true, false), true},
		{"full-in back vs half-in half-out", skill(8, []int{2, 0}, Feet, Straight, true, false), skill(8, []int{1, 1}, Feet, Straight, true, false), false},
		{"half-in rudy-out vs rudy-in half-out", skill(8, []int{1, 3}, Feet, Pike, true, false), skill(8, []int{3, 1}, Feet, Pike, true, false), false},
		{"triple twist phases differ", skill(12, []int{1, 1, 1}, Feet, Straight, false, false), skill(12, []int{2, 1, 0}, Feet, Straight, false, false), false},

		// Different direction, take-off, landing, rotation or twist are different elements.
		{"front tuck vs back tuck", skill(4, []int{0}, Feet, Tuck, false, false), skill(4, []int{0}, Feet, Tuck, true, false), false},
		{"half twist from feet vs from seat", skill(0, []int{1}, Feet, Straight, false, false), skill(0, []int{1}, Seat, Straight, false, false), false},
		{"half twist vs half twist to seat", skill(0, []int{1}, Feet, Straight, false, false), skill(0, []int{1}, Feet, Straight, false, true), false},
		{"front vs 1 3/4 front", skill(4, []int{0}, Feet, Tuck, false, false), skill(7, []int{0, 0}, Feet, Tuck, false, false), false},
		{"front tuck vs barani tuck", skill(4, []int{0}, Feet, Tuck, false, false), skill(4, []int{1}, Feet, Tuck, false, false), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, b := c.a, c.b
			if got := a.Equal(&b); got != c.want {
				t.Errorf("a.Equal(b) = %v, want %v", got, c.want)
			}
			if got := b.Equal(&a); got != c.want {
				t.Errorf("b.Equal(a) = %v, want %v", got, c.want)
			}
		})
	}
}

// TestEnumJSONRoundTrip pins the enum wire format used by the API and by routines
// saved in the browser.
func TestEnumJSONRoundTrip(t *testing.T) {
	for _, pos := range []BodyPosition{Feet, Front, Back, Seat} {
		b, err := json.Marshal(pos)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", pos, err)
		}
		var got BodyPosition
		if err := json.Unmarshal(b, &got); err != nil || got != pos {
			t.Errorf("BodyPosition round trip %s -> %v (err %v)", b, got, err)
		}
	}
	for _, shape := range []Shape{Straight, Tuck, Pike, Straddle} {
		b, err := json.Marshal(shape)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", shape, err)
		}
		var got Shape
		if err := json.Unmarshal(b, &got); err != nil || got != shape {
			t.Errorf("Shape round trip %s -> %v (err %v)", b, got, err)
		}
	}

	// Names are matched case-insensitively; unknown names decode to the invalid value.
	var pos BodyPosition
	if err := json.Unmarshal([]byte(`"feet"`), &pos); err != nil || pos != Feet {
		t.Errorf(`"feet" -> %v (err %v), want Feet`, pos, err)
	}
	if err := json.Unmarshal([]byte(`"Head"`), &pos); err != nil || pos != Invalid {
		t.Errorf(`"Head" -> %v (err %v), want Invalid`, pos, err)
	}
	var shape Shape
	if err := json.Unmarshal([]byte(`"PIKE"`), &shape); err != nil || shape != Pike {
		t.Errorf(`"PIKE" -> %v (err %v), want Pike`, shape, err)
	}
	if err := json.Unmarshal([]byte(`"Banana"`), &shape); err != nil || shape != InvalidShape {
		t.Errorf(`"Banana" -> %v (err %v), want InvalidShape`, shape, err)
	}

	// Malformed JSON is an error, not a silent default.
	if err := json.Unmarshal([]byte(`{"takeoff_position": 3}`), &struct {
		P BodyPosition `json:"takeoff_position"`
	}{}); err == nil {
		t.Errorf("a non-string position should fail to decode")
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
		{"half twist to seat", skill(0, []int{1}, Feet, Straight, false, true), "Half Twist To Seat"},
		{"seat half twist to seat", skill(0, []int{1}, Seat, Straight, false, true), "Seat Half Twist To Seat"},
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
		if !rv.Skills[0].Counted || rv.Skills[1].Counted {
			t.Errorf("the first occurrence should count and the repeat should not")
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
		if !rv.Skills[1].IntermediateJump || !rv.Skills[1].Interrupts || rv.InterruptedAt != 1 {
			t.Errorf("expected the straight jump to interrupt the routine at skill 2 (InterruptedAt %d)", rv.InterruptedAt)
		}
		if rv.Skills[2].IntermediateJump {
			t.Errorf("a tuck jump is an element, not an intermediate jump")
		}
		if rv.Messages[1] != "Straight Jump Interrupts Routine" {
			t.Errorf("message = %q", rv.Messages[1])
		}
	})

	// §15.2-15.3: the interrupting skill gets no credit and later skills are not counted.
	t.Run("skills after an interruption do not count", func(t *testing.T) {
		barani := skill(4, []int{1}, Feet, Tuck, false, false)             // 0.6
		frontDropToSeat := skill(1, []int{0}, Feet, Straight, false, true) // invalid landing, 0.1
		backTuck := skill(4, []int{0}, Feet, Tuck, true, false)            // 0.5

		cases := []struct {
			name          string
			routine       []TrampolineSkill
			interruptedAt int
			total         float64
		}{
			{"no interruption", []TrampolineSkill{frontTuck, barani, backTuck}, -1, 1.6},
			{"straight jump", []TrampolineSkill{frontTuck, skill(0, []int{0}, Feet, Straight, false, false), barani, backTuck}, 1, 0.5},
			{"invalid landing gets no credit", []TrampolineSkill{frontTuck, frontDropToSeat, barani}, 1, 0.5},
			{"interrupted on the first skill", []TrampolineSkill{frontDropToSeat, frontTuck}, 0, 0},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				rv := ValidateRoutine(c.routine)
				if rv.InterruptedAt != c.interruptedAt {
					t.Errorf("InterruptedAt = %d, want %d", rv.InterruptedAt, c.interruptedAt)
				}
				if math.Abs(rv.TotalTariff-c.total) > 1e-9 {
					t.Errorf("TotalTariff = %.2f, want %.2f", rv.TotalTariff, c.total)
				}
				for i, sv := range rv.Skills {
					counted := c.interruptedAt < 0 || i < c.interruptedAt
					if sv.Counted != counted {
						t.Errorf("skill %d Counted = %v, want %v", i+1, sv.Counted, counted)
					}
					after := c.interruptedAt >= 0 && i > c.interruptedAt
					if sv.AfterInterruption != after {
						t.Errorf("skill %d AfterInterruption = %v, want %v", i+1, sv.AfterInterruption, after)
					}
					if after && !strings.Contains(rv.Messages[i], "After Interruption (No Tariff)") {
						t.Errorf("skill %d message = %q", i+1, rv.Messages[i])
					}
				}
			})
		}
	})

	t.Run("only the first interruption interrupts", func(t *testing.T) {
		frontDropToSeat := skill(1, []int{0}, Feet, Straight, false, true)
		backDropToSeat := skill(1, []int{0}, Feet, Straight, true, true)
		rv := ValidateRoutine([]TrampolineSkill{frontTuck, frontDropToSeat, backDropToSeat})
		if rv.Messages[1] != "Invalid Landing Interrupts Routine" {
			t.Errorf("first message = %q", rv.Messages[1])
		}
		if rv.Skills[2].Interrupts || !strings.Contains(rv.Messages[2], "After Interruption") {
			t.Errorf("a later invalid landing should only be marked as after the interruption, got %q", rv.Messages[2])
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
		{"custom name at the limit", withCustomName(skill(4, []int{0}, Feet, Tuck, false, false), strings.Repeat("é", MaxCustomNameLength)), false},
		{"custom name over the limit", withCustomName(skill(4, []int{0}, Feet, Tuck, false, false), strings.Repeat("x", MaxCustomNameLength+1)), true},
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

func TestValidateRoutineAllowingRepeats(t *testing.T) {
	tuck := TrampolineSkill{Rotation: 0, TwistDistribution: []int{0}, TakeoffPosition: Feet, Shape: Tuck}
	routine := []TrampolineSkill{tuck, tuck}
	if rv := ValidateRoutine(routine); !rv.HasDuplicates || rv.Skills[1].Counted || rv.TotalTariff != 0.1 {
		t.Errorf("by default a repeat is flagged and counts once: %+v", rv)
	}
	rv := ValidateRoutineWith(routine, ValidateOptions{AllowRepeats: true})
	if rv.HasDuplicates || rv.Skills[0].IsDuplicate || rv.Skills[1].IsDuplicate || !rv.Skills[1].Counted || rv.Messages[1] != "" || math.Abs(rv.TotalTariff-0.2) > 1e-9 {
		t.Errorf("with repeats allowed, both count and nothing is flagged: %+v", rv)
	}
}

func TestValidateRoutineScoringOnlySomeElements(t *testing.T) {
	s := func(rotation int, twist int, backward bool) TrampolineSkill {
		return TrampolineSkill{Rotation: rotation, TwistDistribution: []int{twist}, TakeoffPosition: Feet, Shape: Tuck, Backward: backward}
	}
	// Back (0.5), Barani (0.6), Full back (0.7), Rudi (0.8, straight), front (0.5).
	rudi := s(4, 3, false)
	rudi.Shape = Straight
	routine := []TrampolineSkill{s(4, 0, true), s(4, 1, false), s(4, 2, true), rudi, s(4, 0, false)}
	rv := ValidateRoutineWith(routine, ValidateOptions{ScoredElements: 2})
	var scored, unscored []int
	for i, sv := range rv.Skills {
		if sv.Counted {
			scored = append(scored, i+1)
		}
		if sv.Unscored {
			unscored = append(unscored, i+1)
		}
	}
	if !slices.Equal(scored, []int{3, 4}) || !slices.Equal(unscored, []int{1, 2, 5}) || math.Abs(rv.TotalTariff-1.5) > 1e-9 || rv.ScoredElements != 2 {
		t.Errorf("the full back and Rudi score: scored %v, unscored %v, total %.1f", scored, unscored, rv.TotalTariff)
	}
	if rv := ValidateRoutine(routine); rv.ScoredElements != 0 || rv.Skills[0].Unscored || math.Abs(rv.TotalTariff-3.1) > 1e-9 {
		t.Errorf("by default every element scores: total %.1f", rv.TotalTariff)
	}
}
