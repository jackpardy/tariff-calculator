package competitions

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Changing times after planning (roadmap 15): a flight's own warm-up and
// competing, or the averages it goes by, changed on a copy of the draft and
// timed again in place. Later flights on the same area and day move back only
// as far as they must, in order and around blocked time; no flight moves
// earlier, so a shorter flight leaves a gap. Flights already started or
// finished on the day keep their times. The caller compares what the copy
// breaks (Breaks) with what the draft does, as for a manual change.

// FlightShift is a flight whose times a change moved: where it is, and when
// it started, started competing and ended, before and after.
type FlightShift struct {
	Flight                  string // its name, e.g. "BUCS L5 · flight 2 of 3"
	ID                      string
	Day                     int
	Area                    string
	Start, NewStart         int
	Competing, NewCompeting int
	End, NewEnd             int
}

// AreaShift is how many of an area's flights on a day start later after a
// change, and by how much: the least and the most minutes.
type AreaShift struct {
	Day         int
	Area        string
	Flights     int
	Least, Most int
}

// AreaShifts are the flights that start later, by area and day in order. A
// flight that only got longer or shorter, starting when it did, isn't one.
func AreaShifts(shifts []FlightShift) []AreaShift {
	var out []AreaShift
	for _, f := range shifts {
		later := f.NewStart - f.Start
		if later <= 0 {
			continue
		}
		i := slices.IndexFunc(out, func(a AreaShift) bool { return a.Day == f.Day && a.Area == f.Area })
		if i < 0 {
			out = append(out, AreaShift{Day: f.Day, Area: f.Area, Least: later, Most: later})
			i = len(out) - 1
		}
		a := &out[i]
		a.Flights++
		a.Least, a.Most = min(a.Least, later), max(a.Most, later)
	}
	slices.SortFunc(out, func(a, b AreaShift) int { return cmp.Or(a.Day-b.Day, strings.Compare(a.Area, b.Area)) })
	return out
}

// Line says an area's shift in words: "3 flights on Panel 2 start 5 minutes
// later", or "5 to 20 minutes later"; with more than one day, which.
func (a AreaShift) Line(days []Day) string {
	where := a.Area
	if len(days) > 1 && a.Day >= 0 && a.Day < len(days) {
		where += " on " + days[a.Day].Name
	}
	by := minutesWord(float64(a.Most))
	if a.Least != a.Most {
		by = fmt.Sprintf("%d to %s", a.Least, by)
	}
	if a.Flights == 1 {
		return fmt.Sprintf("1 flight on %s starts %s later", where, by)
	}
	return fmt.Sprintf("%d flights on %s start %s later", a.Flights, where, by)
}

// Most a flight's own times can be, in minutes.
const (
	maxOwnWarmUp    = 120
	maxOwnCompeting = 600
)

// SetFlightTimes gives a flight (by index) its own warm-up and competing, in
// minutes all told, in place of what the averages give: nil puts either back
// on the averages. The timetable is timed again (see above); fixed says, by
// FlightKey, which flights have started or finished, which keep their times
// (nil for none). It says which flights' times changed, the flight's own
// among them, or why it can't be done.
func (s *Schedule) SetFlightTimes(flight int, warmUp, competing *int, fixed func(key string) bool) ([]FlightShift, error) {
	if flight < 0 || flight >= len(s.Flights) {
		return nil, errors.New("that flight isn't in the timetable")
	}
	f := &s.Flights[flight]
	if fixed != nil && fixed(FlightKey(*f)) {
		return nil, fmt.Errorf("%s has started, so its times stay as they are", f.Name())
	}
	var errs []error
	if warmUp != nil && (*warmUp < 0 || *warmUp > maxOwnWarmUp) {
		errs = append(errs, fmt.Errorf("a flight's warm-up should be from 0 to %d minutes", maxOwnWarmUp))
	}
	if competing != nil && (*competing < 1 || *competing > maxOwnCompeting) {
		errs = append(errs, fmt.Errorf("a flight's competing should be from 1 to %d minutes", maxOwnCompeting))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	f.WarmUpMinutes, f.CompetingMinutes = cloneInt(warmUp), cloneInt(competing)
	return s.retime(fixed), nil
}

// SetTimes makes the setup's timings, by discipline, and events' own times,
// by event, these (nil for none: the defaults, and each event by its
// discipline's), and times the timetable again as SetFlightTimes does. The
// flights aren't made again: a new largest flight waits for the timetable to
// be planned again. The caller checks the setup first (Setup.Check).
func (s *Schedule) SetTimes(timings map[string]Timings, events map[string]EventTimes, fixed func(key string) bool) []FlightShift {
	s.Setup.Timings, s.Setup.EventTimes = timings, events
	return s.retime(fixed)
}

// Replan plans a timetable again, as PlanStaffed does, its fresh flights
// keeping what the flights they were had. Once the flights are made, and
// before any is placed, each takes the ID of the flight it was, as KeepIDs
// gives them (old: the published flights, then the draft's; fresh says
// which nothing has happened to yet), and that flight's own times in the
// draft (draft: its flights, placed and not), so it's placed at its true
// length. It says which of the draft's flights had own times that no fresh
// flight took, because none is that flight any more.
func Replan(entries []SchedEntry, eventOrder []string, setup Setup, staff *Staffing, seed uint64, old, draft []ScheduledFlight, fresh func(id string) bool) (Schedule, []ScheduledFlight) {
	own := map[string]ScheduledFlight{}
	var keys []string
	for _, g := range draft {
		if _, seen := own[g.ID]; g.ID != "" && g.OwnTimes() && !seen {
			own[g.ID] = g
			keys = append(keys, g.ID)
		}
	}
	taken := map[string]bool{}
	s := planStaffed(entries, eventOrder, setup, staff, seed, func(flights []ScheduledFlight) {
		made := Schedule{Flights: flights}
		made.keepIDs(old, fresh)
		for i := range flights {
			if g, ok := own[flights[i].ID]; ok {
				flights[i].WarmUpMinutes, flights[i].CompetingMinutes = cloneInt(g.WarmUpMinutes), cloneInt(g.CompetingMinutes)
				taken[g.ID] = true
			}
		}
	})
	var dropped []ScheduledFlight
	for _, k := range keys {
		if !taken[k] {
			dropped = append(dropped, own[k])
		}
	}
	return s, dropped
}

// cloneInt is a copy of an optional number, so two flights never share one.
func cloneInt(n *int) *int {
	if n == nil {
		return nil
	}
	v := *n
	return &v
}
