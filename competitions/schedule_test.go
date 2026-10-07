package competitions

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

// people makes n entries for an event, each its own person "<event>-<i>", unless
// shared names a person for entry 0.
func people(event, discipline string, n int, shared ...string) []SchedEntry {
	var out []SchedEntry
	for i := range n {
		person := fmt.Sprintf("%s-%d", event, i)
		if i == 0 && len(shared) > 0 {
			person = shared[0]
		}
		out = append(out, SchedEntry{
			PlanEntry:  PlanEntry{ID: fmt.Sprintf("%s#%d", event, i), Level: event, Club: []string{"A", "B", "C"}[i%3]},
			Discipline: discipline, People: []string{person},
		})
	}
	return out
}

func venue() Setup {
	return Setup{
		Areas: []Area{{Name: "Panel 1", Discipline: Trampoline}, {Name: "Panel 2", Discipline: Trampoline}, {Name: "Track", Discipline: Tumbling}},
		Days:  []Day{{Name: "Saturday", Start: "09:00", End: "18:00"}},
	}
}

func check(t *testing.T, s Schedule) {
	t.Helper()
	// Nobody's two turns overlap, no area holds two things at once, and every
	// flight ends by its day's end.
	for i, a := range s.Flights {
		dayEnd, _ := clock(s.Setup.Days[a.Day].End)
		if a.End > dayEnd {
			t.Errorf("%s runs past the day's end", a.Name())
		}
		for _, b := range s.Flights[i+1:] {
			if a.Day == b.Day && a.Start < b.End && b.Start < a.End && a.Area == b.Area {
				t.Errorf("%s and %s overlap on %s", a.Name(), b.Name(), a.Area)
			}
		}
		for _, bl := range s.Blocks {
			for _, area := range bl.Areas {
				if area == a.Area && a.Day == bl.Day && a.Start < bl.End && bl.Start < a.End {
					t.Errorf("%s overlaps %s", a.Name(), bl.Name)
				}
			}
		}
	}
}

func busyOf(s Schedule, entries []SchedEntry, person string) []ScheduledFlight {
	ids := map[string]bool{}
	for _, e := range entries {
		for _, p := range e.People {
			if p == person {
				ids[e.ID] = true
			}
		}
	}
	var out []ScheduledFlight
	for _, f := range s.Flights {
		for _, id := range f.Entries {
			if ids[id] {
				out = append(out, f)
			}
		}
	}
	return out
}

func TestScheduleBasics(t *testing.T) {
	entries := append(people("L1", Trampoline, 10), people("L2", Trampoline, 10)...)
	entries = append(entries, people("Synchro L1", Synchro, 4)...)
	entries = append(entries, people("Tumbling Novice", Tumbling, 8)...)
	s := PlanSchedule(entries, []string{"L1", "L2", "Synchro L1", "Tumbling Novice"}, venue(), 1)
	check(t, s)
	if len(s.Unplaced) != 0 {
		t.Fatalf("everything fits: %+v", s.Unplaced)
	}
	for _, f := range s.Flights {
		switch {
		case f.Discipline == Tumbling && f.Area != "Track":
			t.Errorf("tumbling on the track: %+v", f)
		case f.Discipline == Synchro && !strings.HasPrefix(f.Area, "Panel"):
			t.Errorf("synchro on a trampoline panel: %+v", f)
		}
	}
	// L1: 10 minutes + 10 × 5 = 60 minutes, early on (two panels, little else).
	for _, f := range s.Flights {
		if f.Level == "L1" && (f.End-f.Start != 60 || f.Start > 10*60) {
			t.Errorf("L1 for an hour, starting by 10:00: %s–%s", Clock(f.Start), Clock(f.End))
		}
	}
}

func TestSchedulePeople(t *testing.T) {
	// The first L1 gymnast also tumbles, and the first L2 gymnast is the same person.
	entries := append(people("L1", Trampoline, 10, "Ann"), people("L2", Trampoline, 10, "Ann")...)
	entries = append(entries, people("Tumbling Novice", Tumbling, 8, "Ann")...)
	setup := venue()
	setup.Rest, setup.RestMust = 30, true
	s := PlanSchedule(entries, []string{"L1", "L2", "Tumbling Novice"}, setup, 3)
	check(t, s)
	turns := busyOf(s, entries, "Ann")
	if len(turns) != 3 {
		t.Fatalf("Ann's three turns: %+v", turns)
	}
	for i, a := range turns {
		for _, b := range turns[i+1:] {
			gap := b.Start - a.End
			if a.Start > b.Start {
				gap = a.Start - b.End
			}
			if gap < 30 {
				t.Errorf("Ann needs 30 minutes between %s and %s: %d", a.Name(), b.Name(), gap)
			}
		}
	}
}

