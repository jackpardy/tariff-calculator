package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// On the day (roadmap 2026-10-08): the marshal or chair of judges taps when
// each flight of the published timetable starts and finishes, so planned and
// actual times sit side by side, everyone can see how late each area is
// running, and the delay "what if" can be filled in from it.

func (p *competitionPages) registerDay(handle func(string, http.HandlerFunc)) {
	handle("GET /competitions/admin/{token}/day", p.day)
	handle("POST /competitions/admin/{token}/day/flight", p.markFlight)
	handle("POST /competitions/admin/{token}/day/checkin", p.markCheckin)
}

// lateEnough is how many minutes late or early an area has to run before
// attendees are told.
const lateEnough = 5

// todayIndex is which of the timetable's days is today, in local time; false
// if none is.
func todayIndex(c store.Competition, s *competitions.Schedule, now time.Time) (int, bool) {
	first, err := c.Day()
	if err != nil || s == nil {
		return 0, false
	}
	today := now.In(local)
	for i := range s.Setup.Days {
		d := first.AddDate(0, 0, i)
		if d.Year() == today.Year() && d.Month() == today.Month() && d.Day() == today.Day() {
			return i, true
		}
	}
	return 0, false
}

// actualOf turns the times stored into those Lateness reads.
func actualOf(times map[string]store.FlightTime) map[string]competitions.Actual {
	out := make(map[string]competitions.Actual, len(times))
	for k, t := range times {
		out[k] = competitions.Actual{Started: t.Started, Finished: t.Finished}
	}
	return out
}

