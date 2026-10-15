package web

import (
	"cmp"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// My competition (roadmap: competitions 5): one page for someone at the
// competition, from their member page or their own entry. It has every event
// they're in, its card's status, their flight, where they are in its running
// order and about when they go, what they officiate, and the whole published
// timetable with them picked out.

func (p *competitionPages) registerMine(handle func(string, http.HandlerFunc)) {
	for _, own := range []string{"/clubs/member/{token}/competitions/{id}", "/competitions/entry/{token}"} {
		handle("GET "+own+"/day", p.myDay)
		handle("POST "+own+"/here", p.here)
		handle("GET "+own+"/score-sheets", p.scoreSheetsPage)
	}
}

// myDay is a member's or individual's My competition.
func (p *competitionPages) myDay(w http.ResponseWriter, r *http.Request) {
	a, ok := p.whose(w, r)
	if !ok {
		return
	}
	c, frame := a.c, views.MyCompetitionPage{Back: a.back}
	if a.kind == store.NotifyMember {
		club, err := p.st.ClubName(r.Context(), a.club)
		if err != nil {
			failed(w, r, err)
			return
		}
		frame.Over, frame.Menu, frame.BackLabel = club, memberMenu(r.PathValue("token"), memberCompetitions), "← "+c.Name+": your entries"
	} else {
		frame.Over, frame.BackLabel = "Entered on your own", "← "+c.Name+": your entry"
		if a.entry.IndividualID != "" {
			frame.Menu = gymnastMenu(p.gymnastBase(r, a.entry), gymnastCompetitions)
		}
	}
	p.renderDay(w, r, c, a.key, a.club, a.name, frame)
}

// here is someone scratched saying they're still here for the rest.
func (p *competitionPages) here(w http.ResponseWriter, r *http.Request) {
	a, ok := p.whose(w, r)
	if !ok {
		return
	}
	p.stillHere(w, r, a.c, a.key, a.name, a.day)
}

// hereBox is the box for someone scratched from some events who may still be
// here for the rest: their other entries (any day) or their officiating, so
// long as the organisers haven't been told. nil if there's nothing to say.
func hereBox(entries []store.Entry, keys map[string][]string, key string, officiates bool, checkins map[string]store.Checkin, official map[string]map[string]store.Checkin, clears map[string]map[string]store.ScratchClear, action string) *views.MyHere {
	var scratched []string
	others := 0
	for _, e := range entries {
		if !slices.Contains(keys[e.ID], key) {
			continue
		}
		if checkins[e.ID].Status == store.Scratched {
			scratched = append(scratched, e.Entry.Event())
		} else {
			others++
		}
	}
	if len(scratched) == 0 {
		return nil
	}
	here := hereSomewhere(entries, checkins, official)
	officiating := officiates && warned(key, store.ClearOfficiating, here, clears)
	competing := others > 0 && warned(key, store.ClearCompeting, here, clears)
	box := &views.MyHere{Scratched: joinAnd(scratched), Action: action}
	switch {
	case officiating && competing:
		box.For = "both your other events and your officiating"
	case officiating:
		box.For = "your officiating"
	case competing:
		box.For = "your other events"
	default:
		return nil
	}
	return box
}

// dayFacts are what a person's competition page reads from the store, kept
// for what a page adds to it: the live entries, who is in each, the
// officials, and the check-ins (the panel check-ins and cleared warnings
// only once published).
type dayFacts struct {
	rotaFacts
	checkins map[string]store.Checkin
	official map[string]map[string]store.Checkin
	clears   map[string]map[string]store.ScratchClear
}

// dayOf builds a person's competition: key is who they are, as the timetable
// knows them. It has what the member's page and the organiser's page of that
// person share: their events and cards, their flights, what they officiate,
// and the timetable with them picked out. The pages add the rest.
func (p *competitionPages) dayOf(r *http.Request, c store.Competition, key, name string) (views.MyCompetitionPage, dayFacts, error) {
	var f dayFacts
	var err error
	if f.rotaFacts, err = p.liveRota(r.Context(), c); err != nil {
		return views.MyCompetitionPage{}, f, err
	}
	page := views.MyCompetitionPage{Competition: summary(c.Competition, p.now()), Name: name, Mine: map[string]bool{}, Me: key}
	t := c.Published
	published := t != nil
	page.Published = published
	var mine []competitions.ScheduledFlight // the flights they're in
	warmUp := map[string]int{}              // when each event's warm-up starts, by entry, once published
	if f.checkins, err = p.st.Checkins(r.Context(), c.ID); err != nil {
		return page, f, err
	}
	for _, j := range judge(c.Competition, f.entries) {
		if !slices.Contains(f.people[j.ID], key) {
			continue
		}
		page.Mine[j.ID] = true
		ev := views.MyEvent{ID: j.ID, Event: j.Entry.Entry.Event(), Gymnasts: j.Entry.Entry.Gymnasts(), Category: j.Entry.Entry.Category}
		switch {
		case j.Checked():
			ev.Status = "Checked by the organiser ✓"
		case len(j.problems) > 0:
			ev.Status = problemsWord(len(j.problems)) + " to sort out"
			ev.Problems = j.problems
		default:
			ev.Status = "Sent, not checked yet"
		}
		if f.checkins[j.ID].Status == store.Scratched {
			ev.Status = "Scratched · " + ev.Status
		}
		if published {
			if i, ok := t.Find(j.ID); ok {
				fl := t.Flights[i]
				mine = append(mine, fl)
				warmUp[j.ID] = fl.Day*1440 + fl.Start
				pos := slices.Index(fl.Entries, j.ID)
				timing := t.Setup.TimingsFor(fl.Discipline)
				ev.Flight, ev.Area = fl.Name(), fl.Area
				if fl.Day < len(t.Setup.Days) {
					ev.Day = t.Setup.Days[fl.Day].Name
				}
				ev.WarmUp = competitions.Clock(fl.Start)
				ev.Position, ev.Of = pos+1, len(fl.Entries)
				// Their turns, if every turn takes its share of the time (half
				// of each competitor's minutes per round): a guide only.
				half := timing.PerCompetitor / 2
				first := fl.Start + timing.Between
				ev.Turn = competitions.Clock(first + int(math.Floor(float64(pos)*half)))
				ev.Turn2 = competitions.Clock(first + int(math.Floor(float64(len(fl.Entries)+pos)*half)))
			}
		}
		page.Events = append(page.Events, ev)
	}
	byWarmUp(page.Events, warmUp)
	if published {
		if page.Lateness, err = p.lateNotes(r.Context(), c, mine); err != nil {
			return page, f, err
		}
		page.Duties = dutiesOf(c, key)
		if f.official, err = p.st.OfficialCheckins(r.Context(), c.ID); err != nil {
			return page, f, err
		}
		if f.clears, err = p.st.ScratchClears(r.Context(), c.ID); err != nil {
			return page, f, err
		}
		page.Days = itemsOf(*t, f.entries, f.officials)
	}
	return page, f, nil
}

// byWarmUp orders someone's events by when they warm up (warmUp, by entry,
// in minutes from the first day's start), any not in a flight last, as
// they were.
func byWarmUp(events []views.MyEvent, warmUp map[string]int) {
	at := func(ev views.MyEvent) int {
		if w, ok := warmUp[ev.ID]; ok {
			return w
		}
		return math.MaxInt
	}
	slices.SortStableFunc(events, func(a, b views.MyEvent) int { return cmp.Compare(at(a), at(b)) })
}

// renderDay shows a person's competition: key is who they are, as the
// timetable knows them, and club is their club's id ("" for an individual).
// frame has what the page of theirs it came from gives it: who they are
// with (Over), their tabs, the way back and their club's timetable.
func (p *competitionPages) renderDay(w http.ResponseWriter, r *http.Request, c store.Competition, key, club, name string, frame views.MyCompetitionPage) {
	page, f, err := p.dayOf(r, c, key, name)
	if err != nil {
		failed(w, r, err)
		return
	}
	page.Over, page.Menu, page.Back, page.BackLabel = frame.Over, frame.Menu, frame.Back, frame.BackLabel
	page.Notice = r.URL.Query().Get("notice")
	own := strings.TrimSuffix(r.URL.Path, "/day")
	page.MyLinks = p.competitionLinks(r.Context(), c, linksFor{base: own, key: key, member: club != "", people: []string{key}, clubs: clubIDs(club)})
	if page.Published {
		page.Here = hereBox(f.entries, f.people, key, len(page.Duties) > 0, f.checkins, f.official, f.clears, own+"/here")
	}
	render(w, r, views.MyCompetition(page))
}

// dayPath is a member's My competition page, for a competition.
func dayPath(memberPath, competitionID string) string {
	return fmt.Sprintf("%s/competitions/%s/day", memberPath, competitionID)
}
