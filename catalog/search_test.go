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
	rs := Search("back")
	if len(rs) == 0 || !strings.HasPrefix(rs[0].Skill.Name, "Back") {
		t.Errorf("names starting with the query should come first, got %v", names(rs))
	}
	got := names(Search("barani"))
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
