package competitions

import (
	"slices"
	"strings"
	"testing"
)

// Breaks lists everything a schedule breaks; NewBreaks what a change adds.
func TestBreaks(t *testing.T) {
	names := map[string]string{"ann": "Ann", "bob": "Bob", "cat": "Cat"}
	s := Schedule{
		Setup: Setup{
			Areas: []Area{{Name: "P1"}, {Name: "P2"}},
			Days:  []Day{{Name: "Sat", Start: "09:00", End: "18:00"}},
			Rules: []Rule{{Kind: RuleArea, Must: true, Event: "L1", Area: "P1"}, {Kind: RuleApart, Event: "L1", Event2: "L2"}},
			Rest:  30, RestMust: true,
		},
		Flights: []ScheduledFlight{
			{Flight: Flight{Event: "L1", Entries: []string{"e1"}}, Area: "P2", Day: 0, Start: 600, End: 660, Officials: []Duty{{Role: RoleChair, Person: "cat"}}},
			{Flight: Flight{Event: "L2", Entries: []string{"e2"}}, Area: "P1", Day: 0, Start: 630, End: 690},
			{Flight: Flight{Event: "L3", Entries: []string{"e3"}}, Area: "P1", Day: 0, Start: 680, End: 740},
		},
		Blocks: []ScheduledBlock{{Name: "Judges' meeting", Day: 0, Areas: []string{"P1"}, Start: 620, End: 640, Officials: []Duty{{Role: RoleChair, Person: "cat"}}}},
	}
	people := map[string][]string{"e1": {"ann"}, "e2": {"bob"}, "e3": {"ann"}}
	got := strings.Join(BreakLines(s.Breaks(people, names)), "\n")
	for _, want := range []string{
		"Breaks the rule: L1 on P1 (must)",
		"Breaks the rule: L1 and L2 not at the same time (prefer)",
		"Cat officiates L1 during Judges' meeting, which they're on duty for",
		"Ann has 20 minutes between L1 and L3 (rest is a must: 30 min)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Moving L1 to P1 at 9:00 mends the rule, the apart, the duty and the
	// rest; nothing new.
	mended := s
	mended.Flights = slices.Clone(s.Flights)
	mended.Flights[0].Area, mended.Flights[0].Start, mended.Flights[0].End = "P1", 540, 600
	if added := NewBreaks(s.Breaks(people, names), mended.Breaks(people, names)); len(added) != 0 {
		t.Errorf("nothing new: %v", added)
	}
}

// A delay moves a flight only where the must rules let it; an official
// leaving isn't replaced by someone on duty for blocked time or short of
// rest where it's a must.
func TestWhatIfsKeepTheRules(t *testing.T) {
	s := Schedule{
		Setup: Setup{
			Areas: []Area{{Name: "P1"}, {Name: "P2"}, {Name: "P3"}},
			Days:  []Day{{Name: "Sat", Start: "09:00", End: "11:30"}},
			Rules: []Rule{{Kind: RuleArea, Must: true, Event: "L1", Area: "P3"}},
		},
		Flights: []ScheduledFlight{
			{Flight: Flight{Event: "L1"}, Area: "P1", Day: 0, Start: 600, End: 660},
			{Flight: Flight{Event: "L2"}, Area: "P2", Day: 0, Start: 600, End: 660},
		},
	}
	// Held up an hour on P1, L1 would run past 11:30: P2 is free from 11:00,
	// but only P3 is allowed.
	out, _, err := s.Delayed(Delay{Day: 0, Areas: []string{"P1"}, From: 600, Minutes: 60}, nil)
	if err != nil || out.Flights[0].Area != "P3" {
		t.Errorf("to P3, as the rule says: %+v %v", out.Flights[0], err)
	}

	judge := func(key string) RotaPerson {
		return RotaPerson{Key: key, Name: key, Judge: map[string]bool{"L1": true}}
	}
	officials := []RotaPerson{judge("gone"), judge("meeting"), judge("tired"), judge("free")}
	s.Setup.Rest, s.Setup.RestMust = 30, true
	s.Flights[0].Officials = []Duty{{Role: RoleExecution, Person: "gone"}}
	s.Flights[1].Entries = []string{"e"}
	s.Flights = append(s.Flights, ScheduledFlight{Flight: Flight{Event: "L3", Entries: []string{"t"}}, Area: "P2", Day: 0, Start: 670, End: 700})
	s.Blocks = []ScheduledBlock{{Name: "Meeting", Day: 0, Areas: []string{"P2"}, Start: 610, End: 620, Officials: []Duty{{Role: RoleChair, Person: "meeting"}}}}
	people := map[string][]string{"t": {"tired"}}
	for _, ruled := range [][]RotaPerson{officials[:3], officials} {
		_, fills, _ := s.Left(Leave{Person: "gone", Day: 0, From: 540}, people, ruled)
		want := ""
		if len(ruled) == len(officials) {
			want = "free"
		}
		if got := fills[0]; len(got.Steps) > 0 && got.Steps[0].Person != want || len(got.Steps) == 0 && want != "" {
			t.Errorf("with %d officials, %q takes the seat: %+v", len(ruled), want, got)
		}
	}
}

// BreakLines puts people in two places first, and folds one person's
// repeats into a line with how many more.
func TestBreakLinesOrder(t *testing.T) {
	bs := []Break{
		{Line: "Breaks the rule: L1 on P1 (prefer)", Rank: PreferBreak},
		{Line: "Ann has 5 minutes between L1 and L2 (rest is a must: 30 min)", Person: "ann", Rank: MustBreak},
		{Line: "Bob is in L1 and L2 at once", Person: "bob", Rank: TwoPlaces},
		{Line: "Bob is in L1 and L3 at once", Person: "bob", Rank: TwoPlaces},
		{Line: "Bob is in L2 and L3 at once", Person: "bob", Rank: TwoPlaces},
		{Line: "Cat officiates L1 and L2 at once", Person: "cat", Rank: TwoPlaces},
		{Line: "L4 runs past the end of Sat (18:00)", Rank: MustBreak},
		{Line: "L5 runs past the end of Sat (18:00)", Rank: MustBreak},
	}
	want := []string{
		"Bob is in L1 and L2 at once, and 2 more times",
		"Cat officiates L1 and L2 at once",
		"Ann has 5 minutes between L1 and L2 (rest is a must: 30 min)",
		"L4 runs past the end of Sat (18:00)",
		"L5 runs past the end of Sat (18:00)",
		"Breaks the rule: L1 on P1 (prefer)",
	}
	if got := BreakLines(bs); !slices.Equal(got, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Of those free for a seat, the least busy that day comes first, then
// someone who's had the role already, then the least busy overall; not
// whoever's first by name.
func TestFreeForSeatOrder(t *testing.T) {
	judge := func(key string) RotaPerson {
		return RotaPerson{Key: key, Name: key, Judge: map[string]bool{"L": true}, Chair: map[string]bool{"L": true}}
	}
	officials := []RotaPerson{judge("ann"), judge("bob"), judge("cat"), judge("dan"), judge("eve")}
	flight := func(day, start int, duties ...Duty) ScheduledFlight {
		return ScheduledFlight{Flight: Flight{Event: "L"}, Area: "P1", Day: day, Start: start, End: start + 30, Officials: duties}
	}
	s := Schedule{
		Setup: Setup{Areas: []Area{{Name: "P1"}}, Days: []Day{{Name: "Sat", Start: "09:00", End: "18:00"}, {Name: "Sun", Start: "09:00", End: "18:00"}}},
		Flights: []ScheduledFlight{
			flight(0, 600, Duty{Role: RoleChair, Person: "zed"}),
			flight(0, 540, Duty{Role: RoleChair, Person: "ann"}),     // Ann's busy on Saturday
			flight(1, 540, Duty{Role: RoleExecution, Person: "bob"}), // Bob has judged, on Sunday
			flight(1, 600, Duty{Role: RoleChair, Person: "dan"}),     // Dan has chaired twice, on Sunday
			flight(1, 660, Duty{Role: RoleChair, Person: "dan"}),
			flight(1, 720, Duty{Role: RoleChair, Person: "eve"}), // Eve has chaired once
		},
	}
	got := s.FreeForSeat(0, 0, nil, officials)
	if want := []string{"eve", "dan", "cat", "bob", "ann"}; !slices.Equal(got, want) {
		t.Errorf("free for the chair: %v, want %v", got, want)
	}
}
