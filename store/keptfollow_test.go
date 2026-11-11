package store

import (
	"testing"
	"time"
)

// keptOn reads the requirements kept on the page a personal link opens, as
// "kind:source=name", by kind then name.
func keptOn(t *testing.T, s *Store, link string) []string {
	t.Helper()
	h, err := s.HolderOf(ctx, link)
	if err != nil {
		t.Fatalf("opening %s: %v", link, err)
	}
	kept, err := s.KeptRequirements(ctx, h.PersonID)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, k := range kept {
		out = append(out, k.Kind+":"+k.Source+"="+k.Name)
	}
	return out
}

// keepBoth keeps a set and a level on the page a link opens.
func keepBoth(t *testing.T, s *Store, link, set, level string) {
	t.Helper()
	h, err := s.HolderOf(ctx, link)
	must(t, err)
	_, err = s.KeepRequirements(ctx, h.PersonID, KeptSet, "set-"+set, set, "{}")
	must(t, err)
	_, err = s.KeepRequirements(ctx, h.PersonID, KeptLevel, "level-"+level, level, "{}")
	must(t, err)
}

// wantKept checks what a page keeps now and after the tidy-up that deletes
// requirements nobody owns.
func wantKept(t *testing.T, s *Store, link string, want ...string) {
	t.Helper()
	for _, when := range []string{"at once", "after DeleteExpired"} {
		if when != "at once" {
			if _, _, err := s.DeleteExpired(ctx); err != nil {
				t.Fatal(err)
			}
		}
		got := keptOn(t, s, link)
		if len(got) != len(want) {
			t.Fatalf("%s: kept %v, want %v", when, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: kept %v, want %v", when, got, want)
			}
		}
	}
}

