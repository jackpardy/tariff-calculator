package competitions

import (
	"fmt"
	"strings"
	"testing"
)

// judges makes n people who can judge (and chair) every event given, and help,
// from clubs A, B and C in turn.
func judges(prefix string, n int, events ...string) []RotaPerson {
	var out []RotaPerson
	for i := range n {
		p := RotaPerson{Key: fmt.Sprintf("%s-%d", prefix, i), Name: fmt.Sprintf("%s %d", prefix, i), Club: []string{"A", "B", "C"}[i%3],
			Judge: map[string]bool{}, Chair: map[string]bool{}, Recorder: true, Marshal: true}
		for _, e := range events {
			p.Judge[e], p.Chair[e] = true, true
		}
		out = append(out, p)
	}
	return out
}

func clubsOf(entries []SchedEntry) map[string]string {
	out := map[string]string{}
	for _, e := range entries {
		out[e.ID] = e.Club
	}
	return out
}

func peopleOfEntries(entries []SchedEntry) map[string][]string {
	out := map[string][]string{}
	for _, e := range entries {
		out[e.ID] = e.People
	}
	return out
}

func TestRotaFillsPanels(t *testing.T) {
	setup := venue()
	entries := append(people("BUCS L3", Trampoline, 10), people("BUCS L5", Trampoline, 10)...)
	s := PlanSchedule(entries, []string{"BUCS L3", "BUCS L5"}, setup, 1)
	officials := judges("J", 30, "BUCS L3", "BUCS L5")
	s.Rota(entries, officials, OfficialSettings{}, 1)
	for _, f := range s.Flights {
		if len(f.Officials) != len(CodePanel.Seats()) {
			t.Fatalf("%s has %d seats", f.Name(), len(f.Officials))
		}
		seen := map[string]bool{}
		for _, d := range f.Officials {
			if d.Person == "" || seen[d.Person] {
				t.Errorf("%s: seat %s is %q", f.Name(), d.Role, d.Person)
			}
			seen[d.Person] = true
		}
	}
	if p := s.RotaProblems(peopleOfEntries(entries), officials, nil); len(p) > 0 {
		t.Errorf("problems: %v", p)
	}
	if r := s.RotaReport(clubsOf(entries), officials); len(r.Short) > 0 {
		t.Errorf("short: %v", r.Short)
	}
}

func TestRotaNeverTwoPlaces(t *testing.T) {
	// Two panels at once and few officials: no one sits on both, and no one
	// judges while competing; seats left empty are reported.
	setup := venue()
	entries := append(people("BUCS L3", Trampoline, 10, "J-0"), people("BUCS L5", Trampoline, 10)...)
	s := PlanSchedule(entries, []string{"BUCS L3", "BUCS L5"}, setup, 1)
	if s.Flights[0].Start != s.Flights[1].Start {
		t.Fatalf("expected both flights at once: %+v", s.Flights)
	}
	officials := judges("J", 12, "BUCS L3", "BUCS L5")
	s.Rota(entries, officials, OfficialSettings{}, 1)
	if p := s.RotaProblems(peopleOfEntries(entries), officials, nil); len(p) > 0 {
		t.Errorf("problems: %v", p)
	}
	r := s.RotaReport(clubsOf(entries), officials)
	if len(r.Short) == 0 {
		t.Errorf("11 people for 22 seats should leave some short")
	}
	for _, f := range s.Flights {
		for _, d := range f.Officials {
			if d.Person == "J-0" && f.Level == "BUCS L3" {
				t.Errorf("J-0 competes in BUCS L3")
			}
		}
	}
}

func TestRotaWhoCanDoWhat(t *testing.T) {
	setup := venue()
	setup.Areas = setup.Areas[:1]
	entries := people("BUCS L3", Trampoline, 6)
	s := PlanSchedule(entries, []string{"BUCS L3"}, setup, 1)
	officials := judges("J", 8, "BUCS L3")
	for i := range officials {
		officials[i].Chair = map[string]bool{} // no one can chair
		officials[i].Recorder, officials[i].Marshal = false, false
	}
	officials = append(officials, RotaPerson{Key: "R", Name: "R", Recorder: true})
	panel := Panel{Chair: 1, Execution: 4, Difficulty: 1, Recorder: 1}
	s.Rota(entries, officials, OfficialSettings{Panels: map[string]Panel{Trampoline: panel}}, 1)
	got := map[string]string{}
	for _, d := range s.Flights[0].Officials {
		got[d.Role] += d.Person + ","
	}
	if got[RoleChair] != "," || got[RoleRecorder] != "R," {
		t.Errorf("seats: %v", got)
	}
	r := s.RotaReport(clubsOf(entries), officials)
	if len(r.Short) != 1 || !strings.Contains(r.Short[0], "no one for 1 chair of judges") {
		t.Errorf("short: %v", r.Short)
	}
}

