package store

import (
	"context"
	"errors"
	"slices"
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
		LiveAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
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

	if got, err := s.Member(ctx, club.ID, m.ID); err != nil || got.Name != "A. Murphy" {
		t.Errorf("by id: %+v, %v", got, err)
	}
	if _, err := s.Member(ctx, other.ID, m.ID); !errors.Is(err, ErrNotFound) {
		t.Error("not another club's member")
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
	if in, _ := s.ClubEntered(ctx, club.ID, c.ID); in {
		t.Error("not entered yet")
	}
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	must(t, s.AttachClub(ctx, club.ID, c.ID)) // again: nothing happens
	if in, _ := s.ClubEntered(ctx, club.ID, c.ID); !in {
		t.Error("entered")
	}
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
	must(t, s.WithdrawMemberEntry(ctx, b.ID, c.ID, ""))
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

func TestChecking(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition())
	club, _, _ := s.CreateClub(ctx, "UCD")
	m, _, _ := s.Join(ctx, club.ID, "A")
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	must(t, s.SaveMemberEntry(ctx, m.ID, c.ID, entry("A")))
	_, err := s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	_, token, err := s.AddIndividualEntry(ctx, c.ID, entry("I"))
	must(t, err)
	list, _ := s.Entries(ctx, c.ID)

	for _, e := range list {
		must(t, s.MarkChecked(ctx, c.ID, e.ID, true, "Looks good"))
	}
	if err := s.MarkChecked(ctx, "other", list[0].ID, true, ""); !errors.Is(err, ErrNotFound) {
		t.Error("only the competition's own entries")
	}
	list, _ = s.Entries(ctx, c.ID)
	if !list[0].Checked() || list[0].Note != "Looks good" {
		t.Errorf("checked with a note: %+v", list[0])
	}
	if mine, _ := s.ClubEntries(ctx, club.ID, c.ID); !mine[0].Checked || mine[0].Note != "Looks good" {
		t.Errorf("the club sees the organiser's check and note: %+v", mine[0])
	}

	// Sending the same entry again keeps the check; a changed one loses it.
	now = now.Add(time.Hour)
	_, err = s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	if mine, _ := s.ClubEntries(ctx, club.ID, c.ID); !mine[0].Checked {
		t.Error("re-sending an unchanged entry keeps the check")
	}
	changed := entry("A")
	changed.Exercises[0].Option = "builtin:bucs-l3-option-2"
	must(t, s.SaveMemberEntry(ctx, m.ID, c.ID, changed))
	_, err = s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	if mine, _ := s.ClubEntries(ctx, club.ID, c.ID); mine[0].Checked || mine[0].Note != "Looks good" {
		t.Errorf("a changed entry needs checking again; the note stays: %+v", mine[0])
	}
	must(t, s.ReplaceIndividualEntry(ctx, token, changed))
	if e, _ := s.IndividualEntry(ctx, token); e.Checked() {
		t.Error("a changed individual entry needs checking again")
	}

	// Closing entries now, then reopening them.
	must(t, s.SetDeadline(ctx, c.ID, now))
	if err := s.SaveMemberEntry(ctx, m.ID, c.ID, entry("A")); !errors.Is(err, ErrClosed) {
		t.Errorf("closed: %v", err)
	}
	must(t, s.SetDeadline(ctx, c.ID, now.Add(24*time.Hour)))
	must(t, s.SaveMemberEntry(ctx, m.ID, c.ID, entry("A")))
}

func TestVideo(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp := competition()
	comp.Video = competitions.Video{Need: competitions.VideoRoutine}
	c, admin, err := s.CreateCompetition(ctx, comp)
	must(t, err)
	if got, _ := s.CompetitionByAdmin(ctx, admin); got.Video.Need != competitions.VideoRoutine {
		t.Errorf("the video setting is kept: %+v", got.Video)
	}
	must(t, s.SetVideo(ctx, c.ID, competitions.Video{}))
	if got, _ := s.CompetitionByAdmin(ctx, admin); got.Video.Need != competitions.VideoNone {
		t.Error("and can be changed")
	}

	club, _, _ := s.CreateClub(ctx, "UCD")
	m, _, _ := s.Join(ctx, club.ID, "A")
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	withVideo := entry("A")
	withVideo.Exercises[1].Video = "https://youtu.be/abc"
	must(t, s.SaveMemberEntry(ctx, m.ID, c.ID, withVideo))
	_, err = s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	list, _ := s.Entries(ctx, c.ID)
	if list[0].Entry.Exercises[1].Video != "https://youtu.be/abc" {
		t.Error("the link goes with the entry")
	}

	if err := s.ReviewVideo(ctx, c.ID, list[0].ID, "maybe", ""); err == nil {
		t.Error("an unknown review")
	}
	must(t, s.ReviewVideo(ctx, c.ID, list[0].ID, VideoMore, "The full-in isn't in it"))
	if mine, _ := s.ClubEntries(ctx, club.ID, c.ID); mine[0].VideoReview != VideoMore || mine[0].VideoNote != "The full-in isn't in it" {
		t.Errorf("the club sees the review: %+v", mine[0])
	}
	// A new link clears the review once it's sent; the note stays.
	withVideo.Exercises[1].Video = "https://youtu.be/def"
	must(t, s.SaveMemberEntry(ctx, m.ID, c.ID, withVideo))
	_, err = s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	if list, _ := s.Entries(ctx, c.ID); list[0].VideoReview != "" || list[0].VideoNote != "The full-in isn't in it" {
		t.Errorf("a changed entry needs reviewing again: %+v", list[0])
	}
}

func TestWithdrawnEntries(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition())
	club, _, _ := s.CreateClub(ctx, "UCD")
	a, _, _ := s.Join(ctx, club.ID, "A")
	b, _, _ := s.Join(ctx, club.ID, "B")
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	must(t, s.SaveMemberEntry(ctx, a.ID, c.ID, entry("A")))
	must(t, s.SaveMemberEntry(ctx, b.ID, c.ID, entry("B")))
	_, err := s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	_, _, err = s.AddIndividualEntry(ctx, c.ID, entry("I"))
	must(t, err)

	withdrawn := func() map[string]bool {
		list, err := s.Entries(ctx, c.ID)
		must(t, err)
		out := map[string]bool{}
		for _, e := range list {
			out[e.Entry.Gymnast] = e.Withdrawn
		}
		return out
	}
	if got := withdrawn(); got["A"] || got["B"] || got["I"] {
		t.Errorf("nothing withdrawn yet: %v", got)
	}
	must(t, s.WithdrawMemberEntry(ctx, a.ID, c.ID, ""))
	must(t, s.RemoveMember(ctx, club.ID, b.ID))
	if got := withdrawn(); !got["A"] || !got["B"] || got["I"] {
		t.Errorf("A withdrew and B left: both marked until the club sends again: %v", got)
	}
	list, _ := s.Entries(ctx, c.ID)
	if e, err := s.CompetitionEntry(ctx, c.ID, list[0].ID); err != nil || !e.Withdrawn {
		t.Errorf("one entry, marked too: %+v, %v", e, err)
	}
	_, err = s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	if got := withdrawn(); len(got) != 1 || got["I"] {
		t.Errorf("sending everyone's takes them back: %v", got)
	}
}

