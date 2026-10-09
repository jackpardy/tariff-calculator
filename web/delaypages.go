package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/views"
)

// What if there's a delay: the organiser says which areas are held up, from
// when and for how long, and sees what it does to the rest of the day. Nothing
// is saved.

// delay is the delay page: the form (day, area, from, minutes) and, once
// asked, what the delay does.
func (p *competitionPages) delay(w http.ResponseWriter, r *http.Request) {
	c, s, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if !s.Planned {
		back(w, r, "Plan the timetable first.")
		return
	}
	q := r.URL.Query()
	page := views.DelayPage{
		Base: adminPath(r.PathValue("token")), Competition: summary(c.Competition, p.now()),
		Day: q.Get("day"), Areas: q["area"], From: q.Get("from"), Minutes: q.Get("minutes"),
		Breaks: q.Get("breaks"), Shorten: q.Get("shorten"), Between: q.Get("between"), Quicker: q.Get("quicker"),
		MaxFlight: q.Get("maxFlight"), Overrun: q.Get("overrun"),
	}
	for i, d := range s.Setup.Days {
		page.Days = append(page.Days, views.FlightOption{Value: strconv.Itoa(i), Label: d.Name})
	}
	for _, a := range s.Setup.Areas {
		page.AllAreas = append(page.AllAreas, a.Name)
	}
	if page.Day != "" {
		page.Asked = true
		day, err1 := strconv.Atoi(page.Day)
		from, err2 := clockOf(page.From)
		minutes, err3 := strconv.Atoi(page.Minutes)
		d := competitions.Delay{Day: day, Areas: page.Areas, From: from, Minutes: minutes, Breaks: page.Breaks}
		easing, err4 := delayEasing(&d, page)
		if err1 != nil || err2 != nil || err3 != nil {
			page.Problems = []string{"Give the day, the time it starts (e.g. 10:30) and the minutes."}
		} else if err4 != nil {
			page.Problems = []string{err4.Error()}
		} else {
			people, names := peopleOf(entries)
			officials, err := p.rotaOf(r, c, entries)
			if err != nil {
				failed(w, r, err)
				return
			}
			for _, o := range officials {
				if names[o.Key] == "" {
					names[o.Key] = o.Name
				}
			}
			out, report, err := s.Delayed(d, people)
			if err != nil {
				page.Problems = sentences(err)
			} else if r.Method == http.MethodPost {
				p.keepWhatIf(w, r, c, out, "The delay's timetable is now the draft.")
				return
			} else {
				page.Keep = r.URL.RequestURI()
				page.Result = delayView(report, names)
				page.Result.Easing = easing
				if d.Eased() {
					plain := competitions.Delay{Day: d.Day, Areas: d.Areas, From: d.From, Minutes: d.Minutes}
					if _, without, err := s.Delayed(plain, people); err == nil {
						page.Result.Without = withoutEasing(without)
					}
				}
			}
		}
	}
	render(w, r, views.Delay(page))
}

// delayEasing reads how the organiser would ease the delay (breaks, shorten,
// between, quicker, maxFlight, overrun; blank for none) into d, and describes
// it.
func delayEasing(d *competitions.Delay, page views.DelayPage) ([]string, error) {
	number := func(v, what string) (int, error) {
		if v == "" {
			return 0, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("%s should be a whole number, not %q", what, v)
		}
		return n, nil
	}
	var err error
	var out []string
	if d.Shorten, err = number(page.Shorten, "Shortening breaks"); err != nil {
		return nil, err
	}
	switch d.Breaks {
	case competitions.BreaksMove:
		out = append(out, "breaks on the held-up areas move later")
	case competitions.BreaksShorten:
		out = append(out, fmt.Sprintf("breaks on the held-up areas can be up to %d min shorter", d.Shorten))
	case competitions.BreaksThrough:
		out = append(out, "the held-up areas work through their breaks")
	}
	if page.Between != "" {
		n, err := number(page.Between, "Minutes between flights")
		if err != nil {
			return nil, err
		}
		d.Between = &n
		out = append(out, fmt.Sprintf("%d min between flights", n))
	}
	if d.Quicker, err = number(page.Quicker, "Quicker turns"); err != nil {
		return nil, err
	} else if d.Quicker > 0 {
		out = append(out, fmt.Sprintf("turns %d%% quicker", d.Quicker))
	}
	if d.MaxFlight, err = number(page.MaxFlight, "Flight size"); err != nil {
		return nil, err
	} else if d.MaxFlight > 0 {
		out = append(out, fmt.Sprintf("flights of an event merged up to %d", d.MaxFlight))
	}
	if d.Overrun, err = number(page.Overrun, "Running over"); err != nil {
		return nil, err
	} else if d.Overrun > 0 {
		out = append(out, fmt.Sprintf("up to %d min past the end of the day", d.Overrun))
	}
	return out, nil
}

