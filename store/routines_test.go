package store

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestMemberRoutines(t *testing.T) {
	now := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	m, _, err := s.Join(ctx, club.ID, "A. Murphy")
	must(t, err)
	skills := `[{"rotation":0,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}]`

	saved, full, err := s.SaveRoutines(ctx, club.ID, m.ID, []RoutineIn{{Source: "b1", Name: "  Comp vol  ", Skills: skills}, {Name: "", Skills: skills}})
	must(t, err)
	if saved != 2 || full != 0 {
		t.Fatalf("saved %d, full %d; want 2, 0", saved, full)
	}
	got, err := s.MemberRoutines(ctx, m.ID)
	must(t, err)
	if len(got) != 2 || got[0].Name != "Comp vol" || got[1].Name != "Routine" {
		t.Fatalf("routines = %+v", got)
	}

	// Saving the same browser routine again changes it.
	now = now.Add(time.Hour)
	_, _, err = s.SaveRoutines(ctx, club.ID, m.ID, []RoutineIn{{Source: "b1", Name: "Comp vol 2", Skills: "[]"}})
	must(t, err)
	got, _ = s.MemberRoutines(ctx, m.ID)
	if len(got) != 2 || got[0].Name != "Comp vol 2" || got[0].Skills != "[]" || !got[0].UpdatedAt.Equal(now) {
		t.Fatalf("after saving again: %+v", got)
	}

	// A coach who sees the member notes it; the note is older once it changes.
	coach, _, err := s.CreateCoach(ctx, club.ID, "Ann")
	must(t, err)
	must(t, s.NoteRoutine(ctx, coach, m.ID, got[0].ID, "Straighter legs"))
	r, err := s.MemberRoutine(ctx, m.ID, got[0].ID)
	must(t, err)
	if r.Note != "Straighter legs" || r.NoteBy != "Ann" || r.NoteBefore() {
		t.Fatalf("noted: %+v", r)
	}
	now = now.Add(time.Hour)
	_, _, _ = s.SaveRoutines(ctx, club.ID, m.ID, []RoutineIn{{Source: "b1", Name: "Comp vol 2", Skills: skills}})
	if r, _ = s.MemberRoutine(ctx, m.ID, got[0].ID); !r.NoteBefore() {
		t.Error("a note older than the routine's change should say so")
	}

	// A coach of another member, in a club where coaches don't see everyone, can't.
	bob, _, _ := s.CreateCoach(ctx, club.ID, "Bob")
	must(t, s.SetMemberCoach(ctx, club.ID, m.ID, coach.ID))
	if err := s.NoteRoutine(ctx, bob, m.ID, got[0].ID, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("another coach's note: %v", err)
	}
	if _, err := s.CoachMember(ctx, bob, m.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another coach sees the member: %v", err)
	}
	if ms, _ := s.CoachMembers(ctx, coach); len(ms) != 1 || ms[0].ID != m.ID {
		t.Errorf("coach's members = %+v", ms)
	}

	must(t, s.RenameRoutine(ctx, m.ID, got[1].ID, "Warm-up"))
	if err := s.RenameRoutine(ctx, m.ID, got[1].ID, " "); err == nil {
		t.Error("renamed to nothing")
	}
	must(t, s.DeleteRoutine(ctx, m.ID, got[1].ID))
	if err := s.DeleteRoutine(ctx, "someone-else", got[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted another member's routine: %v", err)
	}
	if counts, _ := s.RoutineCounts(ctx, club.ID); counts[m.ID] != 1 {
		t.Errorf("counts = %v", counts)
	}

	// Room for MaxRoutines; the rest are left out.
	var many []RoutineIn
	for i := range MaxRoutines {
		many = append(many, RoutineIn{Name: fmt.Sprint("R", i), Skills: skills})
	}
	if saved, full, _ = s.SaveRoutines(ctx, club.ID, m.ID, many); saved != MaxRoutines-1 || full != 1 {
		t.Errorf("saved %d, full %d; want %d, 1", saved, full, MaxRoutines-1)
	}

	// They go with the member.
	must(t, s.RemoveMember(ctx, club.ID, m.ID))
	if got, _ := s.MemberRoutines(ctx, m.ID); len(got) != 0 {
		t.Errorf("a removed member's routines stay: %d", len(got))
	}
}

// Opening a club's pages marks it used, at most once a day; a change always does.
func TestClubMarkedUsed(t *testing.T) {
	now := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, admin, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	_, member, err := s.Join(ctx, club.ID, "A. Murphy")
	must(t, err)
	_, coach, err := s.CreateCoach(ctx, club.ID, "Ann")
	must(t, err)
	used := func() time.Time {
		t.Helper()
		c, err := s.ClubByAdmin(ctx, admin)
		must(t, err)
		return c.LastUsed
	}
	start := now

	// Within the day, opening a page writes nothing.
	now = start.Add(23 * time.Hour)
	_, err = s.MemberByLink(ctx, member)
	must(t, err)
	if got := used(); !got.Equal(start) {
		t.Errorf("marked used within the day: %v", got)
	}

	// A day on, a member's page marks it; so does a coach's the next day.
	for i, open := range []func() error{
		func() error { _, err := s.MemberByLink(ctx, member); return err },
		func() error { _, err := s.CoachByLink(ctx, coach); return err },
		func() error { _, err := s.ClubByJoinLink(ctx, club.JoinLink); return err },
	} {
		now = start.Add(time.Duration(i+1) * 25 * time.Hour)
		must(t, open())
		if got := used(); !got.Equal(now) {
			t.Errorf("page %d: last used %v, want %v", i, got, now)
		}
	}
	// The admin link's own page marks it too, and says so.
	now = now.Add(48 * time.Hour)
	if got := used(); !got.Equal(now) {
		t.Errorf("the admin page: last used %v, want %v", got, now)
	}
}
