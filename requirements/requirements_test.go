package requirements

import (
	"slices"
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
		{Rule{Type: Sequence, Sequence: []Matcher{{Label: "Tuck jump"}, {Label: "Seat landing"}}}, "Set routine: Tuck jump, Seat landing"},
		{Rule{Type: Separate, Each: []Matcher{{Label: "To front or back"}, {Twist: &Range{Min: ptr(3)}, Rotation: &Range{Max: ptr(5)}}}}, "Each by a different element: To front or back; at most 1¼ somersaults, at least 1½ twists"},
		{Rule{Type: Different}, "No element repeated"},
		{Rule{Type: Difficulty, Cap: ptr(1.7)}, "Each element's difficulty counts at most 1.7"},
		{Rule{Type: Difficulty, Min: ptr(3.3), Cap: ptr(0.7)}, "Difficulty at least 3.3, each element counting at most 0.7"},
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
	if len(BuiltinGroups()) == 0 || len(Builtins()) == 0 {
		t.Fatal("no built-in sets")
	}
	for _, g := range BuiltinGroups() {
		for _, b := range g.Sets {
			if b.Set.Source == "" {
				t.Errorf("%s: a built-in set must name its source", b.ID)
			}
		}
	}
	if _, ok := LookupBuiltin("builtin:fig-ag1-first"); !ok {
		t.Errorf("a built-in set should be found by reference")
	}
	if _, ok := LookupBuiltin("builtin:nope"); ok {
		t.Errorf("an unknown reference should not be found")
	}
}

// common is a common skill in the given shape.
func common(t *testing.T, key string, shape skills.Shape) skills.TrampolineSkill {
	t.Helper()
	s, ok := skills.CommonSkills[key]
	if !ok {
		t.Fatalf("no common skill %q", key)
	}
	s.Shape = shape
	return s
}

// checkBuiltin evaluates a built-in set against a routine and reports any rule
// that fails, and any transition the routine gets wrong.
func checkBuiltin(t *testing.T, id string, routine []skills.TrampolineSkill) {
	t.Helper()
	set, ok := LookupBuiltin(id)
	if !ok {
		t.Fatalf("no built-in set %s", id)
	}
	rv := skills.ValidateRoutine(routine)
	if rv.HasInvalidTransitions || rv.InterruptedAt >= 0 {
		t.Errorf("%s: the routine itself is wrong: %v", id, rv.Messages)
	}
	for _, r := range Evaluate(set, rv) {
		if !r.Passed {
			t.Errorf("%s: %s: %s", id, r.Description, r.Detail)
		}
	}
}

