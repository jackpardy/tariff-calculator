package competitions

import (
	"slices"
	"strings"
	"testing"

	"tariffCalculator/requirements"
)

func TestSynchroEvents(t *testing.T) {
	ref := func(id string) Level { return Level{Ref: requirements.BuiltinLevelPrefix + id} }
	c := competition(t)
	c.Levels = OrderLevels(nil, []Level{ref("bucs-l1"), ref("bucs-l2"), ref("bucs-l3"), ref("bucs-l4")})
	paired, err := PairLevels([]Level{ref("bucs-l1"), ref("bucs-l2"), ref("bucs-l3")}, [][]string{{"BUCS L1", "BUCS L2"}})
	if err != nil {
		t.Fatal(err)
	}
	c.Synchro = paired
	if got := c.LevelNames(Synchro); !slices.Equal(got, []string{"BUCS L1/L2", "BUCS L3"}) {
		t.Errorf("synchro events: %v", got)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := c.Pairings(); len(got) != 1 || !slices.Equal(got[0], []string{"BUCS L1", "BUCS L2"}) {
		t.Errorf("pairings: %v", got)
	}
	if ev, pairedEv, ok := c.SynchroEventOf("BUCS L2"); ev != "BUCS L1/L2" || !pairedEv || !ok {
		t.Errorf("BUCS L2 is in BUCS L1/L2: %q %v %v", ev, pairedEv, ok)
	}
	if again, err := PairLevels(c.Synchro, [][]string{{"BUCS L2", "BUCS L3"}}); err != nil || len(again) != 2 || len(again[1].With) != 1 {
		t.Errorf("pairing again starts from the levels: %+v, %v", again, err)
	}
	for _, bad := range [][][]string{{{"BUCS L1", "BUCS L7"}}, {{"BUCS L1", "BUCS L2"}, {"BUCS L2", "BUCS L3"}}} {
		if _, err := PairLevels(paired, bad); err == nil {
			t.Errorf("%v should be refused", bad)
		}
	}
	if joinLevelNames([]string{"FIG AG1 (11–12)", "BUCS L7"}) != "FIG AG1 (11–12) / BUCS L7" {
		t.Error("levels that don't share a first part are joined whole")
	}

	// A pair in a paired event says which level they do, and is checked
	// against it.
	e := Entry{Gymnast: "A", Discipline: Synchro, Level: "BUCS L1/L2", Partner: &Partner{Name: "B"},
		Exercises: [2]Exercise{{Option: "builtin:bucs-l2-first"}, {Option: "builtin:bucs-l2-second"}}}
	if err := c.ValidateEntry(&e); err == nil || !strings.Contains(err.Error(), "choose which") {
		t.Errorf("a paired event needs a choice: %v", err)
	}
	e.Choice = "BUCS L2"
	if err := c.ValidateEntry(&e); err != nil {
		t.Fatal(err)
	}
	if _, level, ok := c.EntryLevel(e); !ok || level.Name != "BUCS L2" || e.Event() != "Synchro BUCS L1/L2" || e.DoesLevel() != "BUCS L2" {
		t.Errorf("checked against BUCS L2 in Synchro BUCS L1/L2: %v %q", ok, level.Name)
	}
	if card, err := c.Check(e); err != nil || card.Level.Name != "BUCS L2" || card.Only != 2 || card.Does(0) || !card.Does(1) {
		t.Errorf("the card is BUCS L2's voluntary only: %+v, %v", card, err)
	}
	if e.Exercises[0].Option != "" {
		t.Error("synchro does one routine: the first exercise is dropped")
	}
	// The one routine's problems are the routine's, not an exercise's.
	e.Exercises[1].Skills = nil
	if card, _ := c.Check(e); len(card.Problems()) == 0 || !strings.HasPrefix(card.Problems()[0], "Routine: ") {
		t.Errorf("problems: %v", card.Problems())
	}
	if (Entry{}).Only(requirements.Level{Second: &requirements.Exercise{}}) != 0 || SynchroExercise(requirements.Level{}) != 1 {
		t.Error("only synchro does one exercise; a level with one exercise has it")
	}
	single := Entry{Gymnast: "A", Discipline: Synchro, Level: "BUCS L3", Choice: "BUCS L1", Partner: &Partner{Name: "B"},
		Exercises: [2]Exercise{{Option: "builtin:bucs-l3-option-1"}, {Option: "builtin:bucs-l3-second"}}}
	if err := c.ValidateEntry(&single); err != nil || single.Choice != "" {
		t.Errorf("an event of one level has no choice: %q, %v", single.Choice, err)
	}

	// Pairs do the level they both compete at, or the easier of two a level
	// apart.
	for _, tc := range []struct {
		a, b, want string
		ok         bool
	}{
		{"BUCS L3", "BUCS L3", "BUCS L3", true},
		{"BUCS L3", "BUCS L4", "BUCS L4", true},
		{"BUCS L2", "BUCS L1", "BUCS L2", true},
		{"BUCS L2", "BUCS L4", "", false},
		{"BUCS L3", "BUCS L9", "", false},
	} {
		if got, ok := c.SynchroLevel(tc.a, tc.b); got != tc.want || ok != tc.ok {
			t.Errorf("%s with %s: %q %v, want %q %v", tc.a, tc.b, got, ok, tc.want, tc.ok)
		}
	}

	// Only synchro levels pair, and a level is in one event.
	bad := c
	bad.Levels = []Level{{Ref: "builtin-level:bucs-l3", With: []Level{ref("bucs-l4")}}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "only synchro") {
		t.Errorf("trampoline levels don't pair: %v", err)
	}
	bad = c
	bad.Synchro = []Level{{Ref: "builtin-level:bucs-l3", With: []Level{ref("bucs-l3")}}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("a level in synchro twice: %v", err)
	}
}
