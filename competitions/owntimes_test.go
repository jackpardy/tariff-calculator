package competitions

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// entriesOf are n entry ids for a flight, "<name>.<i>".
func entriesOf(name string, n int) []string {
	var out []string
	for i := range n {
		out = append(out, fmt.Sprintf("%s.%d", name, i))
	}
	return out
}

// With the default timings a flight is as long as it always was, and a
// planned timetable comes out as it did: between + gymnasts times per
// competitor, competing starting after the changeover.
func TestDefaultTimesAsBefore(t *testing.T) {
	var s Setup
	for _, d := range AllDisciplines {
		old := DefaultTimings(d)
		for n := range 20 {
			f := ScheduledFlight{Flight: Flight{Entries: entriesOf("e", n)}, Discipline: d}
			if got, want := s.duration(f), old.Between+int(math.Ceil(float64(n)*old.PerCompetitor)); got != want {
				t.Errorf("%s, %d gymnasts: %d minutes, want %d", DisciplineName(d), n, got, want)
			}
		}
	}
	entries := append(people("L1", Trampoline, 14), people("L2", Trampoline, 9)...)
	entries = append(entries, people("Synchro L1", Synchro, 5)...)
	entries = append(entries, people("T1", Tumbling, 17)...)
	setup := venue()
	setup.Blocks = []Block{{Name: "Lunch", Minutes: 45, At: "12:30"}}
	plan := PlanSchedule(entries, []string{"L1", "L2", "Synchro L1", "T1"}, setup, 1)
	if len(plan.Flights) == 0 || len(plan.Unplaced) > 0 {
		t.Fatalf("planned: %d flights, %d unplaced", len(plan.Flights), len(plan.Unplaced))
	}
	for _, f := range plan.Flights {
		old := DefaultTimings(f.Discipline)
		if f.End-f.Start != old.Between+int(math.Ceil(float64(len(f.Entries))*old.PerCompetitor)) {
			t.Errorf("%s: %s–%s isn't the old length", f.Name(), Clock(f.Start), Clock(f.End))
		}
		if f.CompetingAt != f.Start+old.Between {
			t.Errorf("%s starts competing at %s, want after its changeover", f.Name(), Clock(f.CompetingAt))
		}
	}
	// Timing it again changes nothing.
	again := plan.Copy()
	if shifts := again.retime(nil); len(shifts) != 0 {
		t.Errorf("a fresh plan timed again moves: %+v", shifts)
	}
}

// A flight's warm-up is its gymnasts times the warm-up per competitor, but
// no less than the least; its event's own figures beat its discipline's, and
// its own times beat both.
func TestFlightTimes(t *testing.T) {
	s := Setup{
		Timings:    map[string]Timings{Trampoline: {WarmUp: 1.5, LeastWarmUp: 10, PerCompetitor: 5, Between: 5, MaxFlight: 12}},
		EventTimes: map[string]EventTimes{"Novice": {WarmUp: 2, PerCompetitor: 6}},
	}
	flight := func(event string, n int) ScheduledFlight {
		return ScheduledFlight{Flight: Flight{Event: event, Entries: entriesOf(event, n)}, Discipline: Trampoline}
	}
	for _, c := range []struct {
		f                             ScheduledFlight
		changeover, warmUp, competing int
	}{
		{flight("Elite", 4), 5, 10, 20},  // 4 × 1.5 is 6: the least, 10
		{flight("Elite", 10), 5, 15, 50}, // 10 × 1.5
		{flight("Elite", 3), 5, 10, 15},
		{flight("Novice", 4), 5, 8, 24}, // its own: 4 × 2, no least; 4 × 6
	} {
		ch, w, comp := s.FlightTimes(c.f)
		if ch != c.changeover || w != c.warmUp || comp != c.competing {
			t.Errorf("%s of %d: %d, %d, %d; want %d, %d, %d", c.f.Event, len(c.f.Entries), ch, w, comp, c.changeover, c.warmUp, c.competing)
		}
		if s.duration(c.f) != c.changeover+c.warmUp+c.competing {
			t.Errorf("%s of %d: %d minutes", c.f.Event, len(c.f.Entries), s.duration(c.f))
		}
	}
	own := flight("Novice", 4)
	own.WarmUpMinutes = ptr(15)
	if _, w, comp := s.FlightTimes(own); w != 15 || comp != 24 {
		t.Errorf("its own warm-up: %d, %d", w, comp)
	}
	own.CompetingMinutes = ptr(50)
	if s.duration(own) != 5+15+50 {
		t.Errorf("its own times: %d minutes", s.duration(own))
	}
	if w, comp := s.AverageTimes(own); w != 8 || comp != 24 {
		t.Errorf("what the averages give: %d, %d", w, comp)
	}

	// Planning goes by them too.
	setup := venue()
	setup.Timings, setup.EventTimes = s.Timings, s.EventTimes
	plan := PlanSchedule(append(people("Novice", Trampoline, 4), people("Elite", Trampoline, 10)...), []string{"Novice", "Elite"}, setup, 1)
	for _, f := range plan.Flights {
		ch, w, comp := setup.FlightTimes(f)
		if f.End-f.Start != ch+w+comp || f.CompetingAt != f.Start+ch+w {
			t.Errorf("%s planned %s–%s, competing %s", f.Name(), Clock(f.Start), Clock(f.End), Clock(f.CompetingAt))
		}
	}
}

