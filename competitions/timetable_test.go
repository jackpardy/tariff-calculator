package competitions

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"
)

func rng() *rand.Rand { return rand.New(rand.NewPCG(7, 11)) }

// entries makes n entries for a level and category, cycling through clubs.
func entries(level, category string, n int, clubs ...string) []PlanEntry {
	var out []PlanEntry
	for i := range n {
		club := ""
		if len(clubs) > 0 {
			club = clubs[i%len(clubs)]
		}
		out = append(out, PlanEntry{ID: fmt.Sprintf("%s-%s-%d", level, category, i), Level: level, Category: category, Club: club})
	}
	return out
}

func TestDrawSpreadsClubs(t *testing.T) {
	// Six from UCD, three from DCU, three individuals: never UCD twice running.
	in := append(entries("L", "", 6, "UCD"), entries("M", "", 3, "DCU")...)
	in = append(in, entries("N", "", 3)...)
	for seed := range uint64(50) {
		order := Draw(in, rand.New(rand.NewPCG(seed, 1)))
		if len(order) != len(in) {
			t.Fatalf("drawn %d of %d", len(order), len(in))
		}
		for i := 1; i < len(order); i++ {
			if order[i].Club != "" && order[i].Club == order[i-1].Club {
				t.Fatalf("seed %d: %s twice in a row: %+v", seed, order[i].Club, order)
			}
		}
	}
	// Only one club: it has to repeat.
	if got := Draw(entries("L", "", 3, "UCD"), rng()); len(got) != 3 {
		t.Error("everyone is still drawn")
	}
}

func TestPlan(t *testing.T) {
	levels := []string{"L1", "L2", "L3"}
	var all []PlanEntry
	all = append(all, entries("L1", "", 25, "A", "B")...) // 25: three flights of 8 or 9
	all = append(all, entries("L2", "Men", 5, "A")...)
	all = append(all, entries("L2", "Women", 7, "B")...)
	all = append(all, entries("L3", "", 4, "A")...)
	s := DefaultSettings
	s.MaxFlight = 10
	plan := Plan(all, levels, s, rng())

	var flights []Flight
	placed := 0
	for _, panel := range plan.Panels {
		flights = append(flights, panel...)
		for _, f := range panel {
			placed += len(f.Entries)
		}
	}
	if placed != len(all) {
		t.Errorf("everyone is in a flight: %d of %d", placed, len(all))
	}
	if len(flights) != 6 {
		t.Errorf("L1 in 3 flights, L2 men, L2 women, L3: %d flights", len(flights))
	}
	for _, f := range flights {
		if f.Level == "L1" && (len(f.Entries) < 8 || len(f.Entries) > 9 || f.Of != 3) {
			t.Errorf("L1's flights are as even as can be: %+v", f)
		}
	}
	if name := (Flight{Level: "L1", Number: 2, Of: 3}).Name(); name != "L1 · flight 2 of 3" {
		t.Errorf("named %q", name)
	}
	if name := (Flight{Level: "L2", Category: "Women", Number: 1, Of: 1}).Name(); name != "L2 Women" {
		t.Errorf("named %q", name)
	}

	// A category's flights stay on one panel, in level order.
	panelOf := map[string]int{}
	for p, panel := range plan.Panels {
		for _, f := range panel {
			key := f.Level + f.Category
			if q, ok := panelOf[key]; ok && q != p {
				t.Errorf("%s split across panels", key)
			}
			panelOf[key] = p
		}
		for i := 1; i < len(panel); i++ {
			if panel[i].Level < panel[i-1].Level {
				t.Errorf("panel %d runs %s after %s", p+1, panel[i].Level, panel[i-1].Level)
			}
		}
	}

	// Balanced: L1 (25 gymnasts) on one panel, the rest (16) on the other.
	day := time.Date(2027, 3, 13, 0, 0, 0, 0, time.UTC)
	slots := plan.Slots(day)
	if got := slots[0][0].Start.Format("15:04"); got != "09:00" {
		t.Errorf("starts at %s", got)
	}
	// L1: 3 flights × 10 minutes + 25 × 5 = 155 minutes → 11:35.
	if got := plan.Finish(day).Format("15:04"); got != "11:35" {
		t.Errorf("finishes at %s", got)
	}

	// Mixed flights for L2, though it's ranked separately.
	s.Separate = Split{Mode: SplitSome, Levels: []string{"L9"}}
	mixed := Plan(all, levels, s, rng())
	count, gymnasts := 0, 0
	for _, panel := range mixed.Panels {
		for _, f := range panel {
			if f.Level == "L2" {
				count++
				gymnasts += len(f.Entries)
				if f.Category != "" || len(f.Entries) != 6 {
					t.Errorf("L2's men and women fly together, 6 a flight: %+v", f)
				}
			}
		}
	}
	if count != 2 || gymnasts != 12 {
		t.Errorf("12 in L2 with flights of 10: two flights, got %d with %d", count, gymnasts)
	}
}

