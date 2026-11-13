package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"tariffCalculator/competitions"
)

// clubWithEntries is a competition with a club's two members' entries sent,
// signed off by an approved coach: A by Ann, B by Ann, and an individual.
func clubWithEntries(t *testing.T, now *time.Time) (s *Store, c Competition, club Club, a, b Member, ann Coach) {
	t.Helper()
	s = open(t, now)
	comp := competition()
	comp.Signoff = true
	made, _, err := s.CreateCompetition(ctx, comp)
	must(t, err)
	must(t, s.SetApproveCoaches(ctx, made.ID, true, map[string]int{}))
	club, _, _ = s.CreateClub(ctx, "UCD")
	must(t, s.AttachClub(ctx, club.ID, made.ID))
	a, _, _ = s.Join(ctx, club.ID, "A")
	b, _, _ = s.Join(ctx, club.ID, "B")
	ann, annLink, _ := s.CreateCoach(ctx, club.ID, "Ann Byrne")
	_, err = s.AddQualification(ctx, club.ID, ann.ID, "bg-trampoline-3", "application/pdf", []byte("%PDF"))
	must(t, err)
	must(t, s.SaveMemberEntry(ctx, a.ID, made.ID, entry("A")))
	must(t, s.SaveMemberEntry(ctx, b.ID, made.ID, entry("B")))
	coach, _ := s.CoachByLink(ctx, annLink)
	must(t, s.SignOff(ctx, coach, a.ID, made.ID, competitions.Trampoline, true, ""))
	must(t, s.SignOff(ctx, coach, b.ID, made.ID, competitions.Trampoline, true, ""))
	_, err = s.Send(ctx, club.ID, made.ID, nil)
	must(t, err)
	must(t, s.SendCoaches(ctx, club.ID, made.ID, []string{ann.ID}))
	must(t, s.DecideCoach(ctx, made.ID, ann.ID, CoachApproved, "", false))
	_, _, err = enter(s, made.ID, entry("I"))
	must(t, err)
	c, err = s.Competition(ctx, made.ID)
	must(t, err)
	return s, c, club, a, b, ann
}

// byGymnast are a competition's entries, by gymnast.
func byGymnast(t *testing.T, s *Store, competitionID string) map[string]Entry {
	t.Helper()
	list, err := s.Entries(ctx, competitionID)
	must(t, err)
	out := map[string]Entry{}
	for _, e := range list {
		out[e.Entry.Gymnast] = e
	}
	return out
}

func TestRemovedMemberStays(t *testing.T) {
	for _, late := range []bool{false, true} {
		now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
		s, c, club, a, _, _ := clubWithEntries(t, &now)
		if late {
			now = c.Deadline.Add(time.Hour) // after entries closed: the club couldn't withdraw it
			if err := s.WithdrawMemberEntry(ctx, a.ID, c.ID, ""); !errors.Is(err, ErrClosed) {
				t.Fatalf("withdrawing is closed: %v", err)
			}
		}
		before := s.Writes(c.ID)
		must(t, s.RemoveMember(ctx, club.ID, a.ID, "Club admin"))
		if s.Writes(c.ID) == before {
			t.Error("the competition's pages are worked out again")
		}
		got := byGymnast(t, s, c.ID)
		e, ok := got["A"]
		if !ok {
			t.Fatalf("late %v: A's entry is still listed: %+v", late, got)
		}
		if !e.ClubRemoved() || !e.Withdrawn || e.ClubRemovedBy != "Club admin" || e.ClubRemovedLate != late || e.ClubName != "UCD" || e.Entry.Level != "BUCS L3" {
			t.Errorf("late %v: removed by the club, out as a withdrawn one is: %+v", late, e)
		}
		if e.Key() != MemberKey(a.ID) {
			t.Errorf("their key stays, for their page: %q", e.Key())
		}
		if !e.SignedOff() {
			t.Error("its sign-off stays as sent")
		}
		if got["B"].Withdrawn || got["I"].Withdrawn {
			t.Errorf("only A's: %+v", got)
		}
		history, _ := s.History(ctx, c.ID, 5)
		want := "A · BUCS L3: removed by the club"
		if late {
			want += ", after entries closed"
		}
		if len(history) == 0 || history[0].What != want || history[0].Who != "UCD (Club admin)" {
			t.Errorf("late %v: the history says so: %+v", late, history)
		}
		// Sending everyone's again keeps it; the same name joining again is
		// someone new, and the removed copy stays as it is.
		a2, _, _ := s.Join(ctx, club.ID, "A")
		if !late {
			must(t, s.SaveMemberEntry(ctx, a2.ID, c.ID, entry("A")))
		}
		if !late {
			_, err := s.Send(ctx, club.ID, c.ID, nil)
			must(t, err)
		}
		list, _ := s.Entries(ctx, c.ID)
		removed, in := 0, 0
		for _, e := range list {
			switch {
			case e.Entry.Gymnast == "A" && e.ClubRemoved():
				removed++
			case e.Entry.Gymnast == "A":
				in++
			}
		}
		if removed != 1 || in != map[bool]int{false: 1, true: 0}[late] {
			t.Errorf("late %v: the removed copy stays, and before the close a replacement can be sent: %d removed, %d in", late, removed, in)
		}
	}
}

