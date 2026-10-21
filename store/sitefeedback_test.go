package store

import (
	"errors"
	"testing"
	"time"
)

// Feedback is kept with what was captured, listed newest first, marked dealt
// with and back; the captures go on their own, the words staying.
func TestSiteFeedback(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	s := open(t, &now)
	first, err := s.AddSiteFeedback(ctx, SiteFeedback{Text: "Couldn't find my warm-up", Role: "Member", Page: "/clubs/member/{token}", Details: `{"online":true}`},
		Capture{Picture: []byte("jpeg"), PictureType: "image/jpeg", Snapshot: []byte("gz")})
	must(t, err)
	now = now.Add(time.Hour)
	second, err := s.AddSiteFeedback(ctx, SiteFeedback{Text: "Lovely", Role: "Calculator", Page: "/"}, Capture{})
	must(t, err)

	all, err := s.AllSiteFeedback(ctx)
	must(t, err)
	if len(all) != 2 || all[0].ID != second.ID || all[1].ID != first.ID {
		t.Fatalf("newest first: %+v", all)
	}
	if !all[1].Picture || !all[1].Snapshot || all[0].Picture || all[0].Snapshot || all[1].Details != `{"online":true}` {
		t.Errorf("what's kept with each: %+v", all)
	}
	picture, kind, err := s.SiteFeedbackPicture(ctx, first.ID)
	if err != nil || string(picture) != "jpeg" || kind != "image/jpeg" {
		t.Errorf("the picture: %q %q %v", picture, kind, err)
	}
	if _, _, err := s.SiteFeedbackPicture(ctx, second.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("no picture: %v", err)
	}
	if snapshot, err := s.SiteFeedbackSnapshot(ctx, first.ID); err != nil || string(snapshot) != "gz" {
		t.Errorf("the copy: %q %v", snapshot, err)
	}

	must(t, s.MarkSiteFeedback(ctx, first.ID, true))
	if all, _ := s.AllSiteFeedback(ctx); !all[1].Done() || all[0].Done() {
		t.Errorf("dealt with: %+v", all)
	}
	must(t, s.MarkSiteFeedback(ctx, first.ID, false))
	if all, _ := s.AllSiteFeedback(ctx); all[1].Done() {
		t.Errorf("not dealt with again: %+v", all[1])
	}
	if err := s.MarkSiteFeedback(ctx, "nope", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("marking feedback that isn't there: %v", err)
	}

	if n, err := s.CapturesBytes(ctx); err != nil || n != 6 {
		t.Errorf("captures come to %d %v", n, err)
	}
	if n, err := s.DeleteCaptures(ctx, now.Add(-2*time.Hour)); err != nil || n != 0 {
		t.Errorf("nothing that old: %d %v", n, err)
	}
	if n, err := s.DeleteCaptures(ctx, time.Time{}); err != nil || n != 1 {
		t.Errorf("all deleted: %d %v", n, err)
	}
	if all, _ := s.AllSiteFeedback(ctx); len(all) != 2 || all[1].Picture || all[1].Snapshot {
		t.Errorf("the words stay: %+v", all)
	}
}

// Past the day's limit feedback is refused; past the room for captures, or
// with the disk nearly full, it's kept without them.
func TestSiteFeedbackLimits(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	s := open(t, &now)
	s.free = func() (uint64, error) { return minFreeBytes - 1, nil }
	f, err := s.AddSiteFeedback(ctx, SiteFeedback{Text: "Hi"}, Capture{Picture: []byte("jpeg")})
	must(t, err)
	if f.Picture {
		t.Error("no picture kept with the disk nearly full")
	}
	s.free = func() (uint64, error) { return minFreeBytes, nil }
	for i := 1; i < MaxSiteFeedbackPerDay; i++ {
		_, err := s.AddSiteFeedback(ctx, SiteFeedback{Text: "Hi"}, Capture{})
		must(t, err)
	}
	if _, err := s.AddSiteFeedback(ctx, SiteFeedback{Text: "One more"}, Capture{}); !errors.Is(err, ErrLimit) {
		t.Errorf("past the day's limit: %v", err)
	}
	now = now.Add(24 * time.Hour)
	if _, err := s.AddSiteFeedback(ctx, SiteFeedback{Text: "Next day"}, Capture{}); err != nil {
		t.Errorf("the next day: %v", err)
	}
}

// The maintainer's settings start Full with the button on; the link is
// only ever the latest one made.
func TestFeedbackSettingsAndLink(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	s := open(t, &now)
	if got, err := s.SiteFeedbackSettings(ctx); err != nil || got != (FeedbackSettings{}) {
		t.Errorf("to start with: %+v %v", got, err)
	}
	must(t, s.SetSiteFeedbackSettings(ctx, FeedbackSettings{Off: true, Light: true}))
	if got, _ := s.SiteFeedbackSettings(ctx); got != (FeedbackSettings{Off: true, Light: true}) {
		t.Errorf("kept: %+v", got)
	}
	must(t, s.SetSiteFeedbackSettings(ctx, FeedbackSettings{Light: true}))
	if got, _ := s.SiteFeedbackSettings(ctx); got != (FeedbackSettings{Light: true}) {
		t.Errorf("changed: %+v", got)
	}

	if ok, err := s.IsMaintainerLink(ctx, ""); err != nil || ok {
		t.Errorf("no link yet: %v %v", ok, err)
	}
	old, err := s.NewMaintainerLink(ctx)
	must(t, err)
	if ok, _ := s.IsMaintainerLink(ctx, old); !ok {
		t.Error("the link works")
	}
	latest, err := s.NewMaintainerLink(ctx)
	must(t, err)
	if ok, _ := s.IsMaintainerLink(ctx, old); ok {
		t.Error("an older link stops working")
	}
	if ok, _ := s.IsMaintainerLink(ctx, latest); !ok {
		t.Error("the latest works")
	}
	var value string
	must(t, s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'maintainer_link'`).Scan(&value))
	if value == latest {
		t.Error("only the link's hash is kept")
	}
}