// Joining two links keeps what either page kept, on the one page: the
// member's own and the coach's, and the newer where both kept the same one.
func TestKeptFollowsAJoin(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, _ := s.CreateClub(ctx, "UCD")
	_, mLink, err := s.Join(ctx, club.ID, "Aoife")
	must(t, err)
	c, cLink, err := s.CreateCoach(ctx, club.ID, "Aoife")
	must(t, err)
	keepBoth(t, s, cLink, "Coach set", "Coach level")
	hm, _ := s.HolderOf(ctx, mLink)
	hc, _ := s.HolderOf(ctx, cLink)

	// Both kept "shared": the coach's is newer.
	_, err = s.KeepRequirements(ctx, hm.PersonID, KeptSet, "shared", "Member's old", `{"a":1}`)
	must(t, err)
	now = now.Add(time.Hour)
	_, err = s.KeepRequirements(ctx, hc.PersonID, KeptSet, "shared", "Coach's newer", `{"a":2}`)
	must(t, err)
	// And "shared-2": the member's is newer.
	_, err = s.KeepRequirements(ctx, hc.PersonID, KeptSet, "shared-2", "Coach's old", `{"a":1}`)
	must(t, err)
	now = now.Add(time.Hour)
	_, err = s.KeepRequirements(ctx, hm.PersonID, KeptSet, "shared-2", "Member's newer", `{"a":2}`)
	must(t, err)

	_, err = s.Tie(ctx, mLink, RoleCoach, cLink)
	must(t, err)
	wantKept(t, s, mLink, "set:set-Coach set=Coach set", "set:shared=Coach's newer", "set:shared-2=Member's newer", "level:level-Coach level=Coach level")
	wantKept(t, s, cLink, "set:set-Coach set=Coach set", "set:shared=Coach's newer", "set:shared-2=Member's newer", "level:level-Coach level=Coach level")

	// Undoing the join: they stay with the page the coach was taken off, and
	// the coach, a person of their own again, starts with none; nothing is
	// left owned by nobody.
	must(t, s.Untie(ctx, mLink, RoleCoach, c.ID))
	wantKept(t, s, mLink, "set:set-Coach set=Coach set", "set:shared=Coach's newer", "set:shared-2=Member's newer", "level:level-Coach level=Coach level")
	wantKept(t, s, cLink)
	var n int
	must(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM person_requirements WHERE person_id NOT IN (
		SELECT COALESCE(person_id, id) FROM members UNION SELECT COALESCE(person_id, id) FROM coaches UNION SELECT person_id FROM individuals)`).Scan(&n))
	if n != 0 {
		t.Errorf("%d kept owned by nobody", n)
	}
}

// Joining keeps all of them even past the cap; only new ones are stopped.
func TestKeptJoinPastTheCap(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, _ := s.CreateClub(ctx, "UCD")
	_, mLink, err := s.Join(ctx, club.ID, "Aoife")
	must(t, err)
	_, cLink, err := s.CreateCoach(ctx, club.ID, "Aoife")
	must(t, err)
	hm, _ := s.HolderOf(ctx, mLink)
	hc, _ := s.HolderOf(ctx, cLink)
	for i := range MaxKept {
		_, err = s.KeepRequirements(ctx, hm.PersonID, KeptSet, "m"+string(rune('a'+i%26))+string(rune('a'+i/26)), "M", "{}")
		must(t, err)
		_, err = s.KeepRequirements(ctx, hc.PersonID, KeptSet, "c"+string(rune('a'+i%26))+string(rune('a'+i/26)), "C", "{}")
		must(t, err)
	}
	_, err = s.Tie(ctx, mLink, RoleCoach, cLink)
	must(t, err)
	if got := keptOn(t, s, mLink); len(got) != 2*MaxKept {
		t.Errorf("%d kept, want %d", len(got), 2*MaxKept)
	}
	if _, err := s.KeepRequirements(ctx, hm.PersonID, KeptSet, "new", "N", "{}"); err != ErrLimit {
		t.Errorf("a new one past the cap: %v", err)
	}
}

// A new member link, and a new coach link, keep what the person kept; with
// another page of the old person still there, those stay with it.
func TestKeptFollowsANewLink(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, _ := s.CreateClub(ctx, "UCD")

	m, mLink, err := s.Join(ctx, club.ID, "Aoife")
	must(t, err)
	keepBoth(t, s, mLink, "Grade 1", "Novice")
	newLink, err := s.ReplaceMemberLink(ctx, club.ID, m.ID)
	must(t, err)
	wantKept(t, s, newLink, "set:set-Grade 1=Grade 1", "level:level-Novice=Novice")

	c, cLink, err := s.CreateCoach(ctx, club.ID, "Bea")
	must(t, err)
	keepBoth(t, s, cLink, "Coach set", "Coach level")
	newCoach, err := s.ReplaceCoachLink(ctx, club.ID, c.ID)
	must(t, err)
	wantKept(t, s, newCoach, "set:set-Coach set=Coach set", "level:level-Coach level=Coach level")

	// A member and coach joined, then the member gets a new link: the
	// coach page, which still has the old person, keeps them.
	m2, m2Link, err := s.Join(ctx, club.ID, "Cara")
	must(t, err)
	_, c2Link, err := s.CreateCoach(ctx, club.ID, "Cara")
	must(t, err)
	_, err = s.Tie(ctx, m2Link, RoleCoach, c2Link)
	must(t, err)
	keepBoth(t, s, m2Link, "Joined set", "Joined level")
	// ReplaceMemberLink moves the same-club coach with the member, so use an
	// individual-held person: a coach of another club tied to the member.
	other, _, _ := s.CreateClub(ctx, "DCU")
	_, c3Link, err := s.CreateCoach(ctx, other.ID, "Cara")
	must(t, err)
	_, err = s.Tie(ctx, m2Link, RoleCoach, c3Link)
	must(t, err)
	newM2, err := s.ReplaceMemberLink(ctx, club.ID, m2.ID)
	must(t, err)
	wantKept(t, s, c3Link, "set:set-Joined set=Joined set", "level:level-Joined level=Joined level")
	wantKept(t, s, newM2)
}
