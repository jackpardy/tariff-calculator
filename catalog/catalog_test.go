package catalog

import "testing"

func TestCategoriesCoverEveryCommonSkillOnce(t *testing.T) {
	seen := map[string]string{}
	for _, c := range Categories() {
		for _, e := range c.Entries {
			if prev, dup := seen[e.Key]; dup {
				t.Errorf("%s is in both %s and %s", e.Key, prev, c.Key)
			}
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
			if e.Key == "shapeJump" && e.Skill.Name != "Tuck Jump" {
				t.Errorf("the shape jump should read as Tuck Jump, got %q", e.Skill.Name)
			}
		}
	}
}
