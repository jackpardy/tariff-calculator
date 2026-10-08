package main

import (
	"net/http"
	"slices"
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
	handle("GET /clubs/member/{token}/competitions/{id}/timeline", p.memberTimeline)
	handle("GET /competitions/entry/{token}/timeline", p.individualTimeline)
	handle("GET /clubs/member/{token}/competitions/{id}/club-timeline", p.memberClubTimeline)
	handle("GET /clubs/admin/{token}/competitions/{id}/timeline", p.adminClubTimeline)
	handle("GET /clubs/coach/{token}/competitions/{id}/timeline", p.coachClubTimeline)
}

func (p *competitionPages) memberTimeline(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	c, ok := p.memberCompetition(w, r, m)
	if !ok {
		return
	}
	p.personalTimeline(w, r, c, "m:"+m.ID, memberPath(r.PathValue("token")))
}

func (p *competitionPages) individualTimeline(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.own(w, r)
	if !ok {
		return
	}
	p.personalTimeline(w, r, c, individualKey(e.Entry.Gymnast), "/competitions/entry/"+r.PathValue("token"))
}

func (p *competitionPages) memberClubTimeline(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	c, ok := p.memberCompetition(w, r, m)
	if !ok {
		return
	}
	p.clubTimeline(w, r, c, m.ClubID, memberPath(r.PathValue("token")))
}

func (p *competitionPages) adminClubTimeline(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	c, ok := p.enteredCompetition(w, r, club.ID)
	if !ok {
		return
	}
	p.clubTimeline(w, r, c, club.ID, clubPath(r.PathValue("token")))
}

func (p *competitionPages) coachClubTimeline(w http.ResponseWriter, r *http.Request) {
	coach, _, ok := p.coach(w, r)
	if !ok {
		return
	}
	c, ok := p.enteredCompetition(w, r, coach.ClubID)
	if !ok {
		return
	}
	p.clubTimeline(w, r, c, coach.ClubID, coachPath(r.PathValue("token")))
}

// focusedTimeline is the published timetable as a timeline focused by mark,
// or a page saying it isn't published yet.
func (p *competitionPages) focusedTimeline(w http.ResponseWriter, r *http.Request, c store.Competition, back string,
	mark func(entries []store.Entry, officials []competitions.RotaPerson, name func(string) string) func(timelineItem) (bool, []string)) (views.Timeline, bool) {
	t := c.Published
	if t == nil {
		message(w, r, http.StatusNotFound, "No timetable yet", "The organiser hasn't published the timetable yet. It shows here once they do.")
		return views.Timeline{}, false
	}
	all, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return views.Timeline{}, false
	}
	entries := live(all)
	officials, err := p.rotaOf(r, c, entries)
	if err != nil {
		failed(w, r, err)
		return views.Timeline{}, false
	}
	_, names := peopleOf(entries)
	name := officialNames(officials, names, false)
	only := r.URL.Query().Get("only") == "1"
	out := timeline(*t, name, false, &timelineFocus{mark: mark(entries, officials, name), only: only})
	out.Competition, out.Back, out.BackLabel = summary(c.Competition, p.now()), back, "← Back"
	out.Other, out.OtherLabel = r.URL.Path+"?only=1", "Only these"
	if only {
		out.Other, out.OtherLabel = r.URL.Path, "Everything"
	}
	return out, true
}

// personalTimeline is a person's (by key) timetable: the flights they
// compete in and the panels they sit on, picked out.
func (p *competitionPages) personalTimeline(w http.ResponseWriter, r *http.Request, c store.Competition, key, back string) {
	t, ok := p.focusedTimeline(w, r, c, back, func(entries []store.Entry, _ []competitions.RotaPerson, _ func(string) string) func(timelineItem) (bool, []string) {
		keys := personKeys(entries)
		return func(it timelineItem) (bool, []string) {
			var notes []string
			for _, id := range it.entries {
				if slices.Contains(keys[id], key) {
					notes = append(notes, "You compete")
					break
				}
			}
			for _, d := range it.officials {
				if d.Person == key {
					notes = append(notes, "You: "+competitions.RoleName(d.Role))
				}
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
func (p *competitionPages) clubTimeline(w http.ResponseWriter, r *http.Request, c store.Competition, clubID, back string) {
	clubName, err := p.st.ClubName(r.Context(), clubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	members, err := p.st.Members(r.Context(), clubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	ours := map[string]bool{} // the club's people, by key
	for _, m := range members {
		ours["m:"+m.ID] = true
	}
	t, ok := p.focusedTimeline(w, r, c, back, func(entries []store.Entry, officials []competitions.RotaPerson, name func(string) string) func(timelineItem) (bool, []string) {
		for _, o := range officials {
			if o.Club == clubName {
				ours[o.Key] = true // the organiser's judges from the club too
			}
		}
		byID := map[string]store.Entry{}
		for _, e := range entries {
			byID[e.ID] = e
		}
		return func(it timelineItem) (bool, []string) {
			var competing, officiating []string
			for _, id := range it.entries {
				if e, ok := byID[id]; ok && e.ClubID == clubID {
					competing = append(competing, e.Entry.Gymnasts())
				}
			}
			for _, d := range it.officials {
				if ours[d.Person] {
					officiating = append(officiating, name(d.Person)+" ("+seatLabels[d.Role]+")")
				}
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
	t.Title, t.Heading = clubName+" · "+c.Name, clubName+"'s timetable"
	t.Intro = "Every flight with one of " + clubName + "'s gymnasts competing, or one of its people officiating, picked out in green and named; the rest of the timetable faint."
	render(w, r, views.TimelinePrint(t))
}

// published says whether a competition's timetable is published.
func published(c store.Competition) bool { return c.Published != nil }