// When competing starts: kept on a flight timed now; for one saved before,
// after its changeover, never after it ends.
func TestCompetingStarts(t *testing.T) {
	var f ScheduledFlight
	if err := json.Unmarshal([]byte(`{"event":"L1","day":0,"area":"P1","start":600,"end":660}`), &f); err != nil {
		t.Fatal(err)
	}
	if got := f.CompetingStarts(Setup{}); got != 610 {
		t.Errorf("an old flight starts competing at %s, want 10:10", Clock(got))
	}
	f.End = 605
	if got := f.CompetingStarts(Setup{}); got != 605 {
		t.Errorf("never after it ends: %s", Clock(got))
	}
	f.CompetingAt = 620
	if got := f.CompetingStarts(Setup{}); got != 620 {
		t.Errorf("kept: %s", Clock(got))
	}
	b, _ := json.Marshal(ScheduledFlight{Flight: Flight{Event: "L1"}, Start: 600, End: 660})
	if strings.Contains(string(b), "competing") || strings.Contains(string(b), "warm_up") {
		t.Errorf("nothing new written when unset: %s", b)
	}
}

// A setup saved before warm-up had figures of its own reads as it was:
// "between" is the changeover, with no warm-up, so flights are as long.
func TestOldSetupReads(t *testing.T) {
	old := `{"areas":[{"name":"P1","discipline":""}],"days":[{"name":"Sat","start":"09:00","end":"18:00"}],` +
		`"timings":{"":{"per_competitor":4,"between":12,"max_flight":10},"tumbling":{"per_competitor":2,"between":10,"max_flight":15}},"separate":{}}`
	var s Setup
	if err := json.Unmarshal([]byte(old), &s); err != nil {
		t.Fatal(err)
	}
	f := ScheduledFlight{Flight: Flight{Entries: entriesOf("e", 7)}}
	if got := s.duration(f); got != 12+28 {
		t.Errorf("7 at 4 a competitor and 12 between: %d minutes", got)
	}
	if ch, w, _ := s.FlightTimes(f); ch != 12 || w != 0 {
		t.Errorf("the changeover is between, no warm-up: %d, %d", ch, w)
	}
	again, _ := json.Marshal(s)
	var back Setup
	json.Unmarshal(again, &back)
	b2, _ := json.Marshal(back)
	if strings.Contains(string(again), "warm_up") || strings.Contains(string(again), "event_times") || string(again) != string(b2) {
		t.Errorf("written back as it was: %s", again)
	}
}

