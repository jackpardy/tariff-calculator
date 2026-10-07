package judging

import (
	"strings"
	"testing"

	"tariffCalculator/skills"
)

func skill(rotation int, twists []int, shape skills.Shape, backward bool) skills.TrampolineSkill {
	s := skills.TrampolineSkill{
		Rotation:          rotation,
		TwistDistribution: twists,
		TakeoffPosition:   skills.Feet,
		Shape:             shape,
		Backward:          backward,
	}
	s.NormalizePhases()
	return s
}

// faults is the set of faults the guide lists, by group.
func faults(g Guide) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, gr := range g.Groups {
		out[gr.Title] = map[string]bool{}
		for _, it := range gr.Items {
			out[gr.Title][it.Fault] = true
		}
	}
	return out
}

func TestFor(t *testing.T) {
	seatDrop := skills.TrampolineSkill{TakeoffPosition: skills.Feet, SeatLanding: true, Shape: skills.Straight, TwistDistribution: []int{0}}
	backDrop := skill(1, []int{0}, skills.Straight, true)
	tests := []struct {
		name  string
		skill skills.TrampolineSkill
		last  bool
		has   map[string][]string // group → faults that must be listed
		not   map[string][]string // group → faults that mustn't
	}{
		{
			name:  "tuck jump",
			skill: skill(0, []int{0}, skills.Tuck, false),
			has:   map[string][]string{Shape: {"Open tuck", "Hands behind the knees"}, Legs: {"Feet apart", "Toes not pointed"}},
			not:   map[string][]string{Shape: {"Bent knees"}, Opening: {"Late or no opening", "Piking down"}},
		},
		{
			name:  "straddle jump",
			skill: skill(0, []int{0}, skills.Straddle, false),
			has:   map[string][]string{Shape: {"Legs below horizontal", "Bent knees"}, Legs: {"Toes not pointed"}},
			not:   map[string][]string{Legs: {"Feet apart", "Knees apart"}},
		},
		{
			name:  "full twist jump is judged straight",
			skill: skill(0, []int{2}, skills.Tuck, false),
			has:   map[string][]string{Shape: {"Piked or arched body", "Bent knees"}},
			not:   map[string][]string{Shape: {"Open tuck"}, Arms: {"Arms too wide stopping the twist"}},
		},
		{
			name:  "seat drop has no shape",
			skill: seatDrop,
			has:   map[string][]string{Legs: {"Feet apart"}, Arms: {"Bent elbows"}},
			not:   map[string][]string{Shape: {"Open tuck", "Piked or arched body"}, Opening: {"Piking down"}},
		},
		{
			name:  "back drop has no opening",
			skill: backDrop,
			not:   map[string][]string{Opening: {"Late or no opening", "Piking down"}},
		},
		{
			name:  "tuck back opens",
			skill: skill(4, []int{0}, skills.Tuck, true),
			has:   map[string][]string{Shape: {"Open tuck"}, Opening: {"Late or no opening", "Piking down"}},
			not:   map[string][]string{Opening: {"Twist finishing late", "No opening needed"}, Landing: {"Falling"}},
		},
		{
			name:  "straight back needs no opening",
			skill: skill(4, []int{0}, skills.Straight, true),
			has:   map[string][]string{Opening: {"No opening needed", "Piking down"}},
			not:   map[string][]string{Opening: {"Late or no opening"}},
		},
		{
			name:  "rudi finishes its twist",
			skill: skill(4, []int{3}, skills.Tuck, false),
			has:   map[string][]string{Shape: {"Piked or arched body"}, Opening: {"Twist finishing late"}, Arms: {"Bent elbows", "Arms too wide stopping the twist"}},
		},
		{
			name:  "double full may bend the elbows",
			skill: skill(4, []int{4}, skills.Straight, true),
			has:   map[string][]string{Arms: {"Bent elbows are allowed"}},
			not:   map[string][]string{Arms: {"Bent elbows"}},
		},
		{
			name:  "half out pike",
			skill: skill(8, []int{0, 1}, skills.Pike, false),
			has:   map[string][]string{Shape: {"Open pike", "Bent knees", "When the shape is judged"}, Opening: {"Late or no opening"}},
			not:   map[string][]string{Opening: {"Twist finishing late"}},
		},
		{
			name:  "last element adds the landing",
			skill: skill(8, []int{0, 0}, skills.Tuck, true),
			last:  true,
			has:   map[string][]string{Landing: {"Steps or bounces", "Falling", "An extra element"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := faults(For(tt.skill, tt.last))
			for group, fs := range tt.has {
				for _, f := range fs {
					if !got[group][f] {
						t.Errorf("%s: missing %q; got %v", group, f, got[group])
					}
				}
			}
			for group, fs := range tt.not {
				for _, f := range fs {
					if got[group][f] {
						t.Errorf("%s: should not list %q", group, f)
					}
				}
			}
		})
	}
}

func TestArmAngle(t *testing.T) {
	arms := func(s skills.TrampolineSkill) string {
		for _, g := range For(s, false).Groups {
			for _, it := range g.Items {
				if it.Fault == "Arms too wide stopping the twist" {
					return it.Explain
				}
			}
		}
		return ""
	}
	tests := []struct {
		name  string
		skill skills.TrampolineSkill
		want  string
	}{
		{"barani", skill(4, []int{1}, skills.Tuck, false), "up to 45°"},
		{"full", skill(4, []int{2}, skills.Straight, true), "up to 45°"},
		{"half out", skill(8, []int{0, 1}, skills.Tuck, false), "up to 45°"},
		{"rudi", skill(4, []int{3}, skills.Straight, false), "up to 90°"},
		{"rudi out", skill(8, []int{0, 3}, skills.Tuck, false), "up to 90°"},
		{"full in", skill(8, []int{2, 0}, skills.Tuck, true), "up to 90°"},
	}
	for _, tt := range tests {
		if got := arms(tt.skill); !strings.Contains(got, tt.want) {
			t.Errorf("%s: %q doesn't say %q", tt.name, got, tt.want)
		}
	}
}

// TestEverySkill runs the rules over every skill the builder can make and checks
// each list makes sense for it.
func TestEverySkill(t *testing.T) {
	n := 0
	for _, s := range everySkill() {
		n++
		g := For(s, true)
		got := faults(g)
		somersault := s.Rotation >= 3
		if len(got[Legs]) == 0 || len(got[Arms]) == 0 || len(got[Landing]) == 0 {
			t.Fatalf("%s %s: missing a group: %v", s.Name, s.FIGNotation(), got)
		}
		if !somersault && len(got[Opening]) > 0 {
			t.Errorf("%s %s: opening rules on a non-somersault", s.Name, s.FIGNotation())
		}
		if somersault && !got[Opening]["Piking down"] {
			t.Errorf("%s %s: somersault without piking down", s.Name, s.FIGNotation())
		}
		if got[Opening]["Late or no opening"] && got[Opening]["No opening needed"] {
			t.Errorf("%s %s: says both open and don't", s.Name, s.FIGNotation())
		}
		for _, gr := range g.Groups {
			for _, it := range gr.Items {
				if it.Fault == "" || it.Explain == "" || it.Ref == "" {
					t.Errorf("%s: incomplete item %+v", s.Name, it)
				}
			}
		}
	}
	if n < 1000 {
		t.Fatalf("only %d skills", n)
	}
}

// everySkill is every valid skill with a valid landing: each rotation, twist
// per phase (up to 4 half twists each), take-off, shape, direction and seat landing.
func everySkill() []skills.TrampolineSkill {
	var out []skills.TrampolineSkill
	var twists func(phases int) [][]int
	twists = func(phases int) [][]int {
		if phases == 0 {
			return [][]int{{}}
		}
		var all [][]int
		for _, rest := range twists(phases - 1) {
			for t := 0; t <= 4; t++ {
				all = append(all, append([]int{t}, rest...))
			}
		}
		return all
	}
	for rot := 0; rot <= skills.MaxRotation; rot++ {
		phases := skills.CalculatePhases(rot)
		if phases > 3 {
			phases = 3 // quads: enough combinations to cover the rules
		}
		for _, tw := range twists(phases) {
			for _, from := range []skills.BodyPosition{skills.Feet, skills.Front, skills.Back, skills.Seat} {
				for _, shape := range []skills.Shape{skills.Tuck, skills.Pike, skills.Straight, skills.Straddle} {
					for _, back := range []bool{false, true} {
						for _, seat := range []bool{false, true} {
							s := skills.TrampolineSkill{Rotation: rot, TwistDistribution: tw, TakeoffPosition: from, Shape: shape, Backward: back, SeatLanding: seat}
							s.NormalizePhases()
							if s.Validate() != nil || s.LandingPosition() == skills.Invalid {
								continue
							}
							s.Name = skills.FindCommonSkillName(s)
							out = append(out, s)
						}
					}
				}
			}
		}
	}
	return out
}
