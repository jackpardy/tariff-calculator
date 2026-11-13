package competitions

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// Two blocks with the same name on one day would be taken for one when the
// placed blocks are matched to the setup's, the second dropped without a
// word: the setup refuses them, saying why. On different days, or with
// different names, they're fine.
func TestSameNameBlocksOnOneDay(t *testing.T) {
	setup := Setup{Areas: []Area{{Name: "Panel 1", Discipline: Trampoline}},
		Days:   []Day{{Name: "Sat", Start: "09:00", End: "18:00"}, {Name: "Sun", Start: "09:00", End: "18:00"}},
		Blocks: []Block{{Name: "Break", Minutes: 30, Day: 0, At: "11:00"}, {Name: "Break", Minutes: 30, Day: 0, At: "15:00"}}}
	err := setup.Check([]string{"L1"})
	if err == nil || !strings.Contains(err.Error(), `two blocks on the same day are called "Break"`) {
		t.Errorf("the same name twice on a day is refused: %v", err)
	}
	setup.Blocks[1].Day = 1
	if err := setup.Check([]string{"L1"}); err != nil {
		t.Errorf("the same name on another day: %v", err)
	}
	setup.Blocks[1].Day, setup.Blocks[1].Name = 0, "Lunch"
	if err := setup.Check([]string{"L1"}); err != nil {
		t.Errorf("another name on the same day: %v", err)
	}
}

// An event whose run is longer than any day can't be placed by another
// panel or quicker turns: the report says so, and Fixes offers men and
// women flying separately and a longer day, each tried.
func TestEventLongerThanADay(t *testing.T) {
	setup := Setup{
		Areas: []Area{{Name: "Panel 1", Discipline: Trampoline}, {Name: "Panel 2", Discipline: Trampoline}, {Name: "Panel 3", Discipline: Trampoline}},
		Days:  []Day{{Name: "Fri", Start: "09:00", End: "18:00"}, {Name: "Sat", Start: "09:00", End: "18:00"}, {Name: "Sun", Start: "09:00", End: "18:00"}}}
	var entries []SchedEntry
	for i := range 110 { // 10 flights of 11, 65 minutes each: 650 minutes, more than the 540 of any day
		category := "Men"
		if i%2 == 1 {
			category = "Women"
		}
		entries = append(entries, SchedEntry{PlanEntry: PlanEntry{ID: fmt.Sprintf("e%d", i), Level: "Novice", Category: category}, People: []string{fmt.Sprintf("p%d", i)}})
	}
	events := []string{"Novice"}
	s := PlanSchedule(entries, events, setup, 1)
	if len(s.Unplaced) != 10 {
		t.Fatalf("all ten flights are left: %d", len(s.Unplaced))
	}
	report := s.Report(nil)
	if len(report.TooLong) != 1 || report.TooLong[0] != "Novice takes 10h 50m in one run: the longest day is 9h 00m" {
		t.Errorf("the report says the run is longer than any day: %v", report.TooLong)
	}

	fixes := map[string]Fix{}
	for _, f := range Fixes(entries, events, setup, 1) {
		fixes[f.Change] = f
	}
	split, ok := fixes["men and women in separate flights for Novice"]
	if !ok || !split.Fits || !split.Setup.Separate.Splits("Novice") {
		t.Errorf("men and women apart is offered, and fits: %+v", split)
	}
	longer, ok := fixes["a longer day: Fri to 20:00, not 18:00"]
	if !ok || !longer.Fits || longer.Setup.Days[0].End != "20:00" || setup.Days[0].End != "18:00" {
		t.Errorf("a longer day is offered, and fits, without changing the setup: %+v", longer)
	}
	if !slices.ContainsFunc(slices.Collect(keysOf(fixes)), func(c string) bool { return strings.HasPrefix(c, "Novice capped at") }) {
		t.Error("the cap is still offered")
	}

	// Already apart: only a longer day, and a run that fits has no such fix.
	setup.Separate = Split{Mode: SplitAll}
	for _, f := range Fixes(entries, events, setup, 1) {
		if strings.HasPrefix(f.Change, "men and women") || strings.HasPrefix(f.Change, "a longer day") {
			t.Errorf("nothing is too long when they fly apart: %s", f.Change)
		}
	}
}

func keysOf[V any](m map[string]V) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}
