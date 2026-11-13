package store

import (
	"errors"
	"testing"
	"time"

	"tariffCalculator/competitions"
)

// offered gives A an offer to marshal, sent with the club's entries.
func offered(t *testing.T, s *Store, c Competition, club Club, a Member) {
	t.Helper()
	must(t, s.SaveMemberOffer(ctx, a.ID, c.ID, competitions.Offer{Marshal: true}))
	_, err := s.Send(ctx, club.ID, c.ID, nil)
	must(t, err)
}

func TestTakeOut(t *testing.T) {
	for _, what := range []string{"entries", "offer", "both"} {
		for _, late := range []bool{false, true} {
			now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
			s, c, club, a, _, _ := clubWithEntries(t, &now)
			offered(t, s, c, club, a)
			if late {
				// Entries have closed; changes stay open two days more.
				must(t, s.SetChangesAndSignoffs(ctx, c.ID, c.Deadline.Add(48*time.Hour), time.Time{}))
				now = c.Deadline.Add(time.Hour)
			}
			entries, offer := what != "offer", what != "entries"
			must(t, s.TakeOut(ctx, club.ID, a.ID, c.ID, entries, offer, "Club admin"))
			got := byGymnast(t, s, c.ID)
			e := got["A"]
			if entries != e.ClubRemoved() || (entries && (e.ClubRemovedLate != late || e.MemberID != "" || e.Key() != MemberKey(a.ID))) {
				t.Errorf("%s late %v: the entry: %+v", what, late, e)
			}
			if mine, _ := s.MemberEntries(ctx, a.ID); (len(mine) == 0) != entries {
				t.Errorf("%s: the member's own entry: %+v", what, mine)
			}
			officials, _ := s.Officials(ctx, c.ID)
			back, _ := s.TakenBack(ctx, c.ID)
			if offer != (len(officials) == 0) || offer != (len(back) == 1) {
				t.Errorf("%s: officials %+v, taken back %+v", what, officials, back)
			}
			if offer && (back[0].Name != "A" || back[0].TakenBackBy != "Club admin" || back[0].PersonKey != MemberKey(a.ID)) {
				t.Errorf("taken back: %+v", back[0])
			}
			// Sending again keeps both as they are.
			_, err := s.Send(ctx, club.ID, c.ID, nil)
			must(t, err)
			if back2, _ := s.TakenBack(ctx, c.ID); len(back2) != len(back) {
				t.Errorf("%s: sending keeps what was taken back: %+v", what, back2)
			}
			if entries && !late {
				// They can be entered again: a new copy, the removed one kept.
				must(t, s.SaveMemberEntry(ctx, a.ID, c.ID, entry("A")))
				_, err := s.Send(ctx, club.ID, c.ID, nil)
				must(t, err)
				list, _ := s.Entries(ctx, c.ID)
				n := 0
				for _, e := range list {
					if e.Entry.Gymnast == "A" {
						n++
					}
				}
				if n != 2 {
					t.Errorf("A again, and the removed copy: %d", n)
				}
			}
		}
	}
}

func TestTakeOutClosed(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s, c, club, a, _, _ := clubWithEntries(t, &now)
	now = c.Deadline.Add(time.Hour)
	if err := s.TakeOut(ctx, club.ID, a.ID, c.ID, true, true, "Club admin"); !errors.Is(err, ErrClosed) {
		t.Errorf("as withdrawing, not once changes close: %v", err)
	}
}

func TestRemovingTakesTheOffer(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s, c, club, a, _, _ := clubWithEntries(t, &now)
	offered(t, s, c, club, a)
	must(t, s.RemoveMember(ctx, club.ID, a.ID, "Club admin"))
	if officials, _ := s.Officials(ctx, c.ID); len(officials) != 0 {
		t.Errorf("no live official left: %+v", officials)
	}
	if back, _ := s.TakenBack(ctx, c.ID); len(back) != 1 || back[0].PersonKey != MemberKey(a.ID) {
		t.Errorf("taken back, with their key: %+v", back)
	}
}

func TestPartnerFlagged(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s, c, club, a, b, _ := clubWithEntries(t, &now)
	// A's entry names B as partner, as a synchro pair's does.
	_, err := s.db.ExecContext(ctx, `UPDATE entries SET partner_member = $1 WHERE member_id = $2`, b.ID, a.ID)
	must(t, err)
	must(t, s.TakeOut(ctx, club.ID, b.ID, c.ID, true, false, "Club admin"))
	g := byGymnast(t, s, c.ID)["A"].PartnerGone
	if g == nil || g.Name != "their partner" || g.Club != "UCD" || !g.TakenOut {
		t.Fatalf("flagged: %+v", g)
	}
	if mine, _ := s.MemberEntries(ctx, a.ID); mine[0].PartnerGone == nil {
		t.Error("on A's own entry too")
	}
	// Changing the entry's partner clears it.
	e := entry("A")
	e.Partner = &competitions.Partner{Name: "Cy"}
	must(t, s.SaveMemberEntry(ctx, a.ID, c.ID, e))
	if mine, _ := s.MemberEntries(ctx, a.ID); mine[0].PartnerGone != nil {
		t.Errorf("a new partner clears it: %+v", mine[0].PartnerGone)
	}
}
