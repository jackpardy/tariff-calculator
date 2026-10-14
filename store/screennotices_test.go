package store

import (
	"testing"
	"time"
)

func TestScreenNotices(t *testing.T) {
	now := time.Date(2027, 3, 13, 10, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, err := s.CreateCompetition(ctx, competition())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ShowOnScreen(ctx, c.ID, "Lunch extended by 15 minutes", now.Add(15*time.Minute), "Organiser"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if err := s.ShowOnScreen(ctx, c.ID, "Panel 2 judges to the desk", time.Time{}, "Chair"); err != nil {
		t.Fatal(err)
	}
	shown, err := s.ScreenNotices(ctx, c.ID)
	if err != nil || len(shown) != 2 || shown[0].Text != "Panel 2 judges to the desk" || !shown[0].Until.IsZero() || shown[1].Who != "Organiser" {
		t.Fatalf("both show, latest first: %+v %v", shown, err)
	}

	// The first comes down at its time; the second stays until taken down.
	now = now.Add(15 * time.Minute)
	if shown, _ = s.ScreenNotices(ctx, c.ID); len(shown) != 1 || shown[0].Text != "Panel 2 judges to the desk" {
		t.Fatalf("the timed one is down: %+v", shown)
	}
	if err := s.TakeDownNotice(ctx, "another competition", shown[0].ID); err != ErrNotFound {
		t.Errorf("only the competition's own: %v", err)
	}
	if err := s.TakeDownNotice(ctx, c.ID, shown[0].ID); err != nil {
		t.Fatal(err)
	}
	if shown, _ = s.ScreenNotices(ctx, c.ID); len(shown) != 0 {
		t.Errorf("taken down: %+v", shown)
	}

	// Only the latest are kept.
	for range MaxScreenNotices + 5 {
		now = now.Add(time.Second)
		if err := s.ShowOnScreen(ctx, c.ID, "x", time.Time{}, "Organiser"); err != nil {
			t.Fatal(err)
		}
	}
	if shown, _ = s.ScreenNotices(ctx, c.ID); len(shown) != MaxScreenNotices {
		t.Errorf("kept %d", len(shown))
	}
}
