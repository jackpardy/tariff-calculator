package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"tariffCalculator/competitions"
)

// copied is a competition with people in it, as the copy tests make it.
type copied struct {
	comp                Competition
	admin               string
	club                Club
	clubAdmin           string
	ann, bea            Member
	annLink, coachLink  string
	coach               Coach
	individual, entries []Entry
	individualLink      string
}

// withPeople makes a competition over two days with everything a copy might
// take: a club's entries and an individual's, offers and an official added,
// a coach sent, a payment, a timetable with a rule about an official, and
// what's only the original's: a helper link, history, a concern, a check-in,
// a flight started and someone subscribed.
func withPeople(t *testing.T, s *Store) copied {
	t.Helper()
	var x copied
	comp := competition()
	comp.Signoff = true
	comp.Officials = competitions.OfficialSettings{Judge: competitions.JudgeQualified, Note: "Not Ann Ryan please"}
	comp.Fees = competitions.Fees{PerEntry: map[string]int{"": 1200}, Instructions: "Pay Mary Byrne, IBAN IE00", ChargeRemoved: true, RemovedFee: 500}
	comp.Limits = map[string]int{"BUCS L3": 40}
	comp.Late = competitions.LateChanges{Rules: map[string]competitions.LateRule{competitions.LateLevel: {On: true, Fee: 300}}}
	var err error
	x.comp, x.admin, err = s.CreateCompetition(ctx, comp)
	must(t, err)
	must(t, s.SetClubsSend(ctx, x.comp.ID, true))
	must(t, s.SetOfficialSettings(ctx, x.comp.ID, comp.Officials))
	must(t, s.SetFees(ctx, x.comp.ID, comp.Fees))
	must(t, s.SetLimits(ctx, x.comp.ID, comp.Limits))
	must(t, s.SetLate(ctx, x.comp.ID, comp.Late))
	x.club, x.clubAdmin, err = s.CreateClub(ctx, "UCD")
	must(t, err)
	x.coach, x.coachLink, err = s.CreateCoach(ctx, x.club.ID, "Cora Coach")
	must(t, err)
	x.ann, x.annLink, err = s.Join(ctx, x.club.ID, "Ann Ryan")
	must(t, err)
	x.bea, _, err = s.Join(ctx, x.club.ID, "Bea Byrne")
	must(t, err)
	must(t, s.SetMemberCoach(ctx, x.club.ID, x.ann.ID, x.coach.ID))
	must(t, s.AttachClub(ctx, x.club.ID, x.comp.ID))
	must(t, s.SaveMemberEntry(ctx, x.ann.ID, x.comp.ID, entry("Ann Ryan")))
	must(t, s.SaveMemberEntry(ctx, x.bea.ID, x.comp.ID, entry("Bea Byrne")))
	must(t, s.SignOff(ctx, x.coach, x.ann.ID, x.comp.ID, "", true, "fine"))
	must2(s.Send(ctx, x.club.ID, x.comp.ID, nil))
	must(t, s.SendCoaches(ctx, x.club.ID, x.comp.ID, []string{x.coach.ID}))
	must(t, s.SaveMemberOffer(ctx, x.bea.ID, x.comp.ID, competitions.Offer{Judge: map[string]competitions.JudgeOffer{"": {}}}))
	_, err = s.AddOfficial(ctx, x.comp.ID, "Olive Official", "DCU", competitions.Offer{Recorder: true})
	must(t, err)
	var ind Entry
	ind, x.individualLink, err = enter(s, x.comp.ID, entry("Dee Doyle"))
	must(t, err)
	x.individual = []Entry{ind}
	must(t, s.AddPayment(ctx, x.comp.ID, "club:"+x.club.ID, 2400, "by transfer", "Organiser"))
	x.entries, err = s.Entries(ctx, x.comp.ID)
	must(t, err)
	if len(x.entries) != 3 {
		t.Fatalf("entries: %+v", x.entries)
	}
	var ids, keys []string
	for _, e := range x.entries {
		ids = append(ids, e.ID)
		keys = append(keys, e.Key())
	}
	setup := competitions.DefaultSetup([]string{""})
	setup.Days = []competitions.Day{{Name: "Saturday", Start: "09:00", End: "18:00"}, {Name: "Sunday", Start: "09:00", End: "17:00"}}
	setup.Rules = []competitions.Rule{{Kind: competitions.RuleArea, Event: "BUCS L3", Area: setup.Areas[0].Name},
		{Kind: competitions.RuleOff, Person: MemberKey(x.bea.ID), Name: "Bea Byrne"}}
	setup.Rest = 20
	plan := competitions.Schedule{Setup: setup, Planned: true, Flights: []competitions.ScheduledFlight{{ID: "f1",
		Flight: competitions.Flight{Event: "BUCS L3", Number: 1, Of: 1, Entries: ids}, Area: setup.Areas[0].Name, Start: 540, End: 600,
		Officials: []competitions.Duty{{Person: keys[0], Role: competitions.RoleRecorder}}}}}
	must(t, s.SetTimetable(ctx, x.comp.ID, &plan))
	must(t, s.PublishTimetable(ctx, x.comp.ID, &plan))
	_, _, err = s.AddLink(ctx, x.comp.ID, "Desk", LinkChair)
	must(t, err)
	must(t, s.Record(ctx, x.comp.ID, "Organiser", "Went live"))
	_, err = s.FlagConcern(ctx, x.comp.ID, ids[0], "Desk", "Check her routine")
	must(t, err)
	must(t, s.SetCheckin(ctx, x.comp.ID, ids[0], "here", "Desk"))
	must(t, s.MarkFlight(ctx, x.comp.ID, "f1", "start", "Desk"))
	_, _, err = s.Subscribe(ctx, Subscription{CompetitionID: x.comp.ID, Kind: NotifyMember, OwnerID: x.ann.ID, Channel: ByPush, Address: "https://push.example/1", Keys: "{}"})
	must(t, err)
	x.comp, err = s.Competition(ctx, x.comp.ID)
	must(t, err)
	return x
}

