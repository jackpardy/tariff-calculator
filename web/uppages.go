package web

import (
	"errors"
	"net/http"
	"strconv"

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
	at := up.Routine
	if tapped {
		out.Entry, out.Round = at.Entry, strconv.Itoa(at.Round)
	} else {
		at, _ = o.First()
	}
	if !o.Has(at) {
		return out
	}
	g := upGymnast(c, o, at, byID)
	out.Name, out.No, out.Club, out.Routine = g.Name, g.No, g.Club, g.Routine
	out.Scratched = !o.Open(at)
	out.Last = tapped && len(o.After(at, 1)) == 0
	// Who's next, for the marshal to call.
	if next := o.After(at, 1); tapped && len(next) == 1 {
		n := upGymnast(c, o, next[0], byID)
		out.Next = n.Name + " (" + n.Routine + ")"
	}
	_, back := o.Back(at)
	out.CanBack = tapped && back
	return out
}

// markUp moves a running flight on to the next gymnast, or back one (flight:
// its key; what: next or back; entry and round: the gymnast and routine the
// page showed, "" before the first tap; day: the page to go back to; tapped
// and sent: when a tap kept on the phone was made and sent, see tapAt). A
// tap from a routine the flight has moved on from, as when the same tap is
// sent twice, or one kept on a phone while another phone moved it on,
// quietly changes nothing. A tap that changes nothing otherwise says
// so, and the last routine leaves finishing the flight to the Finished
// button.
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
	var from competitions.Routine
	if entry := r.FormValue("entry"); entry != "" {
		round, err := strconv.Atoi(r.FormValue("round"))
		if err != nil || round < 1 || round > 2 {
			found = false
		}
		from = competitions.Routine{Entry: entry, Round: round}
	}
	if !found || what != "next" && what != "back" {
		message(w, r, http.StatusBadRequest, "Not a flight", "That flight isn't in the published timetable. It may have been changed: go back and try again.")
		return
	}
	at, late := p.tapAt(r)
	if late != "" {
		http.Redirect(w, r, backToDay(r, late, anchor), http.StatusSeeOther)
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
	all, err := p.entriesOf(r.Context(), c.ID)
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
	move := o.Next
	if what == "back" {
		move = o.Back
	}
	_, moved, err := p.st.MoveUpAt(r.Context(), c.ID, key, from, move, linkOf(r).Name, at)
	_, anyone := o.First()
	switch {
	case errors.Is(err, store.ErrUpMoved):
		http.Redirect(w, r, backToDay(r, "", anchor), http.StatusSeeOther)
	case err != nil:
		failed(w, r, err)
	case moved:
		http.Redirect(w, r, backToDay(r, "", anchor), http.StatusSeeOther)
	case what == "back":
		http.Redirect(w, r, backToDay(r, "Nothing to go back to in "+f.Name()+": that's the first gymnast.", anchor), http.StatusSeeOther)
	case !anyone:
		http.Redirect(w, r, backToDay(r, "No one left to compete in "+f.Name()+": everyone is scratched.", anchor), http.StatusSeeOther)
	default:
		http.Redirect(w, r, backToDay(r, "That was the last routine in "+f.Name()+". Tap Finished once it's done.", anchor), http.StatusSeeOther)
	}
}
