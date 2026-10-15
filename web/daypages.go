package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
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
	handle("POST /competitions/admin/{token}/day/clear", p.clearScratch)
	handle("POST /competitions/admin/{token}/day/officials", p.markOfficial)
	handle("POST /competitions/admin/{token}/day/notify", p.notifyOne)
	handle("POST /competitions/admin/{token}/day/up", p.markUp)
	handle("POST /competitions/admin/{token}/day/times", p.correctTimes)
}

// idPart makes text safe to use in an id (and so in a link's #fragment):
// letters, digits and "-" stay, and every other byte becomes "_" and its two
// hex digits, so different text never gives the same id.
func idPart(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "_%02x", c)
		}
	}
	return b.String()
}

// areaID is the id of an area's block on the day page.
func areaID(name string) string { return "area-" + idPart(name) }

// flightID is the id of a flight's box on the day page, from its key.
func flightID(key string) string { return "flight-" + idPart(key) }

// warningsID is the id of the day page's block of scratch warnings, which is
// always there, though it may be empty.
const warningsID = "day-warnings"

// backToDay is where a button on the day page sends the person afterwards: the
// day, the notice if there is one, and the #fragment that puts the page back
// where they were (a flight's box, or the warnings).
func backToDay(r *http.Request, notice, anchor string) string {
	to := adminPath(r.PathValue("token")) + "/day?day=" + url.QueryEscape(r.FormValue("day"))
	if notice != "" {
		to += "&notice=" + url.QueryEscape(notice)
	}
	return to + "#" + anchor
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

// lateText says how an area is running, e.g. "Running 15 min late",
// "Running 1 h 5 min late", "On time" or "5 min early".
func lateText(l competitions.AreaLateness) string {
	switch {
	case l.Minutes > 0:
		return "Running " + minutesText(l.Minutes) + " late"
	case l.Minutes < 0:
		return minutesText(-l.Minutes) + " early"
	}
	return "On time"
}

// minutesText is a number of minutes, in hours and minutes from an hour:
// "45 min", "1 h", "1 h 5 min".
func minutesText(m int) string {
	switch {
	case m < 60:
		return fmt.Sprintf("%d min", m)
	case m%60 == 0:
		return fmt.Sprintf("%d h", m/60)
	}
	return fmt.Sprintf("%d h %d min", m/60, m%60)
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
	all, err := p.entriesOf(r.Context(), c.ID)
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
	scratched := map[string]bool{}
	for id, ch := range checkins {
		scratched[id] = ch.Status == store.Scratched
	}
	ups, err := p.st.UpNow(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	base := adminPath(r.PathValue("token"))
	page := views.DayPage{Base: base, Competition: summary(c.Competition, p.now()), Menu: p.menuFor(r, c, menuDay), Notice: r.URL.Query().Get("notice"), Day: day}
	page.Correct = can(r, "POST", "/day/times")
	if day < len(s.Setup.Days) {
		page.DayName = s.Setup.Days[day].Name
	}
	whatIf := ""
	if can(r, "GET", "/timetable/leave") {
		whatIf = base + "/timetable/leave"
	}
	clears, err := p.st.ScratchClears(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	official, err := p.st.OfficialCheckins(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	page.Scratched = scratchWarnings(*s, day, entries, checkins, official, clears, whatIf)
	rota, err := p.rotaOf(r, c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	_, names := peopleOf(entries)
	nameOf := officialNames(rota, names, false)
	inClub := map[string]bool{} // people who belong to a club, to offer "Their club"
	for _, o := range rota {
		inClub[o.Key] = o.Club != ""
	}
	notify := can(r, "POST", "/day/notify")
	keys := personKeys(entries)
	for i, d := range s.Setup.Days {
		page.Days = append(page.Days, views.DayTab{Name: d.Name, Link: base + "/day?day=" + strconv.Itoa(i), Active: i == day})
	}
	for _, a := range s.Setup.Areas {
		area := views.DayArea{Name: a.Name, ID: areaID(a.Name)}
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
				Key: competitions.FlightKey(f), ID: flightID(competitions.FlightKey(f)), Name: f.Name(),
				Planned: competitions.Clock(f.Start) + "–" + competitions.Clock(f.End),
				Started: clock(t.Started), Finished: clock(t.Finished), Who: t.Who,
				CanStart: t.Started.IsZero(), CanFinish: !t.Started.IsZero() && t.Finished.IsZero(),
			}
			switch {
			case !t.Finished.IsZero():
				df.Undo = "undo-finish"
			case !t.Started.IsZero():
				df.Undo = "undo-start"
			}
			df.Gymnasts = checkinOf(f, byID, checkins, keys, notify)
			for i, g := range df.Gymnasts {
				names := competitions.RoutineNames(c.Competition, byID[g.Entry].Entry)
				df.Gymnasts[i].Routines = names[0] + "|" + names[1]
			}
			switch {
			case df.CanFinish:
				up, tapped := ups[df.Key]
				df.Up = dayUp(c.Competition, f, byID, scratched, up, tapped)
			case df.CanStart:
				// Hidden, for the marshal to go on with if Started is tapped
				// without signal (static/js/taps.js).
				df.Up = dayUp(c.Competition, f, byID, scratched, store.Up{}, false)
				df.Up.Waiting = true
			}
			df.Seats = panelOf(f, day, nameOf, inClub, official[competitions.FlightKey(f)], whatIf, notify)
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

// flightChange is what marking a flight would do.
type flightChange struct {
	flight  competitions.ScheduledFlight
	found   bool   // the flight is in the published timetable
	applies bool   // the mark changes something
	why     string // why a start or finish doesn't apply, for the notice
}

// flightAction finds a published flight by its key, and says whether marking
// it start, finish, undo-finish or undo-start at the time at would change
// anything. Two phones may both have kept a tap for the same flight while
// without signal, so the earliest Started and the earliest Finished win, and
// a Finished before its Started is refused. A tap sent as it's made is
// always the latest, so it changes nothing once someone has marked it.
func (p *competitionPages) flightAction(ctx context.Context, c store.Competition, key, what string, at time.Time) (ch flightChange, err error) {
	if c.Published == nil {
		return ch, nil
	}
	for _, g := range c.Published.Flights {
		if competitions.FlightKey(g) == key {
			ch.flight, ch.found = g, true
		}
	}
	if !ch.found {
		return ch, nil
	}
	times, err := p.st.FlightTimes(ctx, c.ID)
	if err != nil {
		return ch, err
	}
	t := times[key]
	name := ch.flight.Name()
	switch what {
	case "start":
		ch.applies = t.Started.IsZero() || at.Before(t.Started) && (t.Finished.IsZero() || !at.After(t.Finished))
	case "finish":
		switch {
		case t.Started.IsZero():
			ch.why = name + " hasn't started, so Finished wasn't recorded."
		case at.Before(t.Started):
			ch.why = "Finished for " + name + " wasn't recorded: it's earlier than the start (" + t.Started.In(local).Format("15:04") + ")."
		default:
			ch.applies = t.Finished.IsZero() || at.Before(t.Finished)
		}
	case "undo-finish":
		ch.applies = !t.Finished.IsZero()
	case "undo-start":
		ch.applies = !t.Started.IsZero() && t.Finished.IsZero()
	}
	if !ch.applies && ch.why == "" {
		ch.why = "Someone has already done that for " + name + "."
	}
	return ch, nil
}

// markFlight records that a flight started or finished, or takes one back
// (flight: its key; what: start, finish, undo-finish or undo-start; day: the
// page to go back to; tapped and sent: when a tap kept on the phone was made
// and sent, see tapAt). An undo with nothing to take back is the same tap
// sent again, so it quietly changes nothing.
func (p *competitionPages) markFlight(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	key, what := r.FormValue("flight"), r.FormValue("what")
	anchor := flightID(key)
	at, late := p.tapAt(r)
	ch, err := p.flightAction(r.Context(), c, key, what, at)
	switch {
	case err != nil:
		failed(w, r, err)
	case !ch.found || !slices.Contains([]string{"start", "finish", "undo-finish", "undo-start"}, what):
		message(w, r, http.StatusBadRequest, "Not a flight", "That flight isn't in the published timetable. It may have been changed: go back and try again.")
	case late != "":
		http.Redirect(w, r, backToDay(r, late, anchor), http.StatusSeeOther)
	case !ch.applies && strings.HasPrefix(what, "undo-"):
		http.Redirect(w, r, backToDay(r, "", anchor), http.StatusSeeOther)
	case !ch.applies:
		http.Redirect(w, r, backToDay(r, ch.why, anchor), http.StatusSeeOther)
	default:
		if err := p.st.MarkFlightAt(r.Context(), c.ID, key, what, linkOf(r).Name, at); err != nil {
			failed(w, r, err)
			return
		}
		if what == "undo-start" {
			// Undoing the start clears how far the marshal had got (uppages.go).
			if err := p.st.ClearUp(r.Context(), c.ID, key); err != nil {
				failed(w, r, err)
				return
			}
		}
		http.Redirect(w, r, backToDay(r, "", anchor), http.StatusSeeOther)
	}
}

// describeFlightMark says what marking a flight does, for the history; "" if
// it changes nothing.
func (p *competitionPages) describeFlightMark(r *http.Request, c store.Competition) string {
	key, what := r.Form.Get("flight"), r.Form.Get("what")
	at, late := p.tapAt(r)
	if late != "" {
		return ""
	}
	ch, err := p.flightAction(r.Context(), c, key, what, at)
	if err != nil || !ch.found || !ch.applies {
		return ""
	}
	f := ch.flight
	switch what {
	case "start":
		return "Marked a flight started: " + f.Name() + " (" + f.Area + ")" + p.tappedNote(at)
	case "finish":
		return "Marked a flight finished: " + f.Name() + " (" + f.Area + ")" + p.tappedNote(at)
	}
	return "Undid a flight time: " + f.Name() + " (" + f.Area + ")"
}

// lateNotes say how late the areas that the flights run on are, for the day
// that's today: one line per area that's 5 or more minutes late or early,
// e.g. "Panel 2 is running about 15 min late (as of 10:40)" or "Panel 1 is
// running about 1 h 5 min early (as of 09:15)".
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
		// Worded as on the day page and the venue screen (minutesText).
		how := minutesText(l.Minutes) + " late"
		if l.Minutes < 0 {
			how = minutesText(-l.Minutes) + " early"
		}
		out = append(out, fmt.Sprintf("%s is running about %s (as of %s)", f.Area, how, l.At.In(local).Format("15:04")))
	}
	return out, nil
}
