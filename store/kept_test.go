package store

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// A person keeps requirement sets and levels on their page: saved again by
// the browser's id, it changes; at most MaxKept of each kind, none too big;
// taken off by that id; and they go with the person.
func TestKeptRequirements(t *testing.T) {
	now := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	s := open(t, &now)
	club, _, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	m, _, err := s.Join(ctx, club.ID, "A. Murphy")
	must(t, err)
	person := m.PersonID

	added, err := s.KeepRequirements(ctx, person, KeptSet, "set-a", "Grade 1", `{"format":1,"name":"Grade 1","rules":[]}`)
	must(t, err)
	if !added {
		t.Error("a new set is added")
	}
	added, err = s.KeepRequirements(ctx, person, KeptSet, "set-a", "Grade 1 (2027)", `{"format":1,"name":"Grade 1 (2027)","rules":[]}`)
	must(t, err)
	if added {
		t.Error("the same browser id changes it")
	}
	_, err = s.KeepRequirements(ctx, person, KeptLevel, "level-a", "Novice", `{"format":1,"name":"Novice"}`)
	must(t, err)
	kept, err := s.KeptRequirements(ctx, person)
	must(t, err)
	if len(kept) != 2 || kept[0].Kind != KeptSet || kept[0].Name != "Grade 1 (2027)" || kept[1].Kind != KeptLevel || kept[0].InLevel {
		t.Errorf("kept: %+v", kept)
	}
	// A set written for a level only says so, for another device.
	_, err = s.KeepRequirementsIn(ctx, person, KeptSet, "set-a", "Grade 1 (2027)", `{"format":1,"name":"Grade 1 (2027)","rules":[]}`, true)
	must(t, err)
	if kept, _ := s.KeptRequirements(ctx, person); !kept[0].InLevel {
		t.Error("kept as the level's own")
	}

	if _, err := s.KeepRequirements(ctx, person, KeptSet, "big", "Big", strings.Repeat("x", MaxKeptBytes+1)); !errors.Is(err, ErrTooBig) {
		t.Errorf("too big: %v", err)
	}
	if _, err := s.KeepRequirements(ctx, person, "routine", "x", "X", "{}"); err == nil {
		t.Error("only sets and levels are kept")
	}
	for i := range MaxKept - 1 { // one level is kept already
		_, err := s.KeepRequirements(ctx, person, KeptLevel, "l"+string(rune('a'+i%26))+string(rune('a'+i/26)), "L", "{}")
		must(t, err)
	}
	if _, err := s.KeepRequirements(ctx, person, KeptLevel, "one-more", "L", "{}"); !errors.Is(err, ErrLimit) {
		t.Errorf("past the limit: %v", err)
	}

	must(t, s.DropKept(ctx, person, KeptSet, "set-a"))
	kept, _ = s.KeptRequirements(ctx, person)
	for _, k := range kept {
		if k.Kind == KeptSet {
			t.Errorf("taken off: %+v", k)
		}
	}

	// Gone with the person: the member removed, then the tidy-up.
	must(t, s.RemoveMember(ctx, club.ID, m.ID))
	if _, _, err := s.DeleteExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if kept, _ = s.KeptRequirements(ctx, person); len(kept) != 0 {
		t.Errorf("%d kept after the person went", len(kept))
	}
}
