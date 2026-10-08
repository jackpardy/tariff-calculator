package main

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The officials rota on the timetable (ADR 0005 step 5): the people who've
// offered to officiate, put on each flight's panel when the timetable is
// planned, changed seat by seat, and printed.

// officialKey is an official's person key: the same as where they compete,
// so the rota keeps them off panels while they're competing.
func officialKey(o store.Official, keys map[string][]string) string {
	switch {
	case o.MemberID != "":
		return "m:" + o.MemberID
	case o.EntryID != "":
		if k := keys[o.EntryID]; len(k) > 0 {
			return k[0]
		}
		return individualKey(o.Name)
	}
	return "o:" + o.ID
}

// rotaPeople are the people who can officiate, as the rota needs them: each
// once (an individual offering on two entries is one person), with what they
// may do at each event under the competition's judging rule.
func rotaPeople(c competitions.Competition, officials []store.Official, entries []store.Entry) []competitions.RotaPerson {
	keys := personKeys(entries)
	competing := map[string]map[string]string{} // person → discipline → level
	for _, e := range entries {
		for _, k := range keys[e.ID] {
			if competing[k] == nil {
				competing[k] = map[string]string{}
			}
			competing[k][e.Entry.Discipline] = e.Entry.Level
		}
	}
	var out []competitions.RotaPerson
	at := map[string]int{}
	for _, o := range officials {
		key := officialKey(o, keys)
		i, seen := at[key]
		if !seen {
			at[key] = len(out)
			i = len(out)
			out = append(out, competitions.RotaPerson{Key: key, Name: o.Name, Club: o.ClubName, Judge: map[string]bool{}, Chair: map[string]bool{}})
		}
		p := &out[i]
		p.Recorder = p.Recorder || o.Offer.Recorder
		p.Marshal = p.Marshal || o.Offer.Marshal
		for _, d := range c.Disciplines() {
			for _, l := range c.LevelNames(d) {
				ev := competitions.EventName(d, l)
				if c.CanJudge(o.Offer, o.Qualified, competing[key], d, l) {
					p.Judge[ev] = true
					p.Chair[ev] = p.Chair[ev] || o.Offer.Judge[d].Chair
				}
			}
		}
	}
	slices.SortStableFunc(out, func(a, b competitions.RotaPerson) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// rotaOf is the competition's officials as the rota needs them.
func (p *competitionPages) rotaOf(r *http.Request, c store.Competition, entries []store.Entry) ([]competitions.RotaPerson, error) {
	officials, err := p.st.Officials(r.Context(), c.ID)
	if err != nil {
		return nil, err
	}
	return rotaPeople(c.Competition, officials, entries), nil
}

// staffing is who may judge each event, and each discipline's panel's
// judges, for placing flights where their judges are free.
func staffing(c competitions.Competition, people []competitions.RotaPerson) *competitions.Staffing {
	s := &competitions.Staffing{Judges: map[string][]string{}, Need: map[string]int{}}
	for _, d := range c.Disciplines() {
		s.Need[d] = c.Officials.Panel(d).Judges()
	}
	for _, o := range people {
		for ev, ok := range o.Judge {
			if ok {
				s.Judges[ev] = append(s.Judges[ev], o.Key)
			}
		}
	}
	return s
}

// staff fills the timetable's panels.
func (p *competitionPages) staff(s *competitions.Schedule, c store.Competition, entries []store.Entry, people []competitions.RotaPerson) {
	s.Rota(schedEntries(entries), people, c.Officials, uint64(p.now().UnixNano()))
}

// editRota assigns every panel again (action=rota), or gives one seat to a
// person (action=seat: flight, seat, person; "" empties it).
func (p *competitionPages) editRota(w http.ResponseWriter, r *http.Request) {
	c, s, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if !s.Planned {
		back(w, r, "Plan the timetable first.")
		return
	}
	people, err := p.rotaOf(r, c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	notice := ""
	switch r.FormValue("action") {
	case "rota":
		p.staff(&s, c, entries, people)
		notice = "Officials assigned again."
	case "seat":
		seat, err2 := strconv.Atoi(r.FormValue("seat"))
		person := r.FormValue("person")
		known := person == "" || slices.ContainsFunc(people, func(o competitions.RotaPerson) bool { return o.Key == person })
		set := false
		if block, err := strconv.Atoi(r.FormValue("block")); err == nil {
			set = s.SetBlockDuty(block, seat, person)
		} else if flight, err := strconv.Atoi(r.FormValue("flight")); err == nil {
			set = s.SetDuty(flight, seat, person)
		}
		if err2 != nil || !known || !set {
			back(w, r, "That seat has changed since; nothing was changed.")
			return
		}
	default:
		back(w, r, "")
		return
	}
	if err := p.st.SetTimetable(r.Context(), c.ID, &s); err != nil {
		failed(w, r, err)
		return
	}
	back(w, r, notice)
}

// personRule is a rule about an official from the rules form (kind role, off
// or hours), with their name.
func personRule(r *http.Request, people []competitions.RotaPerson) (competitions.Rule, bool) {
	rule := competitions.Rule{Kind: r.FormValue("kind"), Must: r.FormValue("must") == "1", Person: r.FormValue("person"), Event: r.FormValue("event")}
	i := slices.IndexFunc(people, func(o competitions.RotaPerson) bool { return o.Key == rule.Person })
	if i < 0 {
		return rule, false
	}
	rule.Name = people[i].Name
	switch rule.Kind {
	case competitions.RuleRole:
		rule.Role = r.FormValue("role")
	case competitions.RuleHours:
		rule.Event = ""
		rule.Day, _ = strconv.Atoi(r.FormValue("day"))
		rule.From, rule.To = r.FormValue("from"), r.FormValue("to")
	}
	return rule, true
}

// rotaView is the panels and the rota's report, as the page shows them.
func rotaView(s competitions.Schedule, entries []store.Entry, people []competitions.RotaPerson) views.RotaView {
	keys, names := peopleOf(entries)
	clubs := map[string]string{}
	coaches := map[string][]string{}
	for _, e := range schedEntries(entries) {
		clubs[e.ID] = e.Club
		coaches[e.ID] = e.Coaches
	}
	for _, o := range people {
		if names[o.Key] == "" {
			names[o.Key] = o.Name
		}
	}
	rr := s.RotaReport(clubs, people)
	v := views.RotaView{People: len(people), Short: rr.Short, Empty: rr.Empty, Broken: rr.Broken,
		Problems: s.RotaProblems(keys, people, names), Coaches: s.CoachClashes(coaches, names)}
	for _, c := range rr.OwnClub {
		v.OwnClub = append(v.OwnClub, fmt.Sprintf("%s %d", c.Club, c.Times))
	}
	for _, w := range rr.Busiest {
		v.Busiest = append(v.Busiest, fmt.Sprintf("%s: %d, %s", names[w.Person], w.Duties, hoursWord(w.Minutes)))
	}
	for _, o := range people {
		label := o.Name
		if o.Club != "" {
			label += " (" + o.Club + ")"
		}
		v.Options = append(v.Options, views.FlightOption{Value: o.Key, Label: label})
	}
	return v
}

func hoursWord(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	return fmt.Sprintf("%dh %02dm", minutes/60, minutes%60)
}

// seatsOf are a flight's seats as the pages show them.
func seatsOf(f competitions.ScheduledFlight, people []competitions.RotaPerson, names map[string]string) []views.SeatView {
	byKey := map[string]competitions.RotaPerson{}
	for _, o := range people {
		byKey[o.Key] = o
	}
	of := map[string]int{}
	for _, d := range f.Officials {
		of[d.Role]++
	}
	n := map[string]int{}
	var out []views.SeatView
	for _, d := range f.Officials {
		n[d.Role]++
		seat := views.SeatView{Role: competitions.RoleName(d.Role), Person: d.Person, Label: competitions.RoleName(d.Role)}
		if of[d.Role] > 1 {
			seat.Label += " " + strconv.Itoa(n[d.Role])
		}
		if o, ok := byKey[d.Person]; ok {
			seat.Name, seat.Club = o.Name, o.Club
		} else if d.Person != "" {
			seat.Name = names[d.Person]
		}
		out = append(out, seat)
	}
	return out
}

// dutiesOf are a person's seats on a published timetable, in time order,
// e.g. "Saturday 10:40–11:50 · Panel 2 · BUCS L5 · Execution judge".
func dutiesOf(c store.Competition, key string) []string {
	t := c.Published
	if t == nil || key == "" {
		return nil
	}
	return dutiesIn(*t, key)
}

// dutiesIn are a person's seats, in time order, one line for the flights of
// an event they hold a seat for, e.g. "Friday 09:30–15:15 · Panel 2 · BUCS
// L7 Men · all 5 flights · Execution judge".
func dutiesIn(s competitions.Schedule, key string) []string {
	type duty struct {
		f    competitions.ScheduledFlight
		role string
		n    int // flights in a row, of the same event, in the same seat
		last int // the last's number
	}
	jobs := s.Staffed()
	slices.SortStableFunc(jobs, func(a, b competitions.ScheduledFlight) int { return (a.Day*1440 + a.Start) - (b.Day*1440 + b.Start) })
	var held []duty
	for _, f := range jobs {
		for _, d := range f.Officials {
			if d.Person != key {
				continue
			}
			if k := len(held) - 1; k >= 0 && held[k].f.Of > 1 && held[k].f.Event() == f.Event() && held[k].f.Area == f.Area && held[k].f.Day == f.Day &&
				held[k].role == d.Role && held[k].last+1 == f.Number {
				held[k].f.End, held[k].n, held[k].last = f.End, held[k].n+1, f.Number
				continue
			}
			held = append(held, duty{f, d.Role, 1, f.Number})
		}
	}
	var out []string
	for _, h := range held {
		out = append(out, dutyLine(s, h.f, h.role, h.n, h.last))
	}
	return out
}

// dutyLine says when, where and what a duty is, over n of an event's
// flights up to its flight last.
func dutyLine(s competitions.Schedule, f competitions.ScheduledFlight, role string, n, last int) string {
	day := ""
	if f.Day < len(s.Setup.Days) {
		day = s.Setup.Days[f.Day].Name + " "
	}
	name := f.Name()
	switch {
	case n == 2 && n == f.Of:
		name = f.Event() + " · both flights"
	case n > 1 && n == f.Of:
		name = fmt.Sprintf("%s · all %d flights", f.Event(), n)
	case n > 1:
		name = fmt.Sprintf("%s · flights %d–%d of %d", f.Event(), last-n+1, last, f.Of)
	}
	return fmt.Sprintf("%s%s–%s · %s · %s · %s", day, competitions.Clock(f.Start), competitions.Clock(f.End), f.Area, name, competitions.RoleName(role))
}

// rotaSheet is each person's duties, for the noticeboard.
func rotaSheet(s competitions.Schedule, people []competitions.RotaPerson) []views.PersonDuties {
	var out []views.PersonDuties
	for _, o := range people {
		if duties := dutiesIn(s, o.Key); len(duties) > 0 {
			out = append(out, views.PersonDuties{Name: o.Name, Club: o.Club, Duties: duties})
		}
	}
	return out
}