// The BG set routines, built from the app's own skills, meet their sets.
func TestBuiltinSetRoutines(t *testing.T) {
	c := func(key string) skills.TrampolineSkill { return common(t, key, skills.CommonSkills[key].Shape) }
	jump := func(shape skills.Shape) skills.TrampolineSkill { return common(t, "shapeJump", shape) }
	back := func(shape skills.Shape) skills.TrampolineSkill { return common(t, "backSomersault", shape) }
	front := func(shape skills.Shape) skills.TrampolineSkill { return common(t, "front", shape) }
	barani := func(shape skills.Shape) skills.TrampolineSkill { return common(t, "barani", shape) }
	halfToFront := skills.TrampolineSkill{Rotation: 1, TwistDistribution: []int{1}, TakeoffPosition: skills.Feet, Backward: true}

	routines := map[string][]skills.TrampolineSkill{
		"bg-club-l1": {c("frontDrop"), c("frontToFeet"), jump(skills.Straddle), c("seatDrop"), c("seatToFeet"),
			c("halfTwist"), jump(skills.Tuck), jump(skills.Pike), c("backDrop"), c("backToFeet")},
		"bg-club-l2": {halfToFront, c("frontToFeet"), jump(skills.Straddle), c("seatDrop"), c("seatHalfToSeat"),
			c("seatHalfToFeet"), jump(skills.Tuck), jump(skills.Pike), c("backDrop"), c("backHalfToFeet")},
		"bg-club-l3": {c("fullTwist"), jump(skills.Straddle), c("seatDrop"), c("seatHalfToSeat"), c("seatHalfToFeet"),
			jump(skills.Pike), c("backDrop"), c("backHalfToFeet"), jump(skills.Tuck), front(skills.Tuck)},
		"bg-regional-l1-first": {back(skills.Tuck), jump(skills.Straddle), c("seatDrop"), c("seatHalfToFeet"), c("halfTwist"),
			jump(skills.Pike), c("backDrop"), c("backHalfToFeet"), jump(skills.Tuck), front(skills.Pike)},
		"bg-regional-l2-first": {back(skills.Straight), jump(skills.Straddle), back(skills.Tuck), barani(skills.Tuck), c("halfTwist"),
			jump(skills.Tuck), c("backToSeat"), c("seatHalfToFeet"), jump(skills.Pike), front(skills.Pike)},
		"bg-regional-l3-first": {back(skills.Straight), barani(skills.Straight), jump(skills.Straddle), back(skills.Pike), barani(skills.Pike),
			jump(skills.Tuck), barani(skills.Tuck), back(skills.Tuck), jump(skills.Pike), front(skills.Pike)},
		"bucs-l3-option-1": {c("lazyBack"), c("frontToFeet"), jump(skills.Straddle), back(skills.Pike), barani(skills.Pike),
			jump(skills.Tuck), barani(skills.Tuck), back(skills.Tuck), jump(skills.Pike), c("fullTwist")},
		"bucs-l3-option-2": {back(skills.Straight), barani(skills.Straight), jump(skills.Straddle), back(skills.Tuck), barani(skills.Tuck),
			jump(skills.Pike), c("halfTwist"), jump(skills.Tuck), c("crashDive"), c("backHalfToFeet")},
		"bucs-l4-option-1": {back(skills.Straight), jump(skills.Straddle), barani(skills.Tuck), jump(skills.Tuck), c("halfTwist"),
			jump(skills.Pike), c("backDrop"), c("backHalfToFeet"), jump(skills.Tuck), front(skills.Tuck)},
		"bucs-l4-option-2": {back(skills.Pike), jump(skills.Straddle), back(skills.Tuck), jump(skills.Pike), c("halfTwist"),
			jump(skills.Tuck), halfToFront, c("frontToFeet"), jump(skills.Tuck), barani(skills.Pike)},
		"bucs-l5-option-1": {back(skills.Tuck), jump(skills.Straddle), c("seatDrop"), c("seatHalfToSeat"), c("seatHalfToFeet"),
			jump(skills.Pike), c("backDrop"), c("backHalfToFeet"), jump(skills.Tuck), front(skills.Pike)},
		"bucs-l5-option-2": {back(skills.Pike), jump(skills.Straddle), c("halfToSeat"), c("seatHalfToFeet"), c("halfTwist"),
			jump(skills.Tuck), c("frontDrop"), c("frontToFeet"), jump(skills.Pike), front(skills.Tuck)},
		"bucs-l6-option-1": {c("fullTwist"), jump(skills.Straddle), c("seatDrop"), c("seatHalfToSeat"), c("seatHalfToFeet"),
			jump(skills.Pike), c("backDrop"), c("backHalfToFeet"), jump(skills.Tuck), front(skills.Tuck)},
		"bucs-l6-option-2": {back(skills.Tuck), jump(skills.Straddle), c("seatDrop"), c("seatHalfToSeat"), c("seatHalfToFeet"),
			jump(skills.Tuck), halfToFront, c("frontToFeet"), jump(skills.Pike), c("fullTwist")},
		"bucs-l7-option-1": {c("halfTwist"), jump(skills.Straddle), c("seatDrop"), c("seatToFeet"), c("halfTwist"),
			jump(skills.Pike), c("halfToSeat"), c("seatHalfToFeet"), jump(skills.Tuck), c("fullTwist")},
		"bucs-l7-option-2": {c("fullTwist"), jump(skills.Straddle), c("seatDrop"), c("seatHalfToFeet"), jump(skills.Pike),
			c("seatDrop"), c("seatToFeet"), jump(skills.Tuck), c("frontDrop"), c("frontToFeet")},
	}
	// The disability routines repeat others.
	routines["bucs-disability-l1-option-1"] = routines["bg-regional-l1-first"]
	routines["bucs-disability-l1-option-2"] = routines["bg-regional-l2-first"]
	routines["bucs-disability-l2-option-1"] = routines["bucs-l7-option-1"]
	routines["bucs-disability-l2-option-2"] = routines["bg-club-l3"]
	for id, routine := range routines {
		checkBuiltin(t, id, routine)
	}
	// Every built-in set routine is covered.
	for _, b := range Builtins() {
		if b.Set.Rules[0].Type == Sequence && routines[b.ID] == nil {
			t.Errorf("%s: no test routine", b.ID)
		}
	}

	// The wrong shape somewhere is caught.
	wrong := slices.Clone(routines["bg-regional-l3-first"])
	wrong[3] = back(skills.Tuck)
	set, _ := LookupBuiltin("bg-regional-l3-first")
	if r := Evaluate(set, skills.ValidateRoutine(wrong))[0]; r.Passed || !equalInts(r.Elements, []int{4}) || r.Detail != "differs at 4 (Back somersault (P))" {
		t.Errorf("a back tuck instead of a back pike: got %+v", r)
	}
}