func TestRotaRules(t *testing.T) {
	setup := venue()
	setup.Areas = setup.Areas[:1]
	setup.Rules = []Rule{
		{Kind: RuleRole, Must: true, Event: "BUCS L5", Person: "J-7", Name: "Mary", Role: RoleChair},
		{Kind: RuleOff, Must: true, Person: "J-1", Name: "Tom"},
		{Kind: RuleHours, Must: true, Person: "J-2", Name: "Ann", Day: 0, From: "15:00"},
	}
	entries := append(people("BUCS L3", Trampoline, 6), people("BUCS L5", Trampoline, 6)...)
	s := PlanSchedule(entries, []string{"BUCS L3", "BUCS L5"}, setup, 1)
	officials := judges("J", 14, "BUCS L3", "BUCS L5")
	s.Rota(entries, officials, OfficialSettings{}, 3)
	for _, f := range s.Flights {
		for _, d := range f.Officials {
			if d.Person == "J-1" || d.Person == "J-2" {
				t.Errorf("%s sits on %s", d.Person, f.Name())
			}
			if f.Level == "BUCS L5" && d.Role == RoleChair && d.Person != "J-7" {
				t.Errorf("Mary should chair BUCS L5, not %s", d.Person)
			}
		}
	}
	if r := s.RotaReport(clubsOf(entries), officials); len(r.Broken) > 0 {
		t.Errorf("broken: %v", r.Broken)
	}
	if got := setup.Rules[2].Describe(setup.Days); got != "Ann officiates on Saturday only from 15:00 (must)" {
		t.Errorf("describe: %q", got)
	}
	if got := setup.Rules[0].Describe(setup.Days); got != "Mary chairs BUCS L5 (must)" {
		t.Errorf("describe: %q", got)
	}
	if err := setup.Check([]string{"BUCS L3", "BUCS L5"}); err != nil {
		t.Errorf("check: %v", err)
	}
	bad := setup
	bad.Rules = []Rule{{Kind: RuleRole, Person: "x", Name: "X", Role: "boss"}, {Kind: RuleHours, Person: "y", Name: "Y", From: "12:00", To: "11:00"}}
	if err := bad.Check([]string{"BUCS L3"}); err == nil || !strings.Contains(err.Error(), "unknown role") || !strings.Contains(err.Error(), "ends before") {
		t.Errorf("check: %v", err)
	}

	// Broken by hand: the report says so.
	for k, d := range s.Flights[1].Officials {
		if d.Role == RoleChair {
			s.SetDuty(1, k, "J-0")
		}
	}
	if r := s.RotaReport(clubsOf(entries), officials); len(r.Broken) != 1 {
		t.Errorf("broken: %v", r.Broken)
	}
}

func TestRotaSharesOwnClubJudging(t *testing.T) {
	// Every flight has all three clubs' gymnasts, and every club has judges:
	// the times a club judges its own come out within a little of each other.
	setup := venue()
	setup.Areas = setup.Areas[:1]
	var entries []SchedEntry
	var events []string
	for l := range 6 {
		ev := fmt.Sprintf("Level %d", l)
		events = append(events, ev)
		entries = append(entries, people(ev, Trampoline, 6)...)
	}
	s := PlanSchedule(entries, events, setup, 1)
	s.Rota(entries, judges("J", 27, events...), OfficialSettings{Panels: map[string]Panel{Trampoline: {Chair: 1, Execution: 4, Difficulty: 1}}}, 1)
	r := s.RotaReport(clubsOf(entries), judges("J", 27, events...))
	if len(r.OwnClub) != 3 {
		t.Fatalf("own club: %v", r.OwnClub)
	}
	if r.OwnClub[0].Times-r.OwnClub[2].Times > 2 {
		t.Errorf("own-club judging isn't shared: %v", r.OwnClub)
	}
}

