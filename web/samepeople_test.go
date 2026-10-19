package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

// memberEntryIn is a member's entry in an event, coached by a coach record.
func memberEntryIn(id, member, gymnast, level, coach string) store.Entry {
	return store.Entry{ID: id, ClubID: "club", ClubName: "Club", MemberID: member, CoachID: coach,
		Entry: competitions.Entry{Gymnast: gymnast, Discipline: competitions.Trampoline, Level: level}}
}

// overlapping says whether the flights holding two entries run at once.
func overlapping(s competitions.Schedule, a, b string) bool {
	fa, _ := s.Find(a)
	fb, _ := s.Find(b)
	x, y := s.Flights[fa], s.Flights[fb]
	return x.Day == y.Day && x.Start < y.End && y.Start < x.End
}

// One person's keys as a gymnast and as a coach are one person in the
// timetable (ADR 0010): Ann competes in L1 and coaches Bob in L2, and also
// enters L3 on her own. Coaching isn't checked (2026-10-19): only her own
// turns count.
func TestOnePersonAcrossRoles(t *testing.T) {
	ann := memberEntryIn("e1", "ann", "Ann", "L1", "")
	bob := memberEntryIn("e2", "bob", "Bob", "L2", "annCoach")
	own := store.Entry{ID: "e3", Individual: true, PersonKey: store.IndividualKey("annAlone"),
		Entry: competitions.Entry{Gymnast: "Ann", Discipline: competitions.Trampoline, Level: "L3"}}
	entries := []store.Entry{ann, bob, own}
	groups := map[string]string{
		store.MemberKey("ann"): "A", store.CoachKey("annCoach"): "A", store.IndividualKey("annAlone"): "A",
		store.MemberKey("bob"): "B",
	}
	same := newSamePeople(groups, entries, nil)
	if same.of(store.CoachKey("annCoach")) != store.MemberKey("ann") || same.of(store.IndividualKey("annAlone")) != store.MemberKey("ann") {
		t.Fatalf("Ann's keys stand for her member key: %v", same)
	}

	// Timetabled against herself: L1 and L2 on two panels at once, L3 just
	// after L1.
	setup := competitions.Setup{
		Areas: []competitions.Area{{Name: "Panel 1", Discipline: competitions.Trampoline}, {Name: "Panel 2", Discipline: competitions.Trampoline}},
		Days:  []competitions.Day{{Name: "Saturday", Start: "09:00", End: "18:00"}}, Rest: 30,
	}
	flight := func(level, area string, start, end int, ids ...string) competitions.ScheduledFlight {
		return competitions.ScheduledFlight{Flight: competitions.Flight{Event: level, Entries: ids, Of: 1, Number: 1}, Discipline: competitions.Trampoline, Area: area, Start: start, End: end}
	}
	s := competitions.Schedule{Setup: setup, Planned: true, Flights: []competitions.ScheduledFlight{
		flight("L1", "Panel 1", 540, 560, "e1"), flight("L2", "Panel 2", 540, 560, "e2"), flight("L3", "Panel 1", 565, 585, "e3"),
	}}
	people, names := clashPeople(entries, same)
	if p := s.Problems(people, names); len(p) != 0 {
		t.Errorf("Ann coaching Bob while she competes isn't a problem: %v", p)
	}
	if r := s.Report(people); len(r.ShortRest) != 1 || names[r.ShortRest[0].Person] != "Ann" || r.ShortRest[0].Minutes != 5 {
		t.Errorf("Ann has 5 minutes' rest between L1 and L3, as a member and on her own: %+v", r.ShortRest)
	}
	// Keyed apart, as before, nothing's wrong.
	apart, apartNames := clashPeople(entries, nil)
	if p := s.Problems(apart, apartNames); len(p) != 0 {
		t.Errorf("by key alone, no problem: %v", p)
	}
	if r := s.Report(apart); len(r.ShortRest) != 0 {
		t.Errorf("by key alone, no short rest: %+v", r.ShortRest)
	}

	// The planner keeps her two turns apart; by key alone it doesn't.
	order := []string{"L1", "L3", "L2"}
	planned := competitions.PlanSchedule(schedEntries(entries, same), order, setup, 1)
	if overlapping(planned, "e1", "e3") {
		t.Errorf("Ann's two flights are apart: %+v", planned.Flights)
	}
	if p := planned.Problems(people, names); len(p) != 0 {
		t.Errorf("as planned, no problems: %v", p)
	}
	if blind := competitions.PlanSchedule(schedEntries(entries, nil), order, setup, 1); !overlapping(blind, "e1", "e3") {
		t.Errorf("by key alone, L1 and L3 go side by side: %+v", blind.Flights)
	}
}

