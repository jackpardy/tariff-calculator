package store

import (
	"errors"
	"testing"
	"time"

	"tariffCalculator/competitions"
)

// l4 is entry, moved to BUCS L4.
func l4(gymnast string) competitions.Entry {
	e := entry(gymnast)
	e.Level = "BUCS L4"
	e.Exercises[0].Option, e.Exercises[1].Option = "builtin:bucs-l4-option-1", "builtin:bucs-l4-second"
	return e
}

// lateCompetition is a competition allowing level changes, with one
// individual's entry made while entries were open, on a store whose clock
// reads *now.
func lateCompetition(t *testing.T, now *time.Time) (*Store, Competition, Entry) {
	t.Helper()
	s := open(t, now)
	was := *now
	*now = time.Date(2027, 3, 1, 8, 0, 0, 0, time.UTC)
	defer func() { *now = was }()
	c := competition()
	c.Levels = append(c.Levels, competitions.Level{Ref: "builtin-level:bucs-l4"})
	got, _, err := s.CreateCompetition(ctx, c)
	must(t, err)
	e, _, err := enter(s, got.ID, entry("Ann"))
	must(t, err)
	must(t, s.SetLate(ctx, got.ID, competitions.LateChanges{Rules: map[string]competitions.LateRule{competitions.LateLevel: {On: true, Fee: 500}}}))
	return s, got, e
}

// Late changes on the store's clock: asked, signed off, accepted (the entry
// changes and the fee is kept) or rejected, each decided once.
func TestLateChangesStore(t *testing.T) {
	now := time.Date(2027, 3, 8, 10, 0, 0, 0, time.UTC)
	s, c, e := lateCompetition(t, &now)

	if _, err := s.RequestLateChange(ctx, c.ID, e.ID, entry("Ann Changed"), "Ann", "", false); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("routine changes aren't allowed: %v", err)
	}
	r, err := s.RequestLateChange(ctx, c.ID, e.ID, l4("Ann"), "Ann", "Moved up", true)
	must(t, err)
	if r.Kind != competitions.LateLevel || r.Status != LateCoach || !r.At.Equal(now) {
		t.Errorf("a level change waiting for a coach, asked now: %+v", r)
	}
	if _, err := s.AcceptLateChange(ctx, c.ID, r.ID, "", "Organiser", 500); !errors.Is(err, ErrNotFound) {
		t.Errorf("not the organiser's to accept until signed off: %v", err)
	}

	now = now.Add(time.Hour)
	must(t, s.SignLateRequest(ctx, c.ID, r.ID, "Coach C", "", "Seen it"))
	got, ok, err := s.LatestLateRequest(ctx, c.ID, e.ID)
	if err != nil || !ok || got.Status != LateWaiting || got.SignedBy != "Coach C" || !got.SignedAt.Equal(now) || got.SignNote != "Seen it" {
		t.Errorf("signed off now, for the organiser: %+v", got)
	}
	if err := s.SignLateRequest(ctx, c.ID, "nonsense", "Coach C", "", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("no such request: %v", err)
	}

	now = now.Add(time.Hour)
	accepted, err := s.AcceptLateChange(ctx, c.ID, r.ID, "Fine", "Organiser", 500)
	must(t, err)
	if accepted.Status != LateAccepted || accepted.Fee != 500 || accepted.DecidedBy != "Organiser" || !accepted.DecidedAt.Equal(now) || accepted.Reason != "Fine" {
		t.Errorf("accepted now, with the fee: %+v", accepted)
	}
	changed, err := s.CompetitionEntry(ctx, c.ID, e.ID)
	must(t, err)
	if changed.Entry.Level != "BUCS L4" || changed.SignedBy != "Coach C" {
		t.Errorf("the entry is the one asked for, signed off by the coach: %+v", changed)
	}
	if _, err := s.AcceptLateChange(ctx, c.ID, r.ID, "", "Organiser", 500); !errors.Is(err, ErrNotFound) {
		t.Errorf("accepted once: %v", err)
	}
	if err := s.RejectLateChange(ctx, c.ID, r.ID, "No", "Organiser"); !errors.Is(err, ErrNotFound) {
		t.Errorf("an accepted change can't be rejected: %v", err)
	}

	// Asked again and rejected.
	now = now.Add(time.Hour)
	again, err := s.RequestLateChange(ctx, c.ID, e.ID, entry("Ann"), "Ann", "", false)
	must(t, err)
	now = now.Add(time.Hour)
	must(t, s.RejectLateChange(ctx, c.ID, again.ID, "Full", "Organiser"))
	got, _, _ = s.LatestLateRequest(ctx, c.ID, e.ID)
	if got.ID != again.ID || got.Status != LateRejected || got.Reason != "Full" || !got.DecidedAt.Equal(now) {
		t.Errorf("rejected now, with the reason: %+v", got)
	}
	if all, _ := s.LateRequests(ctx, c.ID); len(all) != 2 {
		t.Errorf("both kept: %+v", all)
	}
	if changed, _ := s.CompetitionEntry(ctx, c.ID, e.ID); changed.Entry.Level != "BUCS L4" {
		t.Error("a rejected change leaves the entry as it was")
	}
}

