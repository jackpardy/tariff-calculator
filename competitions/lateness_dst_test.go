package competitions

import (
	"testing"
	"time"
)

// On the day the clocks change, lateness goes by the clock, not by the time
// since midnight, which is an hour out.
func TestLatenessOnClockChangeDays(t *testing.T) {
	dublin, err := time.LoadLocation("Europe/Dublin")
	if err != nil {
		t.Skip("no time zone data")
	}
	f := ScheduledFlight{Flight: Flight{Level: "L", Number: 1, Of: 1}, Area: "Panel 1", Start: 9 * 60, End: 10 * 60}
	s := Schedule{Flights: []ScheduledFlight{f}}
	for _, day := range []time.Time{
		time.Date(2026, time.October, 25, 0, 0, 0, 0, dublin), // the clocks go back
		time.Date(2027, time.March, 28, 0, 0, 0, 0, dublin),   // the clocks go forward
	} {
		finished := time.Date(day.Year(), day.Month(), day.Day(), 10, 0, 0, 0, dublin)
		got := Lateness(s, 0, day, dublin, map[string]Actual{FlightKey(f): {Started: finished.Add(-time.Hour), Finished: finished}})
		if l := got["Panel 1"]; l.Minutes != 0 {
			t.Errorf("%s: finished on time at 10:00, but %d minutes late", day.Format("2 Jan 2006"), l.Minutes)
		}
	}
}
