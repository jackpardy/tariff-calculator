package requirements

import (
	"strings"
	"testing"

	"tariffCalculator/skills"
)

func skill(rotation int, twists []int, shape skills.Shape, backward bool) skills.TrampolineSkill {
	return skills.TrampolineSkill{Rotation: rotation, TwistDistribution: twists, TakeoffPosition: skills.Feet, Shape: shape, Backward: backward}
}

var (
	frontTuck     = skill(4, []int{0}, skills.Tuck, false)
	backPike      = skill(4, []int{0}, skills.Pike, true)
	baraniTuck    = skill(4, []int{1}, skills.Tuck, false)
	rudi          = skill(4, []int{3}, skills.Straight, false)
	doubleBack    = skill(8, []int{0, 0}, skills.Tuck, true)
	seatDrop      = skills.TrampolineSkill{TwistDistribution: []int{0}, TakeoffPosition: skills.Feet, SeatLanding: true}
	seatToFeet    = skills.TrampolineSkill{TwistDistribution: []int{0}, TakeoffPosition: skills.Seat}
	sampleRoutine = []skills.TrampolineSkill{frontTuck, backPike, baraniTuck, rudi, seatDrop, seatToFeet}
)

func ptr[T any](v T) *T { return &v }

func evaluate(t *testing.T, rules ...Rule) []Result {
	t.Helper()
	set := Set{Format: Format, Name: "Test", Rules: rules}
	if err := set.Validate(); err != nil {
		t.Fatalf("invalid test set: %v", err)
	}
	return Evaluate(set, skills.ValidateRoutine(sampleRoutine))
}

func TestParseAndValidate(t *testing.T) {
	good := `{"format":1,"name":"Novice","rules":[
		{"type":"count","match":{"direction":"backward","rotation":{"min":4}},"min":1,"label":"A back somersault"},
		{"type":"every","match":{"rotation":{"max":5}}},
		{"type":"elements","min":10,"max":10},
		{"type":"difficulty","max":3.5},
		{"type":"position","position":10,"match":{"landing":["feet"]}},
		{"type":"sequence","sequence":[{"fig":"4 - o"},{"fig":"(4 1 o)"}]}
	]}`
	set, err := Parse([]byte(good))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(set.Rules) != 6 || set.Rules[0].Label != "A back somersault" {
		t.Errorf("parsed %+v", set)
	}

	bad := map[string]string{
		"unknown field":       `{"format":1,"name":"x","rules":[],"colour":"red"}`,
		"wrong format":        `{"format":2,"name":"x","rules":[]}`,
		"no name":             `{"format":1,"name":" ","rules":[]}`,
		"unknown type":        `{"format":1,"name":"x","rules":[{"type":"vibes"}]}`,
		"count without match": `{"format":1,"name":"x","rules":[{"type":"count","min":1}]}`,
		"count without bound": `{"format":1,"name":"x","rules":[{"type":"count","match":{}}]}`,
		"fractional count":    `{"format":1,"name":"x","rules":[{"type":"count","match":{},"min":1.5}]}`,
		"min over max":        `{"format":1,"name":"x","rules":[{"type":"elements","min":10,"max":8}]}`,
		"bad shape":           `{"format":1,"name":"x","rules":[{"type":"every","match":{"shapes":["layout"]}}]}`,
		"bad position name":   `{"format":1,"name":"x","rules":[{"type":"every","match":{"landing":["head"]}}]}`,
		"bad direction":       `{"format":1,"name":"x","rules":[{"type":"every","match":{"direction":"sideways"}}]}`,
		"position zero":       `{"format":1,"name":"x","rules":[{"type":"position","position":0,"match":{}}]}`,
		"empty sequence":      `{"format":1,"name":"x","rules":[{"type":"sequence","sequence":[]}]}`,
	}
	for name, data := range bad {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}

	// Every problem is reported, not just the first.
	_, err = Parse([]byte(`{"format":1,"name":"","rules":[{"type":"vibes"},{"type":"elements"}]}`))
	if err == nil || strings.Count(err.Error(), "\n") < 2 {
		t.Errorf("want three problems reported, got: %v", err)
	}
}

func TestMatcher(t *testing.T) {
	cases := []struct {
		name string
		m    Matcher
		s    skills.TrampolineSkill
		want bool
	}{
		{"any", Matcher{}, frontTuck, true},
		{"backward somersault", Matcher{Direction: "backward", Rotation: &Range{Min: ptr(4)}}, backPike, true},
		{"forward isn't backward", Matcher{Direction: "backward"}, frontTuck, false},
		{"no twist", Matcher{Twist: &Range{Max: ptr(0)}}, baraniTuck, false},
		{"at least a full twist", Matcher{Twist: &Range{Min: ptr(2)}}, rudi, true},
		{"single somersault at most", Matcher{Rotation: &Range{Max: ptr(7)}}, doubleBack, false},
		{"pike", Matcher{Shapes: []string{"pike"}}, backPike, true},
		{"tuck or pike", Matcher{Shapes: []string{"tuck", "pike"}}, frontTuck, true},
		{"shape that doesn't matter counts as straight", Matcher{Shapes: []string{"straight"}}, rudi, true},
		{"seat landing", Matcher{Landing: []string{"seat"}}, seatDrop, true},
		{"from seat", Matcher{Takeoff: []string{"seat"}}, seatToFeet, true},
		{"tariff at least", Matcher{Tariff: &TariffRange{Min: ptr(0.6)}}, baraniTuck, true},
		{"tariff below", Matcher{Tariff: &TariffRange{Min: ptr(0.6)}}, frontTuck, false},
		{"exact FIG", Matcher{FIG: "4 1 o"}, baraniTuck, true},
		{"FIG ignores brackets and spaces", Matcher{FIG: "(41o)"}, baraniTuck, true},
		{"different FIG", Matcher{FIG: "4 - <"}, frontTuck, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := c.s
			s.SetTariff()
			if got := c.m.Matches(s); got != c.want {
				t.Errorf("Matches = %v, want %v", got, c.want)
			}
		})
	}
}

