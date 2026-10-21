package store

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// A database migrated further than the app knows isn't opened, so an older
// app after a rollback can't write rows without the newer columns.
func TestNoOpeningANewerDatabase(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	must(t, err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO migrations (version, applied_at) VALUES ($1, '')`, len(migrations)+1)
	must(t, err)
	must(t, s.Close())
	if s, err := Open(ctx, dir); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("version %d", len(migrations)+1)) {
		if s != nil {
			s.Close()
		}
		t.Fatalf("refused, saying why: %v", err)
	}
}

// withMigrations adds migrations for a test, the rebuilds among them, and
// takes them away after it.
func withMigrations(t *testing.T, added []string, rebuild ...int) {
	t.Helper()
	was := migrations
	migrations = append(slices.Clone(migrations), added...)
	for _, v := range rebuild {
		rebuilds[v] = true
	}
	t.Cleanup(func() {
		migrations = was
		for _, v := range rebuild {
			delete(rebuilds, v)
		}
	})
}

// A migration that rebuilds a table runs with foreign keys off, so the rows
// referring to it stay, and commits only if every reference still holds.
func TestRebuildingATable(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	must(t, err)
	v := len(migrations) + 1
	withMigrations(t, []string{
		`CREATE TABLE parents (id TEXT PRIMARY KEY);
		CREATE TABLE children (id TEXT PRIMARY KEY, parent TEXT NOT NULL REFERENCES parents (id) ON DELETE CASCADE);
		INSERT INTO parents VALUES ('p');
		INSERT INTO children VALUES ('c', 'p');`,
		`CREATE TABLE parents_new (id TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '');
		INSERT INTO parents_new (id) SELECT id FROM parents;
		DROP TABLE parents;
		ALTER TABLE parents_new RENAME TO parents;`,
	}, v+1)
	s = reopen(t, s, dir)
	var n int
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM children`).Scan(&n))
	if n != 1 {
		t.Errorf("the rows referring to the rebuilt table stay: %d", n)
	}
	// Every connection has its foreign keys back.
	for range 3 {
		var on int
		must(t, s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&on))
		if on != 1 {
			t.Fatal("foreign keys are on again")
		}
	}

	// A rebuild that leaves a reference broken isn't kept.
	withMigrations(t, []string{`DELETE FROM parents;`}, v+2)
	must(t, s.Close())
	if s, err := Open(ctx, dir); err == nil || !strings.Contains(err.Error(), "children") {
		if s != nil {
			s.Close()
		}
		t.Fatalf("a broken reference is refused: %v", err)
	}
	migrations = migrations[:v+1]
	s, err = Open(ctx, dir)
	must(t, err)
	defer s.Close()
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM parents`).Scan(&n))
	if n != 1 {
		t.Errorf("rolled back: %d", n)
	}
}

// The same rebuild with foreign keys on, as a migration not marked as one,
// would lose the rows referring to it: what rebuilds is for.
func TestRebuildingNeedsForeignKeysOff(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	must(t, err)
	withMigrations(t, []string{
		`CREATE TABLE parents (id TEXT PRIMARY KEY);
		CREATE TABLE children (id TEXT PRIMARY KEY, parent TEXT NOT NULL REFERENCES parents (id) ON DELETE CASCADE);
		INSERT INTO parents VALUES ('p');
		INSERT INTO children VALUES ('c', 'p');
		CREATE TABLE parents_new (id TEXT PRIMARY KEY);
		INSERT INTO parents_new (id) SELECT id FROM parents;
		DROP TABLE parents;
		ALTER TABLE parents_new RENAME TO parents;`,
	})
	s = reopen(t, s, dir)
	var n int
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM children`).Scan(&n))
	if n != 0 {
		t.Errorf("with foreign keys on, dropping the table deletes what refers to it: %d", n)
	}
}

// Migration 36 makes an individual's subscriptions the individual's, from the
// entry they were made on; one whose entry has gone stays as it was.
func TestIndividualSubscriptionsMigrate(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	must(t, err)
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	c, _, _ := s.CreateCompetition(ctx, competition())
	e, _, err := enter(s, c.ID, entry("Ann"))
	must(t, err)
	for _, owner := range []string{e.ID, "gone"} {
		_, _, err := s.Subscribe(ctx, Subscription{CompetitionID: c.ID, Kind: NotifyIndividual, OwnerID: owner, Channel: ByPush, Address: "https://push.example/" + owner, Keys: "{}", Topics: Topics})
		must(t, err)
	}
	// Run the migration again on these, as it ran on what was there; running
	// every later one again would make their tables twice.
	_, err = s.db.ExecContext(ctx, individualSubscriptions)
	must(t, err)
	if mine, _ := s.Subscriptions(ctx, c.ID, NotifyIndividual, e.IndividualID); len(mine) != 1 {
		t.Errorf("the individual's: %+v", mine)
	}
	if gone, _ := s.Subscriptions(ctx, c.ID, NotifyIndividual, "gone"); len(gone) != 1 {
		t.Errorf("one whose entry has gone stays: %+v", gone)
	}
}

