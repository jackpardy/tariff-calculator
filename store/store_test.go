package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"tariffCalculator/competitions"
)

var ctx = context.Background()

// open opens a fresh store whose clock reads *now.
func open(t *testing.T, now *time.Time) *Store {
	t.Helper()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	s.now = func() time.Time { return *now }
	return s
}

func competition() competitions.Competition {
	return competitions.Competition{
		Name:        "Student Open",
		Date:        "2027-03-13",
		Deadline:    time.Date(2027, 3, 6, 23, 59, 0, 0, time.UTC),
		Individuals: true,
		Levels:      []competitions.Level{{Ref: "builtin-level:bucs-l3"}},
	}
}

func entry(gymnast string) competitions.Entry {
	return competitions.Entry{Gymnast: gymnast, Level: "BUCS L3", Exercises: [2]competitions.Exercise{{Option: "builtin:bucs-l3-option-1"}, {Option: "builtin:bucs-l3-second"}}}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenMigratesOnce(t *testing.T) {
	dir := t.TempDir()
	for range 2 {
		s, err := Open(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		var n int
		must(t, s.db.QueryRow(`SELECT COUNT(*) FROM migrations`).Scan(&n))
		if n != len(migrations) {
			t.Errorf("%d migrations recorded, want %d", n, len(migrations))
		}
		s.Close()
	}
}

func TestCompetitionLinks(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, admin, err := s.CreateCompetition(ctx, competition())
	must(t, err)
	if len(admin) != 22 || admin == c.ClubLink || c.ClubLink == c.IndividualLink {
		t.Errorf("links should be 128 bits as base64url, all different: %q %q %q", admin, c.ClubLink, c.IndividualLink)
	}

	got, err := s.CompetitionByAdmin(ctx, admin)
	must(t, err)
	if got.ID != c.ID || got.Name != "Student Open" || !got.Deadline.Equal(c.Deadline) || len(got.Levels) != 1 || got.ClubLink != c.ClubLink {
		t.Errorf("read back: %+v", got)
	}
	if _, err := s.CompetitionByClubLink(ctx, c.ClubLink); err != nil {
		t.Error(err)
	}
	if _, err := s.CompetitionByIndividualLink(ctx, c.IndividualLink); err != nil {
		t.Error(err)
	}
	for _, wrong := range []string{"", c.ID, c.ClubLink} {
		if _, err := s.CompetitionByAdmin(ctx, wrong); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q isn't the admin link: %v", wrong, err)
		}
	}

	// Only hashes of admin links are stored.
	var stored string
	must(t, s.db.QueryRow(`SELECT admin_hash FROM competitions`).Scan(&stored))
	if stored == admin || stored != hash(admin) {
		t.Error("the admin link should be stored as its hash")
	}

	replaced, err := s.ReplaceCompetitionAdmin(ctx, c.ID)
	must(t, err)
	if _, err := s.CompetitionByAdmin(ctx, admin); !errors.Is(err, ErrNotFound) {
		t.Error("a replaced admin link stops working")
	}
	if _, err := s.CompetitionByAdmin(ctx, replaced); err != nil {
		t.Error(err)
	}

	// Individuals can be turned off.
	closed := competition()
	closed.Individuals = false
	c2, _, err := s.CreateCompetition(ctx, closed)
	must(t, err)
	if _, err := s.CompetitionByIndividualLink(ctx, c2.IndividualLink); !errors.Is(err, ErrNotFound) {
		t.Error("the individual link doesn't work when individuals can't enter")
	}

	must(t, s.DeleteCompetition(ctx, c.ID))
	if _, err := s.CompetitionByAdmin(ctx, replaced); !errors.Is(err, ErrNotFound) {
		t.Error("a deleted competition is gone")
	}
}

func TestIndividualEntries(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, err := s.CreateCompetition(ctx, competition())
	must(t, err)

	e, token, err := s.AddIndividualEntry(ctx, c.ID, entry("C. Ryan"))
	must(t, err)
	if !e.Individual {
		t.Error("marked individual")
	}
	got, err := s.IndividualEntry(ctx, token)
	must(t, err)
	if got.Entry.Gymnast != "C. Ryan" || got.CompetitionID != c.ID {
		t.Errorf("read back: %+v", got)
	}

	now = now.Add(time.Hour)
	changed := entry("C. Ryan")
	changed.Exercises[0].Option = "builtin:bucs-l3-option-2"
	must(t, s.ReplaceIndividualEntry(ctx, token, changed))
	list, err := s.Entries(ctx, c.ID)
	must(t, err)
	if len(list) != 1 || list[0].Entry.Exercises[0].Option != "builtin:bucs-l3-option-2" || !list[0].SentAt.Equal(now) {
		t.Errorf("replaced: %+v", list)
	}

	if got, err := s.CompetitionEntry(ctx, c.ID, e.ID); err != nil || got.Entry.Gymnast != "C. Ryan" {
		t.Errorf("by id: %+v, %v", got, err)
	}
	other, _, _ := s.CreateCompetition(ctx, competition())
	if _, err := s.CompetitionEntry(ctx, other.ID, e.ID); !errors.Is(err, ErrNotFound) {
		t.Error("another competition's entry isn't found")
	}

	must(t, s.SetIndividuals(ctx, c.ID, false))
	if _, err := s.CompetitionByIndividualLink(ctx, c.IndividualLink); !errors.Is(err, ErrNotFound) {
		t.Error("individual entry turned off")
	}
	must(t, s.SetIndividuals(ctx, c.ID, true))

	now = c.Deadline
	if err := s.ReplaceIndividualEntry(ctx, token, entry("C. Ryan")); !errors.Is(err, ErrClosed) {
		t.Errorf("no changes after the deadline: %v", err)
	}
	if _, _, err := s.AddIndividualEntry(ctx, c.ID, entry("D")); !errors.Is(err, ErrClosed) {
		t.Errorf("no entries after the deadline: %v", err)
	}
	if err := s.WithdrawIndividualEntry(ctx, token); !errors.Is(err, ErrClosed) {
		t.Errorf("no withdrawing after the deadline: %v", err)
	}
	now = c.Deadline.Add(-time.Minute)
	must(t, s.WithdrawIndividualEntry(ctx, token))
	if _, err := s.IndividualEntry(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Error("withdrawn")
	}
}

func TestClubsAndMembers(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, admin, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	if got, err := s.ClubByAdmin(ctx, admin); err != nil || got.ID != club.ID {
		t.Errorf("admin link: %+v, %v", got, err)
	}
	if got, err := s.ClubByJoinLink(ctx, club.JoinLink); err != nil || got.ID != club.ID {
		t.Errorf("join link: %+v, %v", got, err)
	}
	if _, err := s.ClubByAdmin(ctx, club.JoinLink); !errors.Is(err, ErrNotFound) {
		t.Error("the join link isn't the admin link")
	}

	m, token, err := s.Join(ctx, club.ID, "A. Murphy")
	must(t, err)
	if got, err := s.MemberByLink(ctx, token); err != nil || got.ID != m.ID || got.ClubID != club.ID {
		t.Errorf("personal link: %+v, %v", got, err)
	}
	if _, _, err := s.Join(ctx, "nope", "X"); err == nil {
		t.Error("can't join a club that doesn't exist")
	}

	fresh, err := s.ReplaceMemberLink(ctx, club.ID, m.ID)
	must(t, err)
	if _, err := s.MemberByLink(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Error("the old personal link stops working")
	}
	if _, err := s.MemberByLink(ctx, fresh); err != nil {
		t.Error(err)
	}
	other, _, _ := s.CreateClub(ctx, "DCU")
	if _, err := s.ReplaceMemberLink(ctx, other.ID, m.ID); !errors.Is(err, ErrNotFound) {
		t.Error("another club's comp sec can't replace a member's link")
	}

	members, err := s.Members(ctx, club.ID)
	must(t, err)
	if len(members) != 1 || members[0].Name != "A. Murphy" {
		t.Errorf("members: %+v", members)
	}
	must(t, s.RemoveMember(ctx, club.ID, m.ID))
	if _, err := s.MemberByLink(ctx, fresh); !errors.Is(err, ErrNotFound) {
		t.Error("a removed member's link stops working")
	}
}

func TestSending(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, err := s.CreateCompetition(ctx, competition())
	must(t, err)
	club, _, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	a, _, _ := s.Join(ctx, club.ID, "A. Murphy")
	b, _, _ := s.Join(ctx, club.ID, "B. Kelly")

	if err := s.SaveMemberEntry(ctx, a.ID, c.ID, entry("A. Murphy")); !errors.Is(err, ErrNotAttached) {
		t.Errorf("members can only enter competitions their club is in: %v", err)
	}
	if err := s.AttachClub(ctx, club.ID, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("attaching to a missing competition: %v", err)
	}
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	must(t, s.AttachClub(ctx, club.ID, c.ID)) // again: nothing happens
	if comps, err := s.ClubCompetitions(ctx, club.ID); err != nil || len(comps) != 1 || comps[0].ID != c.ID {
		t.Errorf("the club's competitions: %+v, %v", comps, err)
	}
	if clubs, err := s.CompetitionClubs(ctx, c.ID); err != nil || len(clubs) != 1 || clubs[0].Name != "UCD" {
		t.Errorf("the competition's clubs: %+v, %v", clubs, err)
	}

	must(t, s.SaveMemberEntry(ctx, a.ID, c.ID, entry("A. Murphy")))
	must(t, s.SaveMemberEntry(ctx, b.ID, c.ID, entry("Someone else")))
	if mine, _ := s.MemberEntries(ctx, b.ID); len(mine) != 1 || mine[0].Entry.Gymnast != "B. Kelly" {
		t.Errorf("a member's entry is under their name: %+v", mine)
	}
	if list, _ := s.Entries(ctx, c.ID); len(list) != 0 {
		t.Error("the organiser sees nothing until the club sends")
	}

	now = now.Add(time.Hour)
	n, err := s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	if n != 2 {
		t.Errorf("sent %d, want 2", n)
	}
	list, err := s.Entries(ctx, c.ID)
	must(t, err)
	if len(list) != 2 || list[0].ClubName != "UCD" || list[0].Individual || list[0].Entry.Gymnast != "A. Murphy" {
		t.Errorf("the competition's copies: %+v", list)
	}

	// A member changes their entry: the club page shows it, the competition doesn't yet.
	now = now.Add(time.Hour)
	changed := entry("A. Murphy")
	changed.Exercises[0].Option = "builtin:bucs-l3-option-2"
	must(t, s.SaveMemberEntry(ctx, a.ID, c.ID, changed))
	mine, err := s.ClubEntries(ctx, club.ID, c.ID)
	must(t, err)
	if len(mine) != 2 || !mine[0].ChangedSinceSent() || mine[1].ChangedSinceSent() || !mine[1].Sent() {
		t.Errorf("A changed since sent, B didn't: %+v", mine)
	}
	if list, _ := s.Entries(ctx, c.ID); list[0].Entry.Exercises[0].Option != "builtin:bucs-l3-option-1" {
		t.Error("the competition keeps the copy as sent")
	}

	// Re-send just the changed one.
	now = now.Add(time.Hour)
	if n, err := s.Send(ctx, club.ID, c.ID, []string{a.ID}); err != nil || n != 1 {
		t.Errorf("re-sent %d, %v", n, err)
	}
	if list, _ := s.Entries(ctx, c.ID); len(list) != 2 || list[0].Entry.Exercises[0].Option != "builtin:bucs-l3-option-2" {
		t.Errorf("re-sent copy: %+v", list)
	}
	if mine, _ := s.MemberEntries(ctx, a.ID); len(mine) != 1 || mine[0].ChangedSinceSent() {
		t.Errorf("A's entry is up to date: %+v", mine)
	}

	// B withdraws; sending everyone's takes B's entry back.
	must(t, s.WithdrawMemberEntry(ctx, b.ID, c.ID))
	if list, _ := s.Entries(ctx, c.ID); len(list) != 2 {
		t.Error("until the club sends again, the competition keeps B's entry")
	}
	if _, err := s.Send(ctx, club.ID, c.ID, nil); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.Entries(ctx, c.ID); len(list) != 1 || list[0].MemberID != a.ID {
		t.Errorf("only A's entry is left: %+v", list)
	}

	// Removing the club keeps what it sent with the competition.
	must(t, s.DeleteClub(ctx, club.ID))
	if list, _ := s.Entries(ctx, c.ID); len(list) != 1 || list[0].ClubName != "UCD" || list[0].ClubID != "" {
		t.Errorf("sent entries stay with the competition: %+v", list)
	}

	now = c.Deadline
	club2, _, _ := s.CreateClub(ctx, "DCU")
	m, _, _ := s.Join(ctx, club2.ID, "E")
	must(t, s.AttachClub(ctx, club2.ID, c.ID))
	if err := s.SaveMemberEntry(ctx, m.ID, c.ID, entry("E")); !errors.Is(err, ErrClosed) {
		t.Errorf("no changes after the deadline: %v", err)
	}
	if _, err := s.Send(ctx, club2.ID, c.ID, nil); !errors.Is(err, ErrClosed) {
		t.Errorf("no sending after the deadline: %v", err)
	}
}

func TestLimits(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, _ := s.CreateClub(ctx, "Big club")
	for i := range MaxMembers {
		if _, _, err := s.Join(ctx, club.ID, "M"); err != nil {
			t.Fatalf("member %d: %v", i+1, err)
		}
	}
	if _, _, err := s.Join(ctx, club.ID, "One more"); !errors.Is(err, ErrLimit) {
		t.Errorf("a full club: %v", err)
	}

	c, _, _ := s.CreateCompetition(ctx, competition())
	_, err := s.db.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < $1)
		INSERT INTO entries (id, competition_id, club_name, individual, gymnast, entry, sent_at)
		SELECT 'e' || i, $2, '', TRUE, 'G', '{}', '2027-01-01T00:00:00Z' FROM n`, MaxEntries, c.ID)
	must(t, err)
	if _, _, err := s.AddIndividualEntry(ctx, c.ID, entry("One more")); !errors.Is(err, ErrLimit) {
		t.Errorf("a full competition: %v", err)
	}
}

func TestDeleteExpired(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition()) // on 13 March 2027
	club, admin, _ := s.CreateClub(ctx, "UCD")
	m, _, _ := s.Join(ctx, club.ID, "A")
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	must(t, s.SaveMemberEntry(ctx, m.ID, c.ID, entry("A")))
	_, err := s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)

	// 120 days after the competition: it goes, with its entries. The club was
	// used the day before, so it stays, without its entries for that competition.
	now = time.Date(2027, 7, 10, 12, 0, 0, 0, time.UTC)
	_, err = s.ClubByAdmin(ctx, admin)
	must(t, err)
	if comps, clubs, err := s.DeleteExpired(ctx); err != nil || comps != 0 || clubs != 0 {
		t.Errorf("nothing is due yet: %d, %d, %v", comps, clubs, err)
	}
	now = time.Date(2027, 7, 11, 0, 0, 0, 0, time.UTC)
	if comps, clubs, err := s.DeleteExpired(ctx); err != nil || comps != 1 || clubs != 0 {
		t.Errorf("the competition is due: %d, %d, %v", comps, clubs, err)
	}
	if entries, _ := s.MemberEntries(ctx, m.ID); len(entries) != 0 {
		t.Error("the member's entry for it goes too")
	}
	var n int
	must(t, s.db.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&n))
	if n != 0 {
		t.Error("and the competition's copies")
	}

	// 120 days after the club was last used, it goes with its members.
	now = time.Date(2027, 11, 7, 12, 0, 0, 0, time.UTC)
	if _, clubs, err := s.DeleteExpired(ctx); err != nil || clubs != 1 {
		t.Errorf("the club is due: %d, %v", clubs, err)
	}
	must(t, s.db.QueryRow(`SELECT COUNT(*) FROM members`).Scan(&n))
	if n != 0 {
		t.Error("its members go too")
	}
}
