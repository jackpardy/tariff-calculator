package store

import (
	"testing"
	"time"
)

// leftovers counts the rows that refer, without a foreign key, to rows that
// are gone: shared ties to a record or link, check-ins, late changes and
// concerns to an entry.
func leftovers(t *testing.T, s *Store) map[string]int {
	t.Helper()
	out := map[string]int{}
	for table, q := range map[string]string{
		"shared_ties record": `SELECT COUNT(*) FROM shared_ties WHERE record_id NOT IN (
			SELECT id FROM members UNION ALL SELECT id FROM coaches UNION ALL SELECT id FROM individuals)`,
		"shared_ties target": `SELECT COUNT(*) FROM shared_ties WHERE (kind = 'club' AND target_id NOT IN (SELECT id FROM clubs))
			OR (kind = 'organiser' AND target_id NOT IN (SELECT id FROM competitions))
			OR (kind = 'helper' AND target_id NOT IN (SELECT id FROM competition_links))`,
		"checkins":      `SELECT COUNT(*) FROM checkins WHERE entry_id NOT IN (SELECT id FROM entries)`,
		"late_requests": `SELECT COUNT(*) FROM late_requests WHERE entry_id NOT IN (SELECT id FROM entries)`,
		"concerns":      `SELECT COUNT(*) FROM concerns WHERE entry_id <> '' AND entry_id NOT IN (SELECT id FROM entries)`,
	} {
		var n int
		must(t, s.db.QueryRowContext(ctx, q).Scan(&n))
		if n > 0 {
			out[table] = n
		}
	}
	return out
}

// aboutEntry gives an entry a check-in, a late change and a concern.
func aboutEntry(t *testing.T, s *Store, competitionID, entryID string) {
	t.Helper()
	must(t, s.SetCheckin(ctx, competitionID, entryID, "here", "Desk"))
	_, err := s.db.ExecContext(ctx, `INSERT INTO late_requests (id, competition_id, entry_id, kind, entry, who, at) VALUES ($1, $2, $3, 'level', '{}', 'Ann', '')`,
		newID(), competitionID, entryID)
	must(t, err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO concerns (id, competition_id, entry_id, who, text, at) VALUES ($1, $2, $3, 'Ann', 'Hmm', '')`,
		newID(), competitionID, entryID)
	must(t, err)
}

func noLeftovers(t *testing.T, s *Store, after string) {
	t.Helper()
	if got := leftovers(t, s); len(got) > 0 {
		t.Errorf("after %s, rows left behind: %v", after, got)
	}
}

func TestNothingLeftBehind(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, organiser, _ := s.CreateCompetition(ctx, competition())
	helper, helperLink, err := s.AddLink(ctx, c.ID, "Chair", LinkChair)
	must(t, err)
	_ = helper

	// An individual's entry withdrawn; a concern about no entry stays.
	e, token, err := enter(s, c.ID, entry("Mary"))
	must(t, err)
	aboutEntry(t, s, c.ID, e.ID)
	_, err = s.db.ExecContext(ctx, `INSERT INTO concerns (id, competition_id, entry_id, who, text, at) VALUES ($1, $2, '', 'Ann', 'General', '')`, newID(), c.ID)
	must(t, err)
	must(t, s.WithdrawIndividualEntry(ctx, token))
	noLeftovers(t, s, "an individual's entry withdrawn")
	var general int
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM concerns WHERE entry_id = ''`).Scan(&general))
	if general != 1 {
		t.Errorf("a concern about no entry stays: %d", general)
	}

	// A club's entry taken back when it sends everyone's again.
	club, clubAdmin, _ := s.CreateClub(ctx, "UCD")
	m, mLink, _ := s.Join(ctx, club.ID, "Ann")
	co, coLink, _ := s.CreateCoach(ctx, club.ID, "Bob")
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	must(t, s.SaveMemberEntry(ctx, m.ID, c.ID, entry("Ann")))
	must2(s.Send(ctx, club.ID, c.ID, nil))
	sent, _ := s.Entries(ctx, c.ID)
	if len(sent) != 1 {
		t.Fatalf("sent: %+v", sent)
	}
	aboutEntry(t, s, c.ID, sent[0].ID)
	must(t, s.WithdrawMemberEntry(ctx, m.ID, c.ID, ""))
	must2(s.Send(ctx, club.ID, c.ID, nil))
	noLeftovers(t, s, "a club's entry taken back")

	// A club deleted: the ties to its admin link, and those added on its
	// members' and coaches' pages.
	must2(s.Tie(ctx, mLink, RoleOrganiser, organiser))
	must2(s.Tie(ctx, coLink, RoleOrganiser, helperLink))
	other, _, _ := s.CreateClub(ctx, "TCD")
	_, oLink, _ := s.Join(ctx, other.ID, "Cara")
	must2(s.Tie(ctx, oLink, RoleClub, clubAdmin))
	_ = co
	must(t, s.DeleteClub(ctx, club.ID))
	noLeftovers(t, s, "a club deleted")

	// A competition deleted: the ties to its admin and helper links.
	must2(s.Tie(ctx, oLink, RoleOrganiser, organiser))
	must2(s.Tie(ctx, oLink, RoleOrganiser, helperLink))
	must(t, s.DeleteCompetition(ctx, c.ID))
	noLeftovers(t, s, "a competition deleted")

	// Everything expiring: a club, an individual and a competition, each with
	// ties to or from it.
	c2, organiser2, _ := s.CreateCompetition(ctx, competition())
	e2, iLink, err := s.AddIndividualEntry(ctx, c2.ID, "", entry("Dee"))
	must(t, err)
	aboutEntry(t, s, c2.ID, e2.ID)
	must2(s.Tie(ctx, iLink, RoleOrganiser, organiser2))
	must2(s.Tie(ctx, iLink, RoleClub, mustAdmin(t, s, other.ID)))
	now = now.AddDate(2, 0, 0)
	_, _, err = s.DeleteExpired(ctx)
	must(t, err)
	if got := leftovers(t, s); len(got) > 0 {
		t.Errorf("after everything expired, rows left behind: %v", got)
	}
	var ties int
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM shared_ties`).Scan(&ties))
	if ties != 0 {
		t.Errorf("no ties at all: %d", ties)
	}
}

// mustAdmin is a club's admin link, replaced so it's known.
func mustAdmin(t *testing.T, s *Store, clubID string) string {
	t.Helper()
	admin, err := s.ReplaceClubAdmin(ctx, clubID)
	must(t, err)
	return admin
}
