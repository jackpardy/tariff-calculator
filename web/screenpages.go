package web

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The venue screen (roadmap 2026-10-09): a page for a screen at the venue
// that shows what's on each area now and what's next, from the published
// timetable and the times marked on the day. It refreshes itself, and a
// "Venue screen" link can open nothing else.

func (p *competitionPages) registerScreen(handle func(string, http.HandlerFunc)) {
	handle("GET /competitions/admin/{token}/screen", p.screen)
}

// areaNow is what's on an area at a moment, and what's next.
type areaNow struct {
	Flight  *competitions.ScheduledFlight // on now; nil if none
	Planned bool                          // Flight is only there by the plan: nobody has marked it started
	Break   string                        // a block's name, when no flight is on and one covers now
	Next    *competitions.ScheduledFlight // the next flight that hasn't started; nil if none
	// Idle is an area with nothing on and nothing starting in the next
	// screenSoon minutes (or nothing more today), so the screen can shrink it.
	Idle bool
}

// screenSoon is how many minutes ahead of its warm-up an area's next flight
// keeps its card full size on the screen.
const screenSoon = 30

// screenClub is the club the screen shows beside a gymnast: none for one
// entering on their own.
func screenClub(e store.Entry) string {
	if e.Individual {
		return ""
	}
	return e.ClubName
}

// nowOn chooses what's on an area of a day at a moment: the flight that has
// started and not finished by the recorded times (the latest started if
// there are several), else the flight whose planned time contains now, else a
// block covering now. The next flight is the first in start order after that
// which hasn't started or finished (when nothing is on, the first that
// starts after now, or the first of the day when now isn't on that day).
// Once the area's times are being marked that day, the earliest flight that
// hasn't started is on (by the plan) once its time has come, however late,
// so the area never goes blank while it has flights to run; with nothing
// marked, the screen goes by the plan.
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
	// Times marked count whatever date they were marked, as on On the day's
	// pages: the screen shows what the marshals say. Times marked trying the
	// tools out before the day are taken back there (Correct the times).
	minute := n.Hour()*60 + n.Minute()
	untouched := func(f competitions.ScheduledFlight) bool {
		a := actual[competitions.FlightKey(f)]
		return a.Started.IsZero() && a.Finished.IsZero()
	}

	tapped := slices.ContainsFunc(flights, func(f competitions.ScheduledFlight) bool { return !untouched(f) })
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
			// Once the area's times are being marked, a flight whose time has
			// come but that hasn't started is still to come, however late.
			if untouched(f) && f.Start <= minute && (minute < f.End || tapped) {
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
		if on >= 0 && i > on || on < 0 && (!onDay || f.Start > minute || tapped) {
			out.Next = &flights[i]
			break
		}
	}
	out.Idle = on < 0 && out.Break == "" && (out.Next == nil || onDay && out.Next.Start-minute > screenSoon)
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
	page := views.ScreenPage{Competition: c.Name, Time: now.In(local).Format("15:04"), At: now.UnixMilli(), Zone: local.String()}
	notices, err := p.st.ScreenNotices(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	for _, n := range notices {
		page.Notices = append(page.Notices, n.Text)
	}
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
	all, err := p.entriesOf(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	people := map[string]views.FlightGymnast{}
	for _, e := range live(all) {
		people[e.ID] = views.FlightGymnast{Name: e.Entry.Gymnasts(), Club: screenClub(e)}
	}
	scratched, err := p.scratchedOf(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	gymnasts := func(f competitions.ScheduledFlight) []views.FlightGymnast {
		var out []views.FlightGymnast
		for n, id := range f.Entries {
			if g, ok := people[id]; ok {
				g.No, g.Scratched = n+1, scratched[id]
				out = append(out, g)
			}
		}
		return out
	}
	// Only the areas asked for (a screen for some of them), each in the
	// colour of its place in the setup, the same on every screen and day.
	chosen := r.URL.Query()["area"]
	actual := actualOf(times)
	for i, a := range s.Setup.Areas {
		if !slices.ContainsFunc(s.Flights, func(f competitions.ScheduledFlight) bool { return f.Day == day && f.Area == a.Name }) {
			continue
		}
		if len(chosen) > 0 && !slices.Contains(chosen, a.Name) {
			continue
		}
		v := views.ScreenArea{Name: a.Name, Colour: i % views.ScreenColours}
		n := nowOn(*s, day, a.Name, first, actual, now)
		v.Up = p.screenUpOf(r.Context(), c, n, all, scratched)
		switch {
		case n.Flight != nil:
			f := *n.Flight
			v.Now, v.Planned = f.Name(), n.Planned
			v.NowTimes = competitions.Clock(f.Start) + "–" + competitions.Clock(f.End)
			v.Gymnasts = gymnasts(f)
		case n.Break != "":
			v.Now = n.Break
		}
		if n.Next != nil {
			v.Next, v.NextWarmUp = n.Next.Name(), competitions.Clock(n.Next.Start)
			v.NextGymnasts = gymnasts(*n.Next)
		}
		v.Idle, v.Finished = n.Idle, n.Flight == nil && n.Break == "" && n.Next == nil
		if l, ok := late[a.Name]; ok {
			v.Late, v.Behind = lateText(l), l.Minutes > 0
		}
		page.Areas = append(page.Areas, v)
	}
	page.Fit()
	render(w, r, views.Screen(page))
}
