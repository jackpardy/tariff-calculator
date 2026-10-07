package competitions

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// simCompetition offers BUCS L7 and L6, synchro BUCS L6 and tumbling Novice.
func simCompetition() Competition {
	return Competition{
		Name: "Student Open", Date: "2027-03-13", Deadline: time.Date(2027, 3, 6, 23, 59, 0, 0, time.UTC),
		Levels:   []Level{{Ref: "builtin-level:bucs-l7"}, {Ref: "builtin-level:bucs-l6"}},
		Synchro:  []Level{{Ref: "builtin-level:bucs-l6"}},
		Tumbling: []string{"Novice"},
		Officials: OfficialSettings{Panels: map[string]Panel{
			Trampoline: {Chair: 1, Execution: 2},
			Synchro:    {Chair: 1, Execution: 2},
			Tumbling:   {Chair: 1, Execution: 1, Recorder: 1},
		}},
	}
}

func TestSimulate(t *testing.T) {
	c := simCompetition()
	setup := venue()
	sc := Scenario{
		Name:     "Expected",
		Entries:  map[string]int{"BUCS L7": 10, "BUCS L6": 8, "Synchro BUCS L6": 3, "Tumbling Novice": 6},
		Gymnasts: 26, Clubs: 4,
		Judges:    map[string]int{Trampoline: 6, Synchro: 3, Tumbling: 2},
		Chairs:    map[string]int{Trampoline: 2, Synchro: 1, Tumbling: 1},
		Competing: 3, Helpers: 2,
	}
	r, err := c.Simulate(sc, setup, 1)
	if err != nil {
		t.Fatal(err)
	}
	// 18 + 6 + 3 pairs is 30 places for 26 gymnasts: 4 enter two disciplines.
	if r.Entries != 27 || r.Gymnasts != 26 {
		t.Errorf("%d entries and %d gymnasts, want 27 and 26", r.Entries, r.Gymnasts)
	}
	if len(r.Unplaced) > 0 || r.Fix != "" || r.NoFix || len(r.Days) != 1 || r.Days[0].SpareMinutes <= 0 {
		t.Errorf("everything fits: %+v", r)
	}
	if r.Seats != 12 {
		t.Errorf("%d seats, want 12: 3 for each of the 4 flights", r.Seats)
	}
	if r.Basis != c.SimBasis(setup) {
		t.Error("the result says what it was planned with")
	}
	if again, _ := c.Simulate(sc, setup, 1); !reflect.DeepEqual(again, r) {
		t.Error("the same seed plans the same")
	}
	none := sc
	none.Competing = 0
	if r, _ := c.Simulate(none, setup, 1); r.Empty != 0 {
		t.Errorf("with judges who don't compete, every seat is filled: %d of %d empty", r.Empty, r.Seats)
	}

	// The stand-ins: two disciplines each for 4, pairs of two, men and women
	// alternate where the timetable separates them, competing judges are
	// gymnasts.
	setup.Separate = Split{Mode: SplitAll}
	entries, officials := c.standIns(sc, setup)
	in := map[string]map[string]bool{}
	categories := map[string]int{}
	for _, e := range entries {
		if e.Discipline == Synchro && len(e.People) != 2 {
			t.Errorf("a synchro entry is a pair: %v", e.People)
		}
		for _, p := range e.People {
			if in[p] == nil {
				in[p] = map[string]bool{}
			}
			if in[p][e.Discipline] {
				t.Errorf("%s enters %s twice", p, e.Discipline)
			}
			in[p][e.Discipline] = true
		}
		categories[e.Category]++
	}
	two := 0
	for _, ds := range in {
		if len(ds) == 2 {
			two++
		}
	}
	if two != 4 {
		t.Errorf("%d gymnasts in two disciplines, want 4", two)
	}
	if categories["Men"] == 0 || categories["Women"] == 0 || categories[""] != 0 {
		t.Errorf("men and women: %v", categories)
	}
	gymnastJudges, chairs := 0, 0
	for _, o := range officials {
		if in[o.Key] != nil {
			gymnastJudges++
		}
		if o.Chair["BUCS L7"] {
			chairs++
		}
	}
	if len(officials) != 13 || gymnastJudges != 3 || chairs != 2 {
		t.Errorf("%d officials, %d of them gymnasts, %d trampoline chairs", len(officials), gymnastJudges, chairs)
	}

	// Fewer people than the disciplines' judges together: some judge two.
	shared := sc
	shared.JudgePeople = 8
	_, officials = c.standIns(shared, setup)
	judging := map[string]int{}
	for _, o := range officials[:8] {
		ds := map[string]bool{}
		for ev := range o.Judge {
			ds[map[string]string{"BUCS L7": Trampoline, "BUCS L6": Trampoline, "Synchro BUCS L6": Synchro, "Tumbling Novice": Tumbling}[ev]] = true
		}
		for d := range ds {
			judging[d]++
		}
	}
	if len(officials) != 10 || judging[Trampoline] != 6 || judging[Synchro] != 3 || judging[Tumbling] != 2 {
		t.Errorf("8 judges and 2 helpers, judging as many of each discipline as before: %d officials, %v", len(officials), judging)
	}

	// Too many for the day: what doesn't fit, and what would.
	sc.Entries = map[string]int{"BUCS L7": 120, "BUCS L6": 100, "Tumbling Novice": 40}
	sc.Gymnasts = 0
	sc.Judges[Trampoline], sc.Chairs[Trampoline] = 1, 1
	r, err = c.Simulate(sc, venue(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Unplaced) == 0 || r.Fix == "" && !r.NoFix {
		t.Errorf("what doesn't fit, and what would: %+v", r)
	}
	if r.Empty == 0 {
		t.Error("one trampoline judge leaves seats empty")
	}
}

func TestClubQuota(t *testing.T) {
	c := simCompetition()
	sc := Scenario{
		Name:    "Clubs bring judges",
		Entries: map[string]int{"BUCS L7": 10, "BUCS L6": 8, "Synchro BUCS L6": 3, "Tumbling Novice": 6},
		Clubs:   4,
		Quota: map[string]ClubQuota{
			Trampoline: {Per: 4, Judges: 1},            // 5, 5, 4 and 4 trampolinists: 2, 2, 1 and 1 judges
			Tumbling:   {Per: 8, Judges: 1, Chairs: 1}, // 2, 2, 1 and 1 tumblers: a chair each, who can also be a trampoline judge
		},
		Judges: map[string]int{Synchro: 3}, Chairs: map[string]int{Synchro: 1}, // the organiser's own
	}
	// One judge can count for several disciplines: a club brings as many
	// people as its biggest quota, the first of them judging tumbling too.
	_, officials := c.standIns(sc, venue())
	byClub := map[string]int{}
	trampoline, tumblingChairs, both, own := 0, 0, 0, 0
	for _, o := range officials {
		if o.Club == "" {
			own++
			if !o.Judge["Synchro BUCS L6"] || o.Judge["BUCS L7"] {
				t.Errorf("the organiser's judges judge synchro only: %+v", o)
			}
			continue
		}
		if o.Judge["BUCS L7"] && o.Judge["BUCS L6"] {
			trampoline++
			byClub[o.Club]++
		}
		if o.Chair["Tumbling Novice"] {
			tumblingChairs++
			if o.Judge["BUCS L7"] {
				both++
			}
		}
	}
	if own != 3 || trampoline != 6 || tumblingChairs != 4 || both != 4 || len(officials) != 9 {
		t.Errorf("%d own, %d trampoline judges and %d tumbling chairs from clubs (%d judging both), %d in all", own, trampoline, tumblingChairs, both, len(officials))
	}
	if byClub["Club 1"] != 2 || byClub["Club 2"] != 2 || byClub["Club 3"] != 1 || byClub["Club 4"] != 1 {
		t.Errorf("trampoline judges by club, rounding up: %v", byClub)
	}

	// Competing judges are gymnasts of their own club.
	sc.Competing = 3
	entries, officials := c.standIns(sc, venue())
	clubOf := map[string]string{}
	for _, e := range entries {
		for _, p := range e.People {
			clubOf[p] = e.Club
		}
	}
	competing := 0
	for _, o := range officials {
		if club, ok := clubOf[o.Key]; ok {
			competing++
			if o.Club != club || !strings.HasPrefix(club, "Club ") {
				t.Errorf("%s judges for %s but competes for %s", o.Key, o.Club, club)
			}
		}
	}
	if competing != 3 {
		t.Errorf("%d competing judges, want 3", competing)
	}

	r, err := c.Simulate(sc, venue(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if r.ClubJudges != 6 || r.Seats == 0 {
		t.Errorf("the clubs brought %d judges, want 6", r.ClubJudges)
	}
	for want, q := range map[string]map[string]ClubQuota{
		"doesn't offer":  {DMT: {Per: 8, Judges: 1}},
		"every 1 to 100": {Trampoline: {Per: 0, Judges: 1}},
		"no more chairs": {Trampoline: {Per: 8, Judges: 1, Chairs: 2}},
	} {
		bad := sc
		bad.Quota = q
		if err := c.CheckScenario(bad); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", want, err)
		}
	}
}

func TestCheckScenario(t *testing.T) {
	c := simCompetition()
	good := Scenario{Name: "A", Entries: map[string]int{"BUCS L7": 5}, Clubs: 2}
	if err := c.CheckScenario(good); err != nil {
		t.Fatal(err)
	}
	for want, bad := range map[string]Scenario{
		"no event":                  {Name: "A", Entries: map[string]int{"BUCS L1": 5}, Clubs: 2},
		"needs some entries":        {Name: "A", Entries: map[string]int{"BUCS L7": 0}, Clubs: 2},
		"up to 1000":                {Name: "A", Entries: map[string]int{"BUCS L7": 1001}, Clubs: 2},
		"gymnasts should be from 5": {Name: "A", Entries: map[string]int{"BUCS L7": 5}, Clubs: 2, Gymnasts: 6},
		"clubs":                     {Name: "A", Entries: map[string]int{"BUCS L7": 5}},
		"doesn't offer":             {Name: "A", Entries: map[string]int{"BUCS L7": 5}, Clubs: 2, Judges: map[string]int{DMT: 2}},
		"chairs more":               {Name: "A", Entries: map[string]int{"BUCS L7": 5}, Clubs: 2, Judges: map[string]int{Trampoline: 1}, Chairs: map[string]int{Trampoline: 2}},
		"competing judges":          {Name: "A", Entries: map[string]int{"BUCS L7": 5}, Clubs: 2, Competing: 1},
		"scenario":                  {Entries: map[string]int{"BUCS L7": 5}, Clubs: 2},
		"judges and helpers":        {Name: "A", Entries: map[string]int{"BUCS L7": 5}, Clubs: 2, Helpers: 501},
	} {
		if err := c.CheckScenario(bad); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", want, err)
		}
	}
	if _, err := c.Simulate(good, Setup{}, 1); err == nil {
		t.Error("a setup with no venue can't be simulated")
	}
	if SimName([]Scenario{{Name: "Scenario 2"}}) != "Scenario 3" || SimName([]Scenario{{Name: "x"}, {Name: "Scenario 3"}}) != "Scenario 4" {
		t.Error("new scenarios' names")
	}
}
