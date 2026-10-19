package catalog

import (
	"slices"
	"strings"
	"testing"

	"tariffCalculator/skills"
)

func names(rs []Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Skill.Name
	}
	return out
}

func TestSearchByName(t *testing.T) {
	cases := []struct {
		query string
		want  []string // must all appear in the results
	}{
		{"barani", []string{"Tuck Barani", "Pike Barani", "Straight Barani"}},
		{"barani pike", []string{"Pike Barani"}},
		{"BALL out", []string{"Ball-Out Tuck", "Barani Ball-Out Tuck"}},
		{"rudy", []string{"Rudi"}},
		{"randolph", []string{"Randi"}},
		{"half in half out", []string{"Half Half Tuck", "Half Half Pike", "Half Half Straight"}},
		{"full in full out", []string{"Full Full Straight"}},
		{"back somersault", []string{"Tuck Back", "Pike Back", "Straight Back"}},
		{"jump", []string{"Tuck Jump", "Pike Jump", "Straddle Jump"}},
		{"seat", []string{"Seat Drop", "Seat To Feet"}},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			got := strings.Join(names(Search(c.query)), ", ")
			for _, w := range c.want {
				if !strings.Contains(", "+got+",", ", "+w+",") {
					t.Errorf("Search(%q) = [%s], missing %q", c.query, got, w)
				}
			}
		})
	}

	if got := names(Search("barani pike")); len(got) == 0 || got[0] != "Pike Barani" {
		t.Errorf("an exact name should come first, got %v", got)
	}
	if got := Search("straight jump"); len(got) != 0 {
		t.Errorf("a straight jump interrupts a routine, so it isn't offered: %v", names(got))
	}
	if got := Search("   "); got != nil {
		t.Errorf("an empty query should match nothing")
	}
	if got := Search("zzz"); len(got) != 0 {
		t.Errorf("nonsense should match nothing, got %v", names(got))
	}
}

func TestSearchRanksClosestNamesFirst(t *testing.T) {
	// The Back itself (its shape aside), then names starting with "Back".
	got := names(Search("back"))
	if len(got) < 4 || got[0] != "Tuck Back" || !strings.HasPrefix(got[3], "Back") {
		t.Errorf("the exact name, then names starting with the query, should come first, got %v", got)
	}
	got = names(Search("barani"))
	if len(got) < 3 || !strings.HasSuffix(got[0], " Barani") || strings.Contains(strings.Join(got[:3], ","), " To ") {
		t.Errorf("the Barani itself should come before longer names, got %v", got)
	}
}

func TestSearchByNotation(t *testing.T) {
	cases := []struct {
		query string
		want  string // the first named result
	}{
		{"8 1 1 <", "Half Half Pike"},
		{"811<", "Half Half Pike"},
		{"(8 1 1 <)", "Half Half Pike"},
		{"8 11 <", "Half Half Pike"}, // CoP style: twists run together
		{"4 3", "Rudi"},
		{"43", "Rudi"},
		{"4 1 o", "Tuck Barani"},
		{"8 - 1 o", "Half-Out Tuck"},
		{"12 - - 1 o", "Trif Half-Out Tuck"},
		{"1200o", "Triple Back Tuck"},
		{"0 2", "Full Twist"},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			rs := Search(c.query)
			if len(rs) == 0 {
				t.Fatalf("Search(%q) found nothing", c.query)
			}
			if rs[0].Skill.Name != c.want {
				t.Errorf("Search(%q) first = %q, want %q (all: %v)", c.query, rs[0].Skill.Name, c.want, names(rs))
			}
			if !rs[0].FromNotation {
				t.Errorf("notation results should be marked as such")
			}
		})
	}
}