func TestScheduleEndAndDays(t *testing.T) {
	setup := venue()
	setup.Days[0].End = "10:30" // 90 minutes
	entries := append(people("L1", Trampoline, 12), people("L2", Trampoline, 12)...)
	entries = append(entries, people("L3", Trampoline, 12)...)
	s := PlanSchedule(entries, []string{"L1", "L2", "L3"}, setup, 1)
	check(t, s)
	// Each level is 70 minutes; two panels fit two levels by 10:30.
	if len(s.Unplaced) != 1 || s.Unplaced[0].Level != "L3" && s.Unplaced[0].Level != "L1" && s.Unplaced[0].Level != "L2" {
		t.Errorf("one level doesn't fit: %+v", s.Unplaced)
	}
	// A second day takes it.
	setup.Days = append(setup.Days, Day{Name: "Sunday", Start: "09:00", End: "10:30", Areas: []string{"Panel 1"}})
	s = PlanSchedule(entries, []string{"L1", "L2", "L3"}, setup, 1)
	check(t, s)
	if len(s.Unplaced) != 0 {
		t.Fatalf("the second day fits it: %+v", s.Unplaced)
	}
	for _, f := range s.Flights {
		if f.Day == 1 && f.Area != "Panel 1" {
			t.Errorf("Sunday uses Panel 1 only: %+v", f)
		}
	}
}

func TestScheduleBlocks(t *testing.T) {
	setup := venue()
	setup.Blocks = []Block{
		{Name: "Lunch", Minutes: 45, Day: 0, From: "11:30", To: "13:30"},
		{Name: "Awards", Minutes: 30, Day: 0, At: "17:00", Areas: []string{"Panel 1"}},
	}
	var entries []SchedEntry
	for i := range 6 {
		entries = append(entries, people(fmt.Sprintf("L%d", i), Trampoline, 12)...)
	}
	s := PlanSchedule(entries, []string{"L0", "L1", "L2", "L3", "L4", "L5"}, setup, 2)
	check(t, s)
	if len(s.Blocks) != 2 || len(s.UnplacedBlocks) != 0 {
		t.Fatalf("both blocks placed: %+v %v", s.Blocks, s.UnplacedBlocks)
	}
	for _, b := range s.Blocks {
		if b.Name == "Lunch" && (b.Start < 11*60+30 || b.End > 13*60+30 || len(b.Areas) != 3) {
			t.Errorf("lunch within its window, on every area: %+v", b)
		}
		if b.Name == "Awards" && (Clock(b.Start) != "17:00" || len(b.Areas) != 1) {
			t.Errorf("awards at 17:00 on Panel 1: %+v", b)
		}
	}
}

func TestScheduleRules(t *testing.T) {
	setup := venue()
	setup.Rules = []Rule{
		{Kind: RuleArea, Must: true, Event: "Synchro L1", Area: "Panel 2"},
		{Kind: RuleBefore, Must: true, Event: "L2", Event2: "L1"},
		{Kind: RuleApart, Must: true, Event: "L1", Event2: "Tumbling Novice"},
	}
	entries := append(people("L1", Trampoline, 10), people("L2", Trampoline, 10)...)
	entries = append(entries, people("Synchro L1", Synchro, 6)...)
	entries = append(entries, people("Tumbling Novice", Tumbling, 8)...)
	s := PlanSchedule(entries, []string{"L1", "L2", "Synchro L1", "Tumbling Novice"}, setup, 4)
	check(t, s)
	if len(s.Unplaced) != 0 {
		t.Fatalf("everything fits: %+v", s.Unplaced)
	}
	at := map[string]ScheduledFlight{}
	for _, f := range s.Flights {
		at[f.Level] = f
	}
	if at["Synchro L1"].Area != "Panel 2" {
		t.Errorf("synchro on Panel 2: %+v", at["Synchro L1"])
	}
	if at["L2"].End > at["L1"].Start {
		t.Errorf("L2 before L1: L2 ends %s, L1 starts %s", Clock(at["L2"].End), Clock(at["L1"].Start))
	}
	l1, tum := at["L1"], at["Tumbling Novice"]
	if l1.Start < tum.End && tum.Start < l1.End {
		t.Errorf("L1 and tumbling apart: %+v %+v", l1, tum)
	}
	if got := setup.Rules[0].Describe(setup.Days); got != "Synchro L1 on Panel 2 (must)" {
		t.Errorf("described as %q", got)
	}
}

