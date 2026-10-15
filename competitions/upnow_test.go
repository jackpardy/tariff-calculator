package competitions

import (
	"fmt"
	"testing"
)

// A flight's routines go everyone's 1st in running order, then everyone's
// 2nd, skipping those who don't compete; where the marshal is goes with the
// gymnast when someone joins or leaves the flight.
func TestUpOrder(t *testing.T) {
	f := ScheduledFlight{Flight: Flight{Entries: []string{"a", "b", "c"}}}
	o := UpOrderOf(f, func(id string) bool { return id != "b" })
	r := func(entry string, round int) Routine { return Routine{entry, round} }
	show := func(rs ...Routine) string { return fmt.Sprint(rs) }

	if first, ok := o.First(); !ok || first != r("a", 1) {
		t.Errorf("first: %v", first)
	}
	if got := show(o.After(Routine{}, 10)...); got != show(r("a", 1), r("c", 1), r("a", 2), r("c", 2)) {
		t.Errorf("in order, skipping b: %s", got)
	}
	if next, ok := o.Next(r("c", 2)); ok {
		t.Errorf("nothing after the last: %v", next)
	}
	if back, ok := o.Back(r("a", 2)); !ok || back != r("c", 1) {
		t.Errorf("back across the rounds: %v", back)
	}
	if _, ok := o.Back(r("a", 1)); ok {
		t.Error("nothing before the first")
	}
	if !o.Has(r("b", 1)) || o.Open(r("b", 1)) || o.Has(r("x", 1)) || o.Has(r("a", 3)) {
		t.Error("b is in the flight but doesn't compete; x isn't in it, and there's no 3rd round")
	}
	if o.No(r("c", 2)) != 3 {
		t.Errorf("c's running number: %d", o.No(r("c", 2)))
	}

	// A gymnast joins the flight while it runs, at the end of its order:
	// the marshal on c's 1st is still on c's 1st, and the newcomer is next.
	f.Entries = append(f.Entries, "d")
	o = UpOrderOf(f, func(id string) bool { return id != "b" })
	if next, _ := o.Next(r("c", 1)); next != r("d", 1) {
		t.Errorf("the newcomer is next: %v", next)
	}
	// Moved out while on: Next goes to the first in their round.
	if next, _ := o.Next(r("gone", 2)); next != r("a", 2) {
		t.Errorf("from someone moved out: %v", next)
	}
	if _, ok := o.Back(r("gone", 2)); ok {
		t.Error("and there's no going back from them")
	}

	// Everyone scratched: nowhere to start.
	if _, ok := UpOrderOf(f, func(string) bool { return false }).First(); ok {
		t.Error("nobody to start with")
	}
}

// A level's exercises are named where they're a set routine and a voluntary;
// otherwise they're numbered.
func TestRoutineNames(t *testing.T) {
	c := Competition{
		Levels:   []Level{{Ref: "builtin-level:isto-novice"}, {Ref: "builtin-level:bucs-l3"}, {Ref: "builtin-level:fig-ag3"}},
		Tumbling: []string{"Novice"},
	}
	for _, tc := range []struct {
		entry Entry
		want  [2]string
	}{
		{Entry{Level: "ISTO Novice"}, [2]string{"Set", "Voluntary"}},
		{Entry{Level: "BUCS L3"}, [2]string{"Set", "Voluntary"}},
		{Entry{Level: "FIG AG3 (17–21)"}, [2]string{"1st routine", "2nd routine"}}, // two voluntaries
		{Entry{Level: "Not a level"}, [2]string{"1st routine", "2nd routine"}},
		{Entry{Discipline: Tumbling, Level: "Novice"}, [2]string{"1st pass", "2nd pass"}},
	} {
		if got := RoutineNames(c, tc.entry); got != tc.want {
			t.Errorf("%s %s: %v, want %v", tc.entry.Discipline, tc.entry.Level, got, tc.want)
		}
	}
}