// latenessOf is how late each area of a day of the published timetable is
// running.
func (p *competitionPages) latenessOf(ctx context.Context, c store.Competition, day int) (map[string]competitions.AreaLateness, error) {
	first, err := c.Day()
	if err != nil || c.Published == nil {
		return nil, nil
	}
	times, err := p.st.FlightTimes(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	return competitions.Lateness(*c.Published, day, first, local, actualOf(times)), nil
}

// lateText says how an area is running, e.g. "Running 15 min late", "On time"
// or "5 min early".
func lateText(l competitions.AreaLateness) string {
	switch {
	case l.Minutes > 0:
		return fmt.Sprintf("Running %d min late", l.Minutes)
	case l.Minutes < 0:
		return fmt.Sprintf("%d min early", -l.Minutes)
	}
	return "On time"
}

// day is the on-the-day page for a day (default: today, if it's one of the
// timetable's days, else the first).
func (p *competitionPages) day(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	s := c.Published
	if s == nil {
		message(w, r, http.StatusConflict, "Publish the timetable first", "The on-the-day page works from the published timetable. Publish it, then come back.")
		return
	}
	day, _ := todayIndex(c, s, p.now())
	if v, err := strconv.Atoi(r.URL.Query().Get("day")); err == nil && v >= 0 && v < len(s.Setup.Days) {
		day = v
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
	entries := live(all)
	byID := map[string]store.Entry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	checkins, err := p.st.Checkins(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	base := adminPath(r.PathValue("token"))
	page := views.DayPage{Base: base, Competition: summary(c.Competition, p.now()), Notice: r.URL.Query().Get("notice"), Day: day}
	if can(r, "GET", "/screen") {
		page.Screen = base + "/screen?day=" + strconv.Itoa(day)
	}
	whatIf := ""
	if can(r, "GET", "/timetable/leave") {
		whatIf = base + "/timetable/leave"
	}
	page.Scratched = scratchedOfficials(*s, day, entries, checkins, whatIf)
	for i, d := range s.Setup.Days {
		page.Days = append(page.Days, views.DayTab{Name: d.Name, Link: base + "/day?day=" + strconv.Itoa(i), Active: i == day})
	}
	for _, a := range s.Setup.Areas {
		area := views.DayArea{Name: a.Name}
		var flights []competitions.ScheduledFlight
		for _, f := range s.Flights {
			if f.Day == day && f.Area == a.Name {
				flights = append(flights, f)
			}
		}
		if len(flights) == 0 {
			continue
		}
		slices.SortStableFunc(flights, func(x, y competitions.ScheduledFlight) int { return x.Start - y.Start })
		clock := func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.In(local).Format("15:04")
		}
		for _, f := range flights {
			t := times[competitions.FlightKey(f)]
			df := views.DayFlight{
				Key: competitions.FlightKey(f), Name: f.Name(),
				Planned: competitions.Clock(f.Start) + "–" + competitions.Clock(f.End),
				Started: clock(t.Started), Finished: clock(t.Finished), Who: t.Who,
				CanStart: t.Started.IsZero(), CanFinish: !t.Started.IsZero() && t.Finished.IsZero(),
				CanUndo: !t.Started.IsZero() || !t.Finished.IsZero(),
			}
			df.Checkin, df.Gymnasts = checkinOf(f, byID, checkins)
			area.Flights = append(area.Flights, df)
		}
		marked := slices.ContainsFunc(area.Flights, func(f views.DayFlight) bool { return f.Started != "" || f.Finished != "" })
		if _, ok := late[a.Name]; !ok && marked {
			// Times marked on another date (trying the page out before
			// the day, say) say nothing about how late it's running.
			area.Status = "Not measured: times marked on another date than " + s.Setup.Days[day].Name + "'s"
		}
		if l, ok := late[a.Name]; ok {
			area.Status = lateText(l)
			area.Late, area.Early = l.Minutes > 0, l.Minutes < 0
			area.Since = fmt.Sprintf("measured from %s %s at %s", l.Flight, l.As, clock(l.At))
			if l.Minutes > 0 && can(r, "GET", "/timetable/delay") {
				area.WhatIf = base + "/timetable/delay?" + url.Values{
					"day": {strconv.Itoa(day)}, "area": {a.Name},
					"from": {clock(l.At)}, "minutes": {strconv.Itoa(l.Minutes)},
				}.Encode()
			}
		}
		page.Areas = append(page.Areas, area)
	}
	render(w, r, views.Day(page))
}

// flightAction finds a published flight by its key, and says whether marking
// it start, finish or undo would change anything.
func (p *competitionPages) flightAction(ctx context.Context, c store.Competition, key, what string) (f competitions.ScheduledFlight, applies, found bool, err error) {
	if c.Published == nil {
		return f, false, false, nil
	}
	for _, g := range c.Published.Flights {
		if competitions.FlightKey(g) == key {
			f, found = g, true
		}
	}
	if !found {
		return f, false, false, nil
	}
	times, err := p.st.FlightTimes(ctx, c.ID)
	if err != nil {
		return f, false, true, err
	}
	t := times[key]
	switch what {
	case "start":
		applies = t.Started.IsZero()
	case "finish":
		applies = !t.Started.IsZero() && t.Finished.IsZero()
	case "undo":
		applies = !t.Started.IsZero() || !t.Finished.IsZero()
	}
	return f, applies, true, nil
}

// markFlight records that a flight started or finished now, or takes the
// latest back (flight: its key; what: start, finish or undo; day: the page to
// go back to).
func (p *competitionPages) markFlight(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	key, what := r.FormValue("flight"), r.FormValue("what")
	back := adminPath(r.PathValue("token")) + "/day?day=" + url.QueryEscape(r.FormValue("day"))
	f, applies, found, err := p.flightAction(r.Context(), c, key, what)
	switch {
	case err != nil:
		failed(w, r, err)
	case !found || !slices.Contains([]string{"start", "finish", "undo"}, what):
		message(w, r, http.StatusBadRequest, "Not a flight", "That flight isn't in the published timetable. It may have been changed: go back and try again.")
	case !applies:
		http.Redirect(w, r, back+"&notice="+url.QueryEscape("Someone has already done that for "+f.Name()+"."), http.StatusSeeOther)
	default:
		if err := p.st.MarkFlight(r.Context(), c.ID, key, what, linkOf(r).Name); err != nil {
			failed(w, r, err)
			return
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	}
}

// describeFlightMark says what marking a flight does, for the history; "" if
// it changes nothing.
func (p *competitionPages) describeFlightMark(r *http.Request, c store.Competition) string {
	key, what := r.Form.Get("flight"), r.Form.Get("what")
	f, applies, found, err := p.flightAction(r.Context(), c, key, what)
	if err != nil || !found || !applies {
		return ""
	}
	switch what {
	case "start":
		return "Marked a flight started: " + f.Name() + " (" + f.Area + ")"
	case "finish":
		return "Marked a flight finished: " + f.Name() + " (" + f.Area + ")"
	}
	return "Undid a flight time: " + f.Name() + " (" + f.Area + ")"
}

// lateNotes say how late the areas that the flights run on are, for the day
// that's today: one line per area that's 5 or more minutes late or early,
// e.g. "Panel 2 is running about 15 min late (as of 10:40)".
func (p *competitionPages) lateNotes(ctx context.Context, c store.Competition, flights []competitions.ScheduledFlight) ([]string, error) {
	day, today := todayIndex(c, c.Published, p.now())
	if !today || len(flights) == 0 {
		return nil, nil
	}
	late, err := p.latenessOf(ctx, c, day)
	if err != nil || len(late) == 0 {
		return nil, err
	}
	var out []string
	seen := map[string]bool{}
	for _, f := range flights {
		l, ok := late[f.Area]
		if !ok || f.Day != day || seen[f.Area] || l.Minutes < lateEnough && l.Minutes > -lateEnough {
			continue
		}
		seen[f.Area] = true
		how := fmt.Sprintf("%d min late", l.Minutes)
		if l.Minutes < 0 {
			how = fmt.Sprintf("%d min early", -l.Minutes)
		}
		out = append(out, fmt.Sprintf("%s is running about %s (as of %s)", f.Area, how, l.At.In(local).Format("15:04")))
	}
	return out, nil
}
