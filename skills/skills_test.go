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
