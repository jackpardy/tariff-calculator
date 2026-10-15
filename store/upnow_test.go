package store

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"tariffCalculator/competitions"
)

func TestUpNow(t *testing.T) {
	now := time.Date(2027, 3, 13, 9, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, _ := s.CreateCompetition(ctx, competition())
	const key, other = "f1", "f2"
	// Four gymnasts, the second scratched: both rounds skip them.
	order := competitions.UpOrderOf(competitions.ScheduledFlight{Flight: competitions.Flight{Entries: []string{"a", "b", "c", "d"}}},
		func(id string) bool { return id != "b" })
	at := func() (Up, bool) {
		t.Helper()
		all, err := s.UpNow(ctx, c.ID)
		must(t, err)
		u, ok := all[key]
		return u, ok
	}
	var shown competitions.Routine // where the marshal's page shows the flight
	r := func(entry string, round int) competitions.Routine {
		return competitions.Routine{Entry: entry, Round: round}
	}
	step := func(back bool, who string, want competitions.Routine, moves bool) {
		t.Helper()
		move := order.Next
		if back {
			move = order.Back
		}
		u, moved, err := s.MoveUp(ctx, c.ID, key, shown, move, who)
		must(t, err)
		if moved != moves || u.Routine != want {
			t.Errorf("at %v (moved %v), want %v (moved %v)", u.Routine, moved, want, moves)
		}
		if got, ok := at(); !ok || got.Routine != want {
			t.Errorf("stored at %v (%v), want %v", got.Routine, ok, want)
		}
		shown = want
	}

	if _, ok := at(); ok {
		t.Error("nothing tapped yet")
	}
	if _, moved, err := s.MoveUp(ctx, c.ID, key, shown, order.Back, "Bea"); err != nil || moved {
		t.Errorf("back before the first tap does nothing: %v %v", moved, err)
	}
	if _, ok := at(); ok {
		t.Error("and stores nothing")
	}
	step(false, "Ann", r("a", 1), true)
	if u, _ := at(); u.Who != "Ann" || !u.At.Equal(now) {
		t.Errorf("who and when: %+v", u)
	}
	for _, want := range []competitions.Routine{r("c", 1), r("d", 1), r("a", 2), r("c", 2), r("d", 2)} {
		step(false, "Ann", want, true)
	}
	step(false, "Ann", r("d", 2), false) // the last routine: nowhere to go
	now = now.Add(time.Minute)
	step(true, "Bea", r("c", 2), true)
	if u, _ := at(); u.Who != "Bea" || !u.At.Equal(now) {
		t.Errorf("who went back, and when: %+v", u)
	}
	for _, want := range []competitions.Routine{r("a", 2), r("d", 1), r("c", 1), r("a", 1)} {
		step(true, "Bea", want, true)
	}
	step(true, "Bea", r("a", 1), false) // the first: nowhere to go

	// The same tap sent again, from where the flight was, changes nothing.
	step(false, "Ann", r("c", 1), true)
	for _, from := range []competitions.Routine{r("a", 1), r("d", 1), {}} {
		if _, moved, err := s.MoveUp(ctx, c.ID, key, from, order.Next, "Ann"); !errors.Is(err, ErrUpMoved) || moved {
			t.Errorf("a repeat from %v: %v %v", from, moved, err)
		}
		if u, _ := at(); u.Routine != r("c", 1) {
			t.Errorf("a repeat moved it to %v", u.Routine)
		}
	}

	// The first gymnast scratched: Next from nothing starts at the first who competes.
	_, moved, err := s.MoveUp(ctx, c.ID, other, competitions.Routine{}, order.Next, "Ann")
	must(t, err)
	all, err := s.UpNow(ctx, c.ID)
	must(t, err)
	if !moved || all[other].Routine != r("a", 1) {
		t.Errorf("starts at the first who competes: %+v", all[other])
	}
	nobody := competitions.UpOrderOf(competitions.ScheduledFlight{Flight: competitions.Flight{Entries: []string{"x"}}}, func(string) bool { return false })
	if _, moved, err := s.MoveUp(ctx, c.ID, "f3", competitions.Routine{}, nobody.Next, "Ann"); err != nil || moved {
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
	order := competitions.UpOrderOf(competitions.ScheduledFlight{Flight: competitions.Flight{Entries: []string{"a", "b"}}}, func(string) bool { return true })
	if _, moved, err := s.MoveUp(ctx, c.ID, "f1", competitions.Routine{}, order.Next, "Ann"); err != nil || !moved {
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

// Timetables saved before flights had IDs get them, the draft's flights the
// IDs of the same flights published, and the day's marks, kept by each
// flight's place, move to its ID; who's up, a place in the running order,
// becomes the gymnast and routine there (migration 34).
func TestFlightIDsMigration(t *testing.T) {
	dir := t.TempDir()
	all := migrations
	migrations = all[:33]
	old, err := Open(ctx, dir)
	if err != nil {
		migrations = all
		t.Fatal(err)
	}
	c, _, err := old.CreateCompetition(ctx, competition())
	must(t, err)
	flight := func(event string, n int, area string, start int, entries ...string) competitions.ScheduledFlight {
		return competitions.ScheduledFlight{Flight: competitions.Flight{Event: event, Number: n, Of: 2, Entries: entries}, Area: area, Start: start, End: start + 30}
	}
	// Published: L3 flights 1 and 2 on Panel 1. The draft since moved
	// flight 2 to Panel 2, and has a new flight of L4.
	published := competitions.Schedule{Published: true, Planned: true, Flights: []competitions.ScheduledFlight{
		flight("BUCS L3", 1, "Panel 1", 540, "a", "b", "c"), flight("BUCS L3", 2, "Panel 1", 570, "d", "e"),
	}}
	draft := competitions.Schedule{Planned: true, Flights: []competitions.ScheduledFlight{
		flight("BUCS L3", 1, "Panel 1", 540, "a", "b", "c"), flight("BUCS L3", 2, "Panel 2", 600, "e", "d"), flight("BUCS L4", 1, "Panel 1", 600, "f"),
	}}
	first, second := competitions.PlaceKey(published.Flights[0]), competitions.PlaceKey(published.Flights[1])
	d, _ := json.Marshal(draft)
	p, _ := json.Marshal(published)
	_, err = old.db.Exec(`UPDATE competitions SET timetable = $1, published_timetable = $2 WHERE id = $3`, string(d), string(p), c.ID)
	must(t, err)
	must(t, old.MarkFlight(ctx, c.ID, first, "start", "Ann"))
	must(t, old.SetOfficialCheckin(ctx, c.ID, "i:mary", []string{first, second}, OfficialHere, "Chris"))
	// Who's up: the 1st flight at place 4 (b's 2nd routine), the 2nd at 9,
	// a place it doesn't have.
	_, err = old.db.Exec(`INSERT INTO up_now (competition_id, flight, position, who, at) VALUES ($1, $2, 4, 'Ann', ''), ($1, $3, 9, 'Ann', '')`, c.ID, first, second)
	must(t, err)
	old.Close()
	migrations = all

	s, err := Open(ctx, dir)
	must(t, err)
	defer s.Close()
	got, err := s.Competition(ctx, c.ID)
	must(t, err)
	pub, dr := got.Published.Flights, got.Timetable.Flights
	if pub[0].ID == "" || pub[1].ID == "" || pub[0].ID == pub[1].ID || dr[2].ID == "" || dr[2].ID == pub[0].ID || dr[2].ID == pub[1].ID {
		t.Fatalf("every flight has an ID of its own: %+v %+v", pub, dr)
	}
	if dr[0].ID != pub[0].ID || dr[1].ID != pub[1].ID {
		t.Errorf("a draft flight has its published flight's ID, in its place or moved: %s %s, %s %s", dr[0].ID, dr[1].ID, pub[0].ID, pub[1].ID)
	}
	if !got.Published.Published || !got.Timetable.Planned {
		t.Error("the timetables are otherwise as they were")
	}
	times, err := s.FlightTimes(ctx, c.ID)
	must(t, err)
	if times[pub[0].ID].Started.IsZero() || len(times) != 1 {
		t.Errorf("its start moves to the flight's ID: %+v", times)
	}
	checks, err := s.OfficialCheckins(ctx, c.ID)
	must(t, err)
	if checks[pub[0].ID]["i:mary"].Status != OfficialHere || checks[pub[1].ID]["i:mary"].Status != OfficialHere || len(checks) != 2 {
		t.Errorf("check-ins move to the flights' IDs: %+v", checks)
	}
	ups, err := s.UpNow(ctx, c.ID)
	must(t, err)
	if u := ups[pub[0].ID]; u.Routine != (competitions.Routine{Entry: "b", Round: 2}) || len(ups) != 1 {
		t.Errorf("who's up is the gymnast and routine at the place, and a place gone is forgotten: %+v", ups)
	}
}
