package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

// l4Entry is the usual entry at BUCS L4 instead; a name makes it an
// individual's.
func l4Entry(name string) url.Values {
	v := bucsEntry(name)
	v.Set("level", "BUCS L4")
	v.Set("ex1Option", "builtin:bucs-l4-option-1")
	v.Set("ex2Option", "builtin:bucs-l4-second")
	return v
}

// setLimit sets an event's limit from the Settings page's form.
func setLimit(t *testing.T, h http.Handler, admin, event string, n string) {
	t.Helper()
	m := regexp.MustCompile(`name="(limit-\d+)"[^>]*aria-label="Limit for ` + regexp.QuoteMeta(event) + `"`).FindStringSubmatch(do(t, h, http.MethodGet, admin+"/settings", nil).Body.String())
	if m == nil {
		t.Fatalf("no limit for %s on the Settings page", event)
	}
	redirected(t, h, admin+"/limits", url.Values{m[1]: {n}})
}

// lateSetup is a competition with sign-off and BUCS L3 and L4, its
// timetable planned: Ann from UCD and Ivy on her own in BUCS L3, Bea on her
// own in BUCS L4, which takes one. It's the morning of the day, so late
// level changes can be asked for.
type lateSetup struct {
	s          *testServer
	admin, ivy string
	ucd        testClub
}