// peopleTables lists every table holding a competition's people, or
// anything about them, by how its rows say which competition they're for.
var aboutPeople = map[string]string{
	"entries": "competition_id", "member_entries": "competition_id", "member_offers": "competition_id", "officials": "competition_id",
	"club_competitions": "competition_id", "competition_coaches": "competition_id", "competition_links": "competition_id",
	"competition_log": "competition_id", "concerns": "competition_id", "late_requests": "competition_id", "payments": "competition_id",
	"notify_subscriptions": "competition_id", "desk_messages": "competition_id", "flight_times": "competition_id",
	"checkins": "competition_id", "scratch_clears": "competition_id", "official_checkins": "competition_id",
	"up_now": "competition_id", "screen_notices": "competition_id",
}

// counts are how many rows of each table in aboutPeople are a competition's.
func counts(t *testing.T, s *Store, id string) map[string]int {
	t.Helper()
	out := map[string]int{}
	for table, col := range aboutPeople {
		var n int
		must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE `+col+` = $1`, id).Scan(&n))
		out[table] = n
	}
	return out
}

// dump is everything in the database about a competition, by table, to
// compare before and after.
func dump(t *testing.T, s *Store, id string) string {
	t.Helper()
	var b strings.Builder
	tables := append([]string{"competitions"}, slices.Sorted(func(yield func(string) bool) {
		for k := range aboutPeople {
			if !yield(k) {
				return
			}
		}
	})...)
	for _, table := range tables {
		col := aboutPeople[table]
		if table == "competitions" {
			col = "id"
		}
		rows, err := s.db.QueryContext(ctx, `SELECT * FROM `+table+` WHERE `+col+` = $1 ORDER BY 1, 2`, id)
		must(t, err)
		cols, _ := rows.Columns()
		for rows.Next() {
			v := make([]any, len(cols))
			p := make([]any, len(cols))
			for i := range v {
				p[i] = &v[i]
			}
			must(t, rows.Scan(p...))
			fmt.Fprintf(&b, "%s %v\n", table, v)
		}
		rows.Close()
	}
	for _, q := range []string{`SELECT * FROM clubs WHERE id IN (SELECT club_id FROM club_competitions WHERE competition_id = $1)`,
		`SELECT * FROM members WHERE club_id IN (SELECT club_id FROM club_competitions WHERE competition_id = $1)`,
		`SELECT * FROM coaches WHERE club_id IN (SELECT club_id FROM club_competitions WHERE competition_id = $1)`,
		`SELECT * FROM individuals WHERE id IN (SELECT individual_id FROM entries WHERE competition_id = $1)`} {
		rows, err := s.db.QueryContext(ctx, q+` ORDER BY 1`, id)
		must(t, err)
		cols, _ := rows.Columns()
		for rows.Next() {
			v := make([]any, len(cols))
			p := make([]any, len(cols))
			for i := range v {
				p[i] = &v[i]
			}
			must(t, rows.Scan(p...))
			fmt.Fprintf(&b, "%v\n", v)
		}
		rows.Close()
	}
	return b.String()
}

// A plain copy has the set-up, a new name, date and admin link, no
// deadlines, starts private, and holds nobody.
func TestCopySetup(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	x := withPeople(t, s)
	before := dump(t, s, x.comp.ID)
	c, admin, err := s.CopyCompetition(ctx, x.comp.ID, "Student Open 2028", "2028-03-11", false)
	must(t, err)
	if got, err := s.CompetitionByAdmin(ctx, admin); err != nil || got.ID != c.ID {
		t.Fatalf("its own admin link opens it: %v", err)
	}
	if c.ID == x.comp.ID || c.Name != "Student Open 2028" || c.Date != "2028-03-11" || c.MadeAs != MadeCopy || c.CopiedFrom != "Student Open" {
		t.Errorf("new name, date, made as a copy: %+v", c)
	}
	if !c.Deadline.IsZero() || !c.LiveAt.IsZero() || !c.ChangesUntil.IsZero() || !c.SignoffsUntil.IsZero() {
		t.Errorf("no deadlines, private: %+v", c.Competition)
	}
	if want := (competitions.Competition{Date: "2028-03-11"}).DeleteAfter(); !c.DeletesAt.Equal(want) {
		t.Errorf("deleted as any competition: %v, want %v", c.DeletesAt, want)
	}
	o := x.comp
	if !c.Signoff || !c.ClubsSend || !c.Individuals || c.Officials.Judge != competitions.JudgeQualified || c.Officials.Note != o.Officials.Note ||
		c.Fees.Instructions != o.Fees.Instructions || c.Fees.RemovedFee != 500 || !c.Fees.ChargeRemoved || c.Limits["BUCS L3"] != 40 ||
		!c.Late.Allowed(competitions.LateLevel) || c.Late.Rule(competitions.LateLevel).Fee != 300 || len(c.Levels) != 1 || c.Messages != o.Messages {
		t.Errorf("every setting copied: %+v", c.Competition)
	}
	if c.Timetable == nil || len(c.Timetable.Flights) != 0 || c.Published != nil {
		t.Fatalf("the timetable's Setup and no flights: %+v %+v", c.Timetable, c.Published)
	}
	setup := c.Timetable.Setup
	if len(setup.Days) != 2 || setup.Days[0].Name != "Saturday" || setup.Days[1].Name != "Sunday" || setup.Rest != 20 {
		t.Errorf("its days, named for the new date's weekdays (Saturday 11 March 2028): %+v", setup)
	}
	if len(setup.Rules) != 1 || setup.Rules[0].Person != "" {
		t.Errorf("the rules, but not the one about an official: %+v", setup.Rules)
	}
	for table, n := range counts(t, s, c.ID) {
		if n != 0 {
			t.Errorf("a plain copy has nobody: %d rows of %s", n, table)
		}
	}
	if got, _ := s.ClubCompetitions(ctx, x.club.ID); len(got) != 1 {
		t.Errorf("the club is entered only in the original: %d", len(got))
	}
	if after := dump(t, s, x.comp.ID); after != before {
		t.Errorf("the original isn't changed")
	}
	if _, err := s.CompetitionByClubLink(ctx, c.ClubLink); err != nil {
		t.Errorf("a plain copy's club link works, for when it goes live: %v", err)
	}
}

// A rehearsal copy has the same people as the original, rows of its own
// with new ids and no working link, sends to nobody (nobody subscribed),
// and changing it leaves the original as it was.
func TestCopyRehearsal(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	x := withPeople(t, s)
	before := dump(t, s, x.comp.ID)
	start := time.Now()
	c, admin, err := s.CopyCompetition(ctx, x.comp.ID, "Student Open (rehearsal)", "2027-01-16", true)
	must(t, err)
	t.Logf("copied in %v", time.Since(start))
	if c.MadeAs != MadeRehearsal || !c.Rehearsal() || !c.DeletesAt.Equal(now.AddDate(0, 0, RehearsalDays)) {
		t.Errorf("a rehearsal copy, deleted in 30 days: %+v %v", c.MadeAs, c.DeletesAt)
	}
	want, got := counts(t, s, x.comp.ID), counts(t, s, c.ID)
	for _, table := range []string{"entries", "member_entries", "member_offers", "officials", "club_competitions", "competition_coaches", "payments"} {
		if got[table] != want[table] || want[table] == 0 {
			t.Errorf("%s: %d in the copy, %d in the original", table, got[table], want[table])
		}
	}
	for _, table := range []string{"competition_links", "competition_log", "concerns", "late_requests", "notify_subscriptions", "desk_messages",
		"flight_times", "checkins", "scratch_clears", "official_checkins", "up_now", "screen_notices"} {
		if got[table] != 0 {
			t.Errorf("%s isn't copied: %d", table, got[table])
		}
	}
	entries, err := s.Entries(ctx, c.ID)
	must(t, err)
	if len(entries) != 3 {
		t.Fatalf("its entries: %+v", entries)
	}
	old := map[string]bool{x.comp.ID: true, x.club.ID: true, x.ann.ID: true, x.bea.ID: true, x.coach.ID: true, x.individual[0].IndividualID: true}
	for _, e := range x.entries {
		old[e.ID] = true
	}
	signed := 0
	for _, e := range entries {
		for _, id := range []string{e.ID, e.ClubID, e.MemberID, e.IndividualID, e.SignedCoach, e.CoachID} {
			if old[id] {
				t.Errorf("an id of the original's in the copy's entry %s: %s", e.Entry.Gymnast, id)
			}
		}
		if e.SignedOff() {
			signed++
		}
	}
	if signed != 1 {
		t.Errorf("its sign-off copied: %d", signed)
	}
	// No hash or token of the original's anywhere in the copy, and no id.
	all := dump(t, s, c.ID)
	var secrets []string
	for _, q := range []string{
		`SELECT admin_hash || ' ' || club_token || ' ' || club_hash || ' ' || individual_token || ' ' || individual_hash FROM competitions WHERE id = $1`,
		`SELECT admin_hash || ' ' || join_token || ' ' || join_hash FROM clubs WHERE rehearsal_id IS NULL`,
		`SELECT token_hash FROM members WHERE club_id IN (SELECT id FROM clubs WHERE rehearsal_id IS NULL)`,
		`SELECT token_hash FROM coaches WHERE club_id IN (SELECT id FROM clubs WHERE rehearsal_id IS NULL)`,
		`SELECT COALESCE(token_hash, '') FROM individuals WHERE rehearsal_id IS NULL`,
		`SELECT COALESCE(token_hash, '') || ' ' || COALESCE(signoff_token, '') || ' ' || COALESCE(signoff_hash, '') || ' ' || COALESCE(partner_token, '') || ' ' || COALESCE(partner_hash, '') FROM entries WHERE competition_id = $1`,
		`SELECT COALESCE(partner_token, '') || ' ' || COALESCE(partner_hash, '') FROM member_entries WHERE competition_id = $1`,
		`SELECT token_hash FROM competition_links WHERE competition_id = $1`,
	} {
		rows, err := s.db.QueryContext(ctx, q, x.comp.ID)
		must(t, err)
		for rows.Next() {
			var h string
			must(t, rows.Scan(&h))
			secrets = append(secrets, strings.Fields(h)...)
		}
		rows.Close()
	}
	if len(secrets) < 10 {
		t.Fatalf("the original's links: %v", secrets)
	}
	for _, h := range secrets {
		if strings.Contains(all, h) {
			t.Errorf("a hash or token of the original's is in the copy: %s", h)
		}
	}
	for id := range old {
		if strings.Contains(all, id) {
			t.Errorf("an id of the original's is in the copy: %s", id)
		}
	}
	// Its people are rows of its own, with no working link.
	var clubs, members, individuals int
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM clubs WHERE rehearsal_id = $1`, c.ID).Scan(&clubs))
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM members WHERE club_id IN (SELECT id FROM clubs WHERE rehearsal_id = $1)`, c.ID).Scan(&members))
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM individuals WHERE rehearsal_id = $1`, c.ID).Scan(&individuals))
	if clubs != 1 || members != 2 || individuals != 1 {
		t.Errorf("its own club, members and individual: %d %d %d", clubs, members, individuals)
	}
	if _, err := s.CompetitionByClubLink(ctx, c.ClubLink); !errors.Is(err, ErrNotFound) {
		t.Errorf("its club link opens nothing: %v", err)
	}
	if _, err := s.CompetitionByIndividualLink(ctx, c.IndividualLink); !errors.Is(err, ErrNotFound) {
		t.Errorf("its individual link opens nothing: %v", err)
	}
	if got, _ := s.ClubCompetitions(ctx, x.club.ID); len(got) != 1 || got[0].ID != x.comp.ID {
		t.Errorf("the club sees only the original: %+v", got)
	}
	if got, _ := s.IndividualEntries(ctx, x.individual[0].IndividualID); len(got) != 1 || got[0].CompetitionID != x.comp.ID {
		t.Errorf("the individual sees only the original: %+v", got)
	}
	if got, _ := s.Subscriptions(ctx, c.ID, NotifyMember, x.ann.ID); len(got) != 0 {
		t.Errorf("nobody is subscribed to the copy: %+v", got)
	}
	// Its timetable is the original's, with the copy's people.
	if c.Timetable == nil || len(c.Timetable.Flights) != 1 || c.Published == nil {
		t.Fatalf("both timetables copied: %+v", c.Timetable)
	}
	ids := map[string]bool{}
	for _, e := range entries {
		ids[e.ID] = true
	}
	for _, id := range c.Timetable.Flights[0].Entries {
		if !ids[id] {
			t.Errorf("the flight holds the copy's entries: %s", id)
		}
	}
	if k := c.Timetable.Flights[0].Officials[0].Person; !slices.ContainsFunc(entries, func(e Entry) bool { return e.Key() == k }) {
		t.Errorf("the seat is the copy's person: %s", k)
	}
	if c.Timetable.Setup.Days[0].Name != "Saturday" {
		t.Errorf("days named for the new date: %+v", c.Timetable.Setup.Days)
	}

	// Changing the copy: a check-in, a flight started and moved, an entry
	// removed. The original is as it was.
	must(t, s.SetCheckin(ctx, c.ID, entries[0].ID, "here", "Desk"))
	must(t, s.MarkFlight(ctx, c.ID, "f1", "start", "Desk"))
	must(t, s.UpdateTimetable(ctx, c.ID, func(sc *competitions.Schedule) error {
		sc.Flights[0].Start += 30
		return nil
	}))
	must2(s.RemoveEntries(ctx, c.ID, []string{entries[1].ID}, Removed, "", "Organiser"))
	if after := dump(t, s, x.comp.ID); after != before {
		t.Errorf("the original isn't changed by the copy:\nbefore\n%s\nafter\n%s", before, after)
	}
	if got, _ := s.Checkins(ctx, x.comp.ID); len(got) != 1 {
		t.Errorf("the original's check-ins are its own: %+v", got)
	}
	if got, err := s.CompetitionByAdmin(ctx, admin); err != nil || got.ID != c.ID {
		t.Errorf("its admin link: %v", err)
	}

	// Deleting it deletes its people, and nothing of the original's.
	must(t, s.DeleteCompetition(ctx, c.ID))
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM clubs WHERE rehearsal_id IS NOT NULL`).Scan(&clubs))
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM individuals WHERE rehearsal_id IS NOT NULL`).Scan(&individuals))
	if clubs != 0 || individuals != 0 {
		t.Errorf("its club and individual went with it: %d %d", clubs, individuals)
	}
	if after := dump(t, s, x.comp.ID); after != before {
		t.Errorf("the original isn't changed by deleting the copy")
	}
	if _, _, err := s.CopyCompetition(ctx, x.comp.ID, "Again", "2027-01-16", true); err != nil {
		t.Errorf("another can be made: %v", err)
	}
}