// clockOf reads "10:30" as minutes since midnight.
func clockOf(s string) (int, error) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("times should be written as 10:30")
	}
	return h*60 + m, nil
}

// withoutEasing says what the delay alone would do, e.g. "Saturday's flights
// end 18:15, 45 min over; 1 flight no longer fits".
func withoutEasing(r competitions.DelayReport) string {
	var parts []string
	for i, d := range r.After {
		if d == r.Before[i] || d.FlightsEnd == "" {
			continue
		}
		free := fmt.Sprintf("%s free", durationText(d.FreeMinutes))
		if d.FreeMinutes < 0 {
			free = fmt.Sprintf("%s over", durationText(-d.FreeMinutes))
		}
		parts = append(parts, fmt.Sprintf("%s's flights end %s, %s", d.Name, d.FlightsEnd, free))
	}
	switch n := len(r.Unplaced); {
	case n == 1:
		parts = append(parts, "1 flight no longer fits")
	case n > 1:
		parts = append(parts, fmt.Sprintf("%d flights no longer fit", n))
	}
	if len(parts) == 0 {
		return "nothing would change"
	}
	return strings.Join(parts, "; ")
}

// durationText is minutes as "45 min" or "5h 35m".
func durationText(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	return fmt.Sprintf("%dh %02dm", minutes/60, minutes%60)
}

// delayView is a delay's report as the page shows it.
func delayView(r competitions.DelayReport, names map[string]string) *views.DelayResult {
	v := &views.DelayResult{Unplaced: r.Unplaced, ShortRest: r.ShortRest, Merged: r.Merged, Breaks: r.Breaks}
	for i, d := range r.After {
		was := r.Before[i]
		if was == d {
			continue
		}
		v.Days = append(v.Days, views.DelayDay{Name: d.Name, WasEnd: was.FlightsEnd, NowEnd: d.FlightsEnd, WasFree: was.FreeMinutes, NowFree: d.FreeMinutes, End: d.End})
	}
	for _, c := range r.Changes {
		ch := views.DelayChangeView{Flight: c.Flight,
			Was:   fmt.Sprintf("%s %s–%s", c.Area, competitions.Clock(c.Start), competitions.Clock(c.End)),
			Now:   fmt.Sprintf("%s %s–%s", c.NewArea, competitions.Clock(c.NewStart), competitions.Clock(c.NewEnd)),
			Later: c.NewStart - c.Start, Moved: c.Moved}
		switch {
		case c.Moved && c.PastEnd:
			ch.Why = "moved: it would have run past the end of the day"
		case c.Moved && c.Clashes:
			ch.Why = "moved: someone in it would have been needed in two places"
		case c.After != "":
			ch.Why = fmt.Sprintf("%d min later: it wouldn't finish before %s, so it waits for it to end", c.NewStart-c.Start, c.After)
		case c.NewStart > c.Start:
			ch.Why = fmt.Sprintf("%d min later", c.NewStart-c.Start)
		case c.NewStart < c.Start:
			ch.Why = fmt.Sprintf("%d min earlier", c.Start-c.NewStart)
		case c.NewEnd > c.End:
			ch.Why = fmt.Sprintf("under way: finishes %d min late", c.NewEnd-c.End)
		case c.NewEnd < c.End:
			ch.Why = fmt.Sprintf("%d min shorter", c.End-c.NewEnd)
		}
		v.Changes = append(v.Changes, ch)
	}
	for _, c := range r.Clashes {
		name := names[c.Person]
		if name == "" {
			name = "Someone"
		}
		v.Clashes = append(v.Clashes, fmt.Sprintf("%s: %s and %s at %s", name, c.First, c.Second, competitions.Clock(c.At)))
	}
	return v
}
