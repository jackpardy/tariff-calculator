package web

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Panel check-in and Notify (roadmap 2026-10-09): on the On the day page the
// chair of judges marks who of a flight's panel has arrived or is missing, and
// the desk or a chair can tell someone who isn't here where they should be.

// runOf is the run of flights that the flight at index i of the schedule is
// part of: its event's flights back to back on one area and day, in order. One
// panel judges the whole run, so a check-in applies to all of it.
func runOf(s competitions.Schedule, i int) []competitions.ScheduledFlight {
	f := s.Flights[i]
	var same []competitions.ScheduledFlight
	for _, g := range s.Flights {
		if g.Day == f.Day && g.Area == f.Area {
			same = append(same, g)
		}
	}
	slices.SortStableFunc(same, func(a, b competitions.ScheduledFlight) int { return a.Start - b.Start })
	at := slices.IndexFunc(same, func(g competitions.ScheduledFlight) bool {
		return competitions.FlightKey(g) == competitions.FlightKey(f)
	})
	lo, hi := at, at
	for lo > 0 && same[lo-1].Event() == f.Event() {
		lo--
	}
	for hi+1 < len(same) && same[hi+1].Event() == f.Event() {
		hi++
	}
	return same[lo : hi+1]
}

// holdsSeat says whether a person has a seat on a flight's panel.
func holdsSeat(f competitions.ScheduledFlight, person string) bool {
	return person != "" && slices.ContainsFunc(f.Officials, func(d competitions.Duty) bool { return d.Person == person })
}

// panelOf is a flight's panel check-in: how many seats are here and missing,
// in words ("4 of 6 here, 1 missing"), and each seated official with their
// state and what could be done about it. names says who a key is; clubs, who
// belongs to a club (by key); checks are the flight's check-ins by person;
// whatIf is the "what if an official has to leave" page, or "" if the link
// can't use it; notify says whether the link can send Notify messages.
func panelOf(f competitions.ScheduledFlight, day int, names func(string) string, clubs map[string]bool, checks map[string]store.Checkin, whatIf string, notify bool) (string, []views.DaySeat) {
	var out []views.DaySeat
	here, missing := 0, 0
	for _, d := range f.Officials {
		if d.Person == "" {
			continue
		}
		c := checks[d.Person]
		switch c.Status {
		case store.OfficialHere:
			here++
		case store.OfficialMissing:
			missing++
		}
		seat := views.DaySeat{
			Person: d.Person, Name: names(d.Person), Role: competitions.RoleName(d.Role), State: c.Status, Who: c.Who,
			CanHere: c.Status != store.OfficialHere, CanMissing: c.Status != store.OfficialMissing, CanClear: c.Status != "",
		}
		if c.Status == store.OfficialMissing && whatIf != "" {
			seat.WhatIf = whatIf + "?" + url.Values{"person": {d.Person}, "day": {strconv.Itoa(day)}}.Encode()
		}
		if notify && c.Status != store.OfficialHere {
			seat.Notify = views.DayNotify{Person: d.Person, As: "official", Club: clubs[d.Person]}
		}
		out = append(out, seat)
	}
	if len(out) == 0 {
		return "", nil
	}
	summary := fmt.Sprintf("%d of %d here", here, len(out))
	if missing > 0 {
		summary += fmt.Sprintf(", %d missing", missing)
	}
	return summary, out
}

// officialChange is what setting a panel check-in would do: the official, the
// flight asked about, the keys of the flights of its run where they hold a
// seat, and whether any of those would change.
type officialChange struct {
	name    string
	flight  competitions.ScheduledFlight
	flights []string
	applies bool
	found   bool
}

// officialAction finds an official's seat on a flight of the published
// timetable, and the run it's in, and says whether setting their check-in to
// status ("here", "missing", or "" to clear) would change anything. found is
// false for a status that's none of those, a flight that isn't published, or a
// person without a seat on it.
func (p *competitionPages) officialAction(r *http.Request, c store.Competition, person, flight, status string) (ch officialChange, err error) {
	if c.Published == nil || !slices.Contains([]string{"", store.OfficialHere, store.OfficialMissing}, status) {
		return ch, nil
	}
	s := *c.Published
	i := slices.IndexFunc(s.Flights, func(g competitions.ScheduledFlight) bool { return competitions.FlightKey(g) == flight })
	if i < 0 || !holdsSeat(s.Flights[i], person) {
		return ch, nil
	}
	ch.flight, ch.found = s.Flights[i], true
	for _, g := range runOf(s, i) {
		if holdsSeat(g, person) {
			ch.flights = append(ch.flights, competitions.FlightKey(g))
		}
	}
	all, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		return ch, err
	}
	entries := live(all)
	people, err := p.rotaOf(r, c, entries)
	if err != nil {
		return ch, err
	}
	_, names := peopleOf(entries)
	ch.name = officialNames(people, names, false)(person)
	checks, err := p.st.OfficialCheckins(r.Context(), c.ID)
	if err != nil {
		return ch, err
	}
	for _, k := range ch.flights {
		if checks[k][person].Status != status {
			ch.applies = true
		}
	}
	return ch, nil
}

