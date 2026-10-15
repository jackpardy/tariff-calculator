package store

import (
	"testing"
)

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

	// One from before, then the migration again.
	_, err = s.db.ExecContext(ctx, `UPDATE notify_subscriptions SET page = '/clubs/member/s3cr3t' WHERE id = $1`, sub.ID)
	must(t, err)
	_, err = s.db.ExecContext(ctx, `DELETE FROM migrations WHERE version = 33`)
	must(t, err)
	s = reopen(t, s, dir)
	must(t, s.db.QueryRowContext(ctx, `SELECT page FROM notify_subscriptions WHERE id = $1`, sub.ID).Scan(&page))
	if page != "" {
		t.Errorf("cleared: %q", page)
	}
}