func TestCoachClashes(t *testing.T) {
	setup := venue()
	setup.Areas = setup.Areas[:2]
	a, b := people("BUCS L3", Trampoline, 6), people("BUCS L5", Trampoline, 6)
	a[0].Coaches, b[0].Coaches = []string{"c:sam"}, []string{"c:sam"}
	entries := append(a, b...)
	s := PlanSchedule(entries, []string{"BUCS L3", "BUCS L5"}, setup, 1)
	coaches := map[string][]string{a[0].ID: {"c:sam"}, b[0].ID: {"c:sam"}}
	if c := s.CoachClashes(coaches, map[string]string{"c:sam": "Sam"}); len(c) != 0 {
		t.Errorf("the planner keeps Sam's gymnasts apart where it can: %v", c)
	}
	// Put side by side by hand, it's reported.
	s.MoveFlight(1, 0, s.Flights[0].Area)
	s.Flights[1].Area, s.Flights[1].Start, s.Flights[1].End = "Panel 2", s.Flights[0].Start, s.Flights[0].End
	if c := s.CoachClashes(coaches, map[string]string{"c:sam": "Sam"}); len(c) != 1 || !strings.Contains(c[0], "Sam coaches in") {
		t.Errorf("clash: %v", c)
	}
}

func TestPlacedWhereJudgesAreFree(t *testing.T) {
	// L5's only judges compete in L3. Unstaffed, L5 runs alongside L3 on the
	// second panel; staffed, it waits until L3 is over, when they're free.
	setup := venue()
	setup.Areas = setup.Areas[:2]
	var l3 []SchedEntry
	for i, e := range people("L3", Trampoline, 6) {
		e.People = []string{fmt.Sprintf("J-%d", i)}
		l3 = append(l3, e)
	}
	entries := append(l3, people("L5", Trampoline, 6)...)
	order := []string{"L3", "L5"}
	staff := &Staffing{Judges: map[string][]string{"L5": {"J-0", "J-1", "J-2", "J-3", "J-4", "J-5"}}, Need: map[string]int{Trampoline: 6}}
	at := func(s Schedule, ev string) ScheduledFlight {
		for _, f := range s.Flights {
			if f.Level == ev {
				return f
			}
		}
		t.Fatalf("no %s", ev)
		return ScheduledFlight{}
	}
	plain := PlanSchedule(entries, order, setup, 1)
	if at(plain, "L5").Start != at(plain, "L3").Start {
		t.Fatalf("unstaffed, L5 runs alongside L3: %+v", plain.Flights)
	}
	staffed := PlanStaffed(entries, order, setup, staff, 1)
	if at(staffed, "L5").Start < at(staffed, "L3").End {
		t.Errorf("staffed, L5 waits for its judges: %+v", staffed.Flights)
	}
}

func TestRotaStaffsBlocks(t *testing.T) {
	// An ad hoc event on the second panel needs a chair and two judges while
	// L3 runs on the first: five different people, none twice.
	setup := venue()
	setup.Areas = setup.Areas[:2]
	setup.Blocks = []Block{
		{Name: "Ad hoc synchro", Minutes: 40, Day: 0, At: "09:00", Areas: []string{"Panel 2"}, Officials: Panel{Chair: 1, Execution: 2}},
		{Name: "Lunch", Minutes: 30, Day: 0, At: "12:00"},
	}
	entries := people("L3", Trampoline, 6)
	s := PlanSchedule(entries, []string{"L3"}, setup, 1)
	officials := judges("J", 8, "L3")
	panel := Panel{Chair: 1, Execution: 2}
	s.Rota(entries, officials, OfficialSettings{Panels: map[string]Panel{Trampoline: panel}}, 1)
	var adhoc, lunch ScheduledBlock
	for _, b := range s.Blocks {
		switch b.Name {
		case "Ad hoc synchro":
			adhoc = b
		case "Lunch":
			lunch = b
		}
	}
	if len(adhoc.Officials) != 3 || len(lunch.Officials) != 0 {
		t.Fatalf("seats: ad hoc %v, lunch %v", adhoc.Officials, lunch.Officials)
	}
	seen := map[string]bool{}
	for _, d := range append(adhoc.Officials, s.Flights[0].Officials...) {
		if d.Person == "" || seen[d.Person] {
			t.Errorf("seat %s: %q", d.Role, d.Person)
		}
		seen[d.Person] = true
	}
	if p := s.RotaProblems(peopleOfEntries(entries), officials, nil); len(p) > 0 {
		t.Errorf("problems: %v", p)
	}
	// By hand, a clash with the flight is reported.
	s.SetBlockDuty(0, 0, s.Flights[0].Officials[0].Person)
	if p := s.RotaProblems(peopleOfEntries(entries), officials, nil); len(p) != 1 || !strings.Contains(p[0], "officiates") {
		t.Errorf("problems: %v", p)
	}
}
