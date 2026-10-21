package store

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// sandboxMigration is the migration that makes the sandbox's tables, by
// version (51 once club roles' 50 is before it).
func sandboxMigration(t *testing.T) int {
	t.Helper()
	i := slices.IndexFunc(migrations, func(m string) bool { return strings.Contains(m, "CREATE TABLE sandbox_copies") })
	if i < 0 {
		t.Fatal("no migration makes sandbox_copies")
	}
	return i + 1
}

// The sandbox's migration adds its tables to a database from just before
// it, keeping what's there.
func TestSandboxMigration(t *testing.T) {
	dir := t.TempDir()
	v := sandboxMigration(t)
	all := migrations
	migrations = all[:v-1]
	old, err := Open(ctx, dir)
	if err != nil {
		migrations = all
		t.Fatal(err)
	}
	c, _, err := old.CreateCompetition(ctx, competition())
	migrations = all
	must(t, err)
	s := reopen(t, old, dir)
	if _, err := s.Competition(ctx, c.ID); err != nil {
		t.Errorf("the competition is kept: %v", err)
	}
	id, err := s.SaveCopy(ctx, SandboxCopy{CreatedAt: time.Now(), Size: 25, Links: json.RawMessage(`{}`)}, []string{c.ID}, nil)
	must(t, err)
	if got, err := s.SharedCopy(ctx); err != nil || got.ID != id || got.Size != 25 {
		t.Errorf("the copy is kept: %+v, %v", got, err)
	}
}

// Deleting a copy deletes its competitions, clubs and individuals with
// everything they hold, someone who entered only its competitions, and the
// feedback its seed sent; nothing else.
func TestDeleteCopy(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	// What a copy would make, and the same outside it.
	type made struct {
		comp, club, member, individual string
	}
	seed := func(name string) made {
		c, _, err := s.CreateCompetition(ctx, competition())
		must(t, err)
		club, _, err := s.CreateClub(ctx, name)
		must(t, err)
		m, _, err := s.Join(ctx, club.ID, "A "+name)
		must(t, err)
		must(t, s.AttachClub(ctx, club.ID, c.ID))
		must(t, s.SaveMemberEntry(ctx, m.ID, c.ID, entry("A "+name)))
		_, err = s.Send(ctx, club.ID, c.ID, nil)
		must(t, err)
		e, _, err := enter(s, c.ID, entry("B "+name))
		must(t, err)
		aboutEntry(t, s, c.ID, e.ID)
		_, err = s.AddSiteFeedback(ctx, SiteFeedback{Text: "Hmm", Role: "Member", Page: "/", CompetitionID: c.ID, Competition: "Student Open"}, Capture{})
		must(t, err)
		return made{c.ID, club.ID, m.ID, e.IndividualID}
	}
	inside, outside := seed("UCD"), seed("DCU")
	id, err := s.SaveCopy(ctx, SandboxCopy{CreatedAt: now, Size: 1, Links: json.RawMessage(`{"organiser":"/x"}`)}, []string{inside.comp}, []string{inside.club})
	must(t, err)
	// A visitor enters the copy's competition on their own after it's made,
	// and someone else enters both.
	visitor, _, err := enter(s, inside.comp, entry("Visitor"))
	must(t, err)
	both, link, err := s.AddIndividualEntry(ctx, outside.comp, "", entry("Both"))
	must(t, err)
	_, _, err = s.AddIndividualEntry(ctx, inside.comp, link, entry("Both"))
	must(t, err)

	must(t, s.DeleteCopy(ctx, id))
	count := func(q string, args ...any) int {
		var n int
		must(t, s.db.QueryRowContext(ctx, q, args...).Scan(&n))
		return n
	}
	for _, c := range []struct {
		what, query string
		arg         string
		want        int
	}{
		{"the copy's competition", `SELECT COUNT(*) FROM competitions WHERE id = $1`, inside.comp, 0},
		{"its entries", `SELECT COUNT(*) FROM entries WHERE competition_id = $1`, inside.comp, 0},
		{"the copy's club", `SELECT COUNT(*) FROM clubs WHERE id = $1`, inside.club, 0},
		{"its member", `SELECT COUNT(*) FROM members WHERE id = $1`, inside.member, 0},
		{"the copy's individual", `SELECT COUNT(*) FROM individuals WHERE id = $1`, inside.individual, 0},
		{"the visitor, entered only in it", `SELECT COUNT(*) FROM individuals WHERE id = $1`, visitor.IndividualID, 0},
		{"someone entered outside it too", `SELECT COUNT(*) FROM individuals WHERE id = $1`, both.IndividualID, 1},
		{"the seed's feedback", `SELECT COUNT(*) FROM site_feedback WHERE competition_id = $1`, inside.comp, 0},
		{"the copy", `SELECT COUNT(*) FROM sandbox_copies WHERE id = $1`, id, 0},
		{"what it held", `SELECT COUNT(*) FROM sandbox_items WHERE copy_id = $1`, id, 0},
		{"the competition outside", `SELECT COUNT(*) FROM competitions WHERE id = $1`, outside.comp, 1},
		{"its entries", `SELECT COUNT(*) FROM entries WHERE competition_id = $1`, outside.comp, 3},
		{"the club outside", `SELECT COUNT(*) FROM members WHERE id = $1`, outside.member, 1},
		{"the individual outside", `SELECT COUNT(*) FROM individuals WHERE id = $1`, outside.individual, 1},
		{"the feedback outside", `SELECT COUNT(*) FROM site_feedback WHERE competition_id = $1`, outside.comp, 1},
		{"check-ins outside", `SELECT COUNT(*) FROM checkins WHERE competition_id = $1`, outside.comp, 1},
	} {
		if got := count(c.query, c.arg); got != c.want {
			t.Errorf("%s: %d rows, want %d", c.what, got, c.want)
		}
	}
	noLeftovers(t, s, "deleting a copy")
	if err := s.DeleteCopy(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("a copy deleted already isn't there: %v", err)
	}
	if _, err := s.SharedCopy(ctx); !errors.Is(err, ErrNotFound) {
		t.Errorf("no copy left: %v", err)
	}
}

