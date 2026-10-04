package catalog

import (
	"strings"
	"testing"

	"tariffCalculator/skills"
)

func TestCategoriesCoverEveryCommonSkillOnce(t *testing.T) {
	seen := map[string]string{}
	for _, c := range Categories() {
		for _, g := range c.Groups {
			for _, e := range g.Options {
				if prev, dup := seen[e.ID()]; dup {
					t.Errorf("%s is in both %s and %s", e.ID(), prev, c.Key)
				}
				seen[e.ID()] = c.Key
				seen[e.Key] = c.Key
			}
		}
		for _, e := range c.Entries {
			if prev, dup := seen[e.ID()]; dup {
				t.Errorf("%s is in both %s and %s", e.ID(), prev, c.Key)
			}
			seen[e.ID()] = c.Key
			seen[e.Key] = c.Key
		}
	}
	for _, c := range Categories() {
		for i := 1; i < len(c.Entries); i++ {
			if c.Entries[i-1].Skill.Tariff > c.Entries[i].Skill.Tariff {
				t.Errorf("%s is not sorted by tariff at %d", c.Key, i)
			}
		}
	}

	want := map[string]string{
		"shapeJump": "jumps", "halfTwist": "jumps", "fullTwist": "jumps",
		"seatDrop": "drops", "seatToFeet": "drops", "backDrop": "drops", "halfToSeat": "drops", "seatHalfToFront": "drops",
		"front": "somersaults", "backSomersault": "somersaults", "crashDive": "somersaults", "ballOut": "somersaults", "frontToSeat": "somersaults",
		"barani": "twists", "rudi": "twists", "fullBack": "twists", "baraniToFront": "twists",
		"doubleBack": "doubles", "halfOut": "doubles", "miller": "doubles",
		"tripleBack": "triples", "trifHalfOut": "triples",
	}
	for key, cat := range want {
		if seen[key] != cat {
			t.Errorf("%s is in %q, want %q", key, seen[key], cat)
		}
	}
}

func TestEntriesAreNamedAndPriced(t *testing.T) {
	for _, c := range Categories() {
		for _, e := range c.Entries {
			if e.Key == "barani" && (e.Skill.Name != "Barani Tuck" || e.Skill.Tariff != 0.6) {
				t.Errorf("barani = %q %.1f, want Barani Tuck 0.6", e.Skill.Name, e.Skill.Tariff)
			}
			if e.ID() == "shapeJump-tuck" && e.Skill.Name != "Tuck Jump" {
				t.Errorf("the tuck shape jump should read as Tuck Jump, got %q", e.Skill.Name)
			}
		}
	}
}

func TestPickerName(t *testing.T) {
	want := map[string]string{
		"backSomersault": "Back s/s",
		"front":          "Front s/s",
		"backToSeat":     "Back s/s To Seat",
		"frontToSeat":    "Front s/s To Seat",
		"crashDive":      "Crash Dive",
		"lazyBack":       "Lazy Back",
		"ballOut":        "Ball-Out",
		"cody":           "Cody",
		"barani":         "Barani",
		"doubleBack":     "Double Back",
		"shapeJump":      "Tuck Jump", // the shape is the skill
		"rudi":           "Rudi",
		"seatDrop":       "Seat Drop",
	}
	for key, name := range want {
		if got := PickerName(skills.CommonSkills[key]); got != name {
			t.Errorf("%s: PickerName = %q, want %q", key, got, name)
		}
	}
	// Any shape gives the same name.
	pike := skills.CommonSkills["backSomersault"]
	pike.Shape = skills.Pike
	if got := PickerName(pike); got != "Back s/s" {
		t.Errorf("back pike: %q", got)
	}
}

func TestPickerShapes(t *testing.T) {
	var jumps []string
	shapes := map[string]int{}
	for _, c := range Categories() {
		for _, e := range c.Entries {
			if e.Key == "shapeJump" {
				jumps = append(jumps, e.Skill.Name+" "+e.ID())
			}
			shapes[e.Key] = len(e.Shapes())
		}
	}
	if got := strings.Join(jumps, ", "); got != "Tuck Jump shapeJump-tuck, Pike Jump shapeJump-pike, Straddle Jump shapeJump-straddle" {
		t.Errorf("jumps: %s", got)
	}
	for key, want := range map[string]int{"backSomersault": 3, "crashDive": 3, "barani": 3, "doubleBack": 3, "rudi": 0, "fullBack": 0, "backDrop": 0, "halfTwist": 0, "shapeJump": 0} {
		if shapes[key] != want {
			t.Errorf("%s offers %d shapes, want %d", key, shapes[key], want)
		}
	}
}

func TestPickerGroups(t *testing.T) {
	var got []string
	for _, c := range Categories() {
		for _, g := range c.Groups {
			var labels []string
			for _, e := range g.Options {
				labels = append(labels, e.Label)
			}
			got = append(got, c.Key+" "+g.Title+": "+strings.Join(labels, ", "))
		}
		if c.Key == "drops" && len(c.Entries) != 0 {
			t.Errorf("every drop should be in a group, but these aren't: %v", c.Entries)
		}
	}
	want := []string{
		"drops To seat: Seat Drop, ½ Twist To Seat",
		"drops From seat: To Feet, ½ Twist To Feet, ½ Twist To Seat, ½ Twist To Front",
		"drops To back or front: Back Drop, Front Drop, ½ Twist To Front",
		"drops From back or front: Back To Feet, Back ½ Twist To Feet, Front To Feet",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("groups:\n%s", strings.Join(got, "\n"))
	}
}