func TestSetupCheck(t *testing.T) {
	events := []string{"L1", "L2"}
	if err := venue().Check(events); err != nil {
		t.Fatal(err)
	}
	if err := DefaultSetup([]string{Trampoline, Synchro, DMT}).Check(events); err != nil {
		t.Errorf("the default setup: %v", err)
	}
	if got := DefaultSetup([]string{Trampoline, Synchro, DMT}).Areas; len(got) != 2 || got[0].Name != "Panel 1" || got[1].Name != "DMT 1" {
		t.Errorf("one panel serves trampoline and synchro: %+v", got)
	}
	for _, change := range []func(*Setup){
		func(s *Setup) { s.Days = nil },
		func(s *Setup) { s.Days[0].End = "08:00" },
		func(s *Setup) { s.Areas = append(s.Areas, Area{Name: "Panel 1", Discipline: Trampoline}) },
		func(s *Setup) { s.Areas[0].Discipline = Synchro },
		func(s *Setup) { s.Days[0].Areas = []string{"Hall"} },
		func(s *Setup) { s.Blocks = []Block{{Name: "Lunch", Minutes: 60, From: "12:00", To: "12:30"}} },
		func(s *Setup) { s.Rules = []Rule{{Kind: RuleArea, Event: "L1", Area: "Hall"}} },
		func(s *Setup) { s.Rules = []Rule{{Kind: RuleBefore, Event: "L1", Event2: "L1"}} },
		func(s *Setup) { s.Rules = []Rule{{Kind: RuleDay, Event: "L9"}} },
		func(s *Setup) { s.Timings = map[string]Timings{Trampoline: {PerCompetitor: 0, MaxFlight: 10}} },
	} {
		s := venue()
		change(&s)
		if s.Check(events) == nil {
			t.Errorf("%+v should be refused", s)
		}
	}
}

func TestReportAndFixes(t *testing.T) {
	order := []string{"L1", "L2", "L3"}
	peopleOf := func(entries []SchedEntry) map[string][]string {
		out := map[string][]string{}
		for _, e := range entries {
			out[e.ID] = e.People
		}
		return out
	}

	// Both levels prefer Panel 2, but together they don't fit there by 11:00.
	setup := venue()
	setup.Days[0].End = "11:00"
	setup.Rules = []Rule{{Kind: RuleArea, Event: "L1", Area: "Panel 2"}, {Kind: RuleArea, Event: "L2", Area: "Panel 2"}}
	entries := append(people("L1", Trampoline, 12), people("L2", Trampoline, 12)...)
	r := PlanSchedule(entries, order, setup, 1).Report(peopleOf(entries))
	if len(r.Unplaced) != 0 || len(r.Broken) != 1 || r.Days[0].End != "11:00" {
		t.Errorf("both placed, one prefer broken: %+v", r)
	}

	// Ann has two turns close together: rest as a prefer is reported short.
	setup = venue()
	setup.Rest = 30
	entries = append(people("L1", Trampoline, 12, "Ann"), people("Tumbling Novice", Tumbling, 4, "Ann")...)
	r = PlanSchedule(entries, []string{"L1", "Tumbling Novice"}, setup, 1).Report(peopleOf(entries))
	for _, short := range r.ShortRest {
		if short.Person != "Ann" || short.Minutes >= 30 {
			t.Errorf("only Ann can be short of rest: %+v", short)
		}
	}

	// Three levels in 90 minutes on two panels: one doesn't fit; a third panel fixes it.
	setup = venue()
	setup.Days[0].End = "10:30"
	entries = append(people("L1", Trampoline, 12), people("L2", Trampoline, 12)...)
	entries = append(entries, people("L3", Trampoline, 12)...)
	if r := PlanSchedule(entries, order, setup, 1).Report(peopleOf(entries)); len(r.Unplaced) != 1 {
		t.Fatalf("one level doesn't fit: %+v", r.Unplaced)
	}
	fits := map[string]bool{}
	for _, f := range Fixes(entries, order, setup, 1) {
		fits[f.Change] = f.Fits
	}
	if !fits["another trampoline panel"] {
		t.Errorf("another panel fits it all: %+v", fits)
	}
	if fits["Trampoline at 4.5 minutes per competitor, not 5"] {
		t.Error("half a minute less isn't enough")
	}

	// One panel, 90 minutes, an event of two flights: one fits, so a cap at
	// its entries would fit everything.
	setup = venue()
	setup.Areas, setup.Days[0].End = setup.Areas[:1], "10:30"
	entries = people("L1", Trampoline, 24)
	fits = map[string]bool{}
	for _, f := range Fixes(entries, []string{"L1"}, setup, 1) {
		fits[f.Change] = f.Fits
	}
	if !fits["L1 capped at 12 entries (12 fewer)"] {
		t.Errorf("a cap fits: %+v", fits)
	}
}