func lateCompetition(t *testing.T) lateSetup {
	t.Helper()
	s := newTestServer(t, atTime(dayBefore))
	form := newCompetition()
	form.Set("signoff", "1")
	form["level"] = append(form["level"], "builtin-level:bucs-l4")
	admin := created(t, s.h, form)
	ucd := aClub(t, s.h, admin, "UCD", true, "Ann")
	enter := pathIn(t, do(t, s.h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	ivy := withoutQuery(redirected(t, s.h, enter, bucsEntry("Ivy")))
	redirected(t, s.h, enter, l4Entry("Bea"))
	setLimit(t, s.h, admin, "BUCS L4", "1")
	redirected(t, s.h, admin+"/timetable/plan", url.Values{})
	redirected(t, s.h, admin+"/late/settings", url.Values{"late-level": {"1"}, "late-routines": {"1"}})
	s.clock.set(onTheDay(8, 0))
	return lateSetup{s: s, admin: admin, ivy: ivy, ucd: ucd}
}

// draftFlightOf is the draft timetable's flight holding an entry, by name;
// "" if none does.
func draftFlightOf(t *testing.T, st *store.Store, c store.Competition, entryID string) string {
	t.Helper()
	c, err := st.Competition(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range c.Timetable.Flights {
		if slices.Contains(f.Entries, entryID) {
			return f.Name()
		}
	}
	return ""
}

// entryOf is a competition's entry for a gymnast.
func entryOf(t *testing.T, entries []store.Entry, gymnast string) store.Entry {
	t.Helper()
	i := slices.IndexFunc(entries, func(e store.Entry) bool { return e.Entry.Gymnast == gymnast })
	if i < 0 {
		t.Fatalf("no entry for %s", gymnast)
	}
	return entries[i]
}

// What the organiser is told about a late change before deciding: the
// card, the sign-off (approved coach or not), the new event's limit, and the
// draft timetable; each line as it says it.
func TestLateChecks(t *testing.T) {
	ls := lateCompetition(t)
	ctx := context.Background()
	st, p := ls.s.st, ls.s.p
	c, err := st.CompetitionByAdmin(ctx, strings.TrimPrefix(ls.admin, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := st.Entries(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	ivy, ann := entryOf(t, entries, "Ivy"), entryOf(t, entries, "Ann")
	clubs, _ := st.CompetitionClubs(ctx, c.ID)
	coach, _, err := st.CreateCoach(ctx, clubs[0].ID, "Coach C")
	if err != nil {
		t.Fatal(err)
	}
	toL4 := ivy.Entry
	toL4.Level, toL4.Exercises[0].Option, toL4.Exercises[1].Option = "BUCS L4", "builtin:bucs-l4-option-1", "builtin:bucs-l4-second"
	level := store.LateRequest{Kind: competitions.LateLevel, Entry: toL4}
	signed := level
	signed.SignedAt, signed.SignedBy, signed.SignedCoach = onTheDay(8, 0), "Coach C", coach.ID
	routines := store.LateRequest{Kind: competitions.LateRoutines, Entry: ivy.Entry}
	toAG3 := ivy.Entry
	toAG3.Level = "FIG AG3 (17–21)"
	toAG3.Exercises = [2]competitions.Exercise{}
	ag3 := store.LateRequest{Kind: competitions.LateLevel, Entry: toAG3}

	approving := c
	approving.ApproveCoaches = true
	unplanned := c
	unplanned.Timetable = nil
	roomy := c
	roomy.Limits = map[string]int{"BUCS L4": 5}
	for _, tc := range []struct {
		what  string
		c     store.Competition
		req   store.LateRequest
		e     store.Entry
		want  []string // after the card's line
		place string   // the draft timetable's line starts so; "" for none
	}{
		{what: "not signed off, into a full event", c: c, req: level, e: ivy, want: []string{
			"Not signed off by a coach: accepted, it would need signing off.",
			"BUCS L4 is full (1 of 1): accepted, they'd be 1st on its waiting list, unless you let them in."},
			place: "In the draft timetable: into BUCS L4 (Panel 1, warm-up "},
		{what: "signed off, where coaches needn't be approved", c: c, req: signed, e: ann, want: []string{
			"Signed off by Coach C.",
			"BUCS L4 is full (1 of 1): accepted, they'd be 1st on its waiting list, unless you let them in."},
			place: "In the draft timetable: into BUCS L4"},
		{what: "signed off by a coach not approved", c: approving, req: signed, e: ann, want: []string{
			"Signed off by Coach C, who isn't an approved coach: accepted, it would need signing off again.",
			"BUCS L4 is full (1 of 1): accepted, they'd be 1st on its waiting list, unless you let them in."},
			place: "In the draft timetable: into BUCS L4"},
		{what: "an event with room", c: roomy, req: level, e: ivy, want: []string{
			"Not signed off by a coach: accepted, it would need signing off.",
			"BUCS L4 has room: 1 of 5 places taken."},
			place: "In the draft timetable: into BUCS L4"},
		{what: "no draft timetable", c: unplanned, req: level, e: ivy, want: []string{
			"Not signed off by a coach: accepted, it would need signing off.",
			"BUCS L4 is full (1 of 1): accepted, they'd be 1st on its waiting list, unless you let them in."}},
		{what: "no flight of the event yet", c: c, req: ag3, e: ivy, want: []string{
			"Not signed off by a coach: accepted, it would need signing off.",
			"The draft timetable has no flight of " + toAG3.Event() + " yet: accepted, they'd be out of the timetable until you place them."}},
		{what: "new routines: no limit or timetable to check", c: c, req: routines, e: ivy, want: []string{
			"Not signed off by a coach: accepted, it would need signing off."}},
	} {
		got := p.lateChecks(ctx, tc.c, tc.req, tc.e, entries)
		if len(got) == 0 || !strings.HasPrefix(got[0], "The new card ") {
			t.Errorf("%s: the card's line first: %q", tc.what, got)
			continue
		}
		rest := got[1:]
		if tc.place != "" {
			if len(rest) == 0 || !strings.HasPrefix(rest[len(rest)-1], tc.place) {
				t.Errorf("%s: the draft timetable's line: %q", tc.what, got)
				continue
			}
			rest = rest[:len(rest)-1]
		}
		if !slices.Equal(rest, tc.want) {
			t.Errorf("%s:\n got %q\nwant %q", tc.what, rest, tc.want)
		}
	}

	// Approved, the coach's sign-off stands.
	if err := st.SendCoaches(ctx, clubs[0].ID, c.ID, []string{coach.ID}); err != nil {
		t.Fatal(err)
	}
	if err := st.DecideCoach(ctx, c.ID, coach.ID, store.CoachApproved, "", false); err != nil {
		t.Fatal(err)
	}
	if got := p.lateChecks(ctx, approving, signed, ann, entries); !slices.Contains(got, "Signed off by Coach C.") {
		t.Errorf("an approved coach's sign-off: %q", got)
	}
}

// Accepting a level change after planning: into a full event, the gymnast
// waits and leaves the draft timetable; into one with room, they move to
// its flight in the draft, and the published timetable waits to be
// published again.
func TestAcceptLateAfterPlanning(t *testing.T) {
	ls := lateCompetition(t)
	h, admin, ctx := ls.s.h, ls.admin, context.Background()
	c, err := ls.s.st.CompetitionByAdmin(ctx, strings.TrimPrefix(admin, "/competitions/admin/"))
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := ls.s.st.Entries(ctx, c.ID)
	ivy, ann := entryOf(t, entries, "Ivy"), entryOf(t, entries, "Ann")
	if draftFlightOf(t, ls.s.st, c, ivy.ID) != "BUCS L3" || draftFlightOf(t, ls.s.st, c, ann.ID) != "BUCS L3" {
		t.Fatal("Ann and Ivy start in BUCS L3's flight")
	}
	accept := func() string {
		t.Helper()
		id := regexp.MustCompile(`/late/([^/"]+)/accept`).FindStringSubmatch(do(t, h, http.MethodGet, admin+"/late", nil).Body.String())
		if id == nil {
			t.Fatal("nothing to accept")
		}
		return noticeOf(t, redirected(t, h, admin+"/late/"+id[1]+"/accept", url.Values{}))
	}

	// Ivy into BUCS L4, which is full: she waits, out of the draft.
	redirected(t, h, ls.ivy+"/late", l4Entry("Ivy"))
	if notice := accept(); notice != "Accepted: Ivy is 1st on BUCS L4's waiting list." {
		t.Errorf("accepted onto the waiting list: %q", notice)
	}
	if f := draftFlightOf(t, ls.s.st, c, ivy.ID); f != "" {
		t.Errorf("waiting, Ivy is out of the draft timetable: %s", f)
	}

	// BUCS L4 takes three now; Ann moves into its flight.
	setLimit(t, h, admin, "BUCS L4", "3")
	ask := regexp.MustCompile(regexp.QuoteMeta(ls.ucd.page) + `/members/[^"]+/late\?discipline=`).FindString(do(t, h, http.MethodGet, ls.ucd.page, nil).Body.String())
	if ask == "" {
		t.Fatal("the comp sec can ask for Ann")
	}
	redirected(t, h, unescape(ask), l4Entry(""))
	if notice := accept(); notice != "Accepted: Ann is in BUCS L4. The draft timetable has them in BUCS L4: publish it when you're ready." {
		t.Errorf("accepted into the draft: %q", notice)
	}
	if f := draftFlightOf(t, ls.s.st, c, ann.ID); f != "BUCS L4" {
		t.Errorf("Ann's draft flight: %q", f)
	}
	if to := redirected(t, h, admin+"/late/nonsense/accept", url.Values{}); noticeOf(t, to) != "That request has been decided already." {
		t.Errorf("not a request waiting: %s", to)
	}
	if !historyHas(t, h, admin, "Organiser", "Accepted a late change") {
		t.Error("the history")
	}
}