// A coach whose gymnasts are on two areas at once breaks nothing; a gymnast
// in two places does, and says so first.
func TestCoachesArentChecked(t *testing.T) {
	entries := []store.Entry{
		memberEntryIn("e1", "ann", "Ann", "L1", "sam"), memberEntryIn("e2", "bob", "Bob", "L2", "sam"),
		memberEntryIn("e3", "ann", "Ann", "L3", "sam"),
	}
	f := rotaFacts{entries: entries}
	f.one, f.oneNames = clashPeople(entries, nil)
	flight := func(level, area string, start int, ids ...string) competitions.ScheduledFlight {
		return competitions.ScheduledFlight{Flight: competitions.Flight{Event: level, Entries: ids, Of: 1, Number: 1}, Discipline: competitions.Trampoline, Area: area, Start: start, End: start + 20}
	}
	s := competitions.Schedule{Planned: true, Setup: competitions.Setup{
		Areas: []competitions.Area{{Name: "Panel 1"}, {Name: "Panel 2"}},
		Days:  []competitions.Day{{Name: "Saturday", Start: "09:00", End: "18:00"}},
		Rules: []competitions.Rule{{Kind: competitions.RuleArea, Event: "L2", Area: "Panel 1"}},
	}, Flights: []competitions.ScheduledFlight{flight("L1", "Panel 1", 540, "e1"), flight("L2", "Panel 2", 540, "e2"), flight("L3", "Panel 1", 600, "e3")}}
	if got := competitions.BreakLines(f.manualBreaks(s)); len(got) != 1 || !strings.HasPrefix(got[0], "Breaks the rule") {
		t.Errorf("Sam's gymnasts on two panels at once break nothing new: %v", got)
	}
	// L3 moved alongside L1: Ann's in two places, and that comes first.
	moved := s.Copy()
	moved.Flights[2].Area, moved.Flights[2].Start, moved.Flights[2].End = "Panel 2", 545, 565
	got := competitions.BreakLines(competitions.NewBreaks(f.manualBreaks(s), f.manualBreaks(moved)))
	if len(got) == 0 || got[0] != "Ann is in L1 and L3 at once" {
		t.Errorf("Ann in two places comes first: %v", got)
	}
	for _, l := range got {
		if strings.Contains(l, "Sam") || strings.Contains(l, "coach") {
			t.Errorf("no coach is checked: %q", l)
		}
	}
}

// A member who also enters on their own, as themselves, is one person: the
// planner doesn't put their two flights at once.
func TestMemberAlsoIndividual(t *testing.T) {
	h, _ := competitionServerAt(t, inDublin(2026, time.October, 15, 12, 0))
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	club := aClub(t, h, admin, "Bray", true, "Ann Byrne")
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, url.Values{"gymnast": {"Ann Byrne"}, "discipline": {"tumbling"}, "level": {"Novice"}, "as": {"http://example.com" + club.members["Ann Byrne"]}})
	page := do(t, h, http.MethodGet, redirected(t, h, admin+"/timetable/plan", url.Values{}), nil).Body.String()
	times := regexp.MustCompile(`Warm-up (\d\d:\d\d) · finishes about (\d\d:\d\d)`).FindAllStringSubmatch(page, -1)
	if len(times) != 2 {
		t.Fatalf("two flights: %v", times)
	}
	if !(times[0][2] <= times[1][1] || times[1][2] <= times[0][1]) {
		t.Errorf("Ann's two flights don't overlap: %v", times)
	}
	if strings.Contains(page, "Ann Byrne and Ann Byrne") {
		t.Error("one person's two keys don't look alike to each other")
	}
}
