package web

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/views"
)

// The venue screen (roadmap 2026-10-09): a page for a screen at the venue
// that shows what's on each area now and what's next, from the published
// timetable and the times marked on the day. It refreshes itself, and a
// "Venue screen" link can open nothing else.

func (p *competitionPages) registerScreen(handle func(string, http.HandlerFunc)) {
	handle("GET /competitions/admin/{token}/screen", p.screen)
}

// screenGymnasts is how many gymnasts the screen names under Now before it
// says "+N more".
const screenGymnasts = 12

// areaNow is what's on an area at a moment, and what's next.
type areaNow struct {
	Flight  *competitions.ScheduledFlight // on now; nil if none
	Planned bool                          // Flight is only there by the plan: nobody has marked it started
	Break   string                        // a block's name, when no flight is on and one covers now
	Next    *competitions.ScheduledFlight // the next flight that hasn't started; nil if none
}

// nowOn chooses what's on an area of a day at a moment: the flight that has
// started and not finished by the recorded times (the latest started if
// there are several), else the flight whose planned time contains now, else a
// block covering now. The next flight is the first in start order after that
// which hasn't started or finished (when nothing is on, the first that
// starts after now, or the first of the day when now isn't on that day).
// Planned times and blocks only count when now is on the day (firstDay plus
// day, in local time); recorded times count whatever date they were marked.
func nowOn(s competitions.Schedule, day int, area string, firstDay time.Time, actual map[string]competitions.Actual, now time.Time) areaNow {
	var flights []competitions.ScheduledFlight
	for _, f := range s.Flights {
		if f.Day == day && f.Area == area {
			flights = append(flights, f)
		}
	}
	slices.SortStableFunc(flights, func(x, y competitions.ScheduledFlight) int { return x.Start - y.Start })

	n := now.In(local)
	d := firstDay.AddDate(0, 0, day)
	onDay := n.Year() == d.Year() && n.Month() == d.Month() && n.Day() == d.Day()
	// Times marked on another date (trying the tools out before the day,
	// say) say nothing about this day, as for lateness.
	sameDate := func(t time.Time) bool {
		y, m, dd := t.In(local).Date()
		return y == d.Year() && m == d.Month() && dd == d.Day()
	}
	dated := map[string]competitions.Actual{}
	for k, a := range actual {
		if !a.Started.IsZero() && !sameDate(a.Started) {
			a.Started = time.Time{}
		}
		if !a.Finished.IsZero() && !sameDate(a.Finished) {
			a.Finished = time.Time{}
		}
		dated[k] = a
	}
	actual = dated
	minute := n.Hour()*60 + n.Minute()
	untouched := func(f competitions.ScheduledFlight) bool {
		a := actual[competitions.FlightKey(f)]
		return a.Started.IsZero() && a.Finished.IsZero()
	}

	var out areaNow
	on := -1 // index in flights of the one on now
	for i, f := range flights {
		a := actual[competitions.FlightKey(f)]
		if a.Started.IsZero() || !a.Finished.IsZero() {
			continue
		}
		if on < 0 || a.Started.After(actual[competitions.FlightKey(flights[on])].Started) {
			on = i
		}
	}
	if on < 0 && onDay {
		for i, f := range flights {
			if untouched(f) && f.Start <= minute && minute < f.End {
				on, out.Planned = i, true
				break
			}
		}
	}
	if on >= 0 {
		out.Flight = &flights[on]
	} else if onDay {
		for _, b := range s.Blocks {
			if b.Day == day && b.Start <= minute && minute < b.End && (len(b.Areas) == 0 || slices.Contains(b.Areas, area)) {
				out.Break = b.Name
				break
			}
		}
	}
	for i, f := range flights {
		if !untouched(f) {
			continue
		}
		if on >= 0 && i > on || on < 0 && (!onDay || f.Start > minute) {
			out.Next = &flights[i]
			break
		}
	}
	return out
}

// screen is the venue screen for a day (default: today, if it's one of the
// timetable's days, else the first).
func (p *competitionPages) screen(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	now := p.now()
	page := views.ScreenPage{Competition: c.Name, Time: now.In(local).Format("15:04")}
	first, err := c.Day()
	s := c.Published
	if err != nil || s == nil {
		render(w, r, views.Screen(page))
		return
	}
	page.Published = true
	day, _ := todayIndex(c, s, now)
	if v, err := strconv.Atoi(r.URL.Query().Get("day")); err == nil && v >= 0 && v < len(s.Setup.Days) {
		day = v
	}
	if len(s.Setup.Days) > 1 {
		page.Day = s.Setup.Days[day].Name
	}
	times, err := p.st.FlightTimes(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	late, err := p.latenessOf(r.Context(), c, day)
	if err != nil {
		failed(w, r, err)
		return
	}
	all, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	names := map[string]string{}
	for _, e := range live(all) {
		names[e.ID] = e.Entry.Gymnasts()
	}
	scratched, err := p.scratchedOf(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	actual := actualOf(times)
	for _, a := range s.Setup.Areas {
		if !slices.ContainsFunc(s.Flights, func(f competitions.ScheduledFlight) bool { return f.Day == day && f.Area == a.Name }) {
			continue
		}
		v := views.ScreenArea{Name: a.Name}
		n := nowOn(*s, day, a.Name, first, actual, now)
		switch {
		case n.Flight != nil:
			f := *n.Flight
			v.Now, v.Planned = f.Name(), n.Planned
			v.NowTimes = competitions.Clock(f.Start) + "–" + competitions.Clock(f.End)
			for _, id := range f.Entries {
				if name, ok := names[id]; ok {
					v.Gymnasts = append(v.Gymnasts, views.ScreenGymnast{Name: name, Scratched: scratched[id]})
				}
			}
			if more := len(v.Gymnasts) - screenGymnasts; more > 0 {
				v.Gymnasts, v.More = v.Gymnasts[:screenGymnasts], more
			}
		case n.Break != "":
			v.Now, v.Break = n.Break, true
		}
		if n.Next != nil {
			v.Next, v.NextWarmUp = n.Next.Name(), competitions.Clock(n.Next.Start)
		}
		if l, ok := late[a.Name]; ok {
			v.Late, v.Behind, v.Ahead = lateText(l), l.Minutes > 0, l.Minutes < 0
		}
		page.Areas = append(page.Areas, v)
	}
	render(w, r, views.Screen(page))
}
