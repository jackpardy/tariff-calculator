package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Check-in and scratches (roadmap 2026-10-09): on the On the day page the
// marshal or chair marks who has arrived, and a no-show is scratched. Scratched
// gymnasts are struck through on the printed sheets, and the day page says if
// one of them is also an official that day.

// scratchedOf are the entries scratched, by entry id.
func (p *competitionPages) scratchedOf(ctx context.Context, competitionID string) (map[string]bool, error) {
	checkins, err := p.st.Checkins(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for id, c := range checkins {
		if c.Status == store.Scratched {
			out[id] = true
		}
	}
	return out, nil
}

// checkinOf is a flight's check-in: how many are here and scratched, in words
// ("8 of 10 here, 1 scratched"), and each gymnast in running order with their
// state and what could be done about it. Entries no longer in the competition
// are left out.
func checkinOf(f competitions.ScheduledFlight, byID map[string]store.Entry, checkins map[string]store.Checkin) (string, []views.DayGymnast) {
	var out []views.DayGymnast
	here, scratched := 0, 0
	for _, id := range f.Entries {
		e, ok := byID[id]
		if !ok {
			continue
		}
		c := checkins[id]
		switch c.Status {
		case store.CheckedIn:
			here++
		case store.Scratched:
			scratched++
		}
		out = append(out, views.DayGymnast{
			Entry: id, Name: e.Entry.Gymnasts(), Club: clubOf(e), State: c.Status, Who: c.Who,
			CanHere: c.Status != store.CheckedIn, CanScratch: c.Status != store.Scratched, CanClear: c.Status != "",
		})
	}
	if len(out) == 0 {
		return "", nil
	}
	summary := fmt.Sprintf("%d of %d here", here, len(out))
	if scratched > 0 {
		summary += fmt.Sprintf(", %d scratched", scratched)
	}
	return summary, out
}

// scratchedOfficials are the scratched gymnasts who hold seats on a flight on
// a day of the published timetable, with the seats. whatIf is the "what if an
// official has to leave" page, or "" if the link can't use it.
func scratchedOfficials(s competitions.Schedule, day int, entries []store.Entry, checkins map[string]store.Checkin, whatIf string) []views.DayScratched {
	keys, names := peopleOf(entries)
	gone := map[string]bool{} // person keys
	for _, e := range entries {
		if checkins[e.ID].Status == store.Scratched {
			for _, k := range keys[e.ID] {
				gone[k] = true
			}
		}
	}
	if len(gone) == 0 {
		return nil
	}
	var order []string
	duties := map[string][]string{}
	for _, f := range s.Flights {
		if f.Day != day {
			continue
		}
		for _, d := range f.Officials {
			if !gone[d.Person] {
				continue
			}
			if _, seen := duties[d.Person]; !seen {
				order = append(order, d.Person)
			}
			duties[d.Person] = append(duties[d.Person], f.Area+" · "+f.Name()+" · "+competitions.RoleName(d.Role))
		}
	}
	var out []views.DayScratched
	for _, k := range order {
		v := views.DayScratched{Name: names[k], Duties: duties[k]}
		if v.Name == "" {
			v.Name = "Someone"
		}
		if whatIf != "" {
			v.WhatIf = whatIf + "?" + url.Values{"person": {k}, "day": {strconv.Itoa(day)}}.Encode()
		}
		out = append(out, v)
	}
	return out
}

// checkinAction finds a gymnast's entry in the published timetable, and the
// flight it's in, and says whether setting its check-in to status ("here",
// "scratched", or "" to clear) would change anything. found is false for a
// status that's none of those, or an entry that isn't in a flight.
func (p *competitionPages) checkinAction(ctx context.Context, c store.Competition, entryID, status string) (e store.Entry, f competitions.ScheduledFlight, applies, found bool, err error) {
	if c.Published == nil || !slices.Contains([]string{"", store.CheckedIn, store.Scratched}, status) {
		return e, f, false, false, nil
	}
	i, ok := c.Published.Find(entryID)
	if !ok {
		return e, f, false, false, nil
	}
	all, err := p.st.Entries(ctx, c.ID)
	if err != nil {
		return e, f, false, false, err
	}
	for _, g := range live(all) {
		if g.ID == entryID {
			e, found = g, true
		}
	}
	if !found {
		return e, f, false, false, nil
	}
	checkins, err := p.st.Checkins(ctx, c.ID)
	if err != nil {
		return e, f, false, true, err
	}
	return e, c.Published.Flights[i], checkins[entryID].Status != status, true, nil
}

// markCheckin sets a gymnast's check-in (entry: its id; status: here,
// scratched, or empty to clear; day: the page to go back to).
func (p *competitionPages) markCheckin(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	entryID, status := r.FormValue("entry"), r.FormValue("status")
	back := adminPath(r.PathValue("token")) + "/day?day=" + url.QueryEscape(r.FormValue("day"))
	e, _, applies, found, err := p.checkinAction(r.Context(), c, entryID, status)
	switch {
	case err != nil:
		failed(w, r, err)
	case !found:
		message(w, r, http.StatusBadRequest, "Not a gymnast in a flight", "That gymnast isn't in a flight of the published timetable. It may have been changed: go back and try again.")
	case !applies:
		http.Redirect(w, r, back+"&notice="+url.QueryEscape("Someone has already done that for "+e.Entry.Gymnasts()+"."), http.StatusSeeOther)
	default:
		if err := p.st.SetCheckin(r.Context(), c.ID, entryID, status, linkOf(r).Name); err != nil {
			failed(w, r, err)
			return
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	}
}

// describeCheckin says what a check-in does, for the history; "" if it
// changes nothing.
func (p *competitionPages) describeCheckin(r *http.Request, c store.Competition) string {
	e, f, applies, found, err := p.checkinAction(r.Context(), c, r.Form.Get("entry"), r.Form.Get("status"))
	if err != nil || !found || !applies {
		return ""
	}
	switch r.Form.Get("status") {
	case store.CheckedIn:
		return "Checked in: " + e.Entry.Gymnasts() + " (" + f.Name() + ")"
	case store.Scratched:
		return "Scratched: " + e.Entry.Gymnasts() + " (" + f.Name() + ")"
	}
	return "Cleared check-in: " + e.Entry.Gymnasts()
}
