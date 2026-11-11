package competitions

import (
	"strings"
	"testing"
)

func TestOfficialLeaves(t *testing.T) {
	judge := func(key string, chair bool) RotaPerson {
		return RotaPerson{Key: key, Name: strings.ToUpper(key), Judge: map[string]bool{"L": true}, Execution: map[string]bool{"L": true}, Difficulty: map[string]bool{"L": true}, Chair: map[string]bool{"L": chair}}
	}
	helper := func(key string) RotaPerson {
		return RotaPerson{Key: key, Name: strings.ToUpper(key), Recorder: true, Marshal: true}
	}
	officials := []RotaPerson{judge("c", true), judge("e1", true), judge("e2", false), judge("j", false), helper("h"), helper("h2")}
	// r records, and can judge too.
	r := judge("r", false)
	r.Recorder = true
	officials = append(officials, r)
	panel := []Duty{{Role: RoleChair, Person: "c"}, {Role: RoleExecution, Person: "e1"}, {Role: RoleExecution, Person: "e2"}, {Role: RoleRecorder, Person: "r"}}
	s := Schedule{
		Setup: Setup{Areas: []Area{{Name: "P1"}}, Days: []Day{{Name: "Sat", Start: "09:00", End: "18:00"}, {Name: "Sun", Start: "09:00", End: "18:00"}}},
		Flights: []ScheduledFlight{
			{Flight: Flight{Event: "L"}, Area: "P1", Day: 0, Start: 10 * 60, End: 11 * 60, Officials: panel},
			{Flight: Flight{Event: "L"}, Area: "P1", Day: 1, Start: 10 * 60, End: 11 * 60, Officials: panel},
		},
	}
	name := func(k string) string { return strings.ToUpper(k) }
	describe := func(fill SeatFill) string {
		var out []string
		for _, st := range fill.Steps {
			out = append(out, st.Describe(name))
		}
		return strings.Join(out, "; ")
	}

	// The chair leaves: no one free can chair, so E1 moves up, and J, free,
	// judges in their place: two moves, not three (R up from recorder, and a
	// helper recording).
	out, fills, err := s.Left(Leave{Person: "c", Day: 0, From: 9 * 60}, nil, officials)
	if err != nil {
		t.Fatal(err)
	}
	if len(fills) != 1 || describe(fills[0]) != "E1 moves from execution judge to chair of judges; J takes execution judge" {
		t.Fatalf("the chair's seat: %q", describe(fills[0]))
	}
	if got := out.Flights[0].Officials; got[0].Person != "e1" || got[1].Person != "j" || got[3].Person != "r" || out.Flights[1].Officials[0].Person != "c" {
		t.Errorf("the panel after: %+v; Sunday's is as it was", got)
	}

	// An execution judge leaves: J, free, takes the seat; no one moves.
	_, fills, _ = s.Left(Leave{Person: "e2", Day: 0, From: 9 * 60}, nil, officials)
	if got := describe(fills[0]); got != "J takes execution judge" {
		t.Errorf("someone free takes it: %q", got)
	}

	// J competes then, so can't; for the rest of the competition, Sunday
	// too.
	busy := s
	busy.Flights = append(busy.Flights, ScheduledFlight{Flight: Flight{Event: "M", Entries: []string{"x"}}, Area: "P2", Day: 0, Start: 10*60 + 30, End: 11 * 60})
	_, fills, _ = busy.Left(Leave{Person: "e2", Day: 0, From: 9 * 60, ForGood: true}, map[string][]string{"x": {"j"}}, officials)
	if len(fills) != 2 || describe(fills[0]) != "R moves from recorder to execution judge; H takes recorder" || describe(fills[1]) != "J takes execution judge" {
		t.Errorf("J competing on Saturday; Sunday too: %q / %q", describe(fills[0]), describe(fills[1]))
	}

	// No one on P1's panel can chair, but K, judging on P2 at the same time,
	// can: K moves across, and J judges on P2 in their place.
	across := Schedule{Setup: s.Setup, Flights: []ScheduledFlight{
		{Flight: Flight{Event: "L"}, Area: "P1", Start: 10 * 60, End: 11 * 60, Officials: []Duty{{Role: RoleChair, Person: "c"}, {Role: RoleExecution, Person: "e2"}}},
		{Flight: Flight{Event: "L"}, Area: "P2", Start: 10*60 + 30, End: 11*60 + 30, Officials: []Duty{{Role: RoleExecution, Person: "k"}}},
	}}
	_, fills, _ = across.Left(Leave{Person: "c", Day: 0, From: 9 * 60}, nil, []RotaPerson{judge("c", true), judge("e2", false), judge("k", true), judge("j", false)})
	if got := describe(fills[0]); got != "K moves from execution judge on L on P2 to chair of judges; J takes execution judge on L on P2" {
		t.Errorf("moving across: %q", got)
	}

	// No one at all: the seat's left empty.
	_, fills, _ = s.Left(Leave{Person: "c", Day: 0, From: 9 * 60}, nil, []RotaPerson{judge("c", true), helper("h")})
	if fills[0].Empty != RoleChair || len(fills[0].Steps) != 0 {
		t.Errorf("an empty seat: %+v", fills[0])
	}
	if _, _, err := s.Left(Leave{Day: 0}, nil, officials); err == nil {
		t.Error("someone has to leave")
	}
}

