package competitions

import (
	"slices"
	"testing"
)

// What a draft changes from the published timetable: flights moved, added
// and taken out, running orders redrawn, gymnasts moved, a seat given to
// someone else (once for its event, not each flight), and breaks moved.
func TestChangesFrom(t *testing.T) {
	published := Schedule{
		Setup: Setup{Days: []Day{{Name: "Saturday"}}},
		Flights: []ScheduledFlight{
			{ID: "f1", Flight: Flight{Event: "BUCS L3", Number: 1, Of: 2, Entries: []string{"a", "b"}}, Area: "Panel 1", Start: 9 * 60,
				Officials: []Duty{{Role: RoleChair, Person: "mary"}, {Role: RoleExecution, Person: "tom"}}},
			{ID: "f2", Flight: Flight{Event: "BUCS L3", Number: 2, Of: 2, Entries: []string{"c", "d"}}, Area: "Panel 1", Start: 10 * 60,
				Officials: []Duty{{Role: RoleChair, Person: "mary"}, {Role: RoleExecution, Person: "tom"}}},
			{ID: "f3", Flight: Flight{Event: "Tumbling Novice", Number: 1, Of: 1, Entries: []string{"e"}}, Area: "Track 1", Start: 9 * 60},
			{ID: "f5", Flight: Flight{Event: "BUCS L5", Number: 1, Of: 1}, Area: "Panel 3", Start: 9 * 60, End: 10 * 60},
		},
		Blocks: []ScheduledBlock{{Name: "Lunch", Areas: []string{"Panel 1"}, Start: 12 * 60, End: 13 * 60}, {Name: "Lunch", Areas: []string{"Track 1"}, Start: 12 * 60, End: 13 * 60}},
	}
	draft := Schedule{
		Setup: published.Setup,
		Flights: []ScheduledFlight{
			// Moved to Panel 2 at 09:30, its order redrawn.
			{ID: "f1", Flight: Flight{Event: "BUCS L3", Number: 1, Of: 2, Entries: []string{"b", "a"}}, Area: "Panel 2", Start: 9*60 + 30,
				Officials: []Duty{{Role: RoleChair, Person: "mary"}, {Role: RoleExecution, Person: "ann"}}},
			// Where it was; d moved out to a new flight, c the same.
			{ID: "f2", Flight: Flight{Event: "BUCS L3", Number: 2, Of: 2, Entries: []string{"c"}}, Area: "Panel 1", Start: 10 * 60,
				Officials: []Duty{{Role: RoleChair, Person: "mary"}, {Role: RoleExecution, Person: "ann"}}},
			{ID: "f4", Flight: Flight{Event: "DMT Novice", Number: 1, Of: 1, Entries: []string{"d"}}, Area: "DMT 1", Start: 11 * 60},
			// Running late: it finishes later.
			{ID: "f5", Flight: Flight{Event: "BUCS L5", Number: 1, Of: 1}, Area: "Panel 3", Start: 9 * 60, End: 10*60 + 20},
		},
		// Panel 1's lunch later; Track 1's as it was.
		Blocks: []ScheduledBlock{{Name: "Lunch", Areas: []string{"Panel 1"}, Start: 12*60 + 30, End: 13*60 + 30}, {Name: "Lunch", Areas: []string{"Track 1"}, Start: 12 * 60, End: 13 * 60}},
	}
	names := map[string]string{"a": "Ann A", "b": "Ben B", "c": "Cara C", "d": "Dan D", "e": "Eve E", "mary": "Mary", "tom": "Tom", "ann": "Ann"}
	got := draft.ChangesFrom(published, func(k string) string { return names[k] }, func(k string) string { return names[k] })
	want := []Change{
		{ChangeFlights, "BUCS L3 · flight 1 of 2: now Saturday 09:30, Panel 2 (was Saturday 09:00, Panel 1)"},
		{ChangeFlights, "BUCS L3 · flight 1 of 2: running order redrawn"},
		{ChangeFlights, "DMT Novice: added, Saturday 11:00, DMT 1"},
		{ChangeFlights, "BUCS L5: now finishes about 10:20 (was 10:00)"},
		{ChangeFlights, "Tumbling Novice: taken out"},
		{ChangeGymnasts, "Dan D: now in DMT Novice (was BUCS L3 · flight 2 of 2)"},
		{ChangeGymnasts, "Eve E: taken out of Tumbling Novice"},
		{ChangePanels, "BUCS L3 · Execution judge: Ann (was Tom)"},
		{ChangeBreaks, "Lunch: now Saturday 12:30–13:30, Panel 1 (was Saturday 12:00–13:00, Panel 1)"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("changes:\n%v\nwant\n%v", got, want)
	}
	if got := published.ChangesFrom(published, func(string) string { return "" }, func(string) string { return "" }); len(got) != 0 {
		t.Errorf("none from itself: %v", got)
	}
	// A gymnast withdrawn since isn't named.
	if got := draft.ChangesFrom(published, func(k string) string {
		if k == "d" || k == "e" {
			return ""
		}
		return names[k]
	}, func(k string) string { return names[k] }); slices.ContainsFunc(got, func(c Change) bool { return c.Kind == ChangeGymnasts }) {
		t.Errorf("withdrawn gymnasts left out: %v", got)
	}
}
