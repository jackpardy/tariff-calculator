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

	// The gymnast marks two skills; one is theirs to change, as the coach's is.
	must(t, s.MarkAsGymnast(ctx, m, MarkIn{Item: "s:back", Skill: "{}", State: Learning}))
	now = now.Add(time.Minute)
	must(t, s.MarkAsGymnast(ctx, m, MarkIn{Item: "s:front", Skill: "{}", State: CanDo}))
	marks, err := s.MemberMarks(ctx, m.ID)
	must(t, err)
	if len(marks) != 2 || marks[0].Item != "s:front" || !marks[0].ByGymnast() || marks[0].ByName != "A. Murphy" {
		t.Fatalf("marks = %+v", marks)
	}
	can, byGymnast, err := s.MarkCounts(ctx, club.ID)
	must(t, err)
	if can[m.ID] != 1 || byGymnast[m.ID] != 2 {
		t.Errorf("counts: can %v, by the gymnast %v", can, byGymnast)
	}

	// A coach marking it makes it theirs.
	now = now.Add(time.Minute)
	must(t, s.MarkAsCoach(ctx, ann, m.ID, MarkIn{Item: "s:back", Skill: "{}", State: CanDo}))
	marks, _ = s.MemberMarks(ctx, m.ID)
	if len(marks) != 2 || marks[0].Item != "s:back" || marks[0].State != CanDo || marks[0].ByCoach != ann.ID || marks[0].ByName != "Ann" || !marks[0].At.Equal(now) {
		t.Errorf("the coach's mark: %+v", marks[0])
	}
	if _, byGymnast, _ = s.MarkCounts(ctx, club.ID); byGymnast[m.ID] != 1 {
		t.Errorf("one left to confirm: %v", byGymnast)
	}

	// A skill's mark is found by the skill, as Equal compares them: a back
	// somersault marked again, in another form of the same skill, is the same
	// mark. A straight back with a full twist is the same skill whatever its
	// shape is given as; a tuck back isn't a pike one.
	backFull := `{"rotation":4,"twist_distribution":[2],"takeoff_position":"Feet","shape":"Straight","backward":true}`
	must(t, s.MarkAsGymnast(ctx, m, MarkIn{Skill: backFull, State: Learning}))
	now = now.Add(time.Minute)
	must(t, s.MarkAsCoach(ctx, ann, m.ID, MarkIn{Skill: strings.Replace(backFull, "Straight", "Tuck", 1), State: CanDo}))
	must(t, s.MarkAsGymnast(ctx, m, MarkIn{Skill: `{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true}`, State: Learning}))
	must(t, s.MarkAsGymnast(ctx, m, MarkIn{Skill: `{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Pike","backward":true}`, State: Learning}))
	marks, _ = s.MemberMarks(ctx, m.ID)
	full := 0
	for _, k := range marks {
		if strings.Contains(k.Skill, `"twist_distribution":[2]`) {
			full++
			if k.State != CanDo || k.ByName != "Ann" {
				t.Errorf("the full twisting back is the coach's Can do: %+v", k)
			}
		}
	}
	if full != 1 || len(marks) != 5 {
		t.Errorf("one mark for the full twisting back, and the tuck and pike backs apart: %d of %d", full, len(marks))
	}
	for _, k := range marks {
		if strings.HasPrefix(k.Item, "s:") && strings.Contains(k.Skill, "rotation") {
			must(t, s.MarkAsGymnast(ctx, m, MarkIn{Item: k.Item}))
		}
	}

	// Clearing it takes it off; a state that isn't one is refused.
	must(t, s.MarkAsGymnast(ctx, m, MarkIn{Item: "s:front"}))
	if marks, _ = s.MemberMarks(ctx, m.ID); len(marks) != 1 {
		t.Errorf("cleared: %+v", marks)
	}
	if err := s.MarkAsGymnast(ctx, m, MarkIn{Item: "s:x", State: "maybe"}); err == nil {
		t.Error("a state that isn't one")
	}

	// A coach who doesn't see the member can't mark.
	bob, _, _ := s.CreateCoach(ctx, club.ID, "Bob")
	must(t, s.SetMemberCoach(ctx, club.ID, m.ID, ann.ID))
	if err := s.MarkAsCoach(ctx, bob, m.ID, MarkIn{Item: "s:x", State: CanDo}); !errors.Is(err, ErrNotFound) {
		t.Errorf("another coach marks: %v", err)
	}

	// The club's setting, and the marks go with the member.
	must(t, s.SetSkillsOpen(ctx, club.ID, true))
	if c, _ := s.ClubByJoinLink(ctx, club.JoinLink); !c.SkillsOpen {
		t.Error("skills open")
	}
	must(t, s.RemoveMember(ctx, club.ID, m.ID))
	if marks, _ = s.MemberMarks(ctx, m.ID); len(marks) != 0 {
		t.Error("a removed member's marks stay")
	}
}