func TestEvaluate(t *testing.T) {
	results := evaluate(t,
		Rule{Type: Count, Match: &Matcher{Direction: "backward", Rotation: &Range{Min: ptr(4)}}, Min: ptr(1.0)},
		Rule{Type: Count, Match: &Matcher{Rotation: &Range{Min: ptr(8)}}, Max: ptr(0.0)},
		Rule{Type: Count, Match: &Matcher{Twist: &Range{Min: ptr(1)}}, Min: ptr(3.0)},
		Rule{Type: Every, Match: &Matcher{Rotation: &Range{Max: ptr(4)}}},
		Rule{Type: Every, Match: &Matcher{Twist: &Range{Max: ptr(0)}}},
		Rule{Type: Elements, Min: ptr(10.0), Max: ptr(10.0)},
		Rule{Type: Difficulty, Max: ptr(3.0)},
		Rule{Type: Position, Position: 6, Match: &Matcher{Landing: []string{"feet"}}},
		Rule{Type: Position, Position: 10, Match: &Matcher{}},
		Rule{Type: Sequence, Sequence: []Matcher{{FIG: "4 - o", Direction: "forward"}, {FIG: "4 - <"}}},
	)
	want := []struct {
		passed   bool
		elements []int
		detail   string
	}{
		{true, []int{2}, "found 1"},
		{true, nil, "found 0"},
		{false, []int{3, 4}, "found 2"},
		{true, nil, ""},
		{false, []int{3, 4}, "not elements 3, 4"},
		{false, nil, "has 6"},
		{true, nil, "is 2.7"}, // 0.5 + 0.6 + 0.6 + 0.8 + 0.1 + 0.1
		{true, nil, ""},
		{false, []int{10}, "there is no element 10"},
		{false, []int{3, 4, 5, 6}, "differs at 3, 4, 5, 6"},
	}
	for i, w := range want {
		r := results[i]
		if r.Passed != w.passed || r.Detail != w.detail || !equalInts(r.Elements, w.elements) {
			t.Errorf("rule %d (%s): got passed=%v detail=%q elements=%v, want %v %q %v",
				i+1, r.Description, r.Passed, r.Detail, r.Elements, w.passed, w.detail, w.elements)
		}
	}
}

func TestRequiredElements(t *testing.T) {
	set := Set{Format: Format, Name: "x", Rules: []Rule{
		{Type: Count, Match: &Matcher{Direction: "backward"}, Min: ptr(1.0)},
		{Type: Count, Match: &Matcher{Landing: []string{"seat"}}, Min: ptr(1.0)},
		{Type: Count, Match: &Matcher{Rotation: &Range{Min: ptr(8)}}, Max: ptr(0.0)}, // a ban, not a requirement
	}}
	got := RequiredElements(set, Evaluate(set, skills.ValidateRoutine(sampleRoutine)))
	if len(got) != 2 || !got[2] || !got[5] {
		t.Errorf("RequiredElements = %v, want elements 2 and 5", got)
	}
}

func TestDescribe(t *testing.T) {
	cases := []struct {
		rule Rule
		want string
	}{
		{Rule{Type: Count, Label: "A back somersault"}, "A back somersault"},
		{Rule{Type: Count, Match: &Matcher{Direction: "backward", Rotation: &Range{Min: ptr(4)}}, Min: ptr(1.0)}, "At least 1 element: backward, at least 1 somersault"},
		{Rule{Type: Count, Match: &Matcher{Rotation: &Range{Min: ptr(8)}}, Max: ptr(0.0)}, "No elements: at least 2 somersaults"},
		{Rule{Type: Count, Match: &Matcher{Twist: &Range{Min: ptr(3)}}, Min: ptr(2.0), Max: ptr(2.0)}, "Exactly 2 elements: at least 1½ twists"},
		{Rule{Type: Every, Match: &Matcher{Twist: &Range{Max: ptr(0)}, Shapes: []string{"tuck", "pike"}}}, "Every element: no twist, in tuck or pike"},
		{Rule{Type: Every, Match: &Matcher{Rotation: &Range{Max: ptr(5)}}}, "Every element: at most 1¼ somersaults"},
		{Rule{Type: Every, Match: &Matcher{}}, "Every element: any element"},
		{Rule{Type: Elements, Min: ptr(10.0), Max: ptr(10.0)}, "Exactly 10 elements"},
		{Rule{Type: Difficulty, Min: ptr(2.5)}, "Difficulty at least 2.5"},
		{Rule{Type: Position, Position: 10, Match: &Matcher{Landing: []string{"feet"}}}, "Element 10: landing on feet"},
		{Rule{Type: Count, Match: &Matcher{Landing: []string{"seat", "front", "back"}}, Min: ptr(1.0)}, "At least 1 element: landing on seat, front or back"},
		{Rule{Type: Sequence, Sequence: []Matcher{{}, {}}}, "Set routine of 2 elements"},
	}
	for _, c := range cases {
		if got := Describe(c.rule); got != c.want {
			t.Errorf("Describe = %q, want %q", got, c.want)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuiltinsLoad(t *testing.T) {
	if len(Builtins()) == 0 {
		t.Fatal("no built-in sets")
	}
	if _, ok := LookupBuiltin("builtin:example-club-novice"); !ok {
		t.Errorf("the example set should be found by reference")
	}
	if _, ok := LookupBuiltin("builtin:nope"); ok {
		t.Errorf("an unknown reference should not be found")
	}
}