// markOfficial sets an official's panel check-in on every flight of the run
// where they hold a seat (person: their key; flight: one of its flights' keys;
// status: here, missing, or empty to clear; day: the page to go back to).
func (p *competitionPages) markOfficial(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	person, status := r.FormValue("person"), r.FormValue("status")
	back := adminPath(r.PathValue("token")) + "/day?day=" + url.QueryEscape(r.FormValue("day"))
	ch, err := p.officialAction(r, c, person, r.FormValue("flight"), status)
	switch {
	case err != nil:
		failed(w, r, err)
	case !ch.found:
		message(w, r, http.StatusBadRequest, "Not an official on that panel", "That person doesn't have a seat on that flight of the published timetable. It may have been changed: go back and try again.")
	case !ch.applies:
		http.Redirect(w, r, back+"&notice="+url.QueryEscape("Someone has already done that for "+ch.name+"."), http.StatusSeeOther)
	default:
		if err := p.st.SetOfficialCheckin(r.Context(), c.ID, person, ch.flights, status, linkOf(r).Name); err != nil {
			failed(w, r, err)
			return
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	}
}

// describeOfficial says what a panel check-in does, for the history; "" if it
// changes nothing.
func (p *competitionPages) describeOfficial(r *http.Request, c store.Competition) string {
	status := r.Form.Get("status")
	ch, err := p.officialAction(r, c, r.Form.Get("person"), r.Form.Get("flight"), status)
	if err != nil || !ch.found || !ch.applies {
		return ""
	}
	where := ": " + ch.name + " (" + ch.flight.Event() + ", " + ch.flight.Area + ")"
	switch status {
	case store.OfficialHere:
		return "Checked in official" + where
	case store.OfficialMissing:
		return "Marked an official missing" + where
	}
	return "Cleared an official's check-in" + where
}

// clubOfPerson is the id of the club a person belongs to: that of an entry of
// theirs that came from a club, or else the club their member key is in.
// "" for none (an individual, or an official the organiser added).
func (p *competitionPages) clubOfPerson(r *http.Request, c store.Competition, entries []store.Entry, person string) (string, error) {
	keys := personKeys(entries)
	for _, e := range entries {
		if !e.Individual && e.ClubID != "" && slices.Contains(keys[e.ID], person) {
			return e.ClubID, nil
		}
	}
	id, ok := strings.CutPrefix(person, "m:")
	if !ok {
		return "", nil
	}
	clubs, err := p.st.CompetitionClubs(r.Context(), c.ID)
	if err != nil {
		return "", err
	}
	for _, cl := range clubs {
		members, err := p.st.Members(r.Context(), cl.ID)
		if err != nil {
			return "", err
		}
		if slices.ContainsFunc(members, func(m store.Member) bool { return m.ID == id }) {
			return cl.ID, nil
		}
	}
	return "", nil
}

// notifyDraft is a Notify message ready to send, or what's wrong with the
// form: notFound for a person or flight the published timetable doesn't have,
// problem for something the sender can put right.
type notifyDraft struct {
	msg      store.DeskMessage
	about    string // who it is for, for the history, e.g. "Mary (and their club)"
	area     string
	notFound bool
	problem  string
}

// notifyFormOf reads the Notify form (person, flight, as: official or gymnast,
// them, club, note) and builds the message for them. err is the store failing.
func (p *competitionPages) notifyFormOf(r *http.Request, c store.Competition) (d notifyDraft, err error) {
	ctx := r.Context()
	r.ParseForm()
	person, as := r.Form.Get("person"), r.Form.Get("as")
	if c.Published == nil || as != "official" && as != "gymnast" {
		d.notFound = true
		return d, nil
	}
	s := *c.Published
	i := slices.IndexFunc(s.Flights, func(g competitions.ScheduledFlight) bool { return competitions.FlightKey(g) == r.Form.Get("flight") })
	if i < 0 {
		d.notFound = true
		return d, nil
	}
	f := s.Flights[i]
	all, err := p.st.Entries(ctx, c.ID)
	if err != nil {
		return d, err
	}
	entries := live(all)
	keys, names := peopleOf(entries)
	var text, name string
	var people []string
	switch as {
	case "official":
		j := slices.IndexFunc(f.Officials, func(o competitions.Duty) bool { return o.Person == person && person != "" })
		if j < 0 {
			d.notFound = true
			return d, nil
		}
		rota, err := p.rotaOf(r, c, entries)
		if err != nil {
			return d, err
		}
		name, people = officialNames(rota, names, false)(person), []string{person}
		text = fmt.Sprintf("%s should be at %s now to officiate %s (%s).", name, f.Area, f.Name(), competitions.RoleName(f.Officials[j].Role))
	default:
		byID := map[string]store.Entry{}
		for _, e := range entries {
			byID[e.ID] = e
		}
		for _, id := range f.Entries {
			if e, ok := byID[id]; ok && slices.Contains(keys[id], person) {
				name, people = e.Entry.Gymnasts(), keys[id]
				text = fmt.Sprintf("%s should be at %s now to compete in %s, warm-up %s.", name, f.Area, f.Name(), competitions.Clock(f.Start))
				break
			}
		}
		if text == "" {
			d.notFound = true
			return d, nil
		}
	}
	if note := strings.TrimSpace(r.Form.Get("note")); note != "" {
		text += " " + limitNote(note)
	}
	d.area = f.Area
	d.msg = store.DeskMessage{Text: text, Who: linkOf(r).Name}
	them, club := r.Form.Get("them") == "1", r.Form.Get("club") == "1"
	if club {
		id, err := p.clubOfPerson(r, c, entries, person)
		if err != nil {
			return d, err
		}
		if id != "" {
			d.msg.Staff = []string{id}
		} else {
			club = false
			if !them {
				d.problem = name + " doesn't belong to a club."
			}
		}
	}
	switch {
	case d.problem != "":
	case !them && !club:
		d.problem = "Tick who to tell: them, their club, or both."
	case them && club:
		d.msg.People, d.msg.Audience, d.about = people, name+" and their club", name+" (and their club)"
	case them:
		d.msg.People, d.msg.Audience, d.about = people, name, name
	default:
		d.msg.Audience, d.about = name+"'s club", name+"'s club"
	}
	if len([]rune(d.msg.Text)) > maxDeskText {
		d.problem = fmt.Sprintf("A message can be up to %d characters.", maxDeskText)
	}
	return d, nil
}

// notifyOne sends a Notify message: tells an official or gymnast, and/or their
// club's comp sec and coaches, where they should be (person, flight, as, them,
// club, note; day: the page to go back to).
func (p *competitionPages) notifyOne(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	back := adminPath(r.PathValue("token")) + "/day?day=" + url.QueryEscape(r.FormValue("day"))
	d, err := p.notifyFormOf(r, c)
	if err != nil {
		failed(w, r, err)
		return
	}
	if d.notFound {
		message(w, r, http.StatusBadRequest, "Not on that flight", "That person isn't on that flight of the published timetable. It may have been changed: go back and try again.")
		return
	}
	if d.problem != "" {
		http.Redirect(w, r, back+"&notice="+url.QueryEscape(d.problem+" Nothing was sent."), http.StatusSeeOther)
		return
	}
	m, err := p.st.SendDeskMessage(r.Context(), c.ID, d.msg)
	if err == store.ErrLimit {
		http.Redirect(w, r, back+"&notice="+url.QueryEscape(fmt.Sprintf("A competition can keep %d messages. Nothing was sent.", store.MaxDeskMessages)), http.StatusSeeOther)
		return
	}
	if err != nil {
		failed(w, r, err)
		return
	}
	pushed, emailed := p.notify.tellDesk(r.Context(), c, m)
	if err := p.st.SetDeskTold(r.Context(), c.ID, m.ID, pushed, emailed); err != nil {
		log.Printf("Desk: %s: %v", c.ID, err)
	}
	notice := fmt.Sprintf("Sent to %s. Told %d by push and %d by email.", m.Audience, pushed, emailed)
	http.Redirect(w, r, back+"&notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

// describeNotify says what a Notify message does, for the history; "" if it
// won't be sent.
func (p *competitionPages) describeNotify(r *http.Request, c store.Competition) string {
	d, err := p.notifyFormOf(r, c)
	if err != nil || d.notFound || d.problem != "" {
		return ""
	}
	return "Notified " + d.about + ": should be at " + d.area
}
