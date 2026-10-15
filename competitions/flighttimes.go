package competitions

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

// Flights started and finished on the day (roadmap 2026-10-08): the marshal
// or chair of judges records when each flight really started and finished, so
// planned and actual sit side by side and anyone can see how late an area is
// running.

// FlightKey identifies a published flight, for what happens to it on the
// day (its times, its panel's check-ins, who's up): its ID, so they stay with
// it when it's moved or renumbered and the timetable published again. A
// flight without one (made by hand, in a test) is known by its place.
func FlightKey(f ScheduledFlight) string {
	if f.ID != "" {
		return f.ID
	}
	return PlaceKey(f)
}

// PlaceKey is a flight's place in the timetable: its day, area, event and
// number. It's how flights were known on the day before they had IDs.
func PlaceKey(f ScheduledFlight) string {
	return fmt.Sprintf("%d|%s|%s|%s|%d", f.Day, f.Area, f.Event, f.Category, f.Number)
}

// Identify gives each flight that has no ID, placed or not, one of its own:
// random, so it's no other flight's, before or since.
func (s *Schedule) Identify() {
	s.identify(func() uint64 {
		var b [8]byte
		rand.Read(b[:]) // never returns an error (crypto/rand, Go 1.24+)
		return binary.BigEndian.Uint64(b[:])
	})
}

// identify gives each flight with no ID one from next.
func (s *Schedule) identify(next func() uint64) {
	for _, fs := range [][]ScheduledFlight{s.Flights, s.Unplaced} {
		for i := range fs {
			if fs[i].ID == "" {
				fs[i].ID = fmt.Sprintf("%016x", next())
			}
		}
	}
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
// start if it only started). Times recorded on another date than the day's
// (marking a flight before the competition, say) don't count. Areas with
// nothing recorded are absent.
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
		if y, m, d := at.In(loc).Date(); y != midnight.Year() || m != midnight.Month() || d != midnight.Day() {
			continue
		}
		if cur, ok := out[f.Area]; ok && !at.After(cur.At) {
			continue
		}
		out[f.Area] = AreaLateness{
			Minutes: clockMinutes(at.In(loc)) - planned,
			Flight:  f.Name(), As: as, At: at,
		}
	}
	return out
}

// clockMinutes is a time's minutes since midnight by the clock, as the
// timetable counts them: on the day the clocks change, the time since
// midnight would be an hour out.
func clockMinutes(t time.Time) int {
	t = t.Round(time.Minute)
	return t.Hour()*60 + t.Minute()
}
