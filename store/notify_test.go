package store

import (
	"errors"
	"testing"
	"time"
)

// An off link stops every email to its address for its competition, as one
// email covers them all; nothing else.
func TestOffLinkStopsTheAddress(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition())
	other, _, _ := s.CreateCompetition(ctx, competition())
	sub := func(comp, kind, owner, channel, address string) string {
		t.Helper()
		_, token, err := s.Subscribe(ctx, Subscription{CompetitionID: comp, Kind: kind, OwnerID: owner, Channel: channel, Address: address, Topics: Topics})
		must(t, err)
		return token
	}
	member := sub(c.ID, NotifyMember, "m1", ByEmail, "ann@example.com")
	club := sub(c.ID, NotifyClub, "club1", ByEmail, "Ann@Example.com")
	elsewhere := sub(other.ID, NotifyMember, "m1", ByEmail, "ann@example.com")
	someoneElse := sub(c.ID, NotifyMember, "m2", ByEmail, "bob@example.com")
	phone := sub(c.ID, NotifyMember, "m1", ByPush, "https://push.example/ann")

	must(t, s.Unsubscribe(ctx, member))
	for _, token := range []string{member, club} {
		if _, err := s.SubscriptionByToken(ctx, token); !errors.Is(err, ErrNotFound) {
			t.Errorf("stopped: %v", err)
		}
	}
	for _, token := range []string{elsewhere, someoneElse, phone} {
		if _, err := s.SubscriptionByToken(ctx, token); err != nil {
			t.Errorf("another competition, address or a phone stays: %v", err)
		}
	}
	if err := s.Unsubscribe(ctx, member); !errors.Is(err, ErrNotFound) {
		t.Errorf("already stopped: %v", err)
	}
}
