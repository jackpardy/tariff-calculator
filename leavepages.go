package main

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/views"
)

// What if an official has to leave: who, from when, for the day or for good,
// and whether to make each gap the easiest seat to fill or change as few
// seats as can be. Nothing is saved.

func (p *competitionPages) leave(w http.ResponseWriter, r *http.Request) {
	c, s, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if !s.Planned {
		back(w, r, "Plan the timetable first.")
		return
	}
	officials, err := p.rotaOf(r, c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	people, names := peopleOf(entries)
	for _, o := range officials {
		if names[o.Key] == "" {
			names[o.Key] = o.Name
		}
	}
	name := func(k string) string {
		if n := names[k]; n != "" {
			return n
		}
		return "Someone"
	}
	q := r.URL.Query()
	page := views.LeavePage{
		Base: adminPath(r.PathValue("token")), Competition: summary(c.Competition, p.now()),
		Person: q.Get("person"), Day: q.Get("day"), From: q.Get("from"), ForGood: q.Get("forgood") == "1", Prefer: q.Get("prefer"),
	}
	for i, d := range s.Setup.Days {
		page.Days = append(page.Days, views.FlightOption{Value: strconv.Itoa(i), Label: d.Name})
	}
	// Who has seats: they're who can leave.
	seated := map[string]bool{}
	for _, f := range s.Flights {
		for _, d := range f.Officials {
			if d.Person != "" {
				seated[d.Person] = true
			}
		}
	}
	for k := range seated {
		page.People = append(page.People, views.FlightOption{Value: k, Label: name(k)})
	}
	slices.SortFunc(page.People, func(a, b views.FlightOption) int { return strings.Compare(a.Label, b.Label) })

	if page.Person != "" {
		page.Asked = true
		day, err1 := strconv.Atoi(page.Day)
		from, err2 := clockOf(page.From)
		if err1 != nil || err2 != nil {
			page.Problems = []string{"Give the day and the time they leave (e.g. 13:00)."}
		} else {
			_, fills, err := s.Left(competitions.Leave{Person: page.Person, Day: day, From: from, ForGood: page.ForGood, Prefer: page.Prefer}, people, officials)
			if err != nil {
				page.Problems = sentences(err)
			} else {
				page.Result = leaveView(s, fills, name, page.Person)
			}
		}
	}
	render(w, r, views.Leave(page))
}

// leaveView is how each seat is filled, as the page shows it.
func leaveView(s competitions.Schedule, fills []competitions.SeatFill, name func(string) string, person string) *views.LeaveResult {
	v := &views.LeaveResult{Who: name(person)}
	for _, f := range fills {
		sv := views.SeatFillView{
			When:  fmt.Sprintf("%s %s–%s", s.Setup.Days[f.Day].Name, competitions.Clock(f.Start), competitions.Clock(f.End)),
			Where: f.Area, Flight: f.Flight, Role: competitions.RoleName(f.Role),
		}
		switch {
		case f.Empty != "":
			sv.Empty = "No one free can take it: " + strings.ToLower(competitions.RoleName(f.Empty)) + " is left empty."
			v.Empty++
		case len(f.Steps) == 1:
			v.Direct++
		default:
			v.Reworked++
		}
		for _, st := range f.Steps {
			sv.Steps = append(sv.Steps, st.Describe(name))
		}
		if len(f.Steps) > 0 && len(f.Others) > 0 {
			var others []string
			for _, k := range f.Others {
				others = append(others, name(k))
			}
			sv.Others = fmt.Sprintf("Also free for %s: %s", strings.ToLower(competitions.RoleName(f.Steps[len(f.Steps)-1].To)), strings.Join(others, ", "))
		}
		v.Seats = append(v.Seats, sv)
	}
	parts := []string{fmt.Sprintf("%d filled by someone free", v.Direct), fmt.Sprintf("%d by moving others round", v.Reworked)}
	if v.Empty > 0 {
		parts = append(parts, fmt.Sprintf("%d left empty", v.Empty))
	}
	seats := "seats"
	if len(fills) == 1 {
		seats = "seat"
	}
	v.Summary = fmt.Sprintf("%s's %d %s: %s.", v.Who, len(fills), seats, strings.Join(parts, ", "))
	return v
}