// A rehearsal copy is deleted 30 days after it's made, or 60 when kept
// longer, which can be done once.
func TestRehearsalExpires(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	x := withPeople(t, s)
	c, _, err := s.CopyCompetition(ctx, x.comp.ID, "Rehearsal", "2027-01-16", true)
	must(t, err)
	d, _, err := s.CopyCompetition(ctx, x.comp.ID, "Rehearsal 2", "2027-01-16", true)
	must(t, err)
	must(t, s.ExtendRehearsal(ctx, d.ID))
	if err := s.ExtendRehearsal(ctx, d.ID); !errors.Is(err, ErrExtended) {
		t.Errorf("once: %v", err)
	}
	if err := s.ExtendRehearsal(ctx, x.comp.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("only a rehearsal copy: %v", err)
	}
	now = now.AddDate(0, 0, RehearsalDays).Add(-time.Minute)
	expire(t, s)
	if _, err := s.Competition(ctx, c.ID); err != nil {
		t.Errorf("kept until 30 days: %v", err)
	}
	now = now.Add(time.Minute)
	expire(t, s)
	if _, err := s.Competition(ctx, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted at 30 days: %v", err)
	}
	if _, err := s.Competition(ctx, d.ID); err != nil {
		t.Errorf("the one kept longer is kept: %v", err)
	}
	now = now.AddDate(0, 0, RehearsalDays)
	expire(t, s)
	if _, err := s.Competition(ctx, d.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted at 60 days: %v", err)
	}
	var clubs int
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM clubs WHERE rehearsal_id IS NOT NULL`).Scan(&clubs))
	if clubs != 0 {
		t.Errorf("their clubs went with them: %d", clubs)
	}
	if _, err := s.Competition(ctx, x.comp.ID); err != nil {
		t.Errorf("the original stays: %v", err)
	}
}

// At 120 days a competition's people go and its set-up stays, with nothing
// about anyone; the set-up can be copied, and goes 3 years after the
// competition's date.
func TestSetupKept(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	x := withPeople(t, s)
	rehearsal, _, err := s.CopyCompetition(ctx, x.comp.ID, "Rehearsal", "2027-01-16", true)
	must(t, err)
	must(t, s.ExtendRehearsal(ctx, rehearsal.ID))
	must2(s.Tie(ctx, x.annLink, RoleOrganiser, x.admin))
	now = x.comp.DeleteAfter()
	comps, _, err := s.DeleteExpired(ctx)
	must(t, err)
	if comps != 2 {
		t.Errorf("the rehearsal copy deleted and the original's people: %d", comps)
	}
	c, err := s.CompetitionByAdmin(ctx, x.admin)
	if err != nil {
		t.Fatalf("its admin link opens the kept set-up: %v", err)
	}
	if !c.SetupKept() || !c.SetupKeptAt.Equal(now) || !c.DeletesAt.Equal(time.Date(2030, 3, 13, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("kept, until 3 years after its date: %v %v", c.SetupKeptAt, c.DeletesAt)
	}
	for table, n := range counts(t, s, c.ID) {
		if n != 0 {
			t.Errorf("nothing about its people is kept: %d rows of %s", n, table)
		}
	}
	if c.Officials.Note != "" || c.Fees.Instructions != "" || c.Published != nil || c.Timetable == nil || len(c.Timetable.Flights) != 0 {
		t.Errorf("no free text, no flights: %+v %+v %+v", c.Officials, c.Fees, c.Timetable)
	}
	for _, r := range c.Timetable.Setup.Rules {
		if r.Person != "" || r.Name != "" {
			t.Errorf("no rule about a person: %+v", r)
		}
	}
	if c.Fees.RemovedFee != 500 || c.Limits["BUCS L3"] != 40 || !c.Signoff || len(c.Timetable.Setup.Days) != 2 {
		t.Errorf("its set-up kept: %+v", c.Competition)
	}
	// Nothing about anyone in its row: every text column, read.
	var row string
	must(t, s.db.QueryRowContext(ctx, `SELECT name || date || levels || video || split || timetable || events || officials || coach_levels ||
		published_timetable || scenarios || notify_baseline || fees || limits || late FROM competitions WHERE id = $1`, c.ID).Scan(&row))
	for _, who := range []string{"Ann", "Bea", "Mary", "Cora", "Dee", "Olive", x.ann.ID, x.bea.ID} {
		if strings.Contains(row, who) {
			t.Errorf("%s is in the kept set-up: %s", who, row)
		}
	}
	if _, err := s.CompetitionByClubLink(ctx, x.comp.ClubLink); !errors.Is(err, ErrNotFound) {
		t.Errorf("its club link opens nothing: %v", err)
	}
	var ties int
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM shared_ties WHERE target_id = $1`, c.ID).Scan(&ties))
	if ties != 0 {
		t.Errorf("the admin link on someone's page goes: %d", ties)
	}
	if _, err := s.Competition(ctx, rehearsal.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a rehearsal copy is never kept: it goes with the original's people: %v", err)
	}
	// Copied from, as any set-up; not for a rehearsal.
	if _, _, err := s.CopyCompetition(ctx, c.ID, "Rehearsal", "2027-08-01", true); err == nil {
		t.Error("a kept set-up has no entries to rehearse")
	}
	next, _, err := s.CopyCompetition(ctx, c.ID, "Student Open 2028", "2028-03-11", false)
	must(t, err)
	if next.SetupKept() || next.Fees.RemovedFee != 500 || next.Timetable == nil {
		t.Errorf("the copy of a kept set-up: %+v", next)
	}
	now = time.Date(2030, 3, 13, 0, 0, 0, 0, time.UTC)
	expire(t, s)
	if _, err := s.Competition(ctx, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted 3 years after its date: %v", err)
	}
}

