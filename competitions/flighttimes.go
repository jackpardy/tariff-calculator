package competitions

import (
	"fmt"
	"time"
)

// Flights started and finished on the day (roadmap 2026-10-08): the marshal
// or chair of judges records when each flight really started and finished, so
// planned and actual sit side by side and anyone can see how late an area is
// running.

// FlightKey identifies a published flight, for recording its times: its day,
// area, event and flight number.
func FlightKey(f ScheduledFlight) string {
	return fmt.Sprintf("%d|%s|%s|%s|%d", f.Day, f.Area, f.Level, f.Category, f.Number)
}

// Actual is when a flight really started and finished (zero for not yet).
type Actual struct {
	Started, Finished time.Time
}

// AreaLateness is how late an area is running, measured from the latest thing
// that happened on it.
type AreaLateness struct {
	Minutes int       // late; negative for early
	Flight  string    // the flight it's measured from (its name)
	As      string    // what happened to it: "started" or "finished"
	At      time.Time // when
}

// Lateness is how late each area is running on a day, from the flights
// recorded (by FlightKey). Each area is measured from its flight with the
// latest recorded event (finishing beats starting for the same flight): the
// actual time against the planned one (a flight's end if it finished, its
// start if it only started). Areas with nothing recorded are absent.
func Lateness(s Schedule, day int, firstDay time.Time, loc *time.Location, actual map[string]Actual) map[string]AreaLateness {
	midnight := time.Date(firstDay.Year(), firstDay.Month(), firstDay.Day()+day, 0, 0, 0, 0, loc)
	out := map[string]AreaLateness{}
	for _, f := range s.Flights {
		if f.Day != day {
			continue
		}
		a, ok := actual[FlightKey(f)]
		if !ok || a.Started.IsZero() && a.Finished.IsZero() {
			continue
		}
		at, as, planned := a.Started, "started", f.Start
		if !a.Finished.IsZero() {
			at, as, planned = a.Finished, "finished", f.End
		}
		if cur, ok := out[f.Area]; ok && !at.After(cur.At) {
			continue
		}
		out[f.Area] = AreaLateness{
			Minutes: int(at.In(loc).Sub(midnight).Round(time.Minute).Minutes()) - planned,
			Flight:  f.Name(), As: as, At: at,
		}
	}
	return out
}
