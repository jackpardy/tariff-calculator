// Package demo makes a competition full of made-up clubs, gymnasts,
// entries and officials, for showing the competition tools off. Nothing in
// it is real.
package demo

import (
	"encoding/json"
	"math/rand/v2"
	"slices"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/skills"
)

// pool is every skill a demo routine can use, by take-off position.
var pool = func() map[skills.BodyPosition][]skills.TrampolineSkill {
	out := map[skills.BodyPosition][]skills.TrampolineSkill{}
	twists := map[int][][]int{
		0: {{0}, {1}, {2}}, 1: {{0}, {1}}, 2: {{0}}, 3: {{0}, {1}},
		4: {{0}, {1}, {2}, {3}}, 5: {{0}, {1}}, 6: {{0}, {0, 0}},
		7: {{0, 0}}, 8: {{0, 0}, {0, 1}, {1, 0}, {1, 1}, {0, 2}},
	}
	for _, from := range []skills.BodyPosition{skills.Feet, skills.Seat, skills.Front, skills.Back} {
		for rot := 0; rot <= 8; rot++ {
			for _, back := range []bool{false, true} {
				for _, shape := range []skills.Shape{skills.Straight, skills.Tuck, skills.Pike, skills.Straddle} {
					for _, tw := range twists[rot] {
						for _, seat := range []bool{false, true} {
							s := skills.TrampolineSkill{Rotation: rot, TwistDistribution: tw, TakeoffPosition: from, Shape: shape, Backward: back, SeatLanding: seat}
							if s.Validate() != nil {
								continue
							}
							land := s.LandingPosition()
							if land != skills.Feet && land != skills.Seat && land != skills.Front && land != skills.Back {
								continue
							}
							s.Tariff = s.SetTariff()
							s.Name = skills.FindCommonSkillName(s)
							out[from] = append(out[from], s)
						}
					}
				}
			}
		}
	}
	return out
}()

// walk is a random routine of n skills, each taking off where the last
// landed, starting and ending on the feet. somersaults is how likely each
// skill from the feet is a somersault (270° or more); doubles, a double.
// limit, if not zero, is the most quarter rotations and half twists any
// skill may have.
func walk(rng *rand.Rand, n int, somersaults, doubles float64, limit int) []skills.TrampolineSkill {
	var out []skills.TrampolineSkill
	at := skills.Feet
	for len(out) < n {
		var choices []skills.TrampolineSkill
		want := 0 // 0: no somersault, 1: single, 2: double
		if at == skills.Feet {
			switch r := rng.Float64(); {
			case r < doubles:
				want = 2
			case r < somersaults:
				want = 1
			}
		}
		for _, s := range pool[at] {
			kind := 0
			switch {
			case s.Rotation >= 7:
				kind = 2
			case s.Rotation >= 3:
				kind = 1
			}
			if at == skills.Feet && kind != want {
				continue
			}
			if limit > 0 && (s.Rotation > limit || s.TotalTwist() > limit) {
				continue
			}
			if len(out) == n-1 && s.LandingPosition() != skills.Feet {
				continue // finish on the feet
			}
			choices = append(choices, s)
		}
		if len(choices) == 0 {
			return nil
		}
		s := choices[rng.IntN(len(choices))]
		out = append(out, s)
		at = s.LandingPosition()
	}
	return out
}

// style is how a level's voluntaries are drawn: how likely a somersault,
// and a double, and the most rotation and twist (0 for no limit).
var style = map[string][3]float64{
	"BUCS L1": {0.95, 0.5, 0}, "BUCS L2": {0.9, 0.15, 0}, "BUCS L3": {0.75, 0, 0},
	"BUCS L4": {0.55, 0, 0}, "BUCS L5": {0.2, 0, 0}, "BUCS L6": {0.1, 0, 0}, "BUCS L7": {0, 0, 1},
}

// voluntaries finds up to n voluntaries for a level's exercise that pass
// every check, trying at most tries times.
func voluntaries(c competitions.Competition, level string, exercise int, options [2]string, n, tries int, rng *rand.Rand) []string {
	var out []string
	st := style[level]
	for range tries {
		routine := walk(rng, 10, st[0], st[1], int(st[2]))
		if routine == nil {
			continue
		}
		e := competitions.Entry{Gymnast: "X", Level: level}
		e.Exercises[0].Option, e.Exercises[1].Option = options[0], options[1]
		e.Exercises[exercise].Skills = routine
		card, err := c.Check(e)
		if err != nil || slices.ContainsFunc(card.Problems(), func(p string) bool {
			return strings.HasPrefix(p, [...]string{"First exercise", "Second exercise"}[exercise])
		}) {
			continue
		}
		raw, _ := json.Marshal(routine)
		out = append(out, string(raw))
		if len(out) == n {
			break
		}
	}
	return out
}

// jsonRoutine is a routine as the entry form posts it.
func jsonRoutine(r []skills.TrampolineSkill) (string, error) {
	raw, err := json.Marshal(r)
	return string(raw), err
}
