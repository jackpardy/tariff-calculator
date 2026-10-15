package store

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"tariffCalculator/competitions"
)

// Changes to the draft timetable made at once (the organiser's, a helper's,
// an accepted late change) each build on the other: none is lost.
func TestUpdateTimetableInterleaved(t *testing.T) {
	now := time.Date(2027, 3, 13, 9, 0, 0, 0, time.UTC)
	s := open(t, &now)
	c, _, err := s.CreateCompetition(ctx, competition())
	must(t, err)

	// Before there's a draft: one with the default setup.
	must(t, s.UpdateTimetable(ctx, c.ID, func(d *competitions.Schedule) error {
		d.Setup.Rules = nil
		return nil
	}))
	got, err := s.Competition(ctx, c.ID)
	must(t, err)
	if got.Timetable == nil || len(got.Timetable.Setup.Days) == 0 {
		t.Fatalf("a new draft has the default setup: %+v", got.Timetable)
	}

	// One change reads the draft, and another is made before it saves:
	// the second waits for the first, and builds on it.
	first, second := make(chan struct{}), make(chan error)
	go func() {
		<-first
		second <- s.UpdateTimetable(ctx, c.ID, func(d *competitions.Schedule) error {
			d.Setup.Blocks = append(d.Setup.Blocks, competitions.Block{Name: "Second"})
			return nil
		})
	}()
	must(t, s.UpdateTimetable(ctx, c.ID, func(d *competitions.Schedule) error {
		close(first)
		time.Sleep(50 * time.Millisecond) // the other change has started by now
		d.Setup.Blocks = append(d.Setup.Blocks, competitions.Block{Name: "First"})
		return nil
	}))
	must(t, <-second)

	// And many at once.
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.UpdateTimetable(ctx, c.ID, func(d *competitions.Schedule) error {
				d.Setup.Blocks = append(d.Setup.Blocks, competitions.Block{Name: fmt.Sprint(i)})
				return nil
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, err = s.Competition(ctx, c.ID)
	must(t, err)
	if b := got.Timetable.Setup.Blocks; len(b) != 10 || b[0].Name != "First" || b[1].Name != "Second" {
		t.Errorf("every change kept, in turn: %+v", b)
	}

	// A change refused saves nothing.
	no := errors.New("no")
	if err := s.UpdateTimetable(ctx, c.ID, func(d *competitions.Schedule) error {
		d.Setup.Blocks = nil
		return no
	}); !errors.Is(err, no) {
		t.Errorf("the refusal comes back: %v", err)
	}
	if again, _ := s.Competition(ctx, c.ID); len(again.Timetable.Setup.Blocks) != 10 {
		t.Error("a refused change changed the draft")
	}
}
