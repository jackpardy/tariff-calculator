package competitions

import (
	"strings"
	"testing"
	"time"

	"tariffCalculator/requirements"
	"tariffCalculator/skills"
)

var (
	backTuck = skills.TrampolineSkill{Rotation: 4, TwistDistribution: []int{0}, TakeoffPosition: skills.Feet, Shape: skills.Tuck, Backward: true}
	barani   = skills.TrampolineSkill{Rotation: 4, TwistDistribution: []int{1}, TakeoffPosition: skills.Feet, Shape: skills.Tuck}
	fullBack = skills.TrampolineSkill{Rotation: 4, TwistDistribution: []int{2}, TakeoffPosition: skills.Feet, Shape: skills.Straight, Backward: true}
	rudi     = skills.TrampolineSkill{Rotation: 4, TwistDistribution: []int{3}, TakeoffPosition: skills.Feet, Shape: skills.Straight}
)

// competition offers BUCS L3 (a set routine, option 1 or 2, then a voluntary),
// FIG AG3 (two elements carry over) and a coach's own level.
func competition(t *testing.T) Competition {
	t.Helper()
	mine := requirements.Set{Format: requirements.Format, Name: "Club voluntary", Rules: []requirements.Rule{}}
	c := Competition{
		Name:     "Student Open",
		Date:     "2027-03-13",
		Deadline: time.Date(2027, 3, 6, 23, 59, 0, 0, time.UTC),
		LiveAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		Levels: []Level{
			{Ref: "builtin-level:bucs-l3"},
			{Ref: "builtin-level:fig-ag3"},
			{
				Custom: &requirements.Level{Format: requirements.Format, Name: "Club novice", First: requirements.Exercise{Options: []string{"set-mine"}}},
				Sets:   map[string]requirements.Set{"set-mine": mine},
			},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("invalid test competition: %v", err)
	}
	return c
}

func TestValidate(t *testing.T) {
	c := competition(t)
	if got := c.DeleteAfter(); !got.Equal(time.Date(2027, 7, 11, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("deleted 120 days after the competition date, got %v", got)
	}
	if !c.Open(c.Deadline.Add(-time.Minute)) || c.Open(c.Deadline) {
		t.Error("open until the deadline")
	}
	if c.Open(c.LiveAt.Add(-time.Minute)) || !c.Open(c.LiveAt) {
		t.Error("open from going live")
	}
	private := c
	private.LiveAt = time.Time{}
	if private.Open(c.Deadline.Add(-time.Minute)) || private.Live(c.Deadline) {
		t.Error("a private competition takes no entries")
	}
	late := c
	late.LiveAt = c.Deadline
	if late.Validate() == nil {
		t.Error("entries must open before they close")
	}

	for _, tc := range []struct {
		change func(*Competition)
		want   string
	}{
		{func(c *Competition) { c.Name = " " }, "needs a name"},
		{func(c *Competition) { c.Name = strings.Repeat("x", MaxName+1) }, "the most is 80"},
		{func(c *Competition) { c.Date = "13/03/2027" }, "written as 2006-01-02"},
		{func(c *Competition) { c.Deadline = time.Time{} }, "need a deadline"},
		{func(c *Competition) { c.Deadline = time.Date(2027, 3, 14, 9, 0, 0, 0, time.UTC) }, "close by the end of the competition date"},
		{func(c *Competition) { c.Levels = nil }, "at least one level"},
		{func(c *Competition) { c.Levels = append(c.Levels, Level{Ref: "builtin-level:bucs-l3"}) }, `two levels are called "BUCS L3"`},
		{func(c *Competition) { c.Levels = append(c.Levels, Level{Ref: "builtin-level:nope"}) }, "no longer exists"},
		{func(c *Competition) { c.Levels = append(c.Levels, Level{}) }, "needs a built-in reference or a copy"},
		{func(c *Competition) { delete(c.Levels[2].Sets, "set-mine") }, `"set-mine" aren't part of the competition`},
	} {
		c := competition(t)
		c.Levels = append([]Level(nil), c.Levels...)
		c.Levels[2].Sets = map[string]requirements.Set{"set-mine": c.Levels[2].Sets["set-mine"]}
		tc.change(&c)
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("got %v, want %q", err, tc.want)
		}
	}
}

func TestValidateEntry(t *testing.T) {
	c := competition(t)
	e := Entry{Gymnast: " A. Murphy ", Level: "BUCS L3", Exercises: [2]Exercise{
		{Option: "builtin:bucs-l3-option-2"},
		{Skills: []skills.TrampolineSkill{backTuck, barani}},
	}}
	if err := c.ValidateEntry(&e); err != nil {
		t.Fatal(err)
	}
	if e.Gymnast != "A. Murphy" || e.Exercises[1].Option != "builtin:bucs-l3-second" {
		t.Errorf("the name is trimmed and the only option filled in: %+v", e)
	}

	for _, tc := range []struct {
		entry Entry
		want  string
	}{
		{Entry{Level: "BUCS L3"}, "the gymnast needs a name"},
		{Entry{Gymnast: "B", Level: "BUCS L9"}, `doesn't offer the level "BUCS L9"`},
		{Entry{Gymnast: "B", Level: "BUCS L3"}, "first exercise needs one of its options chosen"},
		{Entry{Gymnast: "B", Level: "BUCS L3", Exercises: [2]Exercise{{Option: "builtin:fig-ag3-first"}}}, "isn't one of the level's"},
		{Entry{Gymnast: "B", Level: "FIG AG3 (17–21)", Exercises: [2]Exercise{{Skills: make([]skills.TrampolineSkill, MaxSkills+1)}}}, "the most is 20"},
		{Entry{Gymnast: "B", Level: "FIG AG3 (17–21)", Exercises: [2]Exercise{{Skills: []skills.TrampolineSkill{{Rotation: -1}}}}}, "skill 1"},
	} {
		if err := c.ValidateEntry(&tc.entry); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v: got %v, want %q", tc.entry, err, tc.want)
		}
	}
}