func TestPanelsNeeded(t *testing.T) {
	levels := []string{"L1", "L2", "L3", "L4"}
	var all []PlanEntry
	for _, l := range levels {
		all = append(all, entries(l, "", 20, "A", "B")...)
	}
	s := DefaultSettings
	s.MaxFlight = 10
	// Each level: 2 flights × 10 + 20 × 5 = 120 minutes. By 13:00 (4 hours) needs 2 panels.
	s.FinishBy = "13:00"
	if got := PanelsNeeded(all, levels, s); got != 2 {
		t.Errorf("panels to finish by 13:00: %d", got)
	}
	s.FinishBy = "10:00"
	if got := PanelsNeeded(all, levels, s); got != 0 {
		t.Errorf("one level alone takes two hours: can't finish by 10:00, got %d", got)
	}
}

func TestTimetableEdits(t *testing.T) {
	all := append(entries("L1", "", 4, "A", "B"), entries("L2", "", 3, "A")...)
	s := DefaultSettings
	plan := Plan(all, []string{"L1", "L2"}, s, rng())
	at, ok := plan.Find("L1--0")
	if !ok {
		t.Fatal("placed")
	}
	other := 1 - at.Panel
	if !plan.MoveEntry("L1--0", other, 0) {
		t.Fatal("moved")
	}
	if got, _ := plan.Find("L1--0"); got.Panel != other {
		t.Errorf("moved to the other panel's flight: %+v", got)
	}
	plan.RemoveEntry("L1--0")
	if _, ok := plan.Find("L1--0"); ok {
		t.Error("removed")
	}
	if !plan.MoveFlight(other, 0, at.Panel) || len(plan.Panels[other]) != 0 || len(plan.Panels[at.Panel]) != 2 {
		t.Errorf("both flights on one panel: %+v", plan.Panels)
	}
	if !plan.ShiftFlight(at.Panel, 1, -1) || plan.Panels[at.Panel][0].Level != "L2" {
		t.Error("L2 moved first")
	}
	if plan.ShiftFlight(at.Panel, 0, -1) || plan.MoveFlight(5, 0, 0) {
		t.Error("moves off the end are refused")
	}
	before := len(plan.Panels[at.Panel][1].Entries)
	if !plan.Redraw(at.Panel, 1, map[string]string{}, rng()) || len(plan.Panels[at.Panel][1].Entries) != before {
		t.Error("redrawn, same gymnasts")
	}
}

func TestPlanSettingsCheck(t *testing.T) {
	if err := DefaultSettings.Check(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*PlanSettings){
		func(s *PlanSettings) { s.Panels = 0 },
		func(s *PlanSettings) { s.MinutesPerGymnast = 0 },
		func(s *PlanSettings) { s.MaxFlight = 1 },
		func(s *PlanSettings) { s.Start = "9am" },
		func(s *PlanSettings) { s.FinishBy = "late" },
		func(s *PlanSettings) { s.Separate.Mode = "odd" },
	} {
		s := DefaultSettings
		change(&s)
		if s.Check() == nil {
			t.Errorf("%+v should be refused", s)
		}
	}
}