func TestScheduleEdits(t *testing.T) {
	setup := venue()
	setup.Blocks = []Block{{Name: "Lunch", Minutes: 60, Day: 0, At: "12:00"}}
	entries := append(people("L1", Trampoline, 10, "Ann"), people("L2", Trampoline, 10)...)
	entries = append(entries, people("Tumbling Novice", Tumbling, 4, "Ann")...)
	s := PlanSchedule(entries, []string{"L1", "L2", "Tumbling Novice"}, setup, 1)
	peopleOf := map[string][]string{}
	for _, e := range entries {
		peopleOf[e.ID] = e.People
	}
	names := map[string]string{"Ann": "Ann"}
	if p := s.Problems(peopleOf, names); len(p) != 0 {
		t.Fatalf("a fresh plan has no problems: %v", p)
	}
	l1, _ := s.Find("L1#0")
	l2, _ := s.Find("L2#0")
	// Moving L2's gymnast into L1 makes L1 longer; times are worked out again.
	before := s.Flights[l1].End
	if !s.MoveEntry("L2#0", l1) {
		t.Fatal("moved")
	}
	l1, _ = s.Find("L1#0")
	if s.Flights[l1].End != before+5 {
		t.Errorf("L1 is five minutes longer: %s → %s", Clock(before), Clock(s.Flights[l1].End))
	}
	// Moving L2 onto L1's panel puts it after L1, clear of lunch.
	l2, _ = s.Find("L2#1")
	if !s.MoveFlight(l2, 0, s.Flights[l1].Area) {
		t.Fatal("moved the flight")
	}
	l2, _ = s.Find("L2#1")
	l1, _ = s.Find("L1#0")
	if s.Flights[l2].Start < s.Flights[l1].End {
		t.Errorf("L2 after L1: %+v %+v", s.Flights[l1], s.Flights[l2])
	}
	if s.MoveFlight(l2, 0, "Track") {
		t.Error("trampoline can't go on the track")
	}
	// Putting Ann's tumbling flight on top of L1 is flagged, not refused.
	tum, _ := s.Find("Tumbling Novice#0")
	s.Flights[tum].Start = s.Flights[l1].Start
	s.Retime()
	if p := s.Problems(peopleOf, names); len(p) == 0 || !strings.Contains(p[0], "Ann is in") {
		t.Errorf("Ann in two places at once is flagged: %v", p)
	}
	s.Setup.Days[0].End = "09:30"
	if p := s.Problems(peopleOf, names); !strings.Contains(strings.Join(p, " "), "runs past the end") {
		t.Errorf("a flight past the day's end is flagged: %v", p)
	}
}

func TestBlocksStayInTheirDay(t *testing.T) {
	setup := venue()
	setup.Days[0].End = "10:00"
	setup.Blocks = []Block{{Name: "Lunch", Minutes: 45, From: "12:00", To: "14:00"}}
	s := PlanSchedule(people("L1", Trampoline, 4), []string{"L1"}, setup, 1)
	if len(s.Blocks) != 0 || len(s.UnplacedBlocks) != 1 {
		t.Errorf("lunch after the day's end doesn't fit: %+v %v", s.Blocks, s.UnplacedBlocks)
	}
}