func TestCoaches(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp := competition()
	comp.Signoff = true
	c, admin, _ := s.CreateCompetition(ctx, comp)
	if got, _ := s.CompetitionByAdmin(ctx, admin); !got.Signoff {
		t.Error("the sign-off setting is kept")
	}
	club, clubAdmin, _ := s.CreateClub(ctx, "UCD")
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	ann, annLink, err := s.CreateCoach(ctx, club.ID, "Ann")
	must(t, err)
	bob, _, _ := s.CreateCoach(ctx, club.ID, "Bob")
	if got, err := s.CoachByLink(ctx, annLink); err != nil || got.ID != ann.ID {
		t.Errorf("coach link: %+v, %v", got, err)
	}
	if coaches, _ := s.Coaches(ctx, club.ID); len(coaches) != 2 || coaches[0].Name != "Ann" {
		t.Errorf("coaches: %+v", coaches)
	}

	x, _, _ := s.Join(ctx, club.ID, "X") // Ann's
	y, _, _ := s.Join(ctx, club.ID, "Y") // Bob's
	z, _, _ := s.Join(ctx, club.ID, "Z") // no coach
	must(t, s.SetMemberCoach(ctx, club.ID, x.ID, ann.ID))
	must(t, s.SetMemberCoach(ctx, club.ID, y.ID, bob.ID))
	other, _, _ := s.CreateClub(ctx, "DCU")
	stranger, _, _ := s.CreateCoach(ctx, other.ID, "S")
	if err := s.SetMemberCoach(ctx, club.ID, z.ID, stranger.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("only the club's own coaches: %v", err)
	}
	if m, _ := s.Member(ctx, club.ID, x.ID); m.CoachID != ann.ID {
		t.Errorf("X's coach: %+v", m)
	}
	for _, m := range []Member{x, y, z} {
		must(t, s.SaveMemberEntry(ctx, m.ID, c.ID, entry(m.Name)))
	}

	sees := func(coach Coach) string {
		list, err := s.CoachEntries(ctx, coach)
		must(t, err)
		out := ""
		for _, e := range list {
			out += e.MemberName
		}
		return out
	}
	if got := sees(ann); got != "XZ" {
		t.Errorf("Ann sees her member and the one without a coach: %q", got)
	}
	if got := sees(stranger); got != "" {
		t.Errorf("another club's coach sees nothing: %q", got)
	}
	club, _ = s.ClubByAdmin(ctx, clubAdmin)
	must(t, s.SetCoachesSeeAll(ctx, club.ID, true))
	if got := sees(ann); got != "XYZ" {
		t.Errorf("with see-all, every member: %q", got)
	}
	must(t, s.SetCoachesSeeAll(ctx, club.ID, false))

	if err := s.SignOff(ctx, ann, y.ID, c.ID, "", true, ""); !errors.Is(err, ErrNotFound) {
		t.Error("Ann can't sign off Bob's member")
	}
	must(t, s.SignOff(ctx, ann, x.ID, c.ID, "", true, "Good to go"))
	mine, _ := s.MemberEntries(ctx, x.ID)
	if !mine[0].SignedOff() || mine[0].SignedBy != "Ann" || mine[0].SignNote != "Good to go" {
		t.Errorf("signed off: %+v", mine[0])
	}
	must(t, s.SignOff(ctx, ann, z.ID, c.ID, "", false, "Not the full-in yet"))
	if zs, _ := s.MemberEntries(ctx, z.ID); zs[0].SignedOff() || zs[0].SignedBy != "Ann" || zs[0].SignNote != "Not the full-in yet" {
		t.Errorf("not yet: %+v", zs[0])
	}

	// Sending takes the sign-off with it; a change clears it on the club side,
	// and sending the change clears it at the competition.
	_, err = s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	list, _ := s.Entries(ctx, c.ID)
	if !list[0].SignedOff() || list[0].SignedBy != "Ann" || list[1].SignedOff() {
		t.Errorf("sent with their sign-offs: %+v", list)
	}
	changed := entry("X")
	changed.Exercises[0].Option = "builtin:bucs-l3-option-2"
	must(t, s.SaveMemberEntry(ctx, x.ID, c.ID, changed))
	if mine, _ := s.MemberEntries(ctx, x.ID); mine[0].SignedOff() {
		t.Error("a changed entry needs signing off again")
	}
	must(t, s.SaveMemberEntry(ctx, z.ID, c.ID, entry("Z"))) // unchanged: keeps its note
	if zs, _ := s.MemberEntries(ctx, z.ID); zs[0].SignNote == "" {
		t.Error("saving an unchanged entry keeps the coach's word")
	}
	_, err = s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	if list, _ := s.Entries(ctx, c.ID); list[0].SignedOff() {
		t.Error("the competition's copy is no longer signed off")
	}

	// Removing a coach leaves their members without one.
	must(t, s.RemoveCoach(ctx, club.ID, ann.ID))
	if m, _ := s.Member(ctx, club.ID, x.ID); m.CoachID != "" {
		t.Errorf("no coach now: %+v", m)
	}
	if _, err := s.CoachByLink(ctx, annLink); !errors.Is(err, ErrNotFound) {
		t.Error("a removed coach's link stops working")
	}
	fresh, err := s.ReplaceCoachLink(ctx, club.ID, bob.ID)
	must(t, err)
	if got, err := s.CoachByLink(ctx, fresh); err != nil || got.ID != bob.ID {
		t.Error("a replaced link works")
	}

	// An individual's coach signs off through the entry's sign-off link.
	ind, token, err := s.AddIndividualEntry(ctx, c.ID, entry("I"))
	must(t, err)
	if ind.SignoffLink == "" {
		t.Fatal("an individual entry has a sign-off link")
	}
	if e, err := s.EntryBySignoffLink(ctx, ind.SignoffLink); err != nil || e.Entry.Gymnast != "I" {
		t.Errorf("by sign-off link: %+v, %v", e, err)
	}
	if e, _ := s.IndividualEntry(ctx, token); e.SignoffLink != ind.SignoffLink {
		t.Error("the individual sees their sign-off link to send")
	}
	must(t, s.SignOffIndividual(ctx, ind.SignoffLink, "Coach C", true, ""))
	if e, _ := s.IndividualEntry(ctx, token); !e.SignedOff() || e.SignedBy != "Coach C" {
		t.Errorf("signed off: %+v", e)
	}
	must(t, s.ReplaceIndividualEntry(ctx, token, changed))
	if e, _ := s.IndividualEntry(ctx, token); e.SignedOff() {
		t.Error("a changed individual entry needs signing off again")
	}
}

func TestTimetableStorage(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp := competition()
	comp.Split = competitions.Split{Mode: competitions.SplitAll}
	c, admin, err := s.CreateCompetition(ctx, comp)
	must(t, err)
	got, _ := s.CompetitionByAdmin(ctx, admin)
	if got.Split.Mode != competitions.SplitAll || got.Timetable != nil {
		t.Errorf("split kept, no timetable yet: %+v", got)
	}
	must(t, s.SetSplit(ctx, c.ID, competitions.Split{Mode: competitions.SplitSome, Levels: []string{"BUCS L3"}}))
	plan := competitions.Schedule{Setup: competitions.DefaultSetup([]string{""}), Published: true,
		Flights: []competitions.ScheduledFlight{{Flight: competitions.Flight{Level: "BUCS L3", Number: 1, Of: 1, Entries: []string{"a", "b"}}, Area: "Panel 1", Start: 540, End: 560}}}
	must(t, s.SetTimetable(ctx, c.ID, &plan))
	got, _ = s.CompetitionByAdmin(ctx, admin)
	if !got.Split.Splits("BUCS L3") || got.Timetable == nil || !got.Timetable.Published || got.Timetable.Flights[0].Entries[1] != "b" || got.Timetable.Flights[0].Start != 540 {
		t.Errorf("split and timetable read back: %+v %+v", got.Split, got.Timetable)
	}
	must(t, s.SetTimetable(ctx, c.ID, nil))
	if got, _ := s.CompetitionByAdmin(ctx, admin); got.Timetable != nil {
		t.Error("timetable removed")
	}
}

