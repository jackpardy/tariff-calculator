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
// enters L3 on her own.
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
		return competitions.ScheduledFlight{Flight: competitions.Flight{Level: level, Entries: ids, Of: 1, Number: 1}, Discipline: competitions.Trampoline, Area: area, Start: start, End: end}
	}
	s := competitions.Schedule{Setup: setup, Planned: true, Flights: []competitions.ScheduledFlight{
		flight("L1", "Panel 1", 540, 560, "e1"), flight("L2", "Panel 2", 540, 560, "e2"), flight("L3", "Panel 1", 565, 585, "e3"),
	}}
	people, names := clashPeople(entries, same)
	problems := strings.Join(s.Problems(people, coachesOf(entries, same), names), "; ")
	if !strings.Contains(problems, "Ann coaches in L2 while competing in L1") {
		t.Errorf("Ann coaching while she competes is a problem: %s", problems)
	}
	if r := s.Report(people); len(r.ShortRest) != 1 || names[r.ShortRest[0].Person] != "Ann" || r.ShortRest[0].Minutes != 5 {
		t.Errorf("Ann has 5 minutes' rest between L1 and L3, as a member and on her own: %+v", r.ShortRest)
	}
	// Keyed apart, as before, nothing's wrong.
	apart, apartNames := clashPeople(entries, nil)
	if p := s.Problems(apart, coachesOf(entries, nil), apartNames); len(p) != 0 {
		t.Errorf("by key alone, no problem: %v", p)
	}
	if r := s.Report(apart); len(r.ShortRest) != 0 {
		t.Errorf("by key alone, no short rest: %+v", r.ShortRest)
	}

	// The planner keeps her apart from herself; by key alone it doesn't.
	order := []string{"L1", "L2", "L3"}
	planned := competitions.PlanSchedule(schedEntries(entries, same), order, setup, 1)
	if overlapping(planned, "e1", "e2") || overlapping(planned, "e1", "e3") || overlapping(planned, "e2", "e3") {
		t.Errorf("Ann's flights and the one she coaches are apart: %+v", planned.Flights)
	}
	if p := planned.Problems(people, coachesOf(entries, same), names); len(p) != 0 {
		t.Errorf("as planned, no problems: %v", p)
	}
	if blind := competitions.PlanSchedule(schedEntries(entries, nil), order, setup, 1); !overlapping(blind, "e1", "e2") {
		t.Errorf("by key alone, L1 and L2 go side by side: %+v", blind.Flights)
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