// Migration 57 adds its columns to a database with a competition in it,
// which is a competition made new, with its people.
func TestCopyMigration(t *testing.T) {
	dir := t.TempDir()
	i := slices.IndexFunc(migrations, func(m string) bool { return strings.Contains(m, "ADD COLUMN made_as") })
	if i < 0 {
		t.Fatal("no migration adds made_as")
	}
	all := migrations
	migrations = all[:i]
	old, err := Open(ctx, dir)
	if err != nil {
		migrations = all
		t.Fatal(err)
	}
	c, admin, err := old.CreateCompetition(ctx, competition())
	if err == nil {
		_, _, err = old.AddIndividualEntry(ctx, c.ID, "", entry("Dee"))
	}
	migrations = all
	must(t, err)
	s := reopen(t, old, dir)
	got, err := s.CompetitionByAdmin(ctx, admin)
	must(t, err)
	if got.MadeAs != MadeNew || got.SetupKept() || got.Rehearsal() || got.Extended || !got.DeletesAt.Equal(c.DeleteAfter()) {
		t.Errorf("made new, with its people: %+v", got)
	}
	if es, _ := s.Entries(ctx, c.ID); len(es) != 1 {
		t.Errorf("its entry: %+v", es)
	}
	if _, _, err := s.CopyCompetition(ctx, c.ID, "Copy", "2028-03-11", true); err != nil {
		t.Errorf("copied after: %v", err)
	}
	_ = json.Valid
}

// expire runs DeleteExpired.
func expire(t *testing.T, s *Store) {
	t.Helper()
	_, _, err := s.DeleteExpired(ctx)
	must(t, err)
}
