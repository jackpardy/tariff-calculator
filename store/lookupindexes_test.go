package store

import (
	"strings"
	"testing"
)

// Migration 59 adds indexes to a database with rows in it, which keep their
// rows, and are then what the queries by those columns use.
func TestLookupIndexesMigrate(t *testing.T) {
	dir := t.TempDir()
	was := migrations
	migrations = migrations[:58]
	t.Cleanup(func() { migrations = was })
	s, err := Open(ctx, dir)
	must(t, err)
	c, _, _ := s.CreateCompetition(ctx, competition())
	migrations = was
	s = reopen(t, s, dir)
	if got, err := s.Competition(ctx, c.ID); err != nil || got.Name != c.Name {
		t.Errorf("the competition stays: %+v, %v", got, err)
	}
	for query, index := range map[string]string{
		`SELECT id FROM entries WHERE member_id = 'x'`:                         "entries_member",
		`SELECT id FROM entries WHERE partner_entry = 'x'`:                     "entries_partner_entry",
		`SELECT id FROM entries WHERE partner_member = 'x'`:                    "entries_partner_member",
		`SELECT competition_id FROM member_entries WHERE partner_member = 'x'`: "member_entries_partner_member",
		`SELECT competition_id FROM member_entries WHERE partner_entry = 'x'`:  "member_entries_partner_entry",
		`SELECT id FROM members WHERE coach_id = 'x'`:                          "members_coach",
		`SELECT id FROM officials WHERE member_id = 'x'`:                       "officials_member",
		`SELECT id FROM officials WHERE entry_id = 'x'`:                        "officials_entry",
		`SELECT id FROM officials WHERE club_id = 'x'`:                         "officials_club",
		`SELECT member_id FROM member_offers WHERE competition_id = 'x'`:       "member_offers_competition",
	} {
		rows, err := s.db.QueryContext(ctx, `EXPLAIN QUERY PLAN `+query)
		must(t, err)
		var plan string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			must(t, rows.Scan(&id, &parent, &unused, &detail))
			plan += detail + "; "
		}
		must(t, rows.Close())
		if !strings.Contains(plan, index) {
			t.Errorf("%s should use %s: %s", query, index, plan)
		}
	}
	// Running it again changes nothing (IF NOT EXISTS).
	if _, err := s.db.ExecContext(ctx, migrations[58]); err != nil {
		t.Errorf("a second run: %v", err)
	}
}
