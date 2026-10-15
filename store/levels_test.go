package store

import (
	"errors"
	"testing"
	"time"
)

func TestClubLevels(t *testing.T) {
	now := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	ann, _, err := s.CreateCoach(ctx, club.ID, "Ann")
	must(t, err)
	set := `{"format":1,"name":"Grade 1","rules":[]}`

	// Saving from a browser set adds a level, last; saving it again changes it.
	added, err := s.SaveClubLevel(ctx, ann, "b1", "Grade 1", set)
	must(t, err)
	if !added {
		t.Error("a new level")
	}
	_, err = s.SaveClubLevel(ctx, ann, "b2", "Grade 2", set)
	must(t, err)
	now = now.Add(time.Hour)
	if added, _ = s.SaveClubLevel(ctx, ann, "b1", "Grade 1a", set); added {
		t.Error("the same set changes its level")
	}
	levels, err := s.ClubLevels(ctx, club.ID)
	must(t, err)
	if len(levels) != 2 || levels[0].Name != "Grade 1a" || levels[1].Name != "Grade 2" || levels[0].ByName != "Ann" || !levels[0].UpdatedAt.Equal(now) {
		t.Fatalf("levels = %+v", levels)
	}

	// Its club skills to track: only the club's own, and only ones to track.
	must(t, s.AddClubSkill(ctx, ann, "Kaboom drill", "", ""))
	must(t, s.AddClubSkill(ctx, ann, "Swivel Hips", "", `{"rotation":0,"twist_distribution":[1],"takeoff_position":"Seat","shape":"Straight","seat_landing":true}`))
	own, _ := s.ClubSkills(ctx, club.ID)
	must(t, s.SetLevelClubItems(ctx, ann, levels[0].ID, []string{own[0].ID, own[1].ID, "nope", own[0].ID}))
	if levels, _ = s.ClubLevels(ctx, club.ID); len(levels[0].ClubItems) != 1 || levels[0].ClubItems[0] != own[0].ID {
		t.Errorf("club items = %v", levels[0].ClubItems)
	}

	// Moving and removing; another club's coach can do neither.
	must(t, s.MoveClubLevel(ctx, ann, levels[1].ID, -1))
	if levels, _ = s.ClubLevels(ctx, club.ID); levels[0].Name != "Grade 2" {
		t.Errorf("moved: %+v", levels)
	}
	other, _, _ := s.CreateClub(ctx, "DCU")
	bob, _, _ := s.CreateCoach(ctx, other.ID, "Bob")
	if err := s.MoveClubLevel(ctx, bob, levels[0].ID, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("another club's coach moves it: %v", err)
	}
	if err := s.RemoveClubLevel(ctx, bob, levels[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another club's coach removes it: %v", err)
	}
	must(t, s.RemoveClubLevel(ctx, ann, levels[0].ID))
	if levels, _ = s.ClubLevels(ctx, club.ID); len(levels) != 1 {
		t.Errorf("removed: %+v", levels)
	}

	// Room for MaxClubLevels.
	for i := len(levels); i < MaxClubLevels; i++ {
		_, err := s.SaveClubLevel(ctx, ann, "", "More", set)
		must(t, err)
	}
	if _, err := s.SaveClubLevel(ctx, ann, "", "One too many", set); !errors.Is(err, ErrLimit) {
		t.Errorf("past the limit: %v", err)
	}
}