func TestDisciplines(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp := competition()
	comp.Tumbling = []string{"Novice"}
	comp.Synchro = []competitions.Level{{Ref: "builtin-level:bucs-l3"}}
	c, admin, err := s.CreateCompetition(ctx, comp)
	must(t, err)
	if got, _ := s.CompetitionByAdmin(ctx, admin); len(got.Tumbling) != 1 || len(got.Synchro) != 1 || got.Synchro[0].Ref != "builtin-level:bucs-l3" {
		t.Errorf("the other events are kept: %+v", got)
	}
	must(t, s.SetEvents(ctx, c.ID, got0(comp.Synchro), []string{"Novice", "Elite"}, []string{"Open"}))
	if got, _ := s.CompetitionByAdmin(ctx, admin); len(got.Tumbling) != 2 || len(got.DMT) != 1 {
		t.Errorf("and can be changed: %+v", got)
	}
	club, _, _ := s.CreateClub(ctx, "UCD")
	a, _, _ := s.Join(ctx, club.ID, "A")
	b, bLink, _ := s.Join(ctx, club.ID, "B")
	must(t, s.AttachClub(ctx, club.ID, c.ID))

	// A enters trampoline, tumbling and synchro with B.
	must(t, s.SaveMemberEntry(ctx, a.ID, c.ID, entry("A")))
	must(t, s.SaveMemberEntry(ctx, a.ID, c.ID, competitions.Entry{Discipline: competitions.Tumbling, Level: "Novice"}))
	synchro := competitions.Entry{Discipline: competitions.Synchro, Level: "BUCS L3", Partner: &competitions.Partner{Name: "B", Club: "UCD"}}
	must(t, s.SaveMemberEntry(ctx, a.ID, c.ID, synchro))
	mine, err := s.MemberEntries(ctx, a.ID)
	must(t, err)
	if len(mine) != 3 {
		t.Fatalf("one entry per discipline: %+v", mine)
	}
	var link string
	for _, e := range mine {
		if e.Discipline == competitions.Synchro {
			link = e.PartnerLink
		}
	}
	if link == "" {
		t.Fatal("the synchro entry has a partner link")
	}

	// B confirms through the partner link.
	invite, err := s.PartnerByLink(ctx, link)
	must(t, err)
	if invite.Entry.Gymnast != "A" || invite.Club != "UCD" || invite.Confirmed || invite.CompetitionID != c.ID {
		t.Errorf("the invite: %+v", invite)
	}
	bMember, err := s.MemberByLink(ctx, bLink)
	must(t, err)
	must(t, s.ConfirmPartner(ctx, link, bMember.ID, ""))
	if invite, _ := s.PartnerByLink(ctx, link); !invite.Confirmed {
		t.Error("confirmed")
	}
	// Saving the same pair keeps the link and the confirmation; a new partner doesn't.
	must(t, s.SaveMemberEntry(ctx, a.ID, c.ID, synchro))
	if invite, err := s.PartnerByLink(ctx, link); err != nil || !invite.Confirmed {
		t.Error("the same partner stays confirmed")
	}
	synchro.Partner = &competitions.Partner{Name: "Someone else"}
	must(t, s.SaveMemberEntry(ctx, a.ID, c.ID, synchro))
	if _, err := s.PartnerByLink(ctx, link); !errors.Is(err, ErrNotFound) {
		t.Error("a new partner gets a new link")
	}

	// Sending sends all three; withdrawing one discipline leaves the others.
	_, err = s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	if list, _ := s.Entries(ctx, c.ID); len(list) != 3 {
		t.Errorf("three entries sent: %d", len(list))
	}
	must(t, s.WithdrawMemberEntry(ctx, a.ID, c.ID, competitions.Tumbling))
	list, _ := s.Entries(ctx, c.ID)
	withdrawn := 0
	for _, e := range list {
		if e.Withdrawn {
			withdrawn++
			if e.Entry.Discipline != competitions.Tumbling {
				t.Errorf("only tumbling is withdrawn: %+v", e)
			}
		}
	}
	if withdrawn != 1 {
		t.Errorf("one withdrawn: %d", withdrawn)
	}
	_, err = s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	if list, _ := s.Entries(ctx, c.ID); len(list) != 2 {
		t.Errorf("sending all takes back the tumbling entry: %d", len(list))
	}

	// An individual's synchro entry has its own partner link.
	ind, token, err := s.AddIndividualEntry(ctx, c.ID, competitions.Entry{Gymnast: "I", Discipline: competitions.Synchro, Level: "BUCS L3", Partner: &competitions.Partner{Name: "J"}})
	must(t, err)
	if ind.PartnerLink == "" {
		t.Fatal("an individual's synchro entry has a partner link")
	}
	must(t, s.ConfirmPartner(ctx, ind.PartnerLink, "", ""))
	if e, _ := s.IndividualEntry(ctx, token); !e.PartnerConfirmed || e.PartnerLink != ind.PartnerLink {
		t.Errorf("confirmed: %+v", e)
	}
	_ = b
}

func TestMigrationKeepsEntries(t *testing.T) {
	dir := t.TempDir()
	all := migrations
	migrations = all[:5] // a database from before events
	old, err := Open(ctx, dir)
	if err != nil {
		migrations = all
		t.Fatal(err)
	}
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	old.now = func() time.Time { return now }
	// The competition as the schema then had it (the code now writes more).
	c := Competition{ID: "c1"}
	_, err = old.db.Exec(`INSERT INTO competitions (id, admin_hash, club_token, club_hash, individual_token, individual_hash, name, date, deadline,
		individuals, levels, created_at, delete_after) VALUES ('c1', 'a', 'ct', 'ch', 'it', 'ih', 'Old', '2027-03-13', '2027-03-06T23:59:00.000000Z',
		TRUE, '[{"ref":"builtin-level:bucs-l3"}]', '2027-01-01T00:00:00.000000Z', '2027-07-11T00:00:00.000000Z')`)
	must(t, err)
	club, _, _ := old.CreateClub(ctx, "UCD")
	must(t, old.AttachClub(ctx, club.ID, c.ID))
	var memberID string
	must(t, old.db.QueryRow(`INSERT INTO members (id, club_id, token_hash, name, created_at) VALUES ('m1', $1, 'h', 'A', '2027-01-01T00:00:00.000000Z') RETURNING id`, club.ID).Scan(&memberID))
	_, err = old.db.Exec(`INSERT INTO member_entries (member_id, competition_id, entry, updated_at) VALUES ('m1', $1, '{"gymnast":"A","level":"BUCS L3"}', '2027-01-01T00:00:00.000000Z')`, c.ID)
	must(t, err)
	_, err = old.db.Exec(`INSERT INTO entries (id, competition_id, club_id, member_id, club_name, individual, gymnast, entry, sent_at)
		VALUES ('e1', $1, $2, 'm1', 'UCD', FALSE, 'A', '{"gymnast":"A","level":"BUCS L3"}', '2027-01-02T00:00:00.000000Z')`, c.ID, club.ID)
	must(t, err)
	old.Close()
	migrations = all

	s, err := Open(ctx, dir)
	must(t, err)
	defer s.Close()
	s.now = func() time.Time { return now }
	if mine, err := s.MemberEntries(ctx, memberID); err != nil || len(mine) != 1 || mine[0].Discipline != "" || !mine[0].Sent() {
		t.Errorf("the member's entry is kept, as trampoline, still sent: %+v, %v", mine, err)
	}
	if list, err := s.Entries(ctx, c.ID); err != nil || len(list) != 1 || list[0].Withdrawn {
		t.Errorf("the competition's copy is kept: %+v, %v", list, err)
	}
}

func TestLevelOrderMigration(t *testing.T) {
	dir := t.TempDir()
	all := migrations
	migrations = all[:7] // a database from before levels had an order
	old, err := Open(ctx, dir)
	if err != nil {
		migrations = all
		t.Fatal(err)
	}
	_, err = old.db.Exec(`INSERT INTO competitions (id, admin_hash, club_token, club_hash, individual_token, individual_hash, name, date, deadline,
		individuals, levels, created_at, delete_after, events) VALUES ('c1', 'a', 'ct', 'ch', 'it', 'ih', 'Old', '2027-03-13', '2027-03-06T23:59:00.000000Z',
		TRUE, '[{"ref":"builtin-level:bucs-l1"},{"ref":"builtin-level:bucs-l7"}]', '2027-01-01T00:00:00.000000Z', '2027-07-11T00:00:00.000000Z',
		'{"synchro":[{"ref":"builtin-level:bucs-l3"},{"ref":"builtin-level:bucs-l4"}],"tumbling":["Elite","Novice"]}')`)
	must(t, err)
	old.Close()
	migrations = all

	s, err := Open(ctx, dir)
	must(t, err)
	defer s.Close()
	c, err := s.Competition(ctx, "c1")
	must(t, err)
	if got := c.LevelOrder(competitions.Trampoline); !slices.Equal(got, []string{"BUCS L7", "BUCS L1"}) {
		t.Errorf("an old competition's levels are read easiest first: %v", got)
	}
	if got := c.LevelOrder(competitions.Synchro); !slices.Equal(got, []string{"BUCS L4", "BUCS L3"}) {
		t.Errorf("and its synchro levels: %v", got)
	}

	// Once the organiser moves a level, their order is kept as it is.
	moved := c.Competition
	must(t, moved.MoveLevel(competitions.Trampoline, 0, 1))
	must(t, moved.MoveLevel(competitions.Tumbling, 0, 1))
	must(t, s.SetLevelOrder(ctx, "c1", moved))
	c, err = s.Competition(ctx, "c1")
	must(t, err)
	if got := c.LevelOrder(competitions.Trampoline); !slices.Equal(got, []string{"BUCS L1", "BUCS L7"}) {
		t.Errorf("the organiser's order: %v", got)
	}
	if got := c.LevelOrder(competitions.Tumbling); !slices.Equal(got, []string{"Novice", "Elite"}) {
		t.Errorf("tumbling's order: %v", got)
	}
}