func TestNotationOffersBothDirections(t *testing.T) {
	rs := Search("4 - o")
	got := strings.Join(names(rs), ", ")
	if !strings.Contains(got, "Tuck Front") || !strings.Contains(got, "Tuck Back") {
		t.Errorf("4 - o is both a front and a back tuck, got [%s]", got)
	}

	// Without a shape mark, a skill whose shape matters comes in each shape.
	got = strings.Join(names(Search("4 0")), ", ")
	for _, w := range []string{"Tuck Front", "Pike Front", "Straight Front"} {
		if !strings.Contains(got, w) {
			t.Errorf("4 0 should offer %s, got [%s]", w, got)
		}
	}

	// Impossible notation finds nothing rather than guessing.
	for _, q := range []string{"17 0", "4 1 1 1"} {
		if rs := Search(q); len(rs) != 0 {
			t.Errorf("Search(%q) = %v, want nothing", q, names(rs))
		}
	}
}

// Notation doesn't say where a skill takes off: it's offered from feet, the
// back, front and seat, named skills first (Cody, from the front).
func TestNotationFromEveryTakeoff(t *testing.T) {
	takeoffs := func(q string) []string {
		var out []string
		for _, r := range Search(q) {
			out = append(out, r.Skill.TakeoffPosition.String()+" "+directionText(r.Skill.Backward))
		}
		return out
	}
	// 1¼ straight, no twist: from feet both ways, and Cody (from the front).
	got := strings.Join(takeoffs("5 0 /"), ", ")
	if !strings.Contains(got, "Feet forward") || !strings.Contains(got, "Front backward") || strings.Index(got, "Front backward") > strings.Index(got, "Feet forward") {
		t.Errorf("5 0 /: %s", got)
	}
	// 1½ with no twist can't land from feet, but can from the back or front.
	if got := takeoffs("6 0 /"); len(got) == 0 || slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, "Feet") }) {
		t.Errorf("6 0 /: %v", got)
	}
	// A half turn is back to front or front to back.
	if got := names(Search("20")); !slices.Contains(got, "Back To Front") {
		t.Errorf("20: %v", got)
	}
	// What the notation says, for the builder, even where nothing lands.
	if s, ok := NotationReading("6 0 /"); !ok || s.Rotation != 6 || s.TotalTwist() != 0 || s.Shape != skills.Straight {
		t.Errorf("reading of 6 0 /: %+v %v", s, ok)
	}
	if _, ok := NotationReading("barani"); ok {
		t.Error("a name isn't notation")
	}
}

func directionText(backward bool) string {
	if backward {
		return "backward"
	}
	return "forward"
}

// The picker names somersaults "Back s/s" and so on, so searching that way finds them.
func TestSearchSlashS(t *testing.T) {
	for query, want := range map[string]string{"back s/s": "Tuck Back", "front s/s": "Tuck Front", "back s/s to seat": "Back To Seat Tuck"} {
		results := Search(query)
		if len(results) == 0 || results[0].Skill.Name != want {
			t.Errorf("%q: first result %v, want %s", query, results, want)
		}
	}
}

