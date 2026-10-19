package web

import (
	"net/http"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Personal and club timetables (roadmap 2026-10-08): the published
// timetable laid out as the panel timeline, days side by side and time
// running down evenly, with a person's flights and panels picked out, or
// everywhere a club has someone competing or officiating, named; the rest
// faint, or left out ("only").

func (p *competitionPages) registerTimelines(handle func(string, http.HandlerFunc)) {
	for _, path := range []string{
		"/clubs/member/{token}/competitions/{id}/timeline",
		"/competitions/entry/{token}/timeline",
		"/clubs/member/{token}/competitions/{id}/club-timeline",
		"/clubs/admin/{token}/competitions/{id}/timeline",
		"/clubs/coach/{token}/competitions/{id}/timeline",
	} {
		handle("GET "+path, p.timelinePage)
	}
}

// timelinePage is a person's timetable, or their club's.
func (p *competitionPages) timelinePage(w http.ResponseWriter, r *http.Request) {
	a, ok := p.whose(w, r)
	if !ok {
		return
	}
	if a.clubWide(r) {
		p.clubTimeline(w, r, a, a.club)
		return
	}
	p.personalTimeline(w, r, a, a.key)
}

// focusedTimeline is the published timetable as a timeline focused by mark,
// or a page saying it isn't published yet.
func (p *competitionPages) focusedTimeline(w http.ResponseWriter, r *http.Request, a asker,
	mark func(f rotaFacts) func(timelineItem) (bool, []string)) (views.Timeline, bool) {
	c := a.c
	t := c.Published
	if t == nil {
		message(w, r, http.StatusNotFound, "No timetable yet", "The organiser hasn't published the timetable yet. It shows here once they do.")
		return views.Timeline{}, false
	}
	f, err := p.liveRota(r.Context(), c)
	if err != nil {
		failed(w, r, err)
		return views.Timeline{}, false
	}
	name := f.name(false)
	only := r.URL.Query().Get("only") == "1"
	out := timeline(*t, name, false, &timelineFocus{mark: mark(f), only: only})
	out.Competition = summary(c, p.now())
	out.Back, out.BackLabel = timelineBack(a)
	out.Other, out.OtherLabel = r.URL.Path+"?only=1", "Only these"
	if only {
		out.Other, out.OtherLabel = r.URL.Path, "Everything"
	}
	return out, true
}

// timelineBack is where a timeline's way back goes, and its words: a
// person's My competition, where they open it; a comp sec's page for the
// competition; a coach's entries.
func timelineBack(a asker) (string, string) {
	if a.day != "" {
		return a.day, "← My competition"
	}
	return a.back, "← " + a.backTo()
}

// personalTimeline is a person's (by key) timetable: the flights they
// compete in and the panels they sit on, picked out.
func (p *competitionPages) personalTimeline(w http.ResponseWriter, r *http.Request, a asker, key string) {
	c := a.c
	t, ok := p.focusedTimeline(w, r, a, func(f rotaFacts) func(timelineItem) (bool, []string) {
		return func(it timelineItem) (bool, []string) {
			var notes []string
			in, held := inSlot(key, f.people, it.entries, it.officials)
			if len(in) > 0 {
				notes = append(notes, "You compete")
			}
			for _, d := range held {
				notes = append(notes, "You: "+competitions.RoleName(d.Role))
			}
			return len(notes) > 0, notes
		}
	})
	if !ok {
		return
	}
	t.Title, t.Heading = "Your timetable · "+c.Name, "Your timetable"
	t.Intro = "Where you compete and the panels you sit on, picked out in green; the rest of the timetable faint."
	render(w, r, views.TimelinePrint(t))
}

// clubTimeline is a club's timetable: every flight one of its gymnasts
// competes in, or one of its people officiates, picked out and named.
func (p *competitionPages) clubTimeline(w http.ResponseWriter, r *http.Request, a asker, clubID string) {
	c := a.c
	club, err := p.clubPeople(r.Context(), clubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	t, ok := p.focusedTimeline(w, r, a, func(f rotaFacts) func(timelineItem) (bool, []string) {
		ours, name := club.at(f), f.name(false)
		return func(it timelineItem) (bool, []string) {
			var competing, officiating []string
			in, held := ours.inSlot(it.entries, it.officials)
			for _, e := range in {
				competing = append(competing, e.Entry.Gymnasts())
			}
			for _, d := range held {
				officiating = append(officiating, name(d.Person)+" ("+seatLabels[d.Role]+")")
			}
			var notes []string
			if len(competing) > 0 {
				notes = append(notes, strings.Join(competing, ", "))
			}
			if len(officiating) > 0 {
				notes = append(notes, "Officials: "+strings.Join(officiating, ", "))
			}
			return len(notes) > 0, notes
		}
	})
	if !ok {
		return
	}
	t.Title, t.Heading = club.name+" · "+c.Name, club.name+"'s timetable"
	t.Intro = "Every flight with one of " + club.name + "'s gymnasts competing, or one of its people officiating, picked out in green and named; the rest of the timetable faint. " +
		"Officials' seats: Chair, D difficulty, HD horizontal displacement, S synchronisation, E execution, Rec recorder, Mar marshal. Hover over a short block to read it all."
	render(w, r, views.TimelinePrint(t))
}

// Score sheets for an official (asked 2026-10-08): the published
// timetable's sheets for the flights they have a seat on, to read on a
// phone; the organiser prints them.

// scoreSheetsPage is a person's score sheets, back to My competition.
func (p *competitionPages) scoreSheetsPage(w http.ResponseWriter, r *http.Request) {
	a, ok := p.whose(w, r)
	if !ok {
		return
	}
	p.officialScoreSheets(w, r, a.c, a.key, a.day)
}

// officialScoreSheets are the score sheets of the flights a person (by key)
// officiates, once the timetable is published.
func (p *competitionPages) officialScoreSheets(w http.ResponseWriter, r *http.Request, c store.Competition, key, back string) {
	t := c.Published
	if t == nil {
		message(w, r, http.StatusNotFound, "No timetable yet", "The organiser hasn't published the timetable yet. Score sheets show here once they do.")
		return
	}
	f, err := p.liveRota(r.Context(), c)
	if err != nil {
		failed(w, r, err)
		return
	}
	scratched, err := p.scratchedOf(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	sheets := scoreSheets(c, *t, f.entries, f.officials, scratched, back, p.now(), key)
	sheets.BackLabel = "← My competition"
	sheets.Intro = "The score sheets of the flights you're on the panel for. The organiser prints them for the day; you don't need to."
	render(w, r, views.ScorePrint(sheets))
}
