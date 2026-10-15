package store

import (
	"errors"
	"testing"
	"time"
)

func TestUpNow(t *testing.T) {
	now := time.Date(2027, 3, 13, 9, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition())
	const key, other = "0|Panel 1|BUCS L3||1", "0|Panel 2|BUCS L4||1"
	// Four gymnasts, the second scratched: both rounds skip them.
	open := []bool{true, false, true, true, true, false, true, true}
	at := func() (Up, bool) {
		t.Helper()
		all, err := s.UpNow(ctx, c.ID)
		must(t, err)
		u, ok := all[key]
		return u, ok
	}
	pos := NotUp // where the marshal's page shows the flight
	step := func(move func() (Up, bool, error), want int, moves bool) {
		t.Helper()
		u, moved, err := move()
		must(t, err)
		if moved != moves || u.Position != want {
			t.Errorf("at %d (moved %v), want %d (moved %v)", u.Position, moved, want, moves)
		}
		if got, ok := at(); !ok || got.Position != want {
			t.Errorf("stored at %d (%v), want %d", got.Position, ok, want)
		}
		pos = want
	}
	next := func() (Up, bool, error) { return s.UpNext(ctx, c.ID, key, pos, open, "Ann") }
	back := func() (Up, bool, error) { return s.UpBack(ctx, c.ID, key, pos, open, "Bea") }

	if _, ok := at(); ok {
		t.Error("nothing tapped yet")
	}
	if _, moved, err := back(); err != nil || moved {
		t.Errorf("back before the first tap does nothing: %v %v", moved, err)
	}
	if _, ok := at(); ok {
		t.Error("and stores nothing")
	}
	step(next, 0, true)
	if u, _ := at(); u.Who != "Ann" || !u.At.Equal(now) {
		t.Errorf("who and when: %+v", u)
	}
	for _, want := range []int{2, 3, 4, 6, 7} {
		step(next, want, true)
	}
	step(next, 7, false) // the last routine: nowhere to go
	now = now.Add(time.Minute)
	step(back, 6, true)
	if u, _ := at(); u.Who != "Bea" || !u.At.Equal(now) {
		t.Errorf("who went back, and when: %+v", u)
	}
	for _, want := range []int{4, 3, 2, 0} {
		step(back, want, true)
	}
	step(back, 0, false) // the first: nowhere to go

	// The same tap sent again, from where the flight was, changes nothing.
	step(next, 2, true)
	for _, again := range []func() (Up, bool, error){
		func() (Up, bool, error) { return s.UpNext(ctx, c.ID, key, 0, open, "Ann") },
		func() (Up, bool, error) { return s.UpBack(ctx, c.ID, key, 3, open, "Bea") },
		func() (Up, bool, error) { return s.UpNext(ctx, c.ID, key, NotUp, open, "Ann") },
	} {
		if _, moved, err := again(); !errors.Is(err, ErrUpMoved) || moved {
			t.Errorf("a repeat: %v %v", moved, err)
		}
		if u, _ := at(); u.Position != 2 {
			t.Errorf("a repeat moved it to %d", u.Position)
		}
	}
	step(back, 0, true)

	// The first gymnast scratched: Next from nothing starts at the first who competes.
	_, moved, err := s.UpNext(ctx, c.ID, other, NotUp, []bool{false, true, false, true}, "Ann")
	must(t, err)
	all, err := s.UpNow(ctx, c.ID)
	must(t, err)
	if !moved || all[other].Position != 1 {
		t.Errorf("starts at the first who competes: %+v", all[other])
	}
	if _, moved, err := s.UpNext(ctx, c.ID, "0|Panel 3|BUCS L5||1", NotUp, []bool{false, false}, "Ann"); err != nil || moved {
		t.Errorf("no one to compete: %v %v", moved, err)
	}

	must(t, s.ClearUp(ctx, c.ID, key))
	all, err = s.UpNow(ctx, c.ID)
	must(t, err)
	if _, ok := all[key]; ok || len(all) != 1 {
		t.Errorf("clear forgets the one flight only: %+v", all)
	}
}

func TestUpNowMigration(t *testing.T) {
	dir := t.TempDir()
	all := migrations
	migrations = all[:30] // before who's up now
	old, err := Open(ctx, dir)
	if err != nil {
		migrations = all
		t.Fatal(err)
	}
	now := time.Date(2027, 3, 13, 9, 0, 0, 0, time.UTC)
	old.now = func() time.Time { return now }
	c, _, err := old.CreateCompetition(ctx, competition())
	must(t, err)
	must(t, old.MarkFlight(ctx, c.ID, "f1", "start", "Ann"))
	old.Close()
	migrations = all

	s, err := Open(ctx, dir)
	must(t, err)
	defer s.Close()
	s.now = func() time.Time { return now }
	if times, err := s.FlightTimes(ctx, c.ID); err != nil || times["f1"].Started.IsZero() {
		t.Errorf("flight times are kept: %+v, %v", times, err)
	}
	if _, moved, err := s.UpNext(ctx, c.ID, "f1", NotUp, []bool{true, true}, "Ann"); err != nil || !moved {
		t.Errorf("a flight started before can be tapped through: %v %v", moved, err)
	}
	// Deleting the competition takes its positions with it.
	must(t, s.DeleteCompetition(ctx, c.ID))
	var n int
	must(t, s.db.QueryRow(`SELECT COUNT(*) FROM up_now`).Scan(&n))
	if n != 0 {
		t.Errorf("%d positions left after deleting the competition", n)
	}
}
