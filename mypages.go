package main

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
	p.renderDay(w, r, c, "m:"+m.ID, m.Name, memberPath(r.PathValue("token")))
}

func (p *competitionPages) individualDay(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.own(w, r)
	if !ok {
		return
	}
	p.renderDay(w, r, c, individualKey(e.Entry.Gymnast), e.Entry.Gymnast, "/competitions/entry/"+r.PathValue("token"))
}

// renderDay shows a person's competition: key is who they are, as the
// timetable knows them.
func (p *competitionPages) renderDay(w http.ResponseWriter, r *http.Request, c store.Competition, key, name, back string) {
	all, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	entries := live(all)
	keys := personKeys(entries)
	page := views.MyCompetitionPage{Competition: summary(c.Competition, p.now()), Name: name, Back: back, Mine: map[string]bool{}, Me: key}
	if published(c) {
		page.Timeline = strings.TrimSuffix(r.URL.Path, "/day") + "/timeline"
		page.Calendar = strings.TrimSuffix(r.URL.Path, "/day") + "/calendar.ics"
	}
	page.Notify = p.notifyLink(strings.TrimSuffix(r.URL.Path, "/day") + "/notify")
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