// The setup's check covers events' own times: in range, and for an event
// the competition offers.
func TestCheckEventTimes(t *testing.T) {
	s := venue()
	s.Timings = map[string]Timings{Trampoline: {WarmUp: 1, LeastWarmUp: 15, PerCompetitor: 5, Between: 5, MaxFlight: 12}}
	s.EventTimes = map[string]EventTimes{"L1": {WarmUp: 2, LeastWarmUp: 10, PerCompetitor: 6}}
	if err := s.Check([]string{"L1"}); err != nil {
		t.Errorf("fine: %v", err)
	}
	s.EventTimes["Gone"] = EventTimes{PerCompetitor: 5}
	s.EventTimes["L1"] = EventTimes{WarmUp: 31, PerCompetitor: 6}
	s.Timings[Trampoline] = Timings{LeastWarmUp: 121, PerCompetitor: 5, MaxFlight: 12}
	err := s.Check([]string{"L1"})
	for _, want := range []string{`event "Gone" the competition doesn't offer`, "L1's own times are out of range", "Trampoline's timings are out of range"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q in %v", want, err)
		}
	}
}

// A flight's own times stay with it through moving it, moving a gymnast in
// or out, redrawing it, renumbering and copying.
func TestOwnTimesStay(t *testing.T) {
	s := Schedule{
		Setup: Setup{Areas: []Area{{Name: "P1"}, {Name: "P2"}}, Days: []Day{{Name: "Sat", Start: "09:00", End: "18:00"}}},
		Flights: []ScheduledFlight{
			{ID: "a", Flight: Flight{Event: "L1", Number: 1, Of: 2, Entries: entriesOf("a", 2)}, Area: "P1", Start: 540, End: 560},
			{ID: "b", Flight: Flight{Event: "L1", Number: 2, Of: 2, Entries: entriesOf("b", 2)}, Area: "P1", Start: 560, End: 580},
		},
	}
	s.Flights[1].WarmUpMinutes, s.Flights[1].CompetingMinutes = ptr(15), ptr(40)
	own := func(what string) {
		t.Helper()
		for _, f := range s.Flights {
			if f.ID == "b" {
				if f.WarmUpMinutes == nil || *f.WarmUpMinutes != 15 || f.CompetingMinutes == nil || *f.CompetingMinutes != 40 {
					t.Errorf("after %s, b's own times are lost", what)
				}
				if f.End-f.Start != 10+15+40 {
					t.Errorf("after %s, b is %d minutes", what, f.End-f.Start)
				}
				return
			}
		}
		t.Errorf("after %s, b is gone", what)
	}
	s.Retime()
	own("timing")
	s.MoveEntry("a.0", 1)
	own("a gymnast moving in")
	s.Redraw(1, nil, nil, rand.New(rand.NewPCG(1, 2)))
	own("redrawing")
	s.Earlier(1)
	if s.Flights[0].ID != "b" || s.Flights[0].Number != 1 {
		t.Errorf("b first and numbered 1: %+v", s.Flights)
	}
	own("renumbering")
	s.MoveFlight(0, 0, "P2")
	own("moving")
	c := s.Copy()
	*c.Flights[0].WarmUpMinutes = 99
	own("copying")
	b, _ := json.Marshal(s)
	var published Schedule
	json.Unmarshal(b, &published)
	s = published
	own("publishing")
}

// Retime pushes a flight clear of all blocked time, however the blocks are
// listed: past lunch it doesn't land in awards straight after.
func TestRetimeAroundBlocks(t *testing.T) {
	s := Schedule{
		Setup: Setup{Areas: []Area{{Name: "P1"}}, Days: []Day{{Name: "Sat", Start: "09:00", End: "18:00"}}},
		Flights: []ScheduledFlight{
			{Flight: Flight{Event: "L1", Entries: entriesOf("a", 4)}, Area: "P1", Start: 11*60 + 50, End: 12*60 + 20},
		},
		Blocks: []ScheduledBlock{
			{Name: "Awards", Areas: []string{"P1"}, Start: 13 * 60, End: 13*60 + 30},
			{Name: "Lunch", Areas: []string{"P1"}, Start: 12 * 60, End: 13 * 60},
		},
	}
	s.Retime()
	if f := s.Flights[0]; f.Start != 13*60+30 || f.End != 14*60 {
		t.Errorf("after lunch and awards, 13:30–14:00: %s–%s", Clock(f.Start), Clock(f.End))
	}
	if got := s.Breaks(nil, nil); len(got) != 0 {
		t.Errorf("nothing broken: %v", got)
	}
	// A flight in blocked time (made by hand, say) is reported.
	s.Flights[0].Start, s.Flights[0].End = 12*60+50, 13*60+20
	got := BreakLines(s.Breaks(nil, nil))
	if len(got) != 2 || got[0] != "L1 runs into Awards on P1 (13:00–13:30)" || got[1] != "L1 runs into Lunch on P1 (12:00–13:00)" {
		t.Errorf("in blocked time: %v", got)
	}
	if p := s.Problems(nil, nil); len(p) != 2 {
		t.Errorf("among the problems: %v", p)
	}
}