func TestRemovedMemberFees(t *testing.T) {
	f := competitions.Fees{PerEntry: map[string]int{competitions.Trampoline: 2000}}
	removed := []competitions.Entry{entry("A")}
	if lines, total := f.RemovedLines(removed); len(lines) != 0 || total != 0 || f.RemovedCost(competitions.Trampoline) != 0 {
		t.Errorf("off: nothing charged: %+v %d", lines, total)
	}
	f.ChargeRemoved = true
	if lines, total := f.RemovedLines(removed); len(lines) != 1 || total != 2000 || lines[0].What != "A · BUCS L3 (removed after entries closed)" {
		t.Errorf("its entry fee: %+v %d", lines, total)
	}
	f.RemovedFee = 500
	lines, total := f.RemovedLines(removed)
	if len(lines) != 2 || total != 2500 || lines[1].What != "Removed after entries closed: A · BUCS L3" || f.RemovedCost(competitions.Trampoline) != 2500 {
		t.Errorf("and the fee for removing it: %+v %d", lines, total)
	}
	f.ChargeRemoved = false
	if lines, total := f.RemovedLines(removed); len(lines) != 1 || total != 500 {
		t.Errorf("the fee alone: %+v %d", lines, total)
	}
}

func TestRemovedPartner(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s, c, club, a, b, _ := clubWithEntries(t, &now)
	// A's entry names B as a confirmed partner, as a synchro pair's does.
	_, err := s.db.ExecContext(ctx, `UPDATE entries SET partner_member = $1 WHERE member_id = $2`, b.ID, a.ID)
	must(t, err)
	must(t, s.RemoveMember(ctx, club.ID, b.ID, "Club admin"))
	got := byGymnast(t, s, c.ID)
	if got["A"].Withdrawn {
		t.Error("A's entry, with B as partner, stays in, as when a partner withdraws")
	}
	history, _ := s.History(ctx, c.ID, 5)
	found := false
	for _, h := range history {
		found = found || strings.Contains(h.What, "the pair's entry is A's and stays in")
	}
	if !found {
		t.Errorf("the history says why: %+v", history)
	}
}

func TestRemovedCoachStays(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s, c, club, a, _, ann := clubWithEntries(t, &now)
	must(t, s.RemoveCoach(ctx, club.ID, ann.ID, "Club admin"))
	sent, err := s.CompetitionCoaches(ctx, c.ID)
	must(t, err)
	if len(sent) != 1 || !sent[0].Removed() || sent[0].Name != "Ann Byrne" || sent[0].ClubName != "UCD" || sent[0].Status != CoachApproved ||
		sent[0].RemovedBy != "Club admin" || len(sent[0].RemovedQualifications) != 1 || sent[0].RemovedQualifications[0] != "bg-trampoline-3" {
		t.Fatalf("Ann stays, removed, approved as she was, with what she sent: %+v", sent)
	}
	got := byGymnast(t, s, c.ID)
	if !got["A"].SignedOff() || !got["B"].SignedOff() {
		t.Errorf("her sign-offs keep counting: %+v", got)
	}
	// The club sending its coaches again doesn't take her back.
	must(t, s.SendCoaches(ctx, club.ID, c.ID, nil))
	if sent, _ := s.CompetitionCoaches(ctx, c.ID); len(sent) != 1 {
		t.Errorf("she stays: %+v", sent)
	}

	signed, err := s.SignedByCoach(ctx, c.ID, ann.ID)
	must(t, err)
	if len(signed) != 2 {
		t.Fatalf("two entries signed by her: %+v", signed)
	}
	reason := "Signed off by Ann Byrne, who is no longer one of UCD's coaches: the organiser has asked for another coach to sign it off"
	done, err := s.AskSignoffAgain(ctx, c.ID, ann.ID, reason)
	must(t, err)
	if len(done) != 2 {
		t.Errorf("both: %+v", done)
	}
	got = byGymnast(t, s, c.ID)
	if got["A"].SignedOff() || got["A"].SignoffAgain != reason {
		t.Errorf("waiting again, with the reason: %+v", got["A"])
	}
	mine, _ := s.MemberEntries(ctx, a.ID)
	if len(mine) != 1 || mine[0].SignedOff() || mine[0].SignoffAgain != reason {
		t.Errorf("the member's own too: %+v", mine)
	}
	if sent, _ := s.CompetitionCoaches(ctx, c.ID); sent[0].AskedAgainAt.IsZero() {
		t.Error("when it was asked is kept")
	}
	// Another coach signing it off clears the reason.
	bob, bobLink, _ := s.CreateCoach(ctx, club.ID, "Bob")
	coach, _ := s.CoachByLink(ctx, bobLink)
	must(t, s.SignOff(ctx, coach, a.ID, c.ID, competitions.Trampoline, true, ""))
	if mine, _ := s.MemberEntries(ctx, a.ID); mine[0].SignoffAgain != "" {
		t.Errorf("signed again: %+v", mine)
	}
	// A coach still at the club can't be asked about.
	if _, err := s.AskSignoffAgain(ctx, c.ID, bob.ID, reason); !errors.Is(err, ErrNotFound) {
		t.Errorf("not removed: %v", err)
	}
}

