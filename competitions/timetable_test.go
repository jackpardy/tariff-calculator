package competitions

import (
	"fmt"
	"math/rand/v2"
	"testing"
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

func TestFlights(t *testing.T) {
	var all []PlanEntry
	all = append(all, entries("L1", "", 25, "A", "B")...) // three flights of 8 or 9
	all = append(all, entries("L2", "Men", 5, "A")...)
	all = append(all, entries("L2", "Women", 7, "B")...)
	count := func(separate Split) map[string][]int {
		out := map[string][]int{}
		for _, g := range groups(all, []string{"L1", "L2"}, separate) {
			for _, f := range flightsFor(g, 10, rng()) {
				out[f.Name()] = append(out[f.Name()], len(f.Entries))
			}
		}
		return out
	}
	got := count(Split{Mode: SplitAll})
	if len(got["L1 · flight 1 of 3"]) != 1 || got["L1 · flight 1 of 3"][0] < 8 || got["L2 Men"][0] != 5 || got["L2 Women"][0] != 7 {
		t.Errorf("L1 in three even flights, L2's men and women apart: %v", got)
	}
	// L2 ranked separately but flying together: 12 in two flights of 6.
	if got := count(Split{}); len(got["L2 · flight 1 of 2"]) != 1 || got["L2 · flight 1 of 2"][0] != 6 {
		t.Errorf("L2 mixed: %v", got)
	}
}
