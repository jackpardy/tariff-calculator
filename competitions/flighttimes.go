package competitions

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"slices"
	"time"
)

// Flights started and finished on the day (roadmap 2026-10-08): the marshal
// or chair of judges records when each flight really started and finished
// (and, between them, when its warm-up was over: roadmap 15), so planned and
// actual sit side by side and anyone can see how late an area is running.

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

// KeepIDs gives a fresh plan's flights the IDs of the flights they were in
// the timetable it replaces (old: the published flights, then the draft's),
// so what's marked on the day stays with them. A flight takes the ID of one
// of its event with the same gymnasts; failing that, of one of its event with
// the same number that nothing has happened to yet (fresh says so, by ID):
// its panel's check-ins follow the seat. A flight that's none of them never
// has an old flight's ID.
func (s *Schedule) KeepIDs(old []ScheduledFlight, fresh func(id string) bool) {
	s.keepIDs(old, fresh)
	s.Identify()
}

// keepIDs is KeepIDs, leaving the flights that keep none without an ID.
func (s *Schedule) keepIDs(old []ScheduledFlight, fresh func(id string) bool) {
	gymnasts := func(f ScheduledFlight) []string {
		out := slices.Clone(f.Entries)
		slices.Sort(out)
		return out
	}
	byGymnasts := func(f, g ScheduledFlight) bool {
		return f.Group() == g.Group() && slices.Equal(gymnasts(f), gymnasts(g))
	}
	byNumber := func(f, g ScheduledFlight) bool {
		return f.Group() == g.Group() && f.Number == g.Number && fresh(g.ID)
	}
	was := map[string]bool{}
	var olds []ScheduledFlight
	for _, g := range old {
		if g.ID != "" && !was[g.ID] {
			was[g.ID] = true
			olds = append(olds, g)
		}
	}
	kept := map[*ScheduledFlight]bool{}
	used := map[string]bool{}
	for _, match := range []func(f, g ScheduledFlight) bool{byGymnasts, byNumber} {
		for _, fs := range [][]ScheduledFlight{s.Flights, s.Unplaced} {
			for i := range fs {
				f := &fs[i]
				if kept[f] {
					continue
				}
				for _, g := range olds {
					if !used[g.ID] && match(*f, g) {
						f.ID, kept[f], used[g.ID] = g.ID, true, true
						break
					}
				}
			}
		}
	}
	for _, fs := range [][]ScheduledFlight{s.Flights, s.Unplaced} {
		for i := range fs {
			if !kept[&fs[i]] && was[fs[i].ID] {
				fs[i].ID = ""
			}
		}
	}
}

// Actual is when a flight really started, its warm-up was over and it
// finished (zero for not yet; WarmUpOver also for a tap the marshal skipped).
type Actual struct {
	Started, WarmUpOver, Finished time.Time
}

// AreaLateness is how late an area is running, measured from the latest thing
// that happened on it.
type AreaLateness struct {
	Minutes int       // late; negative for early
	Flight  string    // the flight it's measured from (its name)
	As      string    // what happened to it: "started" or "finished"
	At      time.Time // when
	// When that flight's competing began, if its Warm-up over was tapped
	// (zero if not), and how late against the plan; negative for early.
	CompetingAt time.Time
	Competing   int
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
		l := AreaLateness{
			Minutes: clockMinutes(at.In(loc)) - planned,
			Flight:  f.Name(), As: as, At: at,
		}
		if !a.WarmUpOver.IsZero() {
			l.CompetingAt, l.Competing = a.WarmUpOver, clockMinutes(a.WarmUpOver.In(loc))-f.CompetingStarts(s.Setup)
		}
		out[f.Area] = l
	}
	return out
}

// RealTimes are a discipline's warm-up and competing on the day, in minutes a
// competitor, from the flights whose taps measured them: warm-up from Started
// to Warm-up over, competing from Warm-up over to Finished. A flight whose
// Warm-up over was skipped measures neither.
type RealTimes struct {
	Discipline                      string
	WarmUp, Competing               float64 // minutes a competitor
	WarmUpFlights, CompetingFlights int     // how many flights each is from
}

// MeasuredTimes are the real times of a day's flights (by FlightKey), a
// discipline at a time in the usual order, for those with any measured.
func MeasuredTimes(s Schedule, day int, actual map[string]Actual) []RealTimes {
	type sum struct {
		warmUp, competing               time.Duration
		warmUpN, competingN             int // competitors
		warmUpFlights, competingFlights int
	}
	sums := map[string]*sum{}
	for _, f := range s.Flights {
		a, ok := actual[FlightKey(f)]
		if f.Day != day || !ok || a.WarmUpOver.IsZero() || len(f.Entries) == 0 {
			continue
		}
		t := sums[f.Discipline]
		if t == nil {
			t = &sum{}
			sums[f.Discipline] = t
		}
		if !a.Started.IsZero() && a.WarmUpOver.After(a.Started) {
			t.warmUp += a.WarmUpOver.Sub(a.Started)
			t.warmUpN += len(f.Entries)
			t.warmUpFlights++
		}
		if !a.Finished.IsZero() && a.Finished.After(a.WarmUpOver) {
			t.competing += a.Finished.Sub(a.WarmUpOver)
			t.competingN += len(f.Entries)
			t.competingFlights++
		}
	}
	var out []RealTimes
	for _, d := range AllDisciplines {
		t := sums[d]
		if t == nil || t.warmUpFlights == 0 && t.competingFlights == 0 {
			continue
		}
		r := RealTimes{Discipline: d, WarmUpFlights: t.warmUpFlights, CompetingFlights: t.competingFlights}
		if t.warmUpN > 0 {
			r.WarmUp = t.warmUp.Minutes() / float64(t.warmUpN)
		}
		if t.competingN > 0 {
			r.Competing = t.competing.Minutes() / float64(t.competingN)
		}
		out = append(out, r)
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
