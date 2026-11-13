package store

import (
	"testing"
	"time"
)

// A delivery is kept until it's sent, taken once at a time, tried again
// when asked, and kept as failed, without its body, after its last try.
func TestDeliveries(t *testing.T) {
	now := time.Date(2027, 3, 13, 9, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition())
	ds := []Delivery{
		{Kind: DeliverEmail, CompetitionID: c.ID, Who: "Ann Ryan", About: "test", Address: "ann@example.com", Subject: "Hi", Body: "Hello", Headers: map[string]string{"X": "y"}},
		{Kind: DeliverPush, CompetitionID: c.ID, Who: "Bea Kelly", About: "test", Address: "https://push.example/1", Keys: "{}", Body: `{"title":"T"}`, Drop: []string{"s1", "s2"}},
	}
	must(t, s.AddDeliveries(ctx, Round{CompetitionID: c.ID, Kind: RoundChanges}, ds))
	r, ok, err := s.LatestRound(ctx, c.ID, RoundChanges)
	if err != nil || !ok || r.Sending != 2 || r.At != now {
		t.Fatalf("the round: %+v %v %v", r, ok, err)
	}

	email, ok, err := s.TakeDelivery(ctx)
	if err != nil || !ok || email.Address != "ann@example.com" || email.Tries != 1 || email.Headers["X"] != "y" || email.RoundID != r.ID {
		t.Fatalf("the first: %+v %v %v", email, ok, err)
	}
	push, ok, _ := s.TakeDelivery(ctx)
	if !ok || push.Kind != DeliverPush || len(push.Drop) != 2 {
		t.Fatalf("the second: %+v", push)
	}
	if _, ok, _ := s.TakeDelivery(ctx); ok {
		t.Error("each is taken once")
	}
	if n, _ := s.DeliveriesDue(ctx); n != 2 {
		t.Errorf("being sent counts as due: %d", n)
	}

	// The push goes; the email is tried again in a minute.
	must(t, s.Delivered(ctx, push))
	must(t, s.DeliveryFailed(ctx, email, "the mail server was busy", now.Add(time.Minute)))
	if _, ok, _ := s.TakeDelivery(ctx); ok {
		t.Error("not due again yet")
	}
	if at, ok, _ := s.NextDelivery(ctx); !ok || !at.Equal(now.Add(time.Minute)) {
		t.Errorf("next due: %v %v", at, ok)
	}
	now = now.Add(time.Minute)
	again, ok, _ := s.TakeDelivery(ctx)
	if !ok || again.Tries != 2 || again.Why != "the mail server was busy" {
		t.Fatalf("tried again: %+v", again)
	}
	must(t, s.DeliveryFailed(ctx, again, "the address was refused", time.Time{}))
	r, _, _ = s.LatestRound(ctx, c.ID, RoundChanges)
	if r.Pushed != 1 || r.Emailed != 0 || r.Failed != 1 || r.Sending != 0 {
		t.Errorf("counted: %+v", r)
	}
	failed, err := s.FailedDeliveries(ctx, r.ID)
	if err != nil || len(failed) != 1 || failed[0].Who != "Ann Ryan" || failed[0].Why != "the address was refused" || failed[0].Body != "" {
		t.Errorf("who failed and why, without the body: %+v %v", failed, err)
	}
	if _, ok, _ := s.NextDelivery(ctx); ok {
		t.Error("a failed one isn't waiting")
	}

	// What was being sent when the app stopped is sent again.
	must(t, s.AddDeliveries(ctx, Round{}, []Delivery{{Kind: DeliverEmail, About: "test", Address: "x@example.com", Body: "b"}}))
	taken, _, _ := s.TakeDelivery(ctx)
	must(t, s.ResumeDeliveries(ctx))
	if d, ok, _ := s.TakeDelivery(ctx); !ok || d.ID != taken.ID || d.Tries != 2 || d.RoundID != "" {
		t.Errorf("resumed: %+v", d)
	}

	// A failed one goes after a week, with the round; the maintainer's
	// email stays while it's being sent.
	now = now.Add(FailedKeptDays*24*time.Hour + time.Minute)
	if _, _, err := s.DeleteExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if failed, _ := s.FailedDeliveries(ctx, r.ID); len(failed) != 0 {
		t.Error("failed ones are deleted after a week")
	}
	if _, ok, _ := s.LatestRound(ctx, c.ID, RoundChanges); ok {
		t.Error("so is the round")
	}
	if n, _ := s.DeliveriesDue(ctx); n != 1 {
		t.Errorf("one with no competition stays: %d", n)
	}

	// And with the competition.
	must(t, s.AddDeliveries(ctx, Round{CompetitionID: c.ID, Kind: RoundDesk, ID: "m1"}, ds[:1]))
	must(t, s.DeleteCompetition(ctx, c.ID))
	if _, _, err := s.DeleteExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.DeliveriesDue(ctx); n != 1 {
		t.Errorf("a gone competition's are deleted: %d", n)
	}
}

// Changes are cleared only with the deliveries telling them: if keeping
// those fails, the changes stay due, and nothing is kept.
func TestToldChanges(t *testing.T) {
	now := time.Date(2027, 3, 13, 9, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition())
	must(t, s.MarkChanged(ctx, c.ID, []byte("baseline"), now))
	due, err := s.DueChanges(ctx)
	if err != nil || len(due) != 1 {
		t.Fatalf("due: %+v %v", due, err)
	}

	// Working out the messages failed: nothing was claimed.
	if again, _ := s.DueChanges(ctx); len(again) != 1 {
		t.Error("reading what's due claims nothing")
	}
	// Keeping them fails part way (a kind there isn't): neither is kept.
	bad := []Delivery{{Kind: DeliverEmail, CompetitionID: c.ID, About: "test", Address: "a@example.com", Body: "b"}, {Kind: "letter", CompetitionID: c.ID, About: "test"}}
	if told, err := s.ToldChanges(ctx, due[0], bad); told || err == nil {
		t.Errorf("a failure: %v %v", told, err)
	}
	if again, _ := s.DueChanges(ctx); len(again) != 1 {
		t.Error("still due after a failure")
	}
	if n, _ := s.DeliveriesDue(ctx); n != 0 {
		t.Errorf("nothing kept: %d", n)
	}
	if _, ok, _ := s.LatestRound(ctx, c.ID, RoundChanges); ok {
		t.Error("no round kept")
	}

	if told, err := s.ToldChanges(ctx, due[0], bad[:1]); !told || err != nil {
		t.Fatalf("told: %v %v", told, err)
	}
	if again, _ := s.DueChanges(ctx); len(again) != 0 {
		t.Error("told once")
	}
	// Told again (two runs at once): the second keeps nothing.
	if told, err := s.ToldChanges(ctx, due[0], bad[:1]); told || err != nil {
		t.Errorf("told twice: %v %v", told, err)
	}
	if n, _ := s.DeliveriesDue(ctx); n != 1 {
		t.Errorf("one kept: %d", n)
	}
	if r, ok, _ := s.LatestRound(ctx, c.ID, RoundChanges); !ok || r.Sending != 1 {
		t.Errorf("its round: %+v", r)
	}
}