// Restarting the day moves the competition's dates by as many days and
// clears what was marked on the day; its entries, timetable and messages
// stay.
func TestRestartDay(t *testing.T) {
	now := time.Date(2027, 3, 1, 10, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition()) // on 13 March, closing on the 6th
	other, _, _ := s.CreateCompetition(ctx, competition())
	e, _, err := enter(s, c.ID, entry("Ann"))
	must(t, err)
	now = time.Date(2027, 3, 13, 10, 0, 0, 0, time.UTC)
	for _, id := range []string{c.ID, other.ID} {
		must(t, s.MarkFlightAt(ctx, id, "f1", "start", "Desk", now))
		must(t, s.SetCheckin(ctx, id, e.ID, "here", "Desk"))
		must(t, s.SetOfficialCheckin(ctx, id, "p1", []string{"f1"}, "here", "Desk"))
		must(t, s.ShowOnScreen(ctx, id, "Welcome", time.Time{}, "Desk"))
		_, err = s.SendDeskMessage(ctx, id, DeskMessage{Text: "Hi", Audience: "All", People: []string{"p1"}, Who: "Desk"})
		must(t, err)
	}
	_, err = s.TellOrganiser(ctx, c.ID, About{}, Sender{Role: RoleIndividual, Name: "Ann", ID: "individual:" + e.IndividualID}, "Where do I park?")
	must(t, err)

	must(t, s.RestartDay(ctx, c.ID, time.Date(2027, 3, 16, 0, 0, 0, 0, time.UTC)))
	got, err := s.Competition(ctx, c.ID)
	must(t, err)
	if got.Date != "2027-03-16" || !got.Deadline.Equal(time.Date(2027, 3, 9, 23, 59, 0, 0, time.UTC)) {
		t.Errorf("moved three days: %s, closing %v", got.Date, got.Deadline)
	}
	var after string
	must(t, s.db.QueryRowContext(ctx, `SELECT delete_after FROM competitions WHERE id = $1`, c.ID).Scan(&after))
	if want := got.DeleteAfter().Format(timeLayout); after != want {
		t.Errorf("kept until %s, want %s", after, want)
	}
	for _, table := range dayMarks {
		var mine, others int
		must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE competition_id = $1`, c.ID).Scan(&mine))
		must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE competition_id = $1`, other.ID).Scan(&others))
		if mine != 0 || (others == 0 && table != "scratch_clears" && table != "up_now") {
			t.Errorf("%s: %d left for the competition restarted, %d for the other", table, mine, others)
		}
	}
	if entries, _ := s.Entries(ctx, c.ID); len(entries) != 1 {
		t.Errorf("its entries stay: %d", len(entries))
	}
	if concerns, _ := s.Concerns(ctx, c.ID); len(concerns) != 1 {
		t.Errorf("its messages stay: %d", len(concerns))
	}
	if err := s.RestartDay(ctx, "gone", now); !errors.Is(err, ErrNotFound) {
		t.Errorf("a competition that isn't there: %v", err)
	}
}

// A clocked store stamps at its own time, on the same database.
func TestClocked(t *testing.T) {
	now := time.Date(2027, 3, 13, 15, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition())
	morning := time.Date(2027, 3, 13, 9, 30, 0, 0, time.UTC)
	must(t, s.Clocked(func() time.Time { return morning }).MarkFlightAt(ctx, c.ID, "f1", "start", "Desk", time.Time{}))
	times, err := s.FlightTimes(ctx, c.ID)
	must(t, err)
	if !times["f1"].Started.Equal(morning) {
		t.Errorf("started at %v, want %v", times["f1"].Started, morning)
	}
	if !s.now().Equal(now) {
		t.Errorf("the store keeps its own clock: %v", s.now())
	}
}
