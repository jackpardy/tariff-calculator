package store

import (
	"errors"
	"testing"
	"time"
)

// A removed member's or coach's addresses go with them, and a deleted club's
// with the club's, members' and coaches'; other people's stay.
func TestRemovingAPersonDropsTheirSubscriptions(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp, _, _ := s.CreateCompetition(ctx, competition())
	club, _, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	ann, _, err := s.Join(ctx, club.ID, "Ann")
	must(t, err)
	bob, _, err := s.Join(ctx, club.ID, "Bob")
	must(t, err)
	coach, _, err := s.CreateCoach(ctx, club.ID, "Cara")
	must(t, err)
	sub := func(kind, owner, channel, address string) string {
		t.Helper()
		_, token, err := s.Subscribe(ctx, Subscription{CompetitionID: comp.ID, Kind: kind, OwnerID: owner, Channel: channel, Address: address, Topics: Topics})
		must(t, err)
		return token
	}
	has := func(token string) bool {
		_, err := s.SubscriptionByToken(ctx, token)
		return err == nil
	}
	a := sub(NotifyMember, ann.ID, ByEmail, "ann@example.com")
	aPush := sub(NotifyMember, ann.ID, ByPush, "https://push.example/ann")
	b := sub(NotifyMember, bob.ID, ByEmail, "bob@example.com")
	c := sub(NotifyCoach, coach.ID, ByEmail, "cara@example.com")
	k := sub(NotifyClub, club.ID, ByEmail, "sec@example.com")

	must(t, s.RemoveMember(ctx, club.ID, ann.ID, "Club admin"))
	if has(a) || has(aPush) || !has(b) || !has(c) || !has(k) {
		t.Errorf("only Ann's go: %v %v %v %v %v", has(a), has(aPush), has(b), has(c), has(k))
	}
	must(t, s.RemoveCoach(ctx, club.ID, coach.ID, "Club admin"))
	if has(c) || !has(b) || !has(k) {
		t.Error("the coach's go")
	}
	must(t, s.DeleteClub(ctx, club.ID, "Club admin"))
	if has(b) || has(k) {
		t.Error("the club's, and its members', go")
	}
	if err := s.Unsubscribe(ctx, b); !errors.Is(err, ErrNotFound) {
		t.Errorf("already gone: %v", err)
	}
}

// The expiry job deletes an email nobody confirmed after 7 days, and keeps a
// confirmed one and a newer one.
func TestUnconfirmedEmailsExpire(t *testing.T) {
	now := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	comp, _, _ := s.CreateCompetition(ctx, competition())
	club, _, err := s.CreateClub(ctx, "UCD")
	must(t, err)
	m, _, err := s.Join(ctx, club.ID, "Ann")
	must(t, err)
	_, tok, err := s.Subscribe(ctx, Subscription{CompetitionID: comp.ID, Kind: NotifyMember, OwnerID: m.ID, Channel: ByEmail, Address: "ann@example.com", Topics: Topics})
	must(t, err)
	must(t, s.ConfirmSubscription(ctx, tok)) // confirmed: stays
	now = now.AddDate(0, 0, 3)
	_, stale, err := s.Subscribe(ctx, Subscription{CompetitionID: comp.ID, Kind: NotifyClub, OwnerID: club.ID, Channel: ByEmail, Address: "s@example.com", Topics: Topics})
	must(t, err)
	now = now.AddDate(0, 0, 5) // stale is 5 days old, but this makes it...
	_, _, err = s.DeleteExpired(ctx)
	must(t, err)
	if _, err := s.SubscriptionByToken(ctx, stale); err != nil {
		t.Errorf("5 days old stays: %v", err)
	}
	now = now.AddDate(0, 0, 3) // 8 days old
	_, _, err = s.DeleteExpired(ctx)
	must(t, err)
	if _, err := s.SubscriptionByToken(ctx, stale); !errors.Is(err, ErrNotFound) {
		t.Errorf("8 days old unconfirmed goes: %v", err)
	}
	if _, err := s.SubscriptionByToken(ctx, tok); err != nil {
		t.Errorf("confirmed stays: %v", err)
	}
}
