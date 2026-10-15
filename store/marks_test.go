package store

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSkillMarks(t *testing.T) {
	now := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	m, _, err := s.Join(ctx, club.ID, "A. Murphy")
	must(t, err)
	ann, _, err := s.CreateCoach(ctx, club.ID, "Ann")
	must(t, err)
	backFull := `{"rotation":4,"twist_distribution":[2],"takeoff_position":"Feet","shape":"Straight","backward":true}`
	backTuck := `{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true}`
	backPike := strings.Replace(backTuck, "Tuck", "Pike", 1)

	// The gymnast says Can do; their coach says Learning: both are kept.
	must(t, s.MarkAsGymnast(ctx, m, MarkIn{Skill: backFull, State: CanDo}))
	now = now.Add(time.Minute)
	// The same skill, given in another shape: Equal calls it the same, so
	// it's the same item.
	must(t, s.MarkAsCoach(ctx, ann, m.ID, MarkIn{Skill: strings.Replace(backFull, "Straight", "Tuck", 1), State: Learning}))
	marks, err := s.MemberMarks(ctx, m.ID)
	must(t, err)
	if len(marks) != 2 || marks[0].Item != marks[1].Item {
		t.Fatalf("one item, a mark on each side: %+v", marks)
	}
	if c := marks[0]; c.Side != SideCoach || c.State != Learning || c.ByCoach != ann.ID || c.ByName != "Ann" || !c.At.Equal(now) {
		t.Errorf("the coach's: %+v", c)
	}
	if g := marks[1]; g.Side != SideGymnast || g.State != CanDo || g.ByCoach != "" || g.ByName != "A. Murphy" {
		t.Errorf("the gymnast's: %+v", g)
	}
	can, toConfirm, err := s.MarkCounts(ctx, club.ID)
	must(t, err)
	if can[m.ID] != 1 || toConfirm[m.ID] != 1 {
		t.Errorf("counts: can %v, to confirm %v", can, toConfirm)
	}

	// Once the coach agrees, there's nothing to confirm; a tuck back and a
	// pike back are two skills.
	must(t, s.MarkAsCoach(ctx, ann, m.ID, MarkIn{Skill: backFull, State: CanDo}))
	must(t, s.MarkAsGymnast(ctx, m, MarkIn{Skill: backTuck, State: Learning}))
	must(t, s.MarkAsGymnast(ctx, m, MarkIn{Skill: backPike, State: Learning}))
	if can, toConfirm, _ = s.MarkCounts(ctx, club.ID); can[m.ID] != 1 || toConfirm[m.ID] != 2 {
		t.Errorf("after the coach agrees: can %v, to confirm %v", can, toConfirm)
	}
	if marks, _ = s.MemberMarks(ctx, m.ID); len(marks) != 4 {
		t.Errorf("the full twisting back's two marks, and the tuck and pike backs': %d", len(marks))
	}

	// Each side takes off only its own.
	must(t, s.MarkAsGymnast(ctx, m, MarkIn{Skill: backFull}))
	marks, _ = s.MemberMarks(ctx, m.ID)
	sides := map[string]int{}
	for _, k := range marks {
		if strings.Contains(k.Skill, `"twist_distribution":[2]`) {
			sides[k.Side]++
		}
	}
	if sides[SideGymnast] != 0 || sides[SideCoach] != 1 {
		t.Errorf("the gymnast takes off theirs, and the coach's stays: %v", sides)
	}
	if err := s.MarkAsGymnast(ctx, m, MarkIn{Skill: backTuck, State: "maybe"}); err == nil {
		t.Error("a state that isn't one")
	}

	// A coach who doesn't see the member can't mark.
	bob, _, _ := s.CreateCoach(ctx, club.ID, "Bob")
	must(t, s.SetMemberCoach(ctx, club.ID, m.ID, ann.ID))
	if err := s.MarkAsCoach(ctx, bob, m.ID, MarkIn{Skill: backTuck, State: CanDo}); !errors.Is(err, ErrNotFound) {
		t.Errorf("another coach marks: %v", err)
	}

	// The club's setting, and the marks go with the member.
	must(t, s.SetSkillsOpen(ctx, club.ID, true))
	if c, _ := s.ClubByID(ctx, club.ID); !c.SkillsOpen {
		t.Error("skills open")
	}
	must(t, s.RemoveMember(ctx, club.ID, m.ID))
	if marks, _ = s.MemberMarks(ctx, m.ID); len(marks) != 0 {
		t.Error("a removed member's marks stay")
	}
}