func got0[T any](v T) T { return v }

func TestOfficials(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, admin, _ := s.CreateCompetition(ctx, competition())
	club, _, _ := s.CreateClub(ctx, "UCD")
	a, _, _ := s.Join(ctx, club.ID, "A")
	b, _, _ := s.Join(ctx, club.ID, "B")
	judge := competitions.Offer{Judge: map[string]competitions.JudgeOffer{"": {UpTo: "BUCS L3", Chair: true}}}
	if err := s.SaveMemberOffer(ctx, a.ID, c.ID, judge); !errors.Is(err, ErrNotAttached) {
		t.Errorf("only for a competition the club is in: %v", err)
	}
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	must(t, s.SaveMemberOffer(ctx, a.ID, c.ID, judge))
	must(t, s.SaveMemberOffer(ctx, b.ID, c.ID, competitions.Offer{Marshal: true}))
	if o, _ := s.MemberOffer(ctx, a.ID, c.ID); !o.Judge[""].Chair {
		t.Errorf("A's offer: %+v", o)
	}
	if offers, _ := s.ClubOffers(ctx, club.ID, c.ID); len(offers) != 2 {
		t.Errorf("the club's offers: %+v", offers)
	}
	if list, _ := s.Officials(ctx, c.ID); len(list) != 0 {
		t.Error("the organiser sees nothing until the club sends")
	}

	// Sending takes the offers with the entries, even with no entries.
	_, err := s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	list, _ := s.Officials(ctx, c.ID)
	if len(list) != 2 || list[0].Name != "A" || list[0].ClubName != "UCD" || !list[0].Offer.Judge[""].Chair || list[1].Offer.Marshal != true {
		t.Fatalf("the club's officials: %+v", list)
	}
	must(t, s.SetQualified(ctx, c.ID, list[0].ID, true))

	// B stops offering; A changes theirs: the next send follows, keeping A's mark.
	must(t, s.SaveMemberOffer(ctx, b.ID, c.ID, competitions.Offer{}))
	judge.Recorder = true
	must(t, s.SaveMemberOffer(ctx, a.ID, c.ID, judge))
	_, err = s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	list, _ = s.Officials(ctx, c.ID)
	if len(list) != 1 || !list[0].Offer.Recorder || !list[0].Qualified {
		t.Errorf("only A, updated, still qualified: %+v", list)
	}

	// An individual offers on their entry; the organiser adds a judge with no club.
	ind, token, err := s.AddIndividualEntry(ctx, c.ID, entry("I"))
	must(t, err)
	must(t, s.SetIndividualOffer(ctx, token, competitions.Offer{Recorder: true}))
	if o, _ := s.IndividualOffer(ctx, ind.ID); !o.Recorder {
		t.Error("the individual's offer")
	}
	added, err := s.AddOfficial(ctx, c.ID, "Judge J", "", judge)
	must(t, err)
	list, _ = s.Officials(ctx, c.ID)
	if len(list) != 3 {
		t.Fatalf("A, I and J: %+v", list)
	}
	must(t, s.UpdateOfficial(ctx, c.ID, added.ID, competitions.Offer{Marshal: true}))
	if err := s.UpdateOfficial(ctx, c.ID, list[0].ID, competitions.Offer{}); !errors.Is(err, ErrNotFound) {
		t.Error("the organiser changes only the people they added")
	}
	if err := s.RemoveOfficial(ctx, c.ID, list[0].ID); !errors.Is(err, ErrNotFound) {
		t.Error("people who offered themselves can't be removed by the organiser")
	}
	must(t, s.RemoveOfficial(ctx, c.ID, added.ID))

	// Withdrawing the individual's entry takes their offer too.
	must(t, s.WithdrawIndividualEntry(ctx, token))
	if list, _ := s.Officials(ctx, c.ID); len(list) != 1 {
		t.Errorf("only A is left: %+v", list)
	}

	// The panels and judging rule.
	settings := competitions.OfficialSettings{Judge: competitions.JudgeBelow, Panels: map[string]competitions.Panel{competitions.Synchro: {Chair: 1, Execution: 4}}}
	must(t, s.SetOfficialSettings(ctx, c.ID, settings))
	if got, _ := s.CompetitionByAdmin(ctx, admin); got.Officials.Judge != competitions.JudgeBelow || got.Officials.Panel(competitions.Synchro).Execution != 4 {
		t.Errorf("settings kept: %+v", got.Officials)
	}
}

