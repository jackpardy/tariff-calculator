package web

import (
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
	p.renderDay(w, r, c, "m:"+m.ID, m.ClubID, m.Name, memberPath(r.PathValue("token")))
}

func (p *competitionPages) individualDay(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.own(w, r)
	if !ok {
		return
	}
	p.renderDay(w, r, c, individualKey(e.Entry.Gymnast), "", e.Entry.Gymnast, "/competitions/entry/"+r.PathValue("token"))
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
	p.stillHere(w, r, c, individualKey(e.Entry.Gymnast), e.Entry.Gymnast, "/competitions/entry/"+r.PathValue("token")+"/day")
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

// renderDay shows a person's competition: key is who they are, as the
// timetable knows them, and club is their club's id ("" for an individual).
func (p *competitionPages) renderDay(w http.ResponseWriter, r *http.Request, c store.Competition, key, club, name, back string) {
	all, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	entries := live(all)
	keys := personKeys(entries)
	page := views.MyCompetitionPage{Competition: summary(c.Competition, p.now()), Name: name, Back: back, Mine: map[string]bool{}, Me: key, Notice: r.URL.Query().Get("notice")}
	if published(c) {
		page.Timeline = strings.TrimSuffix(r.URL.Path, "/day") + "/timeline"
		page.Calendar = strings.TrimSuffix(r.URL.Path, "/day") + "/calendar.ics"
	}
	page.Notify = p.notifyLink(strings.TrimSuffix(r.URL.Path, "/day") + "/notify")
	page.Desk = p.deskNotes(r.Context(), c.ID, []string{key}, clubIDs(club), nil)
	t := c.Published
	published := t != nil
	page.Published = published
	var mine []competitions.ScheduledFlight // the flights they're in
	scratched, err := p.scratchedOf(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	for _, j := range judge(c.Competition, entries) {
		if !slices.Contains(keys[j.ID], key) {
			continue
		}
		page.Mine[j.ID] = true
		ev := views.MyEvent{Event: j.Entry.Entry.Event(), Gymnasts: j.Entry.Entry.Gymnasts(), Category: j.Entry.Entry.Category}
		switch {
		case j.Checked():
			ev.Status = "Checked by the organiser ✓"
		case len(j.problems) > 0:
			ev.Status = problemsWord(len(j.problems)) + " to sort out"
			ev.Problems = j.problems
		default:
			ev.Status = "Sent, not checked yet"
		}
		if scratched[j.ID] {
			ev.Status = "Scratched · " + ev.Status
		}
		if published {
			if i, ok := t.Find(j.ID); ok {
				f := t.Flights[i]
				mine = append(mine, f)
				pos := slices.Index(f.Entries, j.ID)
				timing := t.Setup.TimingsFor(f.Discipline)
				ev.Flight, ev.Area = f.Name(), f.Area
				if f.Day < len(t.Setup.Days) {
					ev.Day = t.Setup.Days[f.Day].Name
				}
				ev.WarmUp = competitions.Clock(f.Start)
				ev.Position, ev.Of = pos+1, len(f.Entries)
				// Their turns, if every turn takes its share of the time (half
				// of each competitor's minutes per round): a guide only.
				half := timing.PerCompetitor / 2
				first := f.Start + timing.Between
				ev.Turn = competitions.Clock(first + int(math.Floor(float64(pos)*half)))
				ev.Turn2 = competitions.Clock(first + int(math.Floor(float64(len(f.Entries)+pos)*half)))
			}
		}
		page.Events = append(page.Events, ev)
	}
	if published {
		if page.Lateness, err = p.lateNotes(r.Context(), c, mine); err != nil {
			failed(w, r, err)
			return
		}
		officials, err := p.rotaOf(r, c, entries)
		if err != nil {
			failed(w, r, err)
			return
		}
		page.Duties = dutiesOf(c, key)
		checkins, err := p.st.Checkins(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		official, err := p.st.OfficialCheckins(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		clears, err := p.st.ScratchClears(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		page.Here = hereBox(entries, keys, key, len(page.Duties) > 0, checkins, official, clears, strings.TrimSuffix(r.URL.Path, "/day")+"/here")
		if len(page.Duties) > 0 {
			page.ScoreSheets = strings.TrimSuffix(r.URL.Path, "/day") + "/score-sheets"
		}
		page.Days = itemsOf(*t, entries, officials)
	}
	render(w, r, views.MyCompetition(page))
}

// dayPath is a member's My competition page, for a competition.
func dayPath(memberPath, competitionID string) string {
	return fmt.Sprintf("%s/competitions/%s/day", memberPath, competitionID)
}