func TestOrderForRest(t *testing.T) {
	// One panel, so Dara's two events run back to back: she goes first in the
	// earlier flight and last in the later one, however they're drawn.
	setup := venue()
	setup.Areas = setup.Areas[:1]
	setup.Rest = 20
	entries := append(people("BUCS L3", Trampoline, 6, "Dara"), people("BUCS L5", Trampoline, 6, "Dara")...)
	pp := map[string][]string{}
	clubs := map[string]string{}
	for _, e := range entries {
		pp[e.ID], clubs[e.ID] = e.People, e.Club
	}
	for seed := range uint64(8) {
		s := PlanSchedule(entries, []string{"BUCS L3", "BUCS L5"}, setup, seed)
		first, second := s.Flights[0], s.Flights[1]
		if first.Entries[0] != first.Level+"#0" || second.Entries[len(second.Entries)-1] != second.Level+"#0" {
			t.Fatalf("seed %d: Dara should be first then last: %v, %v", seed, first.Entries, second.Entries)
		}
		s.Redraw(1, clubs, pp, rand.New(rand.NewPCG(seed, 2)))
		if e := s.Flights[1].Entries; e[len(e)-1] != second.Level+"#0" {
			t.Errorf("seed %d: a redraw keeps Dara last: %v", seed, e)
		}
	}

	// A long enough gap (twice the rest or more) leaves the draw alone.
	s := Schedule{Setup: Setup{Rest: 20}, Flights: []ScheduledFlight{
		{Flight: Flight{Entries: []string{"a", "x"}}, Start: 540, End: 600},
		{Flight: Flight{Entries: []string{"x", "b"}}, Start: 640, End: 700},
	}}
	if p := s.restPulls(map[string][]string{"x": {"X"}}); len(p) != 0 {
		t.Errorf("40 minutes apart is enough: %v", p)
	}
	s.Flights[1].Start = 630
	s.OrderForRest(map[string][]string{"x": {"X"}})
	if fmt.Sprint(s.Flights[0].Entries, s.Flights[1].Entries) != "[x a] [b x]" {
		t.Errorf("30 minutes apart: %v %v", s.Flights[0].Entries, s.Flights[1].Entries)
	}

	// The shortest gaps go nearest the edges; someone wanted at both ends of
	// the same flight stays where drawn.
	got := orderForRest([]string{"a", "b", "c", "d", "e", "f"}, map[string]int{"e": -30, "c": -5, "b": 10, "a": 2, "f": 0})
	if fmt.Sprint(got) != "[c e d f b a]" {
		t.Errorf("order: %v", got)
	}
}

func TestFreeAfterFlights(t *testing.T) {
	s := Schedule{
		Setup: Setup{Areas: []Area{{Name: "P1"}, {Name: "P2"}}, Days: []Day{{Name: "Sun", Start: "09:00", End: "17:00"}, {Name: "Mon", Start: "09:00", End: "10:00"}}},
		Flights: []ScheduledFlight{
			{Flight: Flight{Level: "A"}, Day: 0, Area: "P1", Start: 9 * 60, End: 10 * 60},
			{Flight: Flight{Level: "B"}, Day: 0, Area: "P2", Start: 9 * 60, End: 9*60 + 30},
		},
		Blocks: []ScheduledBlock{
			{Name: "Lunch", Day: 0, Areas: []string{"P1", "P2"}, Start: 12 * 60, End: 12*60 + 45},
			{Name: "Ad hoc", Day: 0, Areas: []string{"P2"}, Start: 13 * 60, End: 14 * 60}, // one area: still room on P1
			{Name: "Awards", Day: 0, Areas: []string{"P1", "P2"}, Start: 16 * 60, End: 17 * 60},
		},
	}
	r := s.Report(nil)
	sun, mon := r.Days[0], r.Days[1]
	// The awards fill to the end, but 7 hours after the last flight are free
	// less lunch and the awards.
	if sun.Finish != "17:00" || sun.SpareMinutes != 0 || sun.FlightsEnd != "10:00" || sun.FreeMinutes != 7*60-45-60 {
		t.Errorf("Sunday: %+v", sun)
	}
	if mon.FlightsEnd != "" || mon.FreeMinutes != 60 {
		t.Errorf("a day with nothing on is free: %+v", mon)
	}
}
