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
	handle("GET /clubs/member/{token}/competitions/{id}/day", p.memberDay)
	handle("GET /competitions/entry/{token}/day", p.individualDay)
	handle("POST /clubs/member/{token}/competitions/{id}/here", p.memberHere)
	handle("POST /competitions/entry/{token}/here", p.individualHere)
	handle("GET /clubs/member/{token}/competitions/{id}/score-sheets", p.memberScoreSheets)
	handle("GET /competitions/entry/{token}/score-sheets", p.individualScoreSheets)
}

func (p *competitionPages) memberDay(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	c, ok := p.memberCompetition(w, r, m)
	if !ok {
		return
	}
	club, err := p.memberClub(r, m)
	if err != nil {
		failed(w, r, err)
		return
	}
	token := r.PathValue("token")
	own := memberPath(token) + "/competitions/" + c.ID
	frame := views.MyCompetitionPage{Over: club, Menu: memberMenu(token, memberCompetitions),
		Back: memberCompetitionPath(token, c.ID), BackLabel: "← " + c.Name + ": your entries"}
	if published(c) {
		frame.ClubTimeline, frame.ClubCalendar = own+"/club-timeline", own+"/club-calendar.ics"
	}
	p.renderDay(w, r, c, "m:"+m.ID, m.ClubID, m.Name, frame)
}

func (p *competitionPages) individualDay(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.own(w, r)
	if !ok {
		return
	}
	frame := views.MyCompetitionPage{Over: "Entered on your own", Back: "/competitions/entry/" + r.PathValue("token"), BackLabel: "← " + c.Name + ": your entry"}
	if e.IndividualID != "" {
		frame.Menu = gymnastMenu(p.gymnastBase(r, e), gymnastCompetitions)
	}
	p.renderDay(w, r, c, personKeyOf(e), "", e.Entry.Gymnast, frame)
}

// memberHere is a scratched member saying they're still here for the rest.
func (p *competitionPages) memberHere(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	c, ok := p.memberCompetition(w, r, m)
	if !ok {
		return
	}
	p.stillHere(w, r, c, "m:"+m.ID, m.Name, dayPath(memberPath(r.PathValue("token")), c.ID))
}

// individualHere is a scratched individual saying they're still here for the
// rest.
func (p *competitionPages) individualHere(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.own(w, r)
	if !ok {
		return
	}
	p.stillHere(w, r, c, personKeyOf(e), e.Entry.Gymnast, "/competitions/entry/"+r.PathValue("token")+"/day")
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
// for what a page adds to it: the live entries, who is in each, and the
// check-ins (the panel check-ins and cleared warnings only once published).
type dayFacts struct {
	entries  []store.Entry
	keys     map[string][]string
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
	all, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		return views.MyCompetitionPage{}, f, err
	}
	f.entries = live(all)
	f.keys = personKeys(f.entries)
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
		if !slices.Contains(f.keys[j.ID], key) {
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
		officials, err := p.rotaOf(r, c, f.entries)
		if err != nil {
			return page, f, err
		}
		page.Duties = dutiesOf(c, key)
		if f.official, err = p.st.OfficialCheckins(r.Context(), c.ID); err != nil {
			return page, f, err
		}
		if f.clears, err = p.st.ScratchClears(r.Context(), c.ID); err != nil {
			return page, f, err
		}
		page.Days = itemsOf(*t, f.entries, officials)
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
	page.ClubTimeline, page.ClubCalendar = frame.ClubTimeline, frame.ClubCalendar
	page.Notice = r.URL.Query().Get("notice")
	own := strings.TrimSuffix(r.URL.Path, "/day")
	if page.Published {
		page.Timeline = own + "/timeline"
		page.Calendar = own + "/calendar.ics"
		page.Here = hereBox(f.entries, f.keys, key, len(page.Duties) > 0, f.checkins, f.official, f.clears, own+"/here")
		if len(page.Duties) > 0 {
			page.ScoreSheets = own + "/score-sheets"
		}
	}
	page.Notify = p.notifyLink(own + "/notify")
	page.Desk = p.deskNotes(r.Context(), c.ID, []string{key}, clubIDs(club), nil)
	render(w, r, views.MyCompetition(page))
}

// dayPath is a member's My competition page, for a competition.
func dayPath(memberPath, competitionID string) string {
	return fmt.Sprintf("%s/competitions/%s/day", memberPath, competitionID)
}