// panel is a timetable to change times on: Panel 1 has A (60 minutes), a
// 10-minute gap, B (50) and C (30); Panel 2 has D. Lunch is on Panel 1 at
// 12:00, and the day ends at 13:00.
func panel() Schedule {
	flight := func(event, area string, start, n int) ScheduledFlight {
		return ScheduledFlight{ID: strings.ToLower(event), Flight: Flight{Event: event, Number: 1, Of: 1, Entries: entriesOf(event, n)}, Area: area, Start: start, End: start + 10 + 5*n, CompetingAt: start + 10}
	}
	return Schedule{
		Setup: Setup{Areas: []Area{{Name: "Panel 1"}, {Name: "Panel 2"}}, Days: []Day{{Name: "Sat", Start: "09:00", End: "13:00"}}},
		Flights: []ScheduledFlight{
			flight("A", "Panel 1", 9*60, 10),
			flight("B", "Panel 1", 10*60+10, 8),
			flight("C", "Panel 1", 11*60, 4),
			flight("D", "Panel 2", 9*60, 6),
		},
		Blocks: []ScheduledBlock{{Name: "Lunch", Areas: []string{"Panel 1"}, Start: 12 * 60, End: 12*60 + 30}},
	}
}

func starts(s Schedule) map[string][2]int {
	out := map[string][2]int{}
	for _, f := range s.Flights {
		out[f.Event] = [2]int{f.Start, f.End}
	}
	return out
}

// A flight's own times move later flights on its area back only as far as
// they must, around blocked time; a shorter flight leaves a gap.
func TestSetFlightTimes(t *testing.T) {
	s := panel()
	shifts, err := s.SetFlightTimes(0, nil, ptr(70), nil) // A 20 minutes longer
	if err != nil {
		t.Fatal(err)
	}
	got := starts(s)
	if got["A"] != [2]int{540, 620} || got["B"] != [2]int{620, 670} || got["C"] != [2]int{670, 700} || got["D"] != [2]int{540, 580} {
		t.Errorf("B and C 10 minutes later, the gap taken up: %v", got)
	}
	if len(shifts) != 3 {
		t.Errorf("A, B and C changed: %+v", shifts)
	}
	areas := AreaShifts(shifts)
	if len(areas) != 1 || areas[0].Line(s.Setup.Days) != "2 flights on Panel 1 start 10 minutes later" {
		t.Errorf("what moved: %+v", areas)
	}
	if a := s.Flights[0]; a.CompetingAt != 550 {
		t.Errorf("A starts competing after its changeover: %s", Clock(a.CompetingAt))
	}

	// A warm-up of its own: competing starts later, B and C later still,
	// and C waits for lunch.
	if _, err := s.SetFlightTimes(0, ptr(25), ptr(70), nil); err != nil {
		t.Fatal(err)
	}
	got = starts(s)
	if got["A"] != [2]int{540, 645} || got["B"] != [2]int{645, 695} || got["C"] != [2]int{750, 780} {
		t.Errorf("C after lunch: %v", got)
	}
	if a := s.Flights[0]; a.CompetingAt != 540+10+25 {
		t.Errorf("A starts competing after its warm-up: %s", Clock(a.CompetingAt))
	}

	// Back to the averages: nothing comes earlier.
	if _, err := s.SetFlightTimes(0, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got = starts(s); got["A"] != [2]int{540, 600} || got["B"] != [2]int{645, 695} {
		t.Errorf("a gap left after A: %v", got)
	}

	// Out of range, or started: refused, nothing changed.
	before := starts(s)
	if _, err := s.SetFlightTimes(0, ptr(-1), ptr(0), nil); err == nil || !strings.Contains(err.Error(), "warm-up") || !strings.Contains(err.Error(), "competing") {
		t.Errorf("out of range: %v", err)
	}
	if _, err := s.SetFlightTimes(0, nil, ptr(10), func(key string) bool { return key == "a" }); err == nil {
		t.Error("a flight that has started keeps its times")
	}
	if _, err := s.SetFlightTimes(9, nil, nil, nil); err == nil {
		t.Error("no such flight")
	}
	if got := starts(s); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Errorf("nothing changed: %v", got)
	}
}