// Payments on the store's clock: recorded, listed in order, removed once.
func TestPaymentsStore(t *testing.T) {
	now := time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)
	s, c, e := lateCompetition(t, &now)
	payer := "entry:" + e.ID
	must(t, s.AddPayment(ctx, c.ID, payer, 1200, "Transfer", "Organiser"))
	now = now.Add(time.Hour)
	must(t, s.AddPayment(ctx, c.ID, payer, 500, "", "Treasurer"))
	got, err := s.Payments(ctx, c.ID)
	must(t, err)
	if len(got) != 2 || got[0].Amount != 1200 || got[0].Note != "Transfer" || got[0].Who != "Organiser" || !got[1].At.Equal(now) || got[1].Who != "Treasurer" {
		t.Fatalf("two payments, oldest first: %+v", got)
	}
	must(t, s.RemovePayment(ctx, c.ID, got[0].ID))
	if err := s.RemovePayment(ctx, c.ID, got[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("removed once: %v", err)
	}
	if left, _ := s.Payments(ctx, c.ID); len(left) != 1 || left[0].Amount != 500 {
		t.Errorf("one left: %+v", left)
	}
	other, _, _ := s.CreateCompetition(ctx, competition())
	if err := s.RemovePayment(ctx, other.ID, got[1].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("not another competition's to remove: %v", err)
	}
}

// Subscribing on the store's clock, and turning it off by its link.
func TestSubscribeAndUnsubscribe(t *testing.T) {
	now := time.Date(2027, 2, 1, 8, 30, 0, 0, time.UTC)
	s, c, e := lateCompetition(t, &now)
	push := Subscription{CompetitionID: c.ID, Kind: NotifyIndividual, OwnerID: e.ID, Channel: ByPush, Address: "https://push.example/1", Keys: "{}", Topics: []string{AboutTimetable}}
	got, token, err := s.Subscribe(ctx, push)
	must(t, err)
	if !got.Confirmed || token == "" || !got.CreatedAt.Equal(now) {
		t.Errorf("push needs no confirming, made now: %+v", got)
	}
	// The same endpoint again replaces it.
	now = now.Add(time.Minute)
	if _, token2, _ := s.Subscribe(ctx, push); token2 == token {
		t.Error("a new token for the replacement")
	}
	if mine, _ := s.Subscriptions(ctx, c.ID, NotifyIndividual, e.ID); len(mine) != 1 || !mine[0].CreatedAt.Equal(now) {
		t.Errorf("one push subscription, the latest: %+v", mine)
	}
	if err := s.Unsubscribe(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("the replaced one's link does nothing: %v", err)
	}
	mine, _ := s.Subscriptions(ctx, c.ID, NotifyIndividual, e.ID)
	must(t, s.Unsubscribe(ctx, mine[0].Token))
	if mine, _ := s.Subscriptions(ctx, c.ID, NotifyIndividual, e.ID); len(mine) != 0 {
		t.Errorf("turned off: %+v", mine)
	}
	if err := s.Unsubscribe(ctx, mine[0].Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("turned off once: %v", err)
	}
}
