package competitions

import (
	"strings"
	"testing"
)

func TestOfficialLeaves(t *testing.T) {
	judge := func(key string, chair bool) RotaPerson {
		return RotaPerson{Key: key, Name: strings.ToUpper(key), Judge: map[string]bool{"L": true}, Chair: map[string]bool{"L": chair}}
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
			{Flight: Flight{Level: "L"}, Area: "P1", Day: 0, Start: 10 * 60, End: 11 * 60, Officials: panel},
			{Flight: Flight{Level: "L"}, Area: "P1", Day: 1, Start: 10 * 60, End: 11 * 60, Officials: panel},
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

	// The chair leaves: no one free can chair, so E1 moves up. Easiest to
	// fill, the gap moves on down to the recorder's seat, which two free
	// helpers could take: R moves up to judge, and H records.
	out, fills, err := s.Left(Leave{Person: "c", Day: 0, From: 9 * 60}, nil, officials)
	if err != nil {
		t.Fatal(err)
	}
	if len(fills) != 1 || describe(fills[0]) != "E1 moves from execution judge to chair of judges; R moves from recorder to execution judge; H takes recorder" {
		t.Fatalf("the chair's seat: %q", describe(fills[0]))
	}
	if got := out.Flights[0].Officials; got[0].Person != "e1" || got[1].Person != "r" || got[3].Person != "h" || out.Flights[1].Officials[0].Person != "c" {
		t.Errorf("the panel after: %+v; Sunday's is as it was", got)
	}
	// Fewest changes: E1 moves up and J judges in their place.
	_, fills, _ = s.Left(Leave{Person: "c", Day: 0, From: 9 * 60, Prefer: PreferFewest}, nil, officials)
	if got := describe(fills[0]); got != "E1 moves from execution judge to chair of judges; J takes execution judge" {
		t.Errorf("fewest changes for the chair: %q", got)
	}

	// An execution judge leaves. Easiest to fill: R moves from recorder up,
	// and one of two free helpers records. Fewest changes: J judges.
	_, fills, _ = s.Left(Leave{Person: "e2", Day: 0, From: 9 * 60}, nil, officials)
	if got := describe(fills[0]); got != "R moves from recorder to execution judge; H takes recorder" || len(fills[0].Others) != 1 {
		t.Errorf("easiest to fill: %q, others %v", got, fills[0].Others)
	}
	_, fills, _ = s.Left(Leave{Person: "e2", Day: 0, From: 9 * 60, Prefer: PreferFewest}, nil, officials)
	if got := describe(fills[0]); got != "J takes execution judge" {
		t.Errorf("fewest changes: %q", got)
	}

	// J competes then, so can't; for the rest of the competition, Sunday
	// too.
	busy := s
	busy.Flights = append(busy.Flights, ScheduledFlight{Flight: Flight{Level: "M", Entries: []string{"x"}}, Area: "P2", Day: 0, Start: 10*60 + 30, End: 11 * 60})
	_, fills, _ = busy.Left(Leave{Person: "e2", Day: 0, From: 9 * 60, ForGood: true, Prefer: PreferFewest}, map[string][]string{"x": {"j"}}, officials)
	if len(fills) != 2 || describe(fills[0]) != "R moves from recorder to execution judge; H takes recorder" || describe(fills[1]) != "J takes execution judge" {
		t.Errorf("J competing on Saturday; Sunday too: %q / %q", describe(fills[0]), describe(fills[1]))
	}

	// No one on P1's panel can chair, but K, judging on P2 at the same time,
	// can: K moves across, and J judges on P2 in their place.
	across := Schedule{Setup: s.Setup, Flights: []ScheduledFlight{
		{Flight: Flight{Level: "L"}, Area: "P1", Start: 10 * 60, End: 11 * 60, Officials: []Duty{{Role: RoleChair, Person: "c"}, {Role: RoleExecution, Person: "e2"}}},
		{Flight: Flight{Level: "L"}, Area: "P2", Start: 10*60 + 30, End: 11*60 + 30, Officials: []Duty{{Role: RoleExecution, Person: "k"}}},
	}}
	_, fills, _ = across.Left(Leave{Person: "c", Day: 0, From: 9 * 60, Prefer: PreferFewest}, nil, []RotaPerson{judge("c", true), judge("e2", false), judge("k", true), judge("j", false)})
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