// Routines meeting the FIG special requirements pass them.
func TestBuiltinSpecialRequirements(t *testing.T) {
	c := func(key string) skills.TrampolineSkill { return common(t, key, skills.CommonSkills[key].Shape) }
	back := func(shape skills.Shape) skills.TrampolineSkill { return common(t, "backSomersault", shape) }
	front := func(shape skills.Shape) skills.TrampolineSkill { return common(t, "front", shape) }
	barani := func(shape skills.Shape) skills.TrampolineSkill { return common(t, "barani", shape) }

	// Every element at least ¾ somersault; a back landing (crash dive), a
	// front landing (lazy back) and a full back meet the requirements.
	ag1 := []skills.TrampolineSkill{barani(skills.Tuck), back(skills.Pike), c("crashDive"), c("ballOut"),
		c("lazyBack"), c("cody"), c("fullBack"), front(skills.Pike), back(skills.Straight), barani(skills.Pike)}
	checkBuiltin(t, "fig-ag1-first", ag1)

	// One element under ¾ somersault (the back drop, which is also "to back"),
	// a ball-out "from back", a double back and a Rudi.
	doubleBack := skill(8, []int{0, 0}, skills.Tuck, true)
	ag2 := []skills.TrampolineSkill{barani(skills.Tuck), c("backDrop"), c("ballOut"), doubleBack, c("rudi"),
		back(skills.Pike), front(skills.Pike), back(skills.Straight), barani(skills.Straight), c("fullBack")}
	checkBuiltin(t, "fig-ag2-junior-first", ag2)

	// BUCS Level 1: at least nine somersaults of ¾ or more, including a crash
	// dive (¾ to back) straight into a ball-out (1¼), or a full twisting
	// somersault.
	l1 := []skills.TrampolineSkill{barani(skills.Tuck), back(skills.Pike), c("crashDive"), c("ballOut"),
		back(skills.Tuck), front(skills.Pike), back(skills.Straight), barani(skills.Pike), front(skills.Tuck), barani(skills.Straight)}
	checkBuiltin(t, "bucs-l1-first", l1)
	l1[2], l1[3] = c("fullBack"), front(skills.Straight) // the alternative: a full back
	checkBuiltin(t, "bucs-l1-first", l1)
	l1[2], l1[3] = c("lazyBack"), c("frontToFeet") // neither: ¾ to front, but then only a quarter
	set, _ := LookupBuiltin("bucs-l1-first")
	for _, r := range Evaluate(set, skills.ValidateRoutine(l1)) {
		if set.Rules[r.Rule].Type == Includes && (r.Passed || r.Detail != "not found") {
			t.Errorf("neither option: got %+v", r)
		}
	}

	// Missing the double: reported, and the other requirements still met.
	ag2[3] = back(skills.Tuck)
	set, _ = LookupBuiltin("fig-ag2-junior-first")
	for _, r := range Evaluate(set, skills.ValidateRoutine(ag2)) {
		if set.Rules[r.Rule].Type == Separate && (r.Passed || r.Detail != "missing: A double front or back somersault, with or without twist") {
			t.Errorf("without a double: got %+v", r)
		}
	}
}

func TestSeparateRequirements(t *testing.T) {
	// Twisting elements are 3 (Barani) and 4 (Rudi), but only the Barani
	// matches "4 1 o": the first requirement must give way and take the Rudi.
	results := evaluate(t, Rule{Type: Separate, Each: []Matcher{
		{Twist: &Range{Min: ptr(1)}},
		{FIG: "4 1 o"},
	}})
	if r := results[0]; !r.Passed || !equalInts(r.Assigned, []int{4, 3}) || r.Detail != "by elements 4, 3" {
		t.Errorf("got passed=%v assigned=%v detail=%q", r.Passed, r.Assigned, r.Detail)
	}

	// The seat drop (element 5) is the only element for either requirement,
	// and one element can't meet both.
	results = evaluate(t, Rule{Type: Separate, Each: []Matcher{
		{Takeoff: []string{"feet"}, Rotation: &Range{Max: ptr(0)}},
		{Label: "Landing on the seat", Landing: []string{"seat"}},
	}})
	if r := results[0]; r.Passed || !equalInts(r.Assigned, []int{5, 0}) || r.Detail != "missing: Landing on the seat" {
		t.Errorf("got passed=%v assigned=%v detail=%q", r.Passed, r.Assigned, r.Detail)
	}

	set := Set{Format: Format, Name: "x", Rules: []Rule{{Type: Separate, Each: []Matcher{{FIG: "4 1 o"}, {Landing: []string{"seat"}}}}}}
	if got := RequiredElements(set, Evaluate(set, skills.ValidateRoutine(sampleRoutine))); len(got) != 2 || !got[3] || !got[5] {
		t.Errorf("RequiredElements = %v, want elements 3 and 5", got)
	}
}

