package catalog

import (
	"fmt"
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
				// A body-landing tile gives each of its versions.
				for _, v := range e.Versions() {
					for key, s := range skills.CommonSkills {
						if s.Equal(&v) {
							seen[key] = c.Key
						}
					}
				}
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
		// Somersaults landing on the body are singles, not body landings.
		"front": "singles", "backSomersault": "singles", "crashDive": "singles", "ballOut": "singles", "frontToSeat": "singles",
		"barani": "singles", "rudi": "singles", "fullBack": "singles", "baraniToFront": "singles",
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
			if e.Key == "barani" && (e.Skill.Name != "Tuck Barani" || e.Skill.Tariff != 0.6) {
				t.Errorf("barani = %q %.1f, want Tuck Barani 0.6", e.Skill.Name, e.Skill.Tariff)
			}
			if e.ID() == "shapeJump-tuck" && e.Skill.Name != "Tuck Jump" {
				t.Errorf("the tuck shape jump should read as Tuck Jump, got %q", e.Skill.Name)
			}
		}
	}
}

func TestPickerName(t *testing.T) {
	want := map[string]string{
		"backSomersault": "Back",
		"front":          "Front",
		"backToSeat":     "Back To Seat",
		"frontToSeat":    "Front To Seat",
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
	if got := PickerName(pike); got != "Back" {
		t.Errorf("back pike: %q", got)
	}
	// The rule books' name is said beside it.
	if got := AlsoCalled(pike); got != "Back s/s Pike" {
		t.Errorf("also called: %q", got)
	}
	if got := AlsoCalled(skills.CommonSkills["barani"]); got != "" {
		t.Errorf("a barani is a barani: %q", got)
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
		"drops To feet: To Feet, ½ Twist To Feet",
		"drops To seat: To Seat, ½ Twist To Seat",
		"drops To back: To Back, ½ Twist To Back, Full Twist To Back, 1½ Twist To Back",
		"drops To front: To Front, ½ Twist To Front",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("groups:\n%s", strings.Join(got, "\n"))
	}
}

// A body-landing tile takes off from where the skill before it landed: each
// tile after feet, seat, back and front, and "(usual)" where nothing lands
// like that from there, so it's the tile's usual skill.
func TestLandingTilesFollow(t *testing.T) {
	from := []skills.BodyPosition{skills.Feet, skills.Seat, skills.Back, skills.Front}
	want := map[string][4]string{
		"To Feet":            {"Seat To Feet 0.1 (usual)", "Seat To Feet 0.1", "Back To Feet 0.1", "Front To Feet 0.1"},
		"½ Twist To Feet":    {"Seat Half Twist To Feet 0.1 (usual)", "Seat Half Twist To Feet 0.1", "Back Half Twist To Feet 0.2", "Front Half Twist To Feet 0.2"},
		"To Seat":            {"Seat Drop 0.1", "Seat Drop 0.1 (usual)", "Seat Drop 0.1 (usual)", "Seat Drop 0.1 (usual)"},
		"½ Twist To Seat":    {"Half Twist To Seat 0.1", "Seat Half Twist To Seat 0.1", "Back Half Twist To Seat 0.2", "Front Half Twist To Seat 0.2"},
		"To Back":            {"Back Drop 0.1", "Seat To Back 0.1", "Back Drop 0.1 (usual)", "Front To Back 0.2"},
		"½ Twist To Back":    {"Half Twist To Back 0.2", "Seat Half Twist To Back 0.2", "Back Half Twist To Back 0.3", "Front Half Twist To Back 0.1"},
		"Full Twist To Back": {"Full Twist To Back 0.3", "Seat Full Twist To Back 0.3", "Back Full Twist To Back 0.2", "Front Full Twist To Back 0.4"},
		"1½ Twist To Back":   {"1½ Twist To Back 0.4", "Seat 1½ Twist To Back 0.4", "Back 1½ Twist To Back 0.5", "Front 1½ Twist To Back 0.3"},
		"To Front":           {"Front Drop 0.1", "Seat To Front 0.1", "Back To Front 0.2", "Front Drop 0.1 (usual)"},
		"½ Twist To Front":   {"Half Twist To Front 0.2", "Seat Half Twist To Front 0.2", "Back Half Twist To Front 0.1", "Front Half Twist To Front 0.3"},
	}
	tiles := 0
	for _, c := range Categories() {
		for _, g := range c.Groups {
			for _, e := range g.Options {
				tiles++
				w, ok := want[e.Label]
				if !ok {
					t.Errorf("no test for the %q tile", e.Label)
					continue
				}
				for i, p := range from {
					s, followed := e.Follow(p)
					got := fmt.Sprintf("%s %.1f", s.Name, s.Tariff)
					if !followed {
						got += " (usual)"
					}
					if got != w[i] {
						t.Errorf("%s after %s: %s, want %s", e.Label, p, got, w[i])
					}
					// What it gives lands where the tile says, from there.
					if followed && (s.TakeoffPosition != p || s.LandingPosition() != e.Tile.To || s.TotalTwist() != e.Tile.Twist) {
						t.Errorf("%s after %s gives %+v", e.Label, p, s)
					}
				}
			}
		}
	}
	if tiles != len(want) {
		t.Errorf("%d tiles, want %d", tiles, len(want))
	}

	// The cradle is the catalogue's own, forwards, though backwards lands the same.
	cradle, _ := LandingTile("halfToBack")
	cradleSkill := skills.CommonSkills["cradle"]
	if s, _ := cradle.Follow(skills.Back); !s.Equal(&cradleSkill) || s.Backward {
		t.Errorf("½ twist to back after a back drop: %+v, want the cradle", s)
	}
	if _, ok := LandingTile("barani"); ok {
		t.Error("a barani isn't a body-landing tile")
	}
}

// Picker boxes show the tuck tariff, and what pike and straight add.
func TestShapeModifiers(t *testing.T) {
	want := map[string]string{
		"backSomersault": "0.5: Tuck +0.0, Pike +0.1, Straight +0.1",
		"doubleBack":     "1.1: Tuck +0.0, Pike +0.2, Straight +0.2",
		"barani":         "0.6: Tuck +0.0, Pike +0.0, Straight +0.0", // no bonus with a twist
		"crashDive":      "0.3: Tuck +0.0, Pike +0.0, Straight +0.0", // or under 360°
		"rudi":           "0.8: ",                                    // shape doesn't matter
	}
	for _, c := range Categories() {
		for _, e := range c.Entries {
			w, ok := want[e.Key]
			if !ok {
				continue
			}
			var parts []string
			for _, o := range e.ShapeOptions() {
				parts = append(parts, fmt.Sprintf("%s +%.1f", o.Shape, o.Modifier))
			}
			if got := fmt.Sprintf("%.1f: %s", e.BaseTariff(), strings.Join(parts, ", ")); got != w {
				t.Errorf("%s: %s, want %s", e.Key, got, w)
			}
		}
	}
}
