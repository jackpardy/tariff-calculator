package web

import (
	"net/http"
	"slices"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The marshal's taps for who's up now (screenup.go): on the On the day page,
// a flight that's running shows who's on, with Next gymnast to tap as each
// one starts, and Back. Scratched gymnasts are skipped. Finishing the flight
// stays a tap of its own.

// dayUp is who's up now in a running flight, for the day page; tapped says
// whether the marshal has tapped yet, and up is how far they've got.
func dayUp(c competitions.Competition, f competitions.ScheduledFlight, byID map[string]store.Entry, scratched map[string]bool, up store.Up, tapped bool) *views.DayUp {
	o := upOrderOf(f, byID, scratched)
	out := &views.DayUp{Tapped: tapped}
	pos := up.Position
	if !tapped {
		pos = slices.Index(o.open, true)
	}
	if _, _, ok := o.at(pos); !ok {
		return out
	}
	g := upGymnast(c, o, pos)
	out.Name, out.No, out.Club, out.Routine = g.Name, g.No, g.Club, g.Routine
	out.Scratched = !o.open[pos]
	out.Last = tapped && len(o.after(pos, 1)) == 0
	out.CanBack = tapped && slices.Contains(o.open[:min(pos, len(o.open))], true)
	return out
}

// markUp moves a running flight on to the next gymnast, or back one (flight:
// its key; what: next or back; day: the page to go back to). A tap that
// changes nothing says so, and the last routine leaves finishing the flight to
// the Finished button.
func (p *competitionPages) markUp(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	key, what := r.FormValue("flight"), r.FormValue("what")
	anchor := flightID(key)
	var f competitions.ScheduledFlight
	found := false
	if c.Published != nil {
		for _, g := range c.Published.Flights {
			if competitions.FlightKey(g) == key {
				f, found = g, true
			}
		}
	}
	if !found || what != "next" && what != "back" {
		message(w, r, http.StatusBadRequest, "Not a flight", "That flight isn't in the published timetable. It may have been changed: go back and try again.")
		return
	}
	times, err := p.st.FlightTimes(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	if t := times[key]; t.Started.IsZero() || !t.Finished.IsZero() {
		http.Redirect(w, r, backToDay(r, f.Name()+" isn't running: tap Started first, and Next gymnast before it's finished.", anchor), http.StatusSeeOther)
		return
	}
	all, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	byID := map[string]store.Entry{}
	for _, e := range live(all) {
		byID[e.ID] = e
	}
	scratched, err := p.scratchedOf(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	o := upOrderOf(f, byID, scratched)
	move := p.st.UpNext
	if what == "back" {
		move = p.st.UpBack
	}
	_, moved, err := move(r.Context(), c.ID, key, o.open, linkOf(r).Name)
	switch {
	case err != nil:
		failed(w, r, err)
	case moved:
		http.Redirect(w, r, backToDay(r, "", anchor), http.StatusSeeOther)
	case what == "back":
		http.Redirect(w, r, backToDay(r, "Nothing to go back to in "+f.Name()+": that's the first gymnast.", anchor), http.StatusSeeOther)
	case !slices.Contains(o.open, true):
		http.Redirect(w, r, backToDay(r, "No one left to compete in "+f.Name()+": everyone is scratched.", anchor), http.StatusSeeOther)
	default:
		http.Redirect(w, r, backToDay(r, "That was the last routine in "+f.Name()+". Tap Finished once it's done.", anchor), http.StatusSeeOther)
	}
}
