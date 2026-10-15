package store

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestClubSkills(t *testing.T) {
	now := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	ann, _, err := s.CreateCoach(ctx, club.ID, "Ann")
	must(t, err)
	m, _, err := s.Join(ctx, club.ID, "A. Murphy")
	must(t, err)
	swivel := `{"rotation":0,"twist_distribution":[1],"takeoff_position":"Seat","shape":"Straight","seat_landing":true}`

	// A name for a catalog skill, and something else to track.
	must(t, s.AddClubSkill(ctx, ann, " Swivel Hips ", "", swivel))
	must(t, s.AddClubSkill(ctx, ann, "Kaboom drill", "On the crash mat first", ""))
	list, err := s.ClubSkills(ctx, club.ID)
	must(t, err)
	if len(list) != 2 || list[0].Name != "Kaboom drill" || list[0].Skill != "" || list[0].Note != "On the crash mat first" || list[0].ByName != "Ann" {
		t.Fatalf("club skills = %+v", list)
	}
	if list[1].Name != "Swivel Hips" || !strings.Contains(list[1].Skill, `"custom_name":"Swivel Hips"`) {
		t.Errorf("the named skill keeps its name as its custom name: %+v", list[1])
	}

	// Naming the same skill again renames it; another's name is refused.
	must(t, s.AddClubSkill(ctx, ann, "Swivel", "", swivel))
	if list, _ = s.ClubSkills(ctx, club.ID); len(list) != 2 || list[1].Name != "Swivel" {
		t.Errorf("renamed: %+v", list)
	}
	if err := s.AddClubSkill(ctx, ann, "kaboom DRILL", "", ""); !errors.Is(err, ErrSameName) {
		t.Errorf("a name already used: %v", err)
	}
	if err := s.AddClubSkill(ctx, ann, " ", "", ""); err == nil {
		t.Error("a club skill needs a name")
	}

	// Marks on something else to track go with it.
	kaboom := list[0]
	must(t, s.MarkAsGymnast(ctx, m, MarkIn{Item: kaboom.Item(), State: Learning}))
	must(t, s.MarkAsCoach(ctx, ann, m.ID, MarkIn{Item: kaboom.Item(), State: CanDo}))
	other, _, _ := s.CreateClub(ctx, "DCU")
	bob, _, _ := s.CreateCoach(ctx, other.ID, "Bob")
	if err := s.RemoveClubSkill(ctx, bob, kaboom.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another club's coach removes it: %v", err)
	}
	must(t, s.RemoveClubSkill(ctx, ann, kaboom.ID))
	if marks, _ := s.MemberMarks(ctx, m.ID); len(marks) != 0 {
		t.Errorf("its marks go with it: %+v", marks)
	}
	if list, _ = s.ClubSkills(ctx, club.ID); len(list) != 1 {
		t.Errorf("removed: %+v", list)
	}
}
