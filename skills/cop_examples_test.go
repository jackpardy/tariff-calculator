package skills

import (
	"fmt"
	"math"
	"testing"
)

// copExamples is every worked example in the FIG TRA Code of Points 2025-2028,
// Part II §C "Difficulty Trampoline – Examples" (pages 55-56): singles, doubles,
// triples and quads in each listed shape. Twists are as the CoP writes them, one
// digit per somersault; backward is the column the element appears in.
var copExamples = []struct {
	name     string
	rotation int
	twists   []int
	shape    Shape
	backward bool
	want     float64
}{
	{"Front Drop", 1, []int{0}, Tuck, false, 0.1},
	{"Back Drop", 1, []int{0}, Tuck, true, 0.1},
	{"Front Drop", 1, []int{0}, Pike, false, 0.1},
	{"Back Drop", 1, []int{0}, Pike, true, 0.1},
	{"Front Drop", 1, []int{0}, Straight, false, 0.1},
	{"Back Drop", 1, []int{0}, Straight, true, 0.1},
	{"1/2 Twist to Back", 1, []int{1}, Straight, false, 0.2},
	{"1/2 Twist to Front", 1, []int{1}, Straight, true, 0.2},
	{"Full Twist to Front", 1, []int{2}, Straight, false, 0.3},
	{"Full Twist to Back", 1, []int{2}, Straight, true, 0.3},
	{"3/4 Front", 3, []int{0}, Straight, false, 0.3},
	{"3/4 Back", 3, []int{0}, Tuck, true, 0.3},
	{"Barani to Front", 3, []int{1}, Tuck, false, 0.4},
	{"3/4 Back", 3, []int{0}, Pike, true, 0.3},
	{"Barani to Front", 3, []int{1}, Pike, false, 0.4},
	{"3/4 Back", 3, []int{0}, Straight, true, 0.3},
	{"Barani to Front", 3, []int{1}, Straight, false, 0.4},
	{"Half in 3/4 Front", 3, []int{1}, Straight, true, 0.4},
	{"Back full to Front", 3, []int{2}, Straight, true, 0.5},
	{"Front Somersault", 4, []int{0}, Tuck, false, 0.5},
	{"Back Somersault", 4, []int{0}, Tuck, true, 0.5},
	{"Front Somersault", 4, []int{0}, Pike, false, 0.6},
	{"Back Somersault", 4, []int{0}, Pike, true, 0.6},
	{"Front Somersault", 4, []int{0}, Straight, false, 0.6},
	{"Back Somersault", 4, []int{0}, Straight, true, 0.6},
	{"Barani", 4, []int{1}, Tuck, false, 0.6},
	{"Back Somersault with 1/2 Twist", 4, []int{1}, Tuck, true, 0.6},
	{"Barani", 4, []int{1}, Pike, false, 0.6},
	{"Back Somersault with 1/2 Twist", 4, []int{1}, Pike, true, 0.6},
	{"Barani", 4, []int{1}, Straight, false, 0.6},
	{"Back Somersault with 1/2 Twist", 4, []int{1}, Straight, true, 0.6},
	{"Rudolph (Rudy)", 4, []int{3}, Straight, false, 0.8},
	{"Back Full", 4, []int{2}, Straight, true, 0.7},
	{"Randolph (Randy)", 4, []int{5}, Straight, false, 1.0},
	{"Double Full", 4, []int{4}, Straight, true, 0.9},
	{"3 1/2 Twisting Front", 4, []int{7}, Straight, false, 1.2},
	{"Triple Full", 4, []int{6}, Straight, true, 1.1},
	{"4 1/2 Twisting Front", 4, []int{9}, Straight, false, 1.4},
	{"Quadruple full", 4, []int{8}, Straight, true, 1.3},
	{"Barani Ballout", 5, []int{1}, Tuck, false, 0.7},
	{"Cody or 1 1/4 Back", 5, []int{0}, Tuck, true, 0.6},
	{"Barani Ballout", 5, []int{1}, Pike, false, 0.7},
	{"Cody or 1 1/4 Back", 5, []int{0}, Pike, true, 0.7},
	{"Barani Ballout", 5, []int{1}, Straight, false, 0.7},
	{"Cody or 1 1/4 Back", 5, []int{0}, Straight, true, 0.7},
	{"Rudolph Ballout", 5, []int{3}, Straight, false, 0.9},
	{"Cody with Full Twist", 5, []int{2}, Straight, true, 0.8},
	{"Randolph Ballout", 5, []int{5}, Straight, false, 1.1},
	{"Cody with Double Twist", 5, []int{4}, Straight, true, 1.0},
	{"1 3/4 Front", 7, []int{0}, Tuck, false, 0.8},
	{"1 3/4 Front", 7, []int{0}, Pike, false, 0.9},
	{"1 3/4 Front", 7, []int{0}, Straight, false, 0.9},
	{"Half Out", 8, []int{0, 1}, Tuck, false, 1.1},
	{"Double Back", 8, []int{0, 0}, Tuck, true, 1.1},
	{"Half Out", 8, []int{0, 1}, Pike, false, 1.3},
	{"Double Back", 8, []int{0, 0}, Pike, true, 1.3},
	{"Half Out", 8, []int{0, 1}, Straight, false, 1.3},
	{"Double Back", 8, []int{0, 0}, Straight, true, 1.3},
	{"Rudy Out", 8, []int{0, 3}, Tuck, false, 1.3},
	{"Half In Half Out", 8, []int{1, 1}, Tuck, true, 1.3},
	{"Rudy Out", 8, []int{0, 3}, Pike, false, 1.5},
	{"Half In Half Out", 8, []int{1, 1}, Pike, true, 1.5},
	{"Rudy Out", 8, []int{0, 3}, Straight, false, 1.5},
	{"Half In Half Out", 8, []int{1, 1}, Straight, true, 1.5},
	{"Full Half", 8, []int{2, 1}, Tuck, false, 1.3},
	{"Back In Full Out", 8, []int{0, 2}, Tuck, true, 1.3},
	{"Full Half", 8, []int{2, 1}, Pike, false, 1.5},
	{"Back In Full Out", 8, []int{0, 2}, Pike, true, 1.5},
	{"Full Half", 8, []int{2, 1}, Straight, false, 1.5},
	{"Back In Full Out", 8, []int{0, 2}, Straight, true, 1.5},
	{"Full Rudy", 8, []int{2, 3}, Tuck, false, 1.6},
	{"1 1/2 in Half Out", 8, []int{3, 1}, Tuck, true, 1.5},
	{"Full Rudy", 8, []int{2, 3}, Pike, false, 1.8},
	{"1 1/2 in Half Out", 8, []int{3, 1}, Pike, true, 1.7},
	{"Full Rudy", 8, []int{2, 3}, Straight, false, 1.8},
	{"Full In Full Out", 8, []int{2, 2}, Tuck, true, 1.5},
	{"Randy Out", 8, []int{0, 5}, Tuck, false, 1.6},
	{"Full In Full Out", 8, []int{2, 2}, Straight, true, 1.7},
	{"Randy Out", 8, []int{0, 5}, Pike, false, 1.8},
	{"Half In Rudy Out", 8, []int{1, 3}, Tuck, true, 1.5},
	{"Randy Out", 8, []int{0, 5}, Straight, false, 1.8},
	{"Half In Rudy Out", 8, []int{1, 3}, Pike, true, 1.7},
	{"Full Randy", 8, []int{2, 5}, Tuck, false, 2.0},
	{"1 1/2 In 1 1/2 Out", 8, []int{3, 3}, Tuck, true, 1.9},
	{"Full Randy", 8, []int{2, 5}, Pike, false, 2.2},
	{"1 1/2 In 1 1/2 Out", 8, []int{3, 3}, Pike, true, 2.1},
	{"Full Randy", 8, []int{2, 5}, Straight, false, 2.2},
	{"1 1/2 In 1 1/2 Out", 8, []int{3, 3}, Straight, true, 2.1},
	{"3 1/2 Out", 8, []int{0, 7}, Tuck, false, 2.0},
	{"Half In Randy Out", 8, []int{1, 5}, Tuck, true, 1.9},
	{"3 1/2 Out", 8, []int{0, 7}, Pike, false, 2.2},
	{"Half In Randy Out", 8, []int{1, 5}, Pike, true, 2.1},
	{"3 1/2 Out", 8, []int{0, 7}, Straight, false, 2.2},
	{"1 1/2 In Randy Out", 8, []int{3, 5}, Tuck, true, 2.3},
	{"1 1/2 In Randy Out", 8, []int{3, 5}, Pike, true, 2.5},
	{"Double Full In Double Full Out", 8, []int{4, 4}, Straight, true, 2.5},
	{"Half In 3 1/2 Out", 8, []int{1, 7}, Tuck, true, 2.3},
	{"Half In 3 1/2 Out", 8, []int{1, 7}, Pike, true, 2.5},
	{"2 3/4 Front", 11, []int{0, 0}, Tuck, false, 1.3},
	{"2 3/4 Back with Half Twist", 11, []int{1, 0}, Tuck, true, 1.5},
	{"2 3/4 Front", 11, []int{0, 0}, Pike, false, 1.5},
	{"2 3/4 Back with Half Twist", 11, []int{1, 0}, Pike, true, 1.7},
	{"2 3/4 Front", 11, []int{0, 0}, Straight, false, 1.5},
	{"2 3/4 Back with Half Twist", 11, []int{1, 0}, Straight, true, 1.7},
	{"Front Front Half", 12, []int{0, 0, 1}, Tuck, false, 1.7},
	{"Triple Back", 12, []int{0, 0, 0}, Tuck, true, 1.8},
	{"Front Front Half", 12, []int{0, 0, 1}, Pike, false, 2.0},
	{"Triple Back", 12, []int{0, 0, 0}, Pike, true, 2.1},
	{"Front Front Rudy", 12, []int{0, 0, 3}, Tuck, false, 2.1},
	{"Triple Back", 12, []int{0, 0, 0}, Straight, true, 2.1},
	{"Front Front Rudy", 12, []int{0, 0, 3}, Pike, false, 2.4},
	{"Half Front Half", 12, []int{1, 0, 1}, Tuck, true, 2},
	{"Full Front Half", 12, []int{2, 0, 1}, Tuck, false, 2.1},
	{"Half Front Half", 12, []int{1, 0, 1}, Pike, true, 2.3},
	{"Full Front Half", 12, []int{2, 0, 1}, Pike, false, 2.4},
	{"Half Front Rudy", 12, []int{1, 0, 3}, Tuck, true, 2.6},
	{"Front Full Half", 12, []int{0, 2, 1}, Tuck, false, 2.1},
	{"Half Front Rudy", 12, []int{1, 0, 3}, Pike, true, 2.9},
	{"Front Full Half", 12, []int{0, 2, 1}, Pike, false, 2.4},
	{"Half Full Half", 12, []int{1, 2, 1}, Tuck, true, 2.6},
	{"Full Front Rudy", 12, []int{2, 0, 3}, Tuck, false, 2.7},
	{"Half Full Half", 12, []int{1, 2, 1}, Pike, true, 2.9},
	{"Full Front Rudy", 12, []int{2, 0, 3}, Pike, false, 3},
	{"Full Full Full", 12, []int{2, 2, 2}, Tuck, true, 3.2},
	{"Front Full Rudy", 12, []int{0, 2, 3}, Tuck, false, 2.7},
	{"Full Full Full", 12, []int{2, 2, 2}, Straight, true, 3.5},
	{"Front Full Rudy", 12, []int{0, 2, 3}, Pike, false, 3},
	{"1 1/2 Front Rudy Out", 12, []int{3, 0, 3}, Tuck, true, 3.2},
	{"Full Full Half", 12, []int{2, 2, 1}, Tuck, false, 2.7},
	{"1 1/2 Front Rudy Out", 12, []int{3, 0, 3}, Pike, true, 3.5},
	{"Full Full Half", 12, []int{2, 2, 1}, Pike, false, 3},
	{"Front Front Front Half", 16, []int{0, 0, 0, 1}, Tuck, false, 2.5},
	{"Half in half out quadriffis", 16, []int{1, 0, 0, 1}, Tuck, true, 3.1},
	{"Front Front Front Half", 16, []int{0, 0, 0, 1}, Pike, false, 2.9},
	{"Half in half out quadriffis", 16, []int{1, 0, 0, 1}, Pike, true, 3.5},
	{"Front Front Front Rudy", 16, []int{0, 0, 0, 3}, Tuck, false, 3.1},
	{"Half in rudy out quadriffis", 16, []int{1, 0, 0, 3}, Tuck, true, 3.7},
	{"Front Front Front Rudy", 16, []int{0, 0, 0, 3}, Pike, false, 3.5},
	{"Half in rudy out quadriffis", 16, []int{1, 0, 0, 3}, Pike, true, 4.1},
}

// TestSetTariffCoPExamples pins SetTariff to the Code of Points' own worked examples.
func TestSetTariffCoPExamples(t *testing.T) {
	for _, c := range copExamples {
		// The CoP writes one twist digit per somersault; pad to the engine's phase count.
		twists := make([]int, CalculatePhases(c.rotation))
		copy(twists, c.twists)
		s := skill(c.rotation, twists, Feet, c.shape, c.backward, false)

		t.Run(fmt.Sprintf("%s %s", c.name, c.shape), func(t *testing.T) {
			if got := s.SetTariff(); math.Abs(got-c.want) > 1e-9 {
				t.Errorf("SetTariff = %.2f, want %.2f", got, c.want)
			}
		})
	}
}