func TestDifferentElements(t *testing.T) {
	set := Set{Format: Format, Name: "x", Rules: []Rule{{Type: Different}}}
	if r := Evaluate(set, skills.ValidateRoutine(sampleRoutine))[0]; !r.Passed {
		t.Errorf("no repeats: got %+v", r)
	}
	repeated := append(slices.Clone(sampleRoutine), skill(4, []int{0}, skills.Pike, false), frontTuck)
	if r := Evaluate(set, skills.ValidateRoutine(repeated))[0]; r.Passed || !equalInts(r.Elements, []int{8}) || r.Detail != "8 repeats 1" {
		t.Errorf("a front pike is a different element, a second front tuck isn't: got %+v", r)
	}
}

func TestDifficultyCap(t *testing.T) {
	// Tariffs 0.5, 0.6, 0.6, 0.8, 0.1, 0.1: capped at 0.5, 0.5 + 0.5×3 + 0.2 = 2.2.
	results := evaluate(t,
		Rule{Type: Difficulty, Min: ptr(2.5), Cap: ptr(0.5)},
		Rule{Type: Difficulty, Cap: ptr(1.7)},
	)
	if r := results[0]; r.Passed || r.Detail != "is 2.2 with the cap (2.7 without)" {
		t.Errorf("capped minimum: got passed=%v detail=%q", r.Passed, r.Detail)
	}
	if r := results[1]; !r.Passed || r.Detail != "is 2.7" {
		t.Errorf("cap alone: got passed=%v detail=%q", r.Passed, r.Detail)
	}

	for name, data := range map[string]string{
		"negative cap":           `{"format":1,"name":"x","rules":[{"type":"difficulty","cap":-1}]}`,
		"separate without items": `{"format":1,"name":"x","rules":[{"type":"separate","each":[]}]}`,
	} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestIncludes(t *testing.T) {
	baraniThenFront := []Matcher{{FIG: "4 1 o"}, {FIG: "4 - o"}} // both present, but not in this order
	results := evaluate(t,
		Rule{Type: Includes, Options: [][]Matcher{{{Landing: []string{"seat"}}, {Takeoff: []string{"seat"}}}}},
		Rule{Type: Includes, Options: [][]Matcher{baraniThenFront}},
		Rule{Type: Includes, Options: [][]Matcher{baraniThenFront, {{FIG: "4 3"}}}},
	)
	want := []struct {
		passed   bool
		elements []int
		detail   string
	}{
		{true, []int{5, 6}, "by elements 5, 6"},
		{false, nil, "not found"},
		{true, []int{4}, "by element 4"},
	}
	for i, w := range want {
		if r := results[i]; r.Passed != w.passed || !equalInts(r.Elements, w.elements) || r.Detail != w.detail {
			t.Errorf("rule %d: got passed=%v elements=%v detail=%q", i+1, r.Passed, r.Elements, r.Detail)
		}
	}

	if got := Describe(Rule{Type: Includes, Options: [][]Matcher{
		{{Label: "¾ to front or back"}, {Label: "1¼ somersault"}},
		{{Label: "A full somersault with a full twist"}},
	}}); got != "Includes one of: ¾ to front or back, then 1¼ somersault; or A full somersault with a full twist" {
		t.Errorf("Describe = %q", got)
	}
	for name, data := range map[string]string{
		"no options":   `{"format":1,"name":"x","rules":[{"type":"includes","options":[]}]}`,
		"empty option": `{"format":1,"name":"x","rules":[{"type":"includes","options":[[]]}]}`,
	} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// BUCS FIG Level allows two body landings; seat landings don't count.
func TestBUCSBodyLandings(t *testing.T) {
	c := func(key string) skills.TrampolineSkill { return common(t, key, skills.CommonSkills[key].Shape) }
	set, _ := LookupBuiltin("bucs-fig")
	limit := func(routine []skills.TrampolineSkill) Result {
		t.Helper()
		for _, r := range Evaluate(set, skills.ValidateRoutine(routine)) {
			if strings.Contains(r.Description, "front or back landings") {
				return r
			}
		}
		t.Fatal("no body landing rule")
		return Result{}
	}
	twoAndSeats := []skills.TrampolineSkill{c("crashDive"), c("ballOut"), c("lazyBack"), c("cody"),
		c("seatDrop"), c("seatToFeet"), c("seatDrop"), c("seatToFeet")}
	if r := limit(twoAndSeats); !r.Passed {
		t.Errorf("two body landings and two seat landings: got %+v", r)
	}
	three := append(twoAndSeats, c("backDrop"), c("backToFeet"))
	if r := limit(three); r.Passed || !equalInts(r.Elements, []int{1, 3, 9}) {
		t.Errorf("three body landings: got %+v", r)
	}
}
