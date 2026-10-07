package main

import (
	"fmt"
	"net/http"
	"strconv"

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
		d := competitions.Delay{Day: day, Areas: page.Areas, From: from, Minutes: minutes}
		if err1 != nil || err2 != nil || err3 != nil {
			page.Problems = []string{"Give the day, the time it starts (e.g. 10:30) and the minutes."}
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
			_, report, err := s.Delayed(d, people)
			if err != nil {
				page.Problems = sentences(err)
			} else {
				page.Result = delayView(report, names)
			}
		}
	}
	render(w, r, views.Delay(page))
}

// clockOf reads "10:30" as minutes since midnight.
func clockOf(s string) (int, error) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("times should be written as 10:30")
	}
	return h*60 + m, nil
}

// delayView is a delay's report as the page shows it.
func delayView(r competitions.DelayReport, names map[string]string) *views.DelayResult {
	v := &views.DelayResult{Unplaced: r.Unplaced, ShortRest: r.ShortRest}
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