// reopen closes a store and opens its directory again, running any
// migrations it hasn't had.
func reopen(t *testing.T, s *Store, dir string) *Store {
	t.Helper()
	must(t, s.Close())
	s, err := Open(ctx, dir)
	must(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

// Migration 33 clears the page paths notifications kept: they held people's
// secret links.
func TestNotificationsKeepNoPage(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	must(t, err)
	c, _, _ := s.CreateCompetition(ctx, competition())
	sub, _, err := s.Subscribe(ctx, Subscription{CompetitionID: c.ID, Kind: NotifyMember, OwnerID: "m1", Channel: ByEmail, Address: "ann@example.com", Topics: Topics})
	must(t, err)
	var page string
	must(t, s.db.QueryRowContext(ctx, `SELECT page FROM notify_subscriptions WHERE id = $1`, sub.ID).Scan(&page))
	if page != "" {
		t.Errorf("a new subscription keeps no page: %q", page)
	}

	// One from before, then the migration again (later ones can't run twice).
	_, err = s.db.ExecContext(ctx, `UPDATE notify_subscriptions SET page = '/clubs/member/s3cr3t' WHERE id = $1`, sub.ID)
	must(t, err)
	_, err = s.db.ExecContext(ctx, migrations[33-1])
	must(t, err)
	must(t, s.db.QueryRowContext(ctx, `SELECT page FROM notify_subscriptions WHERE id = $1`, sub.ID).Scan(&page))
	if page != "" {
		t.Errorf("cleared: %q", page)
	}
}

// Migration 47 moves the feedback sent from a competition's pages (40) to
// the maintainer's, with the competition's name, and drops the old table.
func TestSiteFeedbackMigrates(t *testing.T) {
	all := migrations
	t.Cleanup(func() { migrations = all })
	migrations = all[:47-1]
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	must(t, err)
	c, _, err := s.CreateCompetition(ctx, competition())
	must(t, err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO feedback (id, competition_id, text, reply_to, role, page, at, done_at, done_by)
		VALUES ('f1', $1, 'Where is my warm-up?', 'Ann', 'Member', '/clubs/member/{token}/competitions/{id}/day', '2026-10-15T10:00:00Z', '2026-10-16T10:00:00Z', 'Organiser')`, c.ID)
	must(t, err)
	migrations = all
	s = reopen(t, s, dir)
	got, err := s.AllSiteFeedback(ctx)
	must(t, err)
	want := SiteFeedback{ID: "f1", Text: "Where is my warm-up?", ReplyTo: "Ann", Role: "Member", Page: "/clubs/member/{token}/competitions/{id}/day",
		CompetitionID: c.ID, Competition: c.Name, At: parseTime("2026-10-15T10:00:00Z"), DoneAt: parseTime("2026-10-16T10:00:00Z")}
	if len(got) != 1 || got[0] != want {
		t.Errorf("moved: %+v, want %+v", got, want)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'feedback'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("the old table is gone: %d %v", n, err)
	}
}

// Migration 48 makes competitions.feedback the switch for messages to the
// organiser, on for every competition, and names who sent each concern;
// those flagged before are from admin links.
func TestMessagesMigrate(t *testing.T) {
	all := migrations
	t.Cleanup(func() { migrations = all })
	migrations = all[:48-1]
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	must(t, err)
	c, _, err := s.CreateCompetition(ctx, competition())
	must(t, err)
	_, err = s.db.ExecContext(ctx, `UPDATE competitions SET feedback = FALSE WHERE id = $1`, c.ID)
	must(t, err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO concerns (id, competition_id, entry_id, who, text, at) VALUES ('k1', $1, '', 'Chairs', 'Element 7', '2026-10-15T10:00:00Z')`, c.ID)
	must(t, err)
	migrations = all
	s = reopen(t, s, dir)
	got, err := s.Competition(ctx, c.ID)
	must(t, err)
	if !got.Messages {
		t.Error("messages are on, whatever the old button was")
	}
	concerns, err := s.Concerns(ctx, c.ID)
	must(t, err)
	if len(concerns) != 1 || concerns[0].Who != "Chairs" || concerns[0].Role != "" || concerns[0].Club != "" {
		t.Errorf("the concern from before is from an admin link: %+v", concerns)
	}
	must(t, s.SetMessages(ctx, c.ID, false))
	if got, err := s.Competition(ctx, c.ID); err != nil || got.Messages {
		t.Errorf("turned off: %v %v", got.Messages, err)
	}
}

// Migration 49 moves a message's one entry into its list, keeps a concern
// flagged from an admin link as it was, and starts every message
// unacknowledged, without replies.
func TestMessagesAboutSeveralMigrate(t *testing.T) {
	all := migrations
	t.Cleanup(func() { migrations = all })
	migrations = all[:49-1]
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	must(t, err)
	c, _, err := s.CreateCompetition(ctx, competition())
	must(t, err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO concerns (id, competition_id, entry_id, who, text, at, role, club, sender) VALUES
		('m1', $1, 'e1', 'Ann', 'My card', '2026-10-15T10:00:00Z', 'member', 'UCD', 'member:x'),
		('k1', $1, 'e2', 'Chairs', 'Element 7', '2026-10-15T11:00:00Z', '', '', '')`, c.ID)
	must(t, err)
	migrations = all
	s = reopen(t, s, dir)
	got, err := s.Concerns(ctx, c.ID)
	must(t, err)
	if len(got) != 2 {
		t.Fatalf("both kept: %+v", got)
	}
	flag, msg := got[0], got[1]
	if flag.ID != "k1" || flag.EntryID != "e2" || len(flag.Entries) != 0 {
		t.Errorf("the flag keeps its entry: %+v", flag)
	}
	if msg.ID != "m1" || msg.EntryID != "" || !slices.Equal(msg.Entries, []string{"e1"}) || msg.Acknowledged() || len(msg.Thread) != 0 {
		t.Errorf("the message's entry moves into its list: %+v", msg)
	}
}