// The names people type find the skill they mean first: "½", "1/2" and
// "half" alike, other words for a shape, and coaches' names.
func TestSearchEverydayNames(t *testing.T) {
	for query, want := range map[string]string{
		"½ twist jump":         "Half Twist",
		"1/2 twist jump":       "Half Twist",
		"1/2 twist":            "Half Twist",
		"½ twist":              "Half Twist",
		"half twist":           "Half Twist",
		"full twist jump":      "Full Twist",
		"twist jump":           "Half Twist",
		"piked back":           "Pike Back",
		"back pike":            "Pike Back",
		"tucked front":         "Tuck Front",
		"layout back":          "Straight Back",
		"laid out barani":      "Straight Barani",
		"straddle":             "Straddle Jump",
		"back drop":            "Back Drop",
		"back drop to feet":    "Back To Feet",
		"front drop to feet":   "Front To Feet",
		"seat drop to feet":    "Seat To Feet",
		"tuck back s/s":        "Tuck Back",
		"double front":         "Double Front Tuck",
		"double back pike":     "Double Back Pike",
		"swivel hips":          "Seat Half Twist To Seat",
		"cradle":               "Back Half Twist To Back",
		"corkscrew":            "Back 1½ Twist To Back",
		"cat twist":            "Back Full Twist To Back",
		"back half":            "Tuck Back Half",
		"straight back half":   "Straight Back Half",
		"triple full":          "Triple Full Back",
		"full in back":         "Full-In Back Tuck",
		"full-in":              "Full-In Back Tuck",
		"full out":             "Full-Out Back Tuck",
		"rudy out":             "Rudi-Out Tuck",
		"randy out":            "Randi-Out Tuck",
		"half in rudy out":     "Half-In Rudi-Out Tuck",
		"half-in half-out":     "Half Half Tuck",
		"½ in ½ out":           "Half Half Tuck",
		"barani out":           "Half-Out Tuck",
		"full half":            "Full-In Half-Out Tuck",
		"rudi out triffis":     "Trif Rudi-Out Tuck",
		"half out triffis":     "Trif Half-Out Tuck",
		"adolph":               "Ada",
		"3 1/2 twisting front": "Ada",
	} {
		got := names(Search(query))
		if len(got) == 0 || got[0] != want {
			t.Errorf("Search(%q) = %v, want %s first", query, got, want)
		}
	}

	// A shape word on its own finds that shape.
	for _, s := range Search("piked") {
		if s.Skill.Shape != skills.Pike {
			t.Errorf("piked found %s", s.Skill.Name)
		}
	}
	// "½ twist" is no full twist.
	if got := names(Search("½ twist")); slices.Contains(got, "Full Twist") {
		t.Errorf("½ twist found a Full Twist: %v", got)
	}
}

// What coaches call a skill is said beside the app's name.
func TestAlsoCalledCoachesNames(t *testing.T) {
	for query, want := range map[string]string{"swivel hips": "Swivel Hips", "cradle": "Cradle", "corkscrew": "Corkscrew", "cat twist": "Cat Twist"} {
		rs := Search(query)
		if len(rs) == 0 {
			t.Errorf("%s: nothing found", query)
		} else if got := AlsoCalled(rs[0].Skill); got != want {
			t.Errorf("%s: also called %q", query, got)
		}
	}
}

// The reviewer's notation finds the named skill first, backwards where
// that's the one with a name.
func TestNotationNamesTwistingSkills(t *testing.T) {
	for query, want := range map[string]string{
		"8 2 - o":    "Full-In Back Tuck",
		"8 - 2 /":    "Full-Out Back Straight",
		"8 - 3 o":    "Rudi-Out Tuck",
		"8 - 5 /":    "Randi-Out Straight",
		"8 1 3 <":    "Half-In Rudi-Out Pike",
		"8 1 5 o":    "Half-In Randi-Out Tuck",
		"8 2 1 o":    "Full-In Half-Out Tuck",
		"8 2 5 /":    "Full Randi Straight",
		"8 - - o":    "Double Front Tuck",
		"4 6":        "Triple Full Back",
		"4 7":        "Ada",
		"4 2":        "Front Full",
		"12 - - 3 o": "Trif Rudi-Out Tuck",
		"12 1 - 1 <": "Trif Half-In Half-Out Pike",
		"12 1 - 3 o": "Trif Half-In Rudi-Out Tuck",
		"12 2 - 1 o": "Trif Full-In Half-Out Tuck",
		"12 2 2 2 o": "Full Full Full Tuck",
	} {
		rs := Search(query)
		if len(rs) == 0 || rs[0].Skill.Name != want {
			t.Errorf("Search(%q) = %v, want %s first", query, names(rs), want)
		}
	}
	// 4 1 is a Barani forwards and a Back Half backwards; both are named.
	if got := names(Search("4 1 o")); len(got) < 2 || got[0] != "Tuck Barani" || got[1] != "Tuck Back Half" {
		t.Errorf("4 1 o: %v", got)
	}
}