func TestCheck(t *testing.T) {
	c := competition(t)

	// A set routine left empty is performed as prescribed.
	l3 := Entry{Gymnast: "A", Level: "BUCS L3", Exercises: [2]Exercise{{Option: "builtin:bucs-l3-option-1"}, {}}}
	if err := c.ValidateEntry(&l3); err != nil {
		t.Fatal(err)
	}
	card, err := c.Check(l3)
	if err != nil {
		t.Fatal(err)
	}
	if card.Level.Name != "BUCS L3" || len(card.First.Validation.Skills) != 10 || card.First.Met() != len(card.First.Results) {
		t.Errorf("the set routine as prescribed meets its requirements: %+v", card.First)
	}
	if problems := card.Problems(); len(problems) == 0 || !strings.HasPrefix(problems[0], "Second exercise: ") {
		t.Errorf("an empty voluntary is a problem with the second exercise: %v", problems)
	}

	// AG3: the full back and Rudi score in the first exercise, so the Rudi
	// can't be repeated in the second, exactly as in the builder.
	ag3 := Entry{Gymnast: "B", Level: "FIG AG3 (17–21)", Exercises: [2]Exercise{
		{Skills: []skills.TrampolineSkill{backTuck, barani, fullBack, rudi}},
		{Skills: []skills.TrampolineSkill{rudi, backTuck}},
	}}
	if err := c.ValidateEntry(&ag3); err != nil {
		t.Fatal(err)
	}
	card, err = c.Check(ag3)
	if err != nil {
		t.Fatal(err)
	}
	if len(card.Repeated) != 1 || card.Second.Validation.TotalTariff != 0.5 {
		t.Errorf("the repeated Rudi scores nothing: %+v, total %.2f", card.Repeated, card.Second.Validation.TotalTariff)
	}
	found := false
	for _, p := range card.Problems() {
		found = found || p == "Second exercise: element 1: Can't Repeat: Scored In 1st Exercise"
	}
	if !found {
		t.Errorf("the repeat is a problem: %v", card.Problems())
	}

	// A coach's own level checks against its copied requirements.
	own := Entry{Gymnast: "C", Level: "Club novice", Exercises: [2]Exercise{{Skills: []skills.TrampolineSkill{backTuck}}, {}}}
	if err := c.ValidateEntry(&own); err != nil {
		t.Fatal(err)
	}
	if card, err := c.Check(own); err != nil || card.First.SetName != "Club voluntary" || card.Second.SetName != "Club voluntary" {
		t.Errorf("both exercises use the level's own requirements: %+v, %v", card, err)
	}

	if _, err := c.Check(Entry{Level: "Gone"}); err == nil {
		t.Error("a level the competition doesn't offer can't be checked")
	}
}
