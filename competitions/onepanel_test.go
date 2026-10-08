package competitions

import (
	"slices"
	"testing"
)

// onePanelSchedule is L1's three flights back to back on P1, 09:00–10:00,
// beside one flight each of L2 (09:40, with j1 and h1 competing) and L3
// (09:00, with h2 competing) on P2.
func onePanelSchedule() (Schedule, []SchedEntry, []RotaPerson) {
	flight := func(level, area string, n, of, start int, entries ...string) ScheduledFlight {
		return ScheduledFlight{Flight: Flight{Level: level, Number: n, Of: of, Entries: entries}, Discipline: Trampoline, Area: area, Start: start, End: start + 20}
	}
	s := Schedule{
		Setup: Setup{Areas: []Area{{Name: "P1", Discipline: Trampoline}, {Name: "P2", Discipline: Trampoline}}, Days: []Day{{Name: "Sat", Start: "09:00", End: "12:00"}}},
		Flights: []ScheduledFlight{
			flight("L1", "P1", 1, 3, 9*60, "a"), flight("L1", "P1", 2, 3, 9*60+20, "b"), flight("L1", "P1", 3, 3, 9*60+40, "c"),
			flight("L2", "P2", 1, 1, 9*60+40, "x", "y"), flight("L3", "P2", 1, 1, 9*60, "z"),
		},
	}
	entries := []SchedEntry{
		{PlanEntry: PlanEntry{ID: "a", Level: "L1"}, People: []string{"ga"}}, {PlanEntry: PlanEntry{ID: "b", Level: "L1"}, People: []string{"gb"}},
		{PlanEntry: PlanEntry{ID: "c", Level: "L1"}, People: []string{"gc"}},
		{PlanEntry: PlanEntry{ID: "x", Level: "L2"}, People: []string{"j1"}}, {PlanEntry: PlanEntry{ID: "y", Level: "L2"}, People: []string{"h1"}},
		{PlanEntry: PlanEntry{ID: "z", Level: "L3"}, People: []string{"h2"}},
	}
	judge := func(key string, chair bool) RotaPerson {
		return RotaPerson{Key: key, Name: key, Judge: map[string]bool{"L1": true}, Chair: map[string]bool{"L1": chair}}
	}
	helper := func(key string) RotaPerson { return RotaPerson{Key: key, Name: key, Recorder: true} }
	return s, entries, []RotaPerson{judge("ch", true), judge("ch2", true), judge("j1", false), judge("j2", false), judge("j3", false), helper("h1"), helper("h2")}
}

func TestOnePanelPerEvent(t *testing.T) {
	s, entries, officials := onePanelSchedule()
	settings := OfficialSettings{Panels: map[string]Panel{Trampoline: {Chair: 1, Execution: 2, Recorder: 1}}}
	pp := map[string][]string{}
	for _, e := range entries {
		pp[e.ID] = e.People
	}

	// The whole panel stays for L1's three flights: j1 competes in L2 during
	// the third, so they judge none of it; neither recorder is free for all
	// of it, so that seat is empty.
	s.Rota(entries, officials, settings, 1)
	panel := s.Flights[0].Officials
	for _, f := range s.Flights[1:3] {
		if !slices.Equal(f.Officials, panel) {
			t.Errorf("%s has its own panel: %v, not %v", f.Name(), f.Officials, panel)
		}
	}
	if slices.ContainsFunc(panel, func(d Duty) bool { return d.Person == "j1" }) || panel[3].Person != "" {
		t.Errorf("j1 isn't free for all of L1, nor is either recorder: %v", panel)
	}
	r := s.RotaReport(map[string]string{}, officials)
	// (No one can judge L2 or L3: three seats each.)
	if r.Empty != 7 || len(r.Short) != 3 || r.Short[0] != "L1 on P1: no one for 1 recorder" {
		t.Errorf("one of L1's seats empty, for the whole event: %d %v", r.Empty, r.Short)
	}
	for _, w := range r.Busiest {
		if w.Person == panel[0].Person && (w.Duties != 1 || w.Minutes != 60) {
			t.Errorf("the chair has one duty, an hour long: %+v", w)
		}
	}

	// Recorders change between flights, if the organiser lets them: h1
	// records the first two, h2 the last.
	settings.RecordersChange = true
	s.Rota(entries, officials, settings, 1)
	var recorders []string
	for _, f := range s.Flights[:3] {
		recorders = append(recorders, f.Officials[3].Person)
		if !slices.Equal(f.Officials[:3], s.Flights[0].Officials[:3]) {
			t.Errorf("the judges still stay: %v", f.Officials)
		}
	}
	if recorders[0] == "" || recorders[1] == "" || recorders[2] != "h2" || slices.Contains(recorders[:1], "h2") {
		t.Errorf("each flight recorded by someone free then: %v", recorders)
	}

	// A seat given by hand goes with the event's flights.
	settings.RecordersChange = false
	s.Rota(entries, officials, settings, 1)
	if !s.SetDuty(1, 1, "j3") {
		t.Fatal("set")
	}
	for _, f := range s.Flights[:3] {
		if f.Officials[1].Person != "j3" {
			t.Errorf("%s's seat changed too: %v", f.Name(), f.Officials)
		}
	}

	// The chair leaves at 09:25: someone else chairs the rest of L1, its
	// second and third flights, as one seat.
	s.Rota(entries, officials, settings, 1)
	chair := s.Flights[0].Officials[0].Person
	out, fills, err := s.Left(Leave{Person: chair, Day: 0, From: 9*60 + 25}, pp, officials)
	if err != nil {
		t.Fatal(err)
	}
	if len(fills) != 1 || fills[0].Flight != "L1" || fills[0].Start != 9*60+20 || fills[0].End != 10*60 || len(fills[0].Steps) == 0 {
		t.Fatalf("one fill, for the rest of L1: %+v", fills)
	}
	if out.Flights[0].Officials[0].Person != chair || out.Flights[1].Officials[0].Person == chair || out.Flights[1].Officials[0] != out.Flights[2].Officials[0] {
		t.Errorf("the first flight keeps its chair, the rest share a new one: %v / %v / %v", out.Flights[0].Officials, out.Flights[1].Officials, out.Flights[2].Officials)
	}
	if fills[0].Role != RoleChair || fills[0].Steps[0].To != RoleChair {
		t.Errorf("the chair's seat is filled: %+v", fills[0])
	}
}