func TestCoachQualifications(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp := competition()
	comp.Signoff = true
	c, _, _ := s.CreateCompetition(ctx, comp)
	club, _, _ := s.CreateClub(ctx, "UCD")
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	ann, annLink, _ := s.CreateCoach(ctx, club.ID, "Ann")
	other, _, _ := s.CreateClub(ctx, "DCU")

	// Coaches sign off unless the club says not.
	if !ann.SignsOff {
		t.Error("a new coach signs off")
	}
	must(t, s.SetCoachSignsOff(ctx, club.ID, ann.ID, false))
	if got, _ := s.CoachByLink(ctx, annLink); got.SignsOff {
		t.Error("Ann no longer signs off")
	}
	x, _, _ := s.Join(ctx, club.ID, "X")
	must(t, s.SaveMemberEntry(ctx, x.ID, c.ID, entry("X")))
	got, _ := s.CoachByLink(ctx, annLink)
	if err := s.SignOff(ctx, got, x.ID, c.ID, competitions.Trampoline, true, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("a coach who doesn't sign off can't: %v", err)
	}
	if err := s.SetCoachSignsOff(ctx, other.ID, ann.ID, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("only Ann's club: %v", err)
	}

	// Qualifications with certificates, four at most, kept with the coach.
	pdf := []byte("%PDF-1.4 certificate")
	q, err := s.AddQualification(ctx, club.ID, ann.ID, "bg-trampoline-2", "application/pdf", pdf)
	must(t, err)
	if _, err := s.AddQualification(ctx, other.ID, ann.ID, "bg-trampoline-3", "application/pdf", pdf); !errors.Is(err, ErrNotFound) {
		t.Errorf("only for the club's own coaches: %v", err)
	}
	for range 3 {
		_, err := s.AddQualification(ctx, club.ID, ann.ID, "gi-tumbling-1", "image/png", []byte("png"))
		must(t, err)
	}
	if _, err := s.AddQualification(ctx, club.ID, ann.ID, "gi-dmt-1", "image/png", []byte("png")); !errors.Is(err, ErrLimit) {
		t.Errorf("four at most: %v", err)
	}
	qs, _ := s.Qualifications(ctx, club.ID)
	if len(qs[ann.ID]) != 4 || qs[ann.ID][0].Qualification != "bg-trampoline-2" || qs[ann.ID][0].CertType != "application/pdf" {
		t.Errorf("qualifications: %+v", qs)
	}
	if typ, data, err := s.Certificate(ctx, club.ID, q.ID); err != nil || typ != "application/pdf" || string(data) != string(pdf) {
		t.Errorf("certificate: %q %q %v", typ, data, err)
	}
	if _, _, err := s.Certificate(ctx, other.ID, q.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another club can't open it: %v", err)
	}
	if err := s.RemoveQualification(ctx, other.ID, q.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another club can't remove it: %v", err)
	}
	must(t, s.RemoveQualification(ctx, club.ID, q.ID))
	if qs, _ := s.Qualifications(ctx, club.ID); len(qs[ann.ID]) != 3 {
		t.Errorf("removed: %+v", qs)
	}
	// Removing the coach removes their certificates.
	must(t, s.RemoveCoach(ctx, club.ID, ann.ID))
	var n int
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM coach_qualifications`).Scan(&n))
	if n != 0 {
		t.Errorf("%d certificates left", n)
	}
}

func TestCoachApproval(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp := competition()
	comp.Signoff = true
	c, admin, _ := s.CreateCompetition(ctx, comp)
	must(t, s.SetApproveCoaches(ctx, c.ID, true, map[string]int{competitions.Trampoline: 3}))
	if got, _ := s.CompetitionByAdmin(ctx, admin); !got.ApproveCoaches || got.CoachLevel(competitions.Synchro) != 3 || got.CoachLevel(competitions.DMT) != competitions.DefaultCoachLevel {
		t.Errorf("the setting is kept: %+v %v", got.ApproveCoaches, got.CoachLevels)
	}
	club, _, _ := s.CreateClub(ctx, "UCD")
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	ann, annLink, _ := s.CreateCoach(ctx, club.ID, "Ann")
	bob, _, _ := s.CreateCoach(ctx, club.ID, "Bob")
	must(t, s.SetCoachSignsOff(ctx, club.ID, bob.ID, false))
	q, _ := s.AddQualification(ctx, club.ID, ann.ID, "bg-trampoline-3", "application/pdf", []byte("%PDF"))
	x, _, _ := s.Join(ctx, club.ID, "X")
	must(t, s.SaveMemberEntry(ctx, x.ID, c.ID, entry("X")))
	coach, _ := s.CoachByLink(ctx, annLink)
	signed := func() bool {
		t.Helper()
		now = now.Add(time.Minute)
		_, err := s.Send(ctx, club.ID, c.ID, nil)
		must(t, err)
		es, err := s.Entries(ctx, c.ID)
		must(t, err)
		return es[0].SignedOff()
	}
	must(t, s.SignOff(ctx, coach, x.ID, c.ID, competitions.Trampoline, true, ""))
	if signed() {
		t.Error("Ann isn't approved yet")
	}

	// Sent, only Ann (Bob doesn't sign off), waiting.
	must(t, s.SendCoaches(ctx, club.ID, c.ID, []string{ann.ID, bob.ID}))
	sent, _ := s.CompetitionCoaches(ctx, c.ID)
	if len(sent) != 1 || sent[0].Name != "Ann" || sent[0].ClubName != "UCD" || sent[0].Status != CoachWaiting || len(sent[0].Qualifications) != 1 || sent[0].Changed {
		t.Fatalf("sent: %+v", sent)
	}
	if typ, _, err := s.CompetitionCertificate(ctx, c.ID, ann.ID, q.ID); err != nil || typ != "application/pdf" {
		t.Errorf("the organiser can open her certificate: %v", err)
	}
	other, _, _ := s.CreateCompetition(ctx, competition())
	if _, _, err := s.CompetitionCertificate(ctx, other.ID, ann.ID, q.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("not another competition: %v", err)
	}

	// Approved: her sign-off counts. Withdrawn: it doesn't.
	must(t, s.DecideCoach(ctx, c.ID, ann.ID, CoachApproved, "", false))
	if !signed() {
		t.Error("Ann is approved")
	}
	must(t, s.DecideCoach(ctx, c.ID, ann.ID, CoachWithdrawn, "Certificate expired", false))
	if signed() {
		t.Error("her approval was withdrawn")
	}
	// Approved again from now on: the old sign-off doesn't count, a new one does.
	must(t, s.DecideCoach(ctx, c.ID, ann.ID, CoachApproved, "", true))
	if signed() {
		t.Error("only sign-offs from now on")
	}
	now = now.Add(time.Minute)
	must(t, s.SignOff(ctx, coach, x.ID, c.ID, competitions.Trampoline, true, ""))
	if !signed() {
		t.Error("signed off again")
	}
	// Withdrawn and approved again keeping them: it counts again.
	must(t, s.DecideCoach(ctx, c.ID, ann.ID, CoachWithdrawn, "", false))
	must(t, s.DecideCoach(ctx, c.ID, ann.ID, CoachApproved, "", false))
	if !signed() {
		t.Error("approved again with her sign-offs")
	}

	// A new qualification: changed since sent; sent again, she waits again.
	_, err := s.AddQualification(ctx, club.ID, ann.ID, "gi-trampoline-3", "image/png", []byte("png"))
	must(t, err)
	if sent, _ := s.ClubSentCoaches(ctx, club.ID, c.ID); !sent[ann.ID].Changed || !sent[ann.ID].Approved() {
		t.Errorf("changed, still approved until sent again: %+v", sent[ann.ID])
	}
	must(t, s.SendCoaches(ctx, club.ID, c.ID, []string{ann.ID}))
	if sent, _ := s.CoachSentTo(ctx, ann.ID); sent[c.ID].Status != CoachWaiting || sent[c.ID].Changed || len(sent[c.ID].Qualifications) != 2 {
		t.Errorf("waiting again: %+v", sent[c.ID])
	}
	// Taken back: no longer sent.
	must(t, s.SendCoaches(ctx, club.ID, c.ID, nil))
	if sent, _ := s.CompetitionCoaches(ctx, c.ID); len(sent) != 0 {
		t.Errorf("taken back: %+v", sent)
	}
	// A club not entered can't send.
	far, _, _ := s.CreateClub(ctx, "Far")
	if err := s.SendCoaches(ctx, far.ID, c.ID, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("not entered: %v", err)
	}

	// Without approval, any coach's sign-off counts.
	must(t, s.SetApproveCoaches(ctx, c.ID, false, nil))
	if !signed() {
		t.Error("approval off")
	}
}

func TestEntryCoaches(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp := competition()
	comp.Signoff, comp.Individuals = true, true
	c, _, _ := s.CreateCompetition(ctx, comp)
	must(t, s.SetApproveCoaches(ctx, c.ID, true, nil))
	e, token, err := s.AddIndividualEntry(ctx, c.ID, entry("Mary"))
	must(t, err)
	got, _ := s.IndividualEntry(ctx, token)
	signed := func() bool {
		t.Helper()
		got, err := s.IndividualEntry(ctx, token)
		must(t, err)
		return got.SignedOff()
	}
	must(t, s.SignOffIndividual(ctx, got.SignoffLink, "Ann", true, ""))
	if signed() {
		t.Error("no coach named yet")
	}
	if _, err := s.EntryCoachOf(ctx, e.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("none: %v", err)
	}
	must(t, s.SetEntryCoach(ctx, token, "Ann", "gi-trampoline-2", "application/pdf", []byte("%PDF Ann")))
	ec, err := s.EntryCoachOf(ctx, e.ID)
	if err != nil || ec.Name != "Ann" || ec.Gymnast != "Mary" || ec.Status != CoachWaiting {
		t.Fatalf("named: %+v %v", ec, err)
	}
	if typ, data, err := s.EntryCertificate(ctx, c.ID, e.ID); err != nil || typ != "application/pdf" || string(data) != "%PDF Ann" {
		t.Errorf("certificate: %v", err)
	}
	other, _, _ := s.CreateCompetition(ctx, competition())
	if _, _, err := s.EntryCertificate(ctx, other.ID, e.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("only for its competition: %v", err)
	}
	if err := s.DecideEntryCoach(ctx, other.ID, e.ID, CoachApproved, "", false); !errors.Is(err, ErrNotFound) {
		t.Errorf("only its organiser decides: %v", err)
	}
	must(t, s.DecideEntryCoach(ctx, c.ID, e.ID, CoachApproved, "", false))
	if !signed() {
		t.Error("approved: Ann's sign-off counts")
	}
	must(t, s.DecideEntryCoach(ctx, c.ID, e.ID, CoachWithdrawn, "", false))
	if signed() {
		t.Error("withdrawn")
	}
	now = now.Add(time.Minute)
	must(t, s.DecideEntryCoach(ctx, c.ID, e.ID, CoachApproved, "", true))
	if signed() {
		t.Error("afresh")
	}
	// Named again: waiting again.
	must(t, s.SetEntryCoach(ctx, token, "Bob", "bg-trampoline-3", "image/png", []byte("png")))
	if list, _ := s.CompetitionEntryCoaches(ctx, c.ID); len(list) != 1 || list[0].Name != "Bob" || list[0].Status != CoachWaiting {
		t.Errorf("Bob waits: %+v", list)
	}
	// Withdrawing the entry removes its coach and certificate.
	must(t, s.WithdrawIndividualEntry(ctx, token))
	if list, _ := s.CompetitionEntryCoaches(ctx, c.ID); len(list) != 0 {
		t.Errorf("gone with the entry: %+v", list)
	}
	// After the deadline, no coach can be named.
	_, token, _ = s.AddIndividualEntry(ctx, c.ID, entry("Nell"))
	now = comp.Deadline.Add(time.Hour)
	if err := s.SetEntryCoach(ctx, token, "Ann", "gi-trampoline-2", "image/png", []byte("png")); !errors.Is(err, ErrClosed) {
		t.Errorf("closed: %v", err)
	}
}

func TestPairEntries(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp := competition()
	comp.Individuals = true
	comp.Synchro = comp.Levels
	c, _, _ := s.CreateCompetition(ctx, comp)
	ucd, _, _ := s.CreateClub(ctx, "UCD")
	dcu, _, _ := s.CreateClub(ctx, "DCU")
	must(t, s.AttachClub(ctx, ucd.ID, c.ID))
	a, _, _ := s.Join(ctx, ucd.ID, "A")
	b, _, _ := s.Join(ctx, dcu.ID, "B") // DCU isn't entered
	dcuCoach, _, _ := s.CreateCoach(ctx, dcu.ID, "Dee")
	must(t, s.SetMemberCoach(ctx, dcu.ID, b.ID, dcuCoach.ID))

	// A (UCD) enters synchro with B (DCU).
	must(t, s.SaveMemberEntry(ctx, a.ID, c.ID, competitions.Entry{Discipline: competitions.Synchro, Level: "BUCS L3", Partner: &competitions.Partner{Name: "B", Club: "DCU"}}))
	mine, _ := s.MemberEntries(ctx, a.ID)
	if got, _ := s.MemberPairEntries(ctx, b.ID); len(got) != 0 {
		t.Errorf("nothing until B confirms: %+v", got)
	}
	must(t, s.ConfirmPartner(ctx, mine[0].PartnerLink, b.ID, ""))
	got, err := s.MemberPairEntries(ctx, b.ID)
	must(t, err)
	if len(got) != 1 || got[0].Entrant != "A" || got[0].Club != "UCD" || got[0].EntrantMemberID != a.ID || got[0].CopyID != "" || got[0].Entry.Partner.Name != "B" {
		t.Fatalf("B sees the pair's entry, not sent yet: %+v", got)
	}
	_, err = s.Send(ctx, ucd.ID, c.ID, nil)
	must(t, err)
	if got, _ := s.MemberPairEntries(ctx, b.ID); got[0].CopyID == "" {
		t.Error("sent: the competition's copy")
	}
	coach, _ := s.Coaches(ctx, dcu.ID)
	if got, _ := s.CoachPairEntries(ctx, coach[0]); len(got) != 1 {
		t.Errorf("B's coach sees it: %+v", got)
	}
	if got, _ := s.ClubPairEntries(ctx, dcu.ID); len(got) != 1 {
		t.Errorf("B's club sees it: %+v", got)
	}
	if got, _ := s.ClubPairEntries(ctx, ucd.ID); len(got) != 0 {
		t.Errorf("not as UCD's partner entries: %+v", got)
	}

	// An individual enters with B too; and an individual partner of A's.
	ind, _, _ := s.AddIndividualEntry(ctx, c.ID, competitions.Entry{Gymnast: "I", Discipline: competitions.Synchro, Level: "BUCS L3", Partner: &competitions.Partner{Name: "B"}})
	must(t, s.ConfirmPartner(ctx, ind.PartnerLink, b.ID, ""))
	if got, _ := s.MemberPairEntries(ctx, b.ID); len(got) != 2 || got[1].Club != "" || got[1].CopyID != ind.ID {
		t.Errorf("an individual's pair entry: %+v", got)
	}
	j, _, _ := s.AddIndividualEntry(ctx, c.ID, competitions.Entry{Gymnast: "J", Discipline: competitions.Synchro, Level: "BUCS L3", Partner: &competitions.Partner{Name: "K"}})
	k, _, _ := s.AddIndividualEntry(ctx, c.ID, entry("K"))
	must(t, s.ConfirmPartner(ctx, j.PartnerLink, "", k.ID))
	if got, _ := s.IndividualPairEntries(ctx, k.ID); len(got) != 1 || got[0].Entry.Gymnast != "J" {
		t.Errorf("K, an individual, sees J's: %+v", got)
	}
}

func TestRemovingEntries(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp := competition()
	comp.Individuals = true
	c, _, _ := s.CreateCompetition(ctx, comp)
	club, _, _ := s.CreateClub(ctx, "UCD")
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	x, _, _ := s.Join(ctx, club.ID, "X")
	y, _, _ := s.Join(ctx, club.ID, "Y")
	for _, m := range []Member{x, y} {
		must(t, s.SaveMemberEntry(ctx, m.ID, c.ID, entry(m.Name)))
	}
	_, err := s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	ind, token, _ := s.AddIndividualEntry(ctx, c.ID, entry("I"))
	all, _ := s.Entries(ctx, c.ID)
	id := map[string]string{}
	for _, e := range all {
		id[e.Entry.Gymnast] = e.ID
	}

	// X removed (reason for the club), Y held (for both), I held.
	if n, err := s.RemoveEntries(ctx, c.ID, []string{id["X"]}, Removed, "Not paid", ToClub); err != nil || n != 1 {
		t.Fatalf("removed: %d %v", n, err)
	}
	must2(s.RemoveEntries(ctx, c.ID, []string{id["Y"], ind.ID}, Held, "Fix the second routine", ToBoth))
	if left, _ := s.Entries(ctx, c.ID); len(left) != 0 {
		t.Errorf("none left in: %+v", left)
	}
	gone, _ := s.RemovedEntries(ctx, c.ID)
	if len(gone) != 3 {
		t.Fatalf("listed: %+v", gone)
	}
	byClub, _ := s.ClubRemovals(ctx, club.ID, c.ID)
	if e := byClub[x.ID+"/"]; e.Removal != Removed || e.RemovalNote != "Not paid" || !e.ForClub() || e.ForMember() {
		t.Errorf("the club sees X removed, and why: %+v", e)
	}
	if mine, _ := s.MemberRemovals(ctx, y.ID); mine[c.ID+"/"].Removal != Held || !mine[c.ID+"/"].ForMember() {
		t.Errorf("Y sees theirs held: %+v", mine)
	}

	// Sending again: X stays removed, unchanged; Y unchanged isn't resent;
	// changed, it waits for the organiser.
	n, err := s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
	if n != 1 {
		t.Errorf("only Y is sent: %d", n)
	}
	if gone, _ := s.RemovedEntries(ctx, c.ID); gone[1].Resent || gone[2].Resent {
		t.Errorf("Y unchanged: not resent: %+v", gone)
	}
	changed := entry("Y")
	changed.Level = "BUCS L4"
	must(t, s.SaveMemberEntry(ctx, y.ID, c.ID, changed))
	must2(s.Send(ctx, club.ID, c.ID, nil))
	must(t, s.ReplaceIndividualEntry(ctx, token, entry("I2")))
	for _, e := range must3(s.RemovedEntries(ctx, c.ID)) {
		if (e.ID == id["Y"] || e.ID == ind.ID) != e.Resent {
			t.Errorf("%s resent: %v", e.Entry.Gymnast, e.Resent)
		}
	}
	// The organiser accepts Y back in, and restores X; X can be sent again.
	must(t, s.RestoreEntry(ctx, c.ID, id["Y"]))
	must(t, s.RestoreEntry(ctx, c.ID, id["X"]))
	if err := s.RestoreEntry(ctx, c.ID, id["X"]); !errors.Is(err, ErrNotFound) {
		t.Errorf("already restored: %v", err)
	}
	if left, _ := s.Entries(ctx, c.ID); len(left) != 2 {
		t.Errorf("X and Y back in: %+v", left)
	}

	// A removed individual can't change their entry.
	must2(s.RemoveEntries(ctx, c.ID, []string{ind.ID}, Removed, "", ToBoth))
	if err := s.ReplaceIndividualEntry(ctx, token, entry("I3")); !errors.Is(err, ErrRemoved) {
		t.Errorf("blocked: %v", err)
	}
	// Another competition's entries can't be touched.
	other, _, _ := s.CreateCompetition(ctx, competition())
	if n, _ := s.RemoveEntries(ctx, other.ID, []string{id["Y"]}, Removed, "", ToBoth); n != 0 {
		t.Error("only its own entries")
	}
}

func must2[T any](_ T, err error) {
	if err != nil {
		panic(err)
	}
}

func must3[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestDraftAndPublishedTimetables(t *testing.T) {
	// A timetable published before drafts is its own published copy.
	dir := t.TempDir()
	all := migrations
	migrations = all[:13]
	old, err := Open(ctx, dir)
	if err != nil {
		migrations = all
		t.Fatal(err)
	}
	_, err = old.db.Exec(`INSERT INTO competitions (id, admin_hash, club_token, club_hash, individual_token, individual_hash, name, date, deadline,
		individuals, levels, created_at, delete_after, timetable) VALUES ('c1', 'a', 'ct', 'ch', 'it', 'ih', 'Old', '2027-03-13', '2027-03-06T23:59:00.000000Z',
		TRUE, '[{"ref":"builtin-level:bucs-l3"}]', '2027-01-01T00:00:00.000000Z', '2027-07-11T00:00:00.000000Z',
		'{"setup":{"areas":null,"days":null,"separate":{}},"flights":[],"published":true,"planned":true}'),
		('c2', 'b', 'ct2', 'ch2', 'it2', 'ih2', 'Unpublished', '2027-03-13', '2027-03-06T23:59:00.000000Z',
		TRUE, '[{"ref":"builtin-level:bucs-l3"}]', '2027-01-01T00:00:00.000000Z', '2027-07-11T00:00:00.000000Z',
		'{"setup":{"areas":null,"days":null,"separate":{}},"flights":[],"planned":true}')`)
	must(t, err)
	old.Close()
	migrations = all
	s, err := Open(ctx, dir)
	must(t, err)
	defer s.Close()
	if c, _ := s.Competition(ctx, "c1"); c.Published == nil || c.Timetable == nil {
		t.Error("published before: published now")
	}
	if c, _ := s.Competition(ctx, "c2"); c.Published != nil {
		t.Error("never published: not now either")
	}

	// The draft changes; the published copy stays until published again.
	c, _ := s.Competition(ctx, "c2")
	draft := *c.Timetable
	must(t, s.PublishTimetable(ctx, "c2", &draft))
	draft.Flights = []competitions.ScheduledFlight{{Flight: competitions.Flight{Level: "BUCS L3"}, Area: "Panel 1", Start: 540, End: 560}}
	must(t, s.SetTimetable(ctx, "c2", &draft))
	c, _ = s.Competition(ctx, "c2")
	if len(c.Timetable.Flights) != 1 || c.Published == nil || len(c.Published.Flights) != 0 || !c.Published.Published {
		t.Errorf("draft and published apart: %+v / %+v", c.Timetable, c.Published)
	}
	must(t, s.PublishTimetable(ctx, "c2", nil))
	if c, _ := s.Competition(ctx, "c2"); c.Published != nil || c.Timetable == nil {
		t.Error("taken down, the draft kept")
	}
}

func TestGoingLive(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp := competition()
	comp.LiveAt = time.Time{}
	c, admin, _ := s.CreateCompetition(ctx, comp)
	if _, _, err := s.AddIndividualEntry(ctx, c.ID, entry("Ann")); !errors.Is(err, ErrNotOpen) {
		t.Errorf("a private competition takes no entries: %v", err)
	}

	// Going live later: not open until then.
	must(t, s.SetLive(ctx, c.ID, now.Add(time.Hour)))
	if _, _, err := s.AddIndividualEntry(ctx, c.ID, entry("Ann")); !errors.Is(err, ErrNotOpen) {
		t.Errorf("not live yet: %v", err)
	}
	now = now.Add(time.Hour)
	_, token, err := s.AddIndividualEntry(ctx, c.ID, entry("Ann"))
	must(t, err)
	if got, _ := s.CompetitionByAdmin(ctx, admin); !got.LiveAt.Equal(now) || !got.Open(now) {
		t.Errorf("live from then: %v", got.LiveAt)
	}

	// Private again pauses entries, keeping those made.
	must(t, s.SetLive(ctx, c.ID, time.Time{}))
	if err := s.ReplaceIndividualEntry(ctx, token, entry("Ann")); !errors.Is(err, ErrNotOpen) {
		t.Errorf("paused: %v", err)
	}
	if got, _ := s.Entries(ctx, c.ID); len(got) != 1 {
		t.Errorf("entries made are kept: %d", len(got))
	}
	if got, _ := s.CompetitionByAdmin(ctx, admin); !got.LiveAt.IsZero() {
		t.Errorf("private: %v", got.LiveAt)
	}
}

func TestSubscriptions(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition())
	sub := Subscription{CompetitionID: c.ID, Kind: NotifyMember, OwnerID: "m1", Channel: ByEmail, Address: "ann@example.com", Topics: []string{AboutCards, "nonsense"}, Page: "/p"}
	got, token, err := s.Subscribe(ctx, sub)
	must(t, err)
	if token == "" || got.Confirmed || len(got.Topics) != 1 {
		t.Errorf("waiting to be confirmed, cards only: %+v", got)
	}
	if all, _ := s.CompetitionSubscriptions(ctx, c.ID); len(all) != 0 {
		t.Error("an unconfirmed email isn't told")
	}
	must(t, s.ConfirmSubscription(ctx, token))

	// The same address keeps its confirmation; another needs confirming.
	sub.Topics = []string{AboutTimetable, AboutCards}
	if got, token, _ := s.Subscribe(ctx, sub); token != "" || !got.Confirmed || !got.About(AboutTimetable) {
		t.Errorf("the same address: %+v %q", got, token)
	}
	sub.Address = "bea@example.com"
	if got, token, _ := s.Subscribe(ctx, sub); token == "" || got.Confirmed {
		t.Errorf("a new address: %+v", got)
	}
	if mine, _ := s.Subscriptions(ctx, c.ID, NotifyMember, "m1"); len(mine) != 1 || mine[0].Address != "bea@example.com" {
		t.Errorf("one email each: %+v", mine)
	}

	// Waiting changes: the first change's baseline stays; claimed when due.
	must(t, s.MarkChanged(ctx, c.ID, []byte("first"), now.Add(10*time.Minute)))
	must(t, s.MarkChanged(ctx, c.ID, []byte("second"), now.Add(20*time.Minute)))
	if due, _ := s.ClaimDue(ctx); len(due) != 0 {
		t.Error("not due yet")
	}
	now = now.Add(10 * time.Minute)
	if due, _ := s.ClaimDue(ctx); len(due) != 1 || string(due[0].Baseline) != "first" {
		t.Errorf("due: %+v", due)
	}
	if due, _ := s.ClaimDue(ctx); len(due) != 0 {
		t.Error("claimed once")
	}

	// Deleted with the competition.
	must(t, s.DeleteCompetition(ctx, c.ID))
	if mine, _ := s.Subscriptions(ctx, c.ID, NotifyMember, "m1"); len(mine) != 0 {
		t.Error("deleted with the competition")
	}
}

// Sign-offs from before coaches were recorded (migration 11) are matched to
// the club's coach of that name, if there's exactly one (migration 18).
func TestOldSignoffsMatchedByName(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition())
	club, _, _ := s.CreateClub(ctx, "UCD")
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	declan, _, _ := s.CreateCoach(ctx, club.ID, "Declan Ward")
	s.CreateCoach(ctx, club.ID, "Pat Kelly")
	s.CreateCoach(ctx, club.ID, "Pat Kelly") // two of that name: can't tell which
	var members []Member
	for _, name := range []string{"X", "Y", "Z"} {
		m, _, _ := s.Join(ctx, club.ID, name)
		must(t, s.SaveMemberEntry(ctx, m.ID, c.ID, entry(name)))
		members = append(members, m)
	}
	_, err := s.Send(ctx, club.ID, c.ID, []string{members[0].ID, members[1].ID, members[2].ID})
	must(t, err)
	// As they were before migration 11: a name and no coach.
	for i, by := range []string{" declan ward ", "Pat Kelly", "Someone Else"} {
		for _, table := range []string{"member_entries", "entries"} {
			_, err := s.db.ExecContext(ctx, `UPDATE `+table+` SET signed_at = $1, signed_by = $2, signed_coach = '' WHERE member_id = $3`, s.stamp(), by, members[i].ID)
			must(t, err)
		}
	}
	_, err = s.db.ExecContext(ctx, migrations[17])
	must(t, err)
	coachOf := func(table, member string) string {
		var id string
		must(t, s.db.QueryRowContext(ctx, `SELECT signed_coach FROM `+table+` WHERE member_id = $1`, member).Scan(&id))
		return id
	}
	for _, table := range []string{"member_entries", "entries"} {
		if got := coachOf(table, members[0].ID); got != declan.ID {
			t.Errorf("%s: Declan's, whatever the case and spaces round it: %q", table, got)
		}
		if coachOf(table, members[1].ID) != "" || coachOf(table, members[2].ID) != "" {
			t.Errorf("%s: two coaches of that name, or none: left alone", table)
		}
	}
}

func TestWaitingLists(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp := competition()
	c, _, _ := s.CreateCompetition(ctx, comp)
	var tokens []string
	for _, name := range []string{"Ann", "Bea", "Cat", "Dee"} {
		_, token, err := s.AddIndividualEntry(ctx, c.ID, entry(name))
		must(t, err)
		tokens = append(tokens, token)
		now = now.Add(time.Minute)
	}
	places := func() map[string]int {
		t.Helper()
		entries, err := s.Entries(ctx, c.ID)
		must(t, err)
		out := map[string]int{}
		for _, e := range entries {
			out[e.Entry.Gymnast] = e.Waiting
		}
		return out
	}
	if p := places(); p["Dee"] != 0 {
		t.Error("no limit, no waiting")
	}
	must(t, s.SetLimits(ctx, c.ID, map[string]int{"BUCS L3": 2}))
	if p := places(); p["Ann"] != 0 || p["Bea"] != 0 || p["Cat"] != 1 || p["Dee"] != 2 {
		t.Errorf("first come, first in: %v", p)
	}
	// Changing an entry keeps its place.
	must(t, s.ReplaceIndividualEntry(ctx, tokens[0], entry("Ann")))
	if p := places(); p["Ann"] != 0 || p["Cat"] != 1 {
		t.Errorf("Ann keeps her place: %v", p)
	}
	// Let in over the limit: no one else moves.
	entries, _ := s.Entries(ctx, c.ID)
	var dee string
	for _, e := range entries {
		if e.Entry.Gymnast == "Dee" {
			dee = e.ID
		}
	}
	must(t, s.LetIn(ctx, c.ID, dee, true))
	if p := places(); p["Dee"] != 0 || p["Cat"] != 1 || p["Bea"] != 0 {
		t.Errorf("Dee let in over the limit: %v", p)
	}
	// Bea withdraws: Cat moves up.
	must(t, s.WithdrawIndividualEntry(ctx, tokens[1]))
	if p := places(); p["Cat"] != 0 {
		t.Errorf("Cat moves up: %v", p)
	}
}

func TestLaterDeadlines(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp := competition()
	c, _, _ := s.CreateCompetition(ctx, comp)
	_, token, err := s.AddIndividualEntry(ctx, c.ID, entry("Ann"))
	must(t, err)
	must(t, s.SetChangesAndSignoffs(ctx, c.ID, comp.Deadline.Add(48*time.Hour), comp.Deadline.Add(72*time.Hour)))
	now = comp.Deadline.Add(time.Hour)
	if _, _, err := s.AddIndividualEntry(ctx, c.ID, entry("Bea")); !errors.Is(err, ErrClosed) {
		t.Errorf("no new entries after the deadline: %v", err)
	}
	must(t, s.ReplaceIndividualEntry(ctx, token, entry("Ann")))
	now = comp.Deadline.Add(49 * time.Hour)
	if err := s.ReplaceIndividualEntry(ctx, token, entry("Ann")); !errors.Is(err, ErrClosed) {
		t.Errorf("no changes after changes close: %v", err)
	}
	if c2, err := s.Competition(ctx, c.ID); err != nil || !c2.SignoffsOpen(now) || c2.ChangesOpen(now) || c2.Open(now) {
		t.Errorf("sign-offs still open, changes and entries not: %v", err)
	}
}

func TestFlightTimes(t *testing.T) {
	now := time.Date(2027, 3, 13, 9, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition())
	const key = "0|Panel 1|BUCS L3||1"
	times := func() FlightTime {
		t.Helper()
		all, err := s.FlightTimes(ctx, c.ID)
		must(t, err)
		return all[key]
	}
	if got := times(); !got.Started.IsZero() {
		t.Errorf("nothing yet: %+v", got)
	}
	must(t, s.MarkFlight(ctx, c.ID, key, "start", "Ann"))
	if got := times(); !got.Started.Equal(now) || !got.Finished.IsZero() || got.Who != "Ann" {
		t.Errorf("started: %+v", got)
	}
	now = now.Add(40 * time.Minute)
	must(t, s.MarkFlight(ctx, c.ID, key, "finish", "Bea"))
	if got := times(); !got.Started.Equal(now.Add(-40*time.Minute)) || !got.Finished.Equal(now) || got.Who != "Bea" {
		t.Errorf("finished: %+v", got)
	}
	must(t, s.MarkFlight(ctx, c.ID, key, "undo", "Bea"))
	if got := times(); got.Started.IsZero() || !got.Finished.IsZero() {
		t.Errorf("undo takes back the finish: %+v", got)
	}
	must(t, s.MarkFlight(ctx, c.ID, key, "undo", "Bea"))
	if got := times(); !got.Started.IsZero() || !got.Finished.IsZero() {
		t.Errorf("undo again takes back the start: %+v", got)
	}
	if err := s.MarkFlight(ctx, c.ID, key, "nonsense", "Bea"); err == nil {
		t.Error("an unknown mark is refused")
	}
}

func TestCheckins(t *testing.T) {
	now := time.Date(2027, 3, 13, 9, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition())
	all := func() map[string]Checkin {
		t.Helper()
		got, err := s.Checkins(ctx, c.ID)
		must(t, err)
		return got
	}
	if got := all(); len(got) != 0 {
		t.Errorf("nothing yet: %+v", got)
	}
	must(t, s.SetCheckin(ctx, c.ID, "a", CheckedIn, "Ann"))
	must(t, s.SetCheckin(ctx, c.ID, "b", Scratched, "Bea"))
	got := all()
	if len(got) != 2 || got["a"].Status != CheckedIn || got["a"].Who != "Ann" || !got["a"].At.Equal(now) || got["b"].Status != Scratched {
		t.Errorf("here and scratched: %+v", got)
	}
	now = now.Add(time.Hour)
	must(t, s.SetCheckin(ctx, c.ID, "a", Scratched, "Cal"))
	if got := all()["a"]; got.Status != Scratched || got.Who != "Cal" || !got.At.Equal(now) {
		t.Errorf("changing the mark replaces it: %+v", got)
	}
	must(t, s.SetCheckin(ctx, c.ID, "a", "", "Cal"))
	if got := all(); len(got) != 1 || got["b"].Status != Scratched {
		t.Errorf("clear takes the mark away: %+v", got)
	}
	if err := s.SetCheckin(ctx, c.ID, "a", "nonsense", "Cal"); err == nil {
		t.Error("an unknown status is refused")
	}
}
