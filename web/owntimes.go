package web

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Changing times after planning (roadmap 15): a flight's own warm-up and
// competing, or the timings on Setup, tried on a copy of the draft as it
// stands. Later flights on an area move back only as far as they must, and
// flights started or finished on the day keep their times. A change that
// breaks nothing new is made, saying what moved; one that would asks first,
// offering to plan again with the new times (not once the day has begun),
// to make it anyway, or to leave it.

// timesChange is a change to the times, for tryTimes.
type timesChange struct {
	what string // for the ask, e.g. "Change the times of BUCS L5 · flight 2 of 3"
	menu string // the menu's page it's asked from
	// change makes it on s; fixed says, by FlightKey, which flights keep
	// their times. It says which flights' times it changed, or refuses.
	change func(s *competitions.Schedule, fixed func(key string) bool) ([]competitions.FlightShift, error)
	done   string // the notice once made, e.g. "Times saved."
	anchor string // where on the timetable page it shows
	back   string // where Leave it goes: the page it was asked from
}

// flightTimes gives a flight (flight: its index) times of its own, warmUp and
// competing in minutes all told, or puts it back on the averages
// (action=averages). A figure left as the averages give it, where the flight
// has none of its own, stays on the averages.
func (p *competitionPages) flightTimes(w http.ResponseWriter, r *http.Request) {
	c, draft, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if !draft.Planned {
		back(w, r, "Plan the timetable first; nothing was changed.")
		return
	}
	flight, err := strconv.Atoi(r.FormValue("flight"))
	if err != nil || flight < 0 || flight >= len(draft.Flights) {
		back(w, r, "That flight has moved since; nothing was changed.")
		return
	}
	f := draft.Flights[flight]
	var warmUp, competing *int
	done := "Times saved."
	if r.FormValue("action") == "averages" {
		done = "Back on the averages."
	} else {
		avgWarmUp, avgCompeting := draft.Setup.AverageTimes(f)
		var bad bool
		own := func(field string, have *int, average int) *int {
			v := strings.TrimSpace(r.FormValue(field))
			if v == "" {
				return nil
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				bad = true
				return nil
			}
			if have == nil && n == average {
				return nil // left as the averages give it
			}
			return &n
		}
		warmUp, competing = own("warmUp", f.WarmUpMinutes, avgWarmUp), own("competing", f.CompetingMinutes, avgCompeting)
		if bad {
			back(w, r, "Nothing was changed: a flight's warm-up and competing are whole minutes.")
			return
		}
	}
	marks, err := p.dayMarksOf(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	if marks.underWay(competitions.FlightKey(f)) {
		back(w, r, f.Name()+" has started, so its times stay as they are; nothing was changed. Correct the times is on On the day.")
		return
	}
	key := competitions.FlightKey(f)
	p.tryTimes(w, r, c, draft, entries, marks, timesChange{
		what: "Change the times of " + f.Name(), menu: menuTimetable,
		change: func(s *competitions.Schedule, fixed func(string) bool) ([]competitions.FlightShift, error) {
			i := flight
			// The same flight, by its key, should the draft differ from the
			// page's (a form without its version).
			if j := slices.IndexFunc(s.Flights, func(g competitions.ScheduledFlight) bool { return competitions.FlightKey(g) == key }); j >= 0 {
				i = j
			}
			return s.SetFlightTimes(i, warmUp, competing, fixed)
		},
		done: done, anchor: flightAnchor(f), back: timetablePath(r) + "#" + flightAnchor(f),
	})
}

// tryTimes makes a change to the times on the draft, after planning. It's
// tried on a copy first: if it breaks something that holds now, the
// organiser is asked first (unless asked already, anyway=1), with planning
// again with the new times offered (plan=1) until the day has begun.
func (p *competitionPages) tryTimes(w http.ResponseWriter, r *http.Request, c store.Competition, draft competitions.Schedule, entries []store.Entry, marks dayMarks, ch timesChange) {
	begun := c.Published != nil && len(marks.begun(*c.Published)) > 0
	fixed := func(key string) bool { return marks.underWay(key) }
	people, same, err := p.rotaAndSame(r.Context(), c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	if r.FormValue("plan") == "1" {
		if begun {
			changed(w, r, refused(dayBegunNote), "")
			return
		}
		var dropped []competitions.ScheduledFlight
		err := p.changeDraft(r, c, func(s *competitions.Schedule) error {
			if _, err := ch.change(s, fixed); err != nil {
				return timesRefused(err)
			}
			var err error
			dropped, err = p.replan(c, s, entries, people, same, marks)
			return err
		})
		notice := ch.done + " Planned the timetable again with them."
		if len(dropped) > 0 {
			notice += " Own times dropped: " + flightNames(dropped) + "."
		}
		changed(w, r, err, notice)
		return
	}
	if !asked(r) {
		f, err := p.liveRota(r.Context(), c)
		if err != nil {
			failed(w, r, err)
			return
		}
		trial := draft.Copy()
		if _, err := ch.change(&trial, fixed); err != nil {
			changedAt(w, r, timesRefused(err), "", ch.anchor)
			return
		}
		if newly := competitions.NewBreaks(f.manualBreaks(draft), f.manualBreaks(trial)); len(newly) > 0 {
			page := views.ChangeCheckPage{
				Base: adminPath(r.PathValue("token")), Competition: summary(c, p.now()), Menu: p.menuFor(r, c, ch.menu),
				What: ch.what, Breaks: competitions.BreakLines(newly), Action: r.URL.Path, Fields: postedFields(r), Back: ch.back, Leave: "Leave it",
				Help: "Made anyway, what it breaks shows with the timetable's problems. Either way it goes into the draft: attendees see it only once you publish.",
			}
			if begun {
				page.Begun = "The competition has begun, so the timetable isn't planned again: what's run has run."
			} else {
				// Planned on a copy, to say which own times it would drop.
				plan := trial.Copy()
				dropped, err := p.replan(c, &plan, entries, people, same, marks)
				if err == nil {
					page.Plan = withField(postedFields(r), "plan", "1")
					page.PlanSays = planAgainSays(c, dropped)
				}
			}
			noteAsked(w)
			render(w, r, views.ChangeCheck(page))
			return
		}
	}
	var shifts []competitions.FlightShift
	var days []competitions.Day
	err = p.changeDraft(r, c, func(s *competitions.Schedule) error {
		var err error
		if shifts, err = ch.change(s, fixed); err != nil {
			return timesRefused(err)
		}
		days = s.Setup.Days
		return nil
	})
	changedAt(w, r, err, ch.done+" "+movedLine(shifts, days), ch.anchor)
}

// timesRefused is a change to the times turned down, with why.
func timesRefused(err error) error {
	if no, ok := err.(refused); ok {
		return no
	}
	return refused("Nothing was changed: " + strings.Join(sentences(err), " "))
}

// movedLine says which flights start later after a change to the times, an
// area at a time, e.g. "3 flights on Panel 2 start 5 minutes later.", or
// "Nothing else moved."
func movedLine(shifts []competitions.FlightShift, days []competitions.Day) string {
	areas := competitions.AreaShifts(shifts)
	if len(areas) == 0 {
		return "Nothing else moved."
	}
	var lines []string
	for _, a := range areas {
		lines = append(lines, a.Line(days))
	}
	return strings.Join(lines, "; ") + "."
}

// replan plans the timetable s afresh from its setup, as Plan again does:
// keeping flights' IDs (so what's marked on the day stays with them), their
// own times where the flight is still there afterwards, and giving every
// panel its officials. It says which flights' own times were dropped.
func (p *competitionPages) replan(c store.Competition, s *competitions.Schedule, entries []store.Entry, people []competitions.RotaPerson, same samePeople, marks dayMarks) ([]competitions.ScheduledFlight, error) {
	if err := s.Setup.Check(c.EventNames()); err != nil {
		return nil, refused("Fix the setup first: " + strings.Join(sentences(err), " "))
	}
	// The flights it replaces, the published ones first, for what's marked
	// on the day to stay with them.
	draft := slices.Concat(s.Flights, s.Unplaced)
	var old []competitions.ScheduledFlight
	if c.Published != nil {
		old = slices.Clone(c.Published.Flights)
	}
	old = slices.Concat(old, draft)
	*s = competitions.PlanStaffed(schedEntries(entries, same), c.EventNames(), s.Setup, staffing(c.Competition, people), uint64(p.now().UnixNano()))
	s.KeepIDs(old, func(id string) bool { return !marks.underWay(id) })
	dropped, _ := s.KeepOwnTimes(draft)
	p.staff(s, c, entries, people, same)
	return dropped, nil
}

// planAgainSays is what planning again does, in the words Plan again uses,
// naming the flights whose own times it would drop.
func planAgainSays(c store.Competition, dropped []competitions.ScheduledFlight) []string {
	out := []string{planAgainNote}
	if len(dropped) > 0 {
		out = append(out, "It drops the own times of "+flightNames(dropped)+": planning again doesn't make "+map[bool]string{true: "that flight", false: "those flights"}[len(dropped) == 1]+" again.")
	}
	if c.Published != nil {
		out = append(out, "Clubs, gymnasts and officials keep seeing what you last published until you publish again.")
	}
	return out
}

// planAgainNote is what planning again does to the timetable.
const planAgainNote = "Planning again starts afresh: every flight, running order and seat is made anew, and what you changed by hand is lost."

// flightNames names flights in a line, e.g. "BUCS L5 · flight 1 of 2 and
// Novice".
func flightNames(fs []competitions.ScheduledFlight) string {
	var names []string
	for _, f := range fs {
		names = append(names, f.Name())
	}
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return fmt.Sprintf("%s and %s", strings.Join(names[:len(names)-1], ", "), names[len(names)-1])
}
