package store

import (
	"testing"
	"time"

	"tariffCalculator/competitions"
)

// letInOf says whether the entry is let in over its event's limit now.
func letInOf(t *testing.T, s *Store, competitionID, id string) bool {
	t.Helper()
	e, err := s.CompetitionEntry(ctx, competitionID, id)
	must(t, err)
	return e.LetIn
}

// An entry let in over one event's limit stays let in while it's changed
// within the event, and isn't once it moves to another: through the
// individual's own change, the club's sending and the organiser accepting
// a late level change.
func TestLetInClearsOnLevelChange(t *testing.T) {
	now := time.Date(2027, 3, 8, 10, 0, 0, 0, time.UTC)
	// openTo is a competition with BUCS L3 and L4, taking entries (the clock
	// is before its deadline).
	openTo := func(t *testing.T) (*Store, Competition) {
		early := time.Date(2027, 3, 1, 8, 0, 0, 0, time.UTC)
		s := open(t, &early)
		c := competition()
		c.Levels = append(c.Levels, competitions.Level{Ref: "builtin-level:bucs-l4"})
		got, _, err := s.CreateCompetition(ctx, c)
		must(t, err)
		return s, got
	}

	t.Run("an individual changes their entry", func(t *testing.T) {
		s, c := openTo(t)
		bea, tok, err := enter(s, c.ID, entry("Bea"))
		must(t, err)
		must(t, s.LetIn(ctx, c.ID, bea.ID, true))
		changed := entry("Bea")
		changed.Exercises[0].Option = "builtin:bucs-l3-option-2"
		must(t, s.ReplaceIndividualEntry(ctx, tok, changed))
		if !letInOf(t, s, c.ID, bea.ID) {
			t.Error("a change within the event keeps it let in")
		}
		must(t, s.ReplaceIndividualEntry(ctx, tok, l4("Bea")))
		if letInOf(t, s, c.ID, bea.ID) {
			t.Error("moved to another event, it isn't let in any more")
		}
	})

	t.Run("the club sends a changed entry", func(t *testing.T) {
		s, c := openTo(t)
		club, _, err := s.CreateClub(ctx, "UCD")
		must(t, err)
		m, _, err := s.Join(ctx, club.ID, "A. Murphy")
		must(t, err)
		must(t, s.AttachClub(ctx, club.ID, c.ID))
		must(t, s.SaveMemberEntry(ctx, m.ID, c.ID, entry("A. Murphy")))
		_, err = s.Send(ctx, club.ID, c.ID, nil)
		must(t, err)
		id := ""
		all, _ := s.Entries(ctx, c.ID)
		for _, e := range all {
			if e.MemberID == m.ID {
				id = e.ID
			}
		}
		must(t, s.LetIn(ctx, c.ID, id, true))
		changed := entry("A. Murphy")
		changed.Exercises[0].Option = "builtin:bucs-l3-option-2"
		must(t, s.SaveMemberEntry(ctx, m.ID, c.ID, changed))
		_, err = s.Send(ctx, club.ID, c.ID, nil)
		must(t, err)
		if !letInOf(t, s, c.ID, id) {
			t.Error("sent changed within the event, it stays let in")
		}
		must(t, s.SaveMemberEntry(ctx, m.ID, c.ID, l4("A. Murphy")))
		_, err = s.Send(ctx, club.ID, c.ID, nil)
		must(t, err)
		if letInOf(t, s, c.ID, id) {
			t.Error("sent in another event, it isn't let in any more")
		}
	})

	t.Run("the organiser accepts a late level change", func(t *testing.T) {
		s, c, e := lateCompetition(t, &now)
		must(t, s.LetIn(ctx, c.ID, e.ID, true))
		ann := Sender{Role: RoleIndividual, Name: "Ann", ID: "individual:" + e.Key()}
		r, err := s.RequestLateChange(ctx, c.ID, e.ID, l4("Ann"), "Ann", "", ann, false)
		must(t, err)
		_, err = s.AcceptLateChange(ctx, c.ID, r.ID, "", "Organiser", 0)
		must(t, err)
		if letInOf(t, s, c.ID, e.ID) {
			t.Error("accepted into another event, it isn't let in any more")
		}
	})

	t.Run("a late change within the level", func(t *testing.T) {
		s, c, e := lateCompetition(t, &now)
		must(t, s.SetLate(ctx, c.ID, competitions.LateChanges{Rules: map[string]competitions.LateRule{
			competitions.LateRoutines: {On: true}}}))
		must(t, s.LetIn(ctx, c.ID, e.ID, true))
		ann := Sender{Role: RoleIndividual, Name: "Ann", ID: "individual:" + e.Key()}
		changed := entry("Ann")
		changed.Exercises[0].Option = "builtin:bucs-l3-option-2"
		r, err := s.RequestLateChange(ctx, c.ID, e.ID, changed, "Ann", "", ann, false)
		must(t, err)
		_, err = s.AcceptLateChange(ctx, c.ID, r.ID, "", "Organiser", 0)
		must(t, err)
		if !letInOf(t, s, c.ID, e.ID) {
			t.Error("same level, still let in")
		}
	})
}