// New averages re-length every flight they touch; flights that have started
// keep their times and own figures, and nothing is pushed into them.
func TestSetTimes(t *testing.T) {
	s := panel()
	s.Flights[0].CompetingMinutes = ptr(50)
	started := func(key string) bool { return key == "a" }
	shifts := s.SetTimes(map[string]Timings{Trampoline: {WarmUp: 1, PerCompetitor: 5, Between: 10, MaxFlight: 12}}, nil, started)
	got := starts(s)
	// A keeps 09:00–10:00; B is 8 longer, from 10:10; C 4 longer, from
	// 11:08; D 6 longer.
	if got["A"] != [2]int{540, 600} || got["B"] != [2]int{610, 668} || got["C"] != [2]int{668, 702} || got["D"] != [2]int{540, 586} {
		t.Errorf("re-lengthed: %v", got)
	}
	if a := s.Flights[0]; a.CompetingMinutes == nil || *a.CompetingMinutes != 50 || a.CompetingAt != 550 {
		t.Errorf("A as it was: %+v", a)
	}
	if areas := AreaShifts(shifts); len(areas) != 1 || areas[0].Line(nil) != "1 flight on Panel 1 starts 8 minutes later" {
		t.Errorf("only C starts later: %+v", areas)
	}
	// An event's own times.
	s.SetTimes(nil, map[string]EventTimes{"B": {PerCompetitor: 6}}, started)
	if got := starts(s); got["B"] != [2]int{610, 668} || got["C"] != [2]int{668, 698} {
		t.Errorf("B by its own, the rest by the defaults: %v", got)
	}

	// A flight before one that has started, grown into it, goes after it.
	s = panel()
	s.Flights[1].Start, s.Flights[1].End = 10*60, 10*60+50
	shifts = s.SetTimes(map[string]Timings{Trampoline: {PerCompetitor: 6, Between: 10, MaxFlight: 12}}, nil, func(key string) bool { return key == "b" })
	if got := starts(s); got["B"] != [2]int{600, 650} || got["A"] != [2]int{650, 720} || got["C"] != [2]int{750, 784} {
		t.Errorf("B where it was, A after it, C after lunch: %v", got)
	}
	if len(shifts) != 3 {
		t.Errorf("A, C and D changed: %+v", shifts)
	}
}

// Each thing a change of times can break is among the breaks, for the page
// to ask first.
func TestTimesBreaks(t *testing.T) {
	people := map[string][]string{}
	for _, f := range panel().Flights {
		for _, id := range f.Entries {
			people[id] = []string{id}
		}
	}
	try := func(change func(*Schedule), setup func(*Schedule)) []string {
		t.Helper()
		s := panel()
		if setup != nil {
			setup(&s)
		}
		before := s.Breaks(people, nil)
		after := s.Copy()
		change(&after)
		return BreakLines(NewBreaks(before, after.Breaks(people, nil)))
	}
	longer := func(flight, minutes int) func(*Schedule) {
		return func(s *Schedule) {
			if _, err := s.SetFlightTimes(flight, nil, ptr(minutes), nil); err != nil {
				t.Fatal(err)
			}
		}
	}
	has := func(got []string, want string) {
		t.Helper()
		if !slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, want) }) {
			t.Errorf("want %q in %v", want, got)
		}
	}
	// Past the end of the day: B after lunch, to 13:20.
	has(try(longer(0, 150), nil), "B runs past the end of Sat (13:00)")
	// A must rule: D before B, on another panel.
	has(try(longer(3, 100), func(s *Schedule) {
		s.Setup.Rules = []Rule{{Kind: RuleBefore, Must: true, Event: "D", Event2: "B"}}
	}), "Breaks the rule: D before B (must)")
	// Someone in two places: B's first gymnast is in D too.
	has(try(longer(3, 100), func(s *Schedule) { people["D.0"] = []string{"B.0"} }), "is in B and D at once")
	delete(people, "D.0")
	// Rest, where it's a must.
	has(try(longer(3, 60), func(s *Schedule) {
		people["D.0"] = []string{"B.0"}
		s.Setup.Rest, s.Setup.RestMust = 30, true
	}), "between D and B (rest is a must: 30 min)")
	delete(people, "D.0")
	// An event's flights either side of lunch.
	has(try(longer(0, 100), func(s *Schedule) {
		c := &s.Flights[2]
		c.Event, c.Number, c.Of = "B", 2, 2
		s.Flights[1].Of = 2
		c.Entries = entriesOf("B2", 4)
	}), "B runs either side of Lunch")
}

