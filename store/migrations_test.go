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
	_, err = s.db.ExecContext(ctx, individualSubscriptions) // the migration alone
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

// Migration 38 adds feedback, with the button on for competitions already
// made too.
func TestFeedbackMigrates(t *testing.T) {
	all := migrations
	t.Cleanup(func() { migrations = all })
	migrations = all[:38-1]
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	must(t, err)
	c, _, err := s.CreateCompetition(ctx, competition())
	must(t, err)
	migrations = all
	s = reopen(t, s, dir)
	got, err := s.Competition(ctx, c.ID)
	must(t, err)
	if !got.FeedbackButton {
		t.Error("the button is on for a competition made before")
	}
	f, err := s.AddFeedback(ctx, c.ID, Feedback{Text: "Hi", Role: "Member", Page: "/clubs/member/{token}/competitions/{id}"})
	must(t, err)
	if kept, err := s.FeedbackFor(ctx, c.ID); err != nil || len(kept) != 1 || kept[0].ID != f.ID || kept[0].Page != f.Page {
		t.Errorf("feedback is kept: %+v %v", kept, err)
	}
}