// A chair who moves to cover the leaver gives up all of their own event's
// flights still to come, so one person chairs them: not Pat keeping flight 1
// while Joan has flight 2, though Joan could have done both.
func TestOfficialLeaveKeepsAnEventsChairWhole(t *testing.T) {
	chairs := func(key string, events ...string) RotaPerson {
		p := RotaPerson{Key: key, Name: strings.ToUpper(key), Judge: map[string]bool{}, Execution: map[string]bool{}, Difficulty: map[string]bool{}, Chair: map[string]bool{}}
		for _, e := range events {
			p.Judge[e], p.Chair[e] = true, true
		}
		return p
	}
	officials := []RotaPerson{chairs("l", "L"), chairs("pat", "L", "D"), chairs("joan", "D")}
	seat := func(who string) []Duty { return []Duty{{Role: RoleChair, Person: who}} }
	s := Schedule{
		Setup: Setup{Areas: []Area{{Name: "P1"}, {Name: "P2"}}, Days: []Day{{Name: "Sat", Start: "09:00", End: "18:00"}}},
		Flights: []ScheduledFlight{
			{Flight: Flight{Event: "L"}, Area: "P1", Start: 11 * 60, End: 12 * 60, Officials: seat("l")},
			{Flight: Flight{Event: "D", Number: 1, Of: 2}, Area: "P2", Start: 9*60 + 30, End: 10*60 + 30, Officials: seat("pat")},
			{Flight: Flight{Event: "D", Number: 2, Of: 2}, Area: "P2", Start: 11*60 + 30, End: 12*60 + 30, Officials: seat("pat")},
		},
	}
	out, fills, err := s.Left(Leave{Person: "l", Day: 0, From: 9 * 60}, nil, officials)
	if err != nil {
		t.Fatal(err)
	}
	chair := func(i int) string { return out.Flights[i].Officials[0].Person }
	if chair(0) != "pat" || chair(1) != "joan" || chair(2) != "joan" {
		t.Errorf("chairs after: L %s, D flight 1 %s, D flight 2 %s; Joan should have both of D", chair(0), chair(1), chair(2))
	}
	if len(fills) != 1 || len(fills[0].Steps) != 2 {
		t.Fatalf("fills: %+v", fills)
	}

	// A seat from the second flight on is named for what it is.
	three := Schedule{
		Setup: s.Setup,
		Flights: []ScheduledFlight{
			{Flight: Flight{Event: "E", Number: 1, Of: 3}, Area: "P1", Start: 9 * 60, End: 10 * 60, Officials: seat("l")},
			{Flight: Flight{Event: "E", Number: 2, Of: 3}, Area: "P1", Start: 10 * 60, End: 11 * 60, Officials: seat("l")},
			{Flight: Flight{Event: "E", Number: 3, Of: 3}, Area: "P1", Start: 11 * 60, End: 12 * 60, Officials: seat("l")},
		},
	}
	out, fills, _ = three.Left(Leave{Person: "l", Day: 0, From: 10*60 + 15}, nil, []RotaPerson{chairs("l", "E"), chairs("pat", "E")})
	if len(fills) != 1 || fills[0].Flight != "E · from flight 2" {
		t.Errorf("the seat is named for the flights still to come: %+v", fills)
	}
	if got := out.Flights[0].Officials[0].Person; got != "l" {
		t.Errorf("flight 1 had ended: %s", got)
	}
}