// Planning again keeps a flight's own times with the flight, by the IDs it
// keeps, and places it at that length: on a day with room it fits like any
// other flight, and with none it's left out. Own times whose flight is gone
// are said to be dropped.
func TestReplanOwnTimes(t *testing.T) {
	entries := append(people("L1", Trampoline, 14), people("L2", Trampoline, 6)...)
	events := []string{"L1", "L2"}
	// One panel, two days of 6 hours: both events fit on the first by the
	// averages, and not once L1's flight has its own times.
	setup := venue()
	setup.Areas = setup.Areas[:1]
	setup.Days = []Day{{Name: "Saturday", Start: "09:00", End: "15:00"}, {Name: "Sunday", Start: "09:00", End: "15:00"}}
	draft := PlanSchedule(entries, events, setup, 1)
	if slices.ContainsFunc(draft.Flights, func(f ScheduledFlight) bool { return f.Day != 0 }) {
		t.Fatal("by the averages, all on Saturday")
	}
	i := slices.IndexFunc(draft.Flights, func(f ScheduledFlight) bool { return f.Event == "L1" && f.Number == 1 })
	draft.Flights[i].WarmUpMinutes = ptr(240) // L1 is then 330 minutes of the day's 360
	id := draft.Flights[i].ID
	all := func(string) bool { return true }
	own := func(s Schedule) (ScheduledFlight, bool) {
		for _, f := range slices.Concat(s.Flights, s.Unplaced) {
			if f.ID == id {
				return f, true
			}
		}
		return ScheduledFlight{}, false
	}
	for seed := range uint64(5) {
		s, dropped := Replan(entries, events, setup, nil, seed, draft.Flights, draft.Flights, all)
		f, ok := own(s)
		if !ok || f.WarmUpMinutes == nil || *f.WarmUpMinutes != 240 || len(dropped) != 0 {
			t.Fatalf("seed %d: the flight keeps its ID and own times: %+v, dropped %v", seed, f, dropped)
		}
		if len(s.Unplaced) > 0 || f.End-f.Start != setup.duration(f) || f.CompetingAt != f.Start+10+240 {
			t.Errorf("seed %d: placed at its true length: %s–%s, %d unplaced", seed, Clock(f.Start), Clock(f.End), len(s.Unplaced))
		}
		check(t, s) // nothing past the day's end, nothing overlapping
	}

	// A gymnast fewer in L1: the flight is known by its number, and its
	// own times go where its ID does.
	fewer := slices.DeleteFunc(slices.Clone(entries), func(e SchedEntry) bool { return e.ID == draft.Flights[i].Entries[0] })
	s, _ := Replan(fewer, events, setup, nil, 1, draft.Flights, draft.Flights, all)
	if f, ok := own(s); !ok || f.Event != "L1" || f.Number != 1 || f.WarmUpMinutes == nil {
		t.Errorf("by its number, the ID and own times together: %+v", f)
	}

	// No room for it: left out, with its own times, not run past the end.
	draft.Flights[i].WarmUpMinutes = ptr(400)
	s, _ = Replan(entries, events, setup, nil, 1, draft.Flights, draft.Flights, all)
	if f, ok := own(s); !ok || !slices.ContainsFunc(s.Unplaced, func(g ScheduledFlight) bool { return g.ID == id }) || f.WarmUpMinutes == nil {
		t.Errorf("left out: %+v", s.Unplaced)
	}
	check(t, s)

	// L1 no longer entered: its own times are dropped.
	s, dropped := Replan(people("L2", Trampoline, 6), events, setup, nil, 1, draft.Flights, draft.Flights, all)
	if len(dropped) != 1 || dropped[0].ID != id {
		t.Errorf("dropped: %+v", dropped)
	}
	if _, ok := own(s); ok {
		t.Error("no flight has its ID")
	}
}