func TestDeletedClubStays(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s, c, club, a, _, _ := clubWithEntries(t, &now)
	must(t, s.WithdrawMemberEntry(ctx, a.ID, c.ID, ""))
	must(t, s.DeleteClub(ctx, club.ID, "Club admin"))
	got := byGymnast(t, s, c.ID)
	for _, g := range []string{"A", "B"} {
		if e := got[g]; !e.ClubRemoved() || !e.Withdrawn || e.ClubName != "UCD" {
			t.Errorf("%s: removed by the club, not a live entry of no club: %+v", g, e)
		}
	}
	if got["I"].Withdrawn {
		t.Error("the individual stays in")
	}
	sent, _ := s.CompetitionCoaches(ctx, c.ID)
	if len(sent) != 1 || !sent[0].Removed() || sent[0].ClubName != "UCD" {
		t.Errorf("the coach stays, with the club's name: %+v", sent)
	}
	if !got["B"].SignedOff() {
		t.Error("sign-offs count")
	}
}

func TestExpiredClubStays(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s, c, club, _, _, _ := clubWithEntries(t, &now)
	_, err := s.db.ExecContext(ctx, `UPDATE clubs SET last_used = '2020-01-01T00:00:00.000000Z' WHERE id = $1`, club.ID)
	must(t, err)
	_, clubs, err := s.DeleteExpired(ctx)
	must(t, err)
	if clubs != 1 {
		t.Fatalf("%d clubs deleted", clubs)
	}
	got := byGymnast(t, s, c.ID)
	if e := got["A"]; !e.ClubRemoved() || e.ClubRemovedBy != "" {
		t.Errorf("removed when its record expired: %+v", e)
	}
	if history, _ := s.History(ctx, c.ID, 1); len(history) == 0 || history[0].Who != "UCD" {
		t.Errorf("by the club: %+v", history)
	}
}

// Migration 56 keeps the coaches sent, with their club's name, and the
// entries sent, live; a coach removed from then on stays.
func TestRemovedStayMigrates(t *testing.T) {
	all := migrations
	t.Cleanup(func() { migrations = all })
	migrations = all[:56-1]
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	must(t, err)
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	c, _, _ := s.CreateCompetition(ctx, competition())
	club, _, _ := s.CreateClub(ctx, "UCD")
	must(t, s.AttachClub(ctx, club.ID, c.ID))
	a, _, _ := s.Join(ctx, club.ID, "A")
	ann, _, _ := s.CreateCoach(ctx, club.ID, "Ann")
	data, _ := json.Marshal(entry("A"))
	_, err = s.db.ExecContext(ctx, `INSERT INTO member_entries (member_id, competition_id, discipline, entry, updated_at) VALUES ($1, $2, '', $3, $4)`,
		a.ID, c.ID, string(data), s.stamp())
	must(t, err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO entries (id, competition_id, club_id, member_id, discipline, club_name, individual, gymnast, entry, sent_at, entered_at)
		VALUES ('e1', $1, $2, $3, '', 'UCD', FALSE, 'A', $4, $5, $5)`, c.ID, club.ID, a.ID, string(data), s.stamp())
	must(t, err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO competition_coaches (competition_id, coach_id, club_id, name, sent_at, status) VALUES ($1, $2, $3, 'Ann', $4, 'approved')`,
		c.ID, ann.ID, club.ID, s.stamp())
	must(t, err)
	migrations = all
	s = reopen(t, s, dir)
	s.now = func() time.Time { return now }
	sent, err := s.CompetitionCoaches(ctx, c.ID)
	must(t, err)
	if len(sent) != 1 || sent[0].Removed() || sent[0].Status != CoachApproved || sent[0].ClubName != "UCD" {
		t.Fatalf("Ann is kept: %+v", sent)
	}
	if got := byGymnast(t, s, c.ID); got["A"].Withdrawn || got["A"].ClubRemoved() {
		t.Errorf("A's entry is in: %+v", got)
	}
	must(t, s.RemoveCoach(ctx, club.ID, ann.ID, "Club admin"))
	if sent, _ := s.CompetitionCoaches(ctx, c.ID); len(sent) != 1 || !sent[0].Removed() {
		t.Errorf("removed, she stays: %+v", sent)
	}
}
