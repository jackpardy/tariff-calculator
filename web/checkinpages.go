package web

import (
	"context"
	"fmt"
	"log"
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
// are left out. keys are the people of each entry; notify says whether to
// offer to Notify the gymnasts who aren't here.
func checkinOf(f competitions.ScheduledFlight, byID map[string]store.Entry, checkins map[string]store.Checkin, keys map[string][]string, notify bool) (string, []views.DayGymnast) {
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
		g := views.DayGymnast{
			Entry: id, Name: e.Entry.Gymnasts(), Club: clubOf(e), State: c.Status, Who: c.Who,
			CanHere: c.Status != store.CheckedIn, CanScratch: c.Status != store.Scratched, CanClear: c.Status != "",
		}
		if notify && c.Status == "" && len(keys[id]) > 0 {
			g.Notify = views.DayNotify{Person: keys[id][0], As: "gymnast", Club: !e.Individual && e.ClubID != ""}
		}
		out = append(out, g)
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

// scratchedPeople are the people with an entry scratched (a synchro pair's
// partner too), by person key, with the ids of their scratched entries.
func scratchedPeople(entries []store.Entry, checkins map[string]store.Checkin) map[string][]string {
	keys := personKeys(entries)
	out := map[string][]string{}
	for _, e := range entries {
		if checkins[e.ID].Status == store.Scratched {
			for _, k := range keys[e.ID] {
				out[k] = append(out[k], e.ID)
			}
		}
	}
	return out
}

// hereSomewhere are the people with an entry checked in, or checked in for a
// panel (official, by flight and then person): they've turned up for
// something, so a scratch elsewhere isn't a worry.
func hereSomewhere(entries []store.Entry, checkins map[string]store.Checkin, official map[string]map[string]store.Checkin) map[string]bool {
	keys := personKeys(entries)
	out := map[string]bool{}
	for _, e := range entries {
		if checkins[e.ID].Status == store.CheckedIn {
			for _, k := range keys[e.ID] {
				out[k] = true
			}
		}
	}
	for _, people := range official {
		for k, c := range people {
			if c.Status == store.OfficialHere {
				out[k] = true
			}
		}
	}
	return out
}

// warned says whether a kind of scratch warning (store.ClearOfficiating or
// store.ClearCompeting) still stands for a person: it hasn't been cleared, and
// no entry of theirs, or seat on a panel, is checked in (which clears both,
// unrecorded).
func warned(person, kind string, here map[string]bool, clears map[string]map[string]store.ScratchClear) bool {
	_, cleared := clears[person][kind]
	return !here[person] && !cleared
}

// scratchWarnings are the scratched people who still officiate or compete on a
// day of the published timetable, with their seats on its flights and their
// other entries in them, less what's been cleared. whatIf is the "what if an
// official has to leave" page, or "" if the link can't use it.
func scratchWarnings(s competitions.Schedule, day int, entries []store.Entry, checkins map[string]store.Checkin, official map[string]map[string]store.Checkin, clears map[string]map[string]store.ScratchClear, whatIf string) []views.DayScratched {
	gone := scratchedPeople(entries, checkins)
	if len(gone) == 0 {
		return nil
	}
	keys, names := peopleOf(entries)
	here := hereSomewhere(entries, checkins, official)
	byID := map[string]store.Entry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	var order []string
	officiating, competing := map[string][]string{}, map[string][]string{}
	seen := func(k string) {
		if len(officiating[k]) == 0 && len(competing[k]) == 0 {
			order = append(order, k)
		}
	}
	for _, f := range s.Flights {
		if f.Day != day {
			continue
		}
		for _, d := range f.Officials {
			if gone[d.Person] != nil && warned(d.Person, store.ClearOfficiating, here, clears) {
				seen(d.Person)
				officiating[d.Person] = append(officiating[d.Person], f.Area+" · "+f.Name()+" · "+competitions.RoleName(d.Role))
			}
		}
		for _, id := range f.Entries {
			if _, ok := byID[id]; !ok || checkins[id].Status == store.Scratched {
				continue
			}
			for _, k := range keys[id] {
				if gone[k] != nil && warned(k, store.ClearCompeting, here, clears) {
					seen(k)
					competing[k] = append(competing[k], f.Name()+" · "+f.Area+" · "+competitions.Clock(f.Start))
				}
			}
		}
	}
	var out []views.DayScratched
	for _, k := range order {
		v := views.DayScratched{Person: k, Name: names[k], Officiating: officiating[k], Competing: competing[k]}
		if v.Name == "" {
			v.Name = "Someone"
		}
		if whatIf != "" && len(v.Officiating) > 0 {
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

// clearAction finds a scratched person by key, and says whether clearing a
// kind of their scratch warning would change anything. found is false for a
// kind that's neither, a person who isn't scratched, or no published timetable.
func (p *competitionPages) clearAction(ctx context.Context, c store.Competition, person, kind string) (name string, applies, found bool, err error) {
	if c.Published == nil || kind != store.ClearOfficiating && kind != store.ClearCompeting {
		return "", false, false, nil
	}
	all, err := p.st.Entries(ctx, c.ID)
	if err != nil {
		return "", false, false, err
	}
	entries := live(all)
	checkins, err := p.st.Checkins(ctx, c.ID)
	if err != nil {
		return "", false, false, err
	}
	if scratchedPeople(entries, checkins)[person] == nil {
		return "", false, false, nil
	}
	_, names := peopleOf(entries)
	name = names[person]
	if name == "" {
		name = "Someone"
	}
	clears, err := p.st.ScratchClears(ctx, c.ID)
	if err != nil {
		return name, false, true, err
	}
	_, cleared := clears[person][kind]
	return name, !cleared, true, nil
}

// clearScratch clears a kind of scratch warning for a person: they're still
// here to officiate, or to compete in their other entries (person: their key;
// kind: officiating or competing; day: the page to go back to).
func (p *competitionPages) clearScratch(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	person, kind := r.FormValue("person"), r.FormValue("kind")
	back := adminPath(r.PathValue("token")) + "/day?day=" + url.QueryEscape(r.FormValue("day"))
	name, applies, found, err := p.clearAction(r.Context(), c, person, kind)
	switch {
	case err != nil:
		failed(w, r, err)
	case !found:
		message(w, r, http.StatusBadRequest, "Not a scratched gymnast", "That person isn't scratched in the published timetable. It may have been changed: go back and try again.")
	case !applies:
		http.Redirect(w, r, back+"&notice="+url.QueryEscape("Someone has already done that for "+name+"."), http.StatusSeeOther)
	default:
		if err := p.st.ClearScratch(r.Context(), c.ID, person, kind, linkOf(r).Name); err != nil {
			failed(w, r, err)
			return
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	}
}

// describeClear says what clearing a scratch warning does, for the history;
// "" if it changes nothing.
func (p *competitionPages) describeClear(r *http.Request, c store.Competition) string {
	name, applies, found, err := p.clearAction(r.Context(), c, r.Form.Get("person"), r.Form.Get("kind"))
	if err != nil || !found || !applies {
		return ""
	}
	return "Cleared " + name + "'s scratch warning (" + r.Form.Get("kind") + ")"
}

// stillHere is a scratched person saying they're still here (the My
// competition page's button): it clears both kinds of warning, as the person,
// and goes back to the page at back. Only once the timetable is published, and
// only for someone scratched.
func (p *competitionPages) stillHere(w http.ResponseWriter, r *http.Request, c store.Competition, key, name, back string) {
	if c.Published == nil {
		message(w, r, http.StatusConflict, "Not published yet", "The timetable isn't published yet, so there's nothing to tell the organisers. Check back once it is.")
		return
	}
	all, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	checkins, err := p.st.Checkins(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	notice := "You're not scratched from anything."
	if scratchedPeople(live(all), checkins)[key] != nil {
		for _, kind := range []string{store.ClearOfficiating, store.ClearCompeting} {
			if err := p.st.ClearScratch(r.Context(), c.ID, key, kind, name); err != nil {
				failed(w, r, err)
				return
			}
		}
		notice = "Thanks: the organisers can see you're here."
		if err := p.st.Record(r.Context(), c.ID, name, name+" said they're here for the rest (from their own page)"); err != nil {
			log.Printf("History: %s: %v", c.ID, err)
		}
	}
	http.Redirect(w, r, back+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}