// The delay "what if": quicker turns shorten competing only, shorter
// changeovers the changeover only, from a flight's own times where it has
// them; competing starts follow.
func TestDelayEasesTimes(t *testing.T) {
	s := Schedule{
		Setup: Setup{
			Areas: []Area{{Name: "P1"}}, Days: []Day{{Name: "Sat", Start: "09:00", End: "18:00"}},
			Timings: map[string]Timings{Trampoline: {WarmUp: 2, PerCompetitor: 5, Between: 10, MaxFlight: 12}},
		},
		Flights: []ScheduledFlight{
			{Flight: Flight{Event: "A", Entries: entriesOf("A", 2)}, Area: "P1", Start: 600, End: 624, CompetingAt: 614},
			{Flight: Flight{Event: "B", Entries: entriesOf("B", 2)}, Area: "P1", Start: 624, End: 648, CompetingAt: 638, CompetingMinutes: ptr(10)},
		},
	}
	s.Flights[1].End = 624 + 10 + 4 + 10
	s.Flights[1].CompetingAt = 624 + 14
	quick := Delay{From: 540, Minutes: 30, Quicker: 50}
	out, _ := delayed(t, s, quick, nil)
	a, b := out.Flights[0], out.Flights[1]
	// A: 10 + 4 + 5 from 10:00; B, no earlier: 10 + 4 + its own 10 halved.
	if a.Start != 600 || a.End != 619 || a.CompetingAt != 614 || b.Start != 624 || b.End != 643 || b.CompetingAt != 638 {
		t.Errorf("quicker turns: A %d–%d (%d), B %d–%d (%d)", a.Start, a.End, a.CompetingAt, b.Start, b.End, b.CompetingAt)
	}
	none := 0
	short := Delay{From: 540, Minutes: 30, Between: &none}
	out, _ = delayed(t, s, short, nil)
	a, b = out.Flights[0], out.Flights[1]
	if a.Start != 600 || a.End != 614 || a.CompetingAt != 604 || b.Start != 624 || b.End != 638 || b.CompetingAt != 628 {
		t.Errorf("no changeovers: A %d–%d (%d), B %d–%d (%d)", a.Start, a.End, a.CompetingAt, b.Start, b.End, b.CompetingAt)
	}
	// Held up during A's warm-up: it starts competing that much later.
	out, _ = delayed(t, s, Delay{From: 605, Minutes: 20}, nil)
	if a, b := out.Flights[0], out.Flights[1]; a.End != 644 || a.CompetingAt != 634 || b.Start != 644 || b.CompetingAt != 658 {
		t.Errorf("held up: A to %d (%d), B from %d (%d)", a.End, a.CompetingAt, b.Start, b.CompetingAt)
	}
}

// What changed since publishing: nothing, for a timetable that only gained
// when competing starts; a later competing start where both kept it.
func TestChangesCompetingStarts(t *testing.T) {
	published := Schedule{
		Setup:   Setup{Areas: []Area{{Name: "P1"}}, Days: []Day{{Name: "Sat", Start: "09:00", End: "18:00"}}},
		Flights: []ScheduledFlight{{ID: "a", Flight: Flight{Event: "L1", Number: 1, Of: 1, Entries: entriesOf("a", 4)}, Area: "P1", Start: 600, End: 630}},
	}
	draft := published.Copy()
	draft.Retime()
	if draft.Flights[0].CompetingAt != 610 {
		t.Fatalf("timed again, it gains when competing starts: %+v", draft.Flights[0])
	}
	name := func(string) string { return "" }
	if got := draft.ChangesFrom(published, name, name); len(got) != 0 {
		t.Errorf("nothing changed: %v", got)
	}
	published = draft.Copy()
	draft.SetFlightTimes(0, ptr(5), ptr(15), nil)
	got := draft.ChangesFrom(published, name, name)
	if len(got) != 1 || !strings.Contains(got[0].Line, "now starts competing about 10:15 (was 10:10)") {
		t.Errorf("competing starts later: %v", got)
	}
}
