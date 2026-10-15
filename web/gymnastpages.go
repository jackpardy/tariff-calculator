package web

import (
	"errors"
	"net/http"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// A gymnast entering on their own has one personal page (ADR 0010), like a
// club member's without the club: Competitions, a page for each competition
// with a row per discipline, and Your details. Each entry's own page is at
// /competitions/entry/{token}, the token being an entry link from before or
// the personal link's Via to the entry, so every entry page works as it did
// and every entry link already sent still opens it.

func gymnastPath(token string) string        { return "/competitions/gymnast/" + token }
func gymnastDetailsPath(token string) string { return gymnastPath(token) + "/details" }
func gymnastCompetitionPath(token, competitionID string) string {
	return gymnastPath(token) + "/competitions/" + competitionID
}

// entryPath is one of the person's entries, opened from a page of theirs.
func entryPath(token string, e store.Entry) string {
	return "/competitions/entry/" + store.Via(store.Personal(token), e.ID)
}

// The tabs of an individual's pages.
const (
	gymnastCompetitions = "competitions"
	gymnastDetails      = "details"
)

// gymnastMenu is the menu of an individual's pages, with the tab shown
// current. A competition's page, and each entry's, is in Competitions.
func gymnastMenu(token, current string) []views.MenuSection {
	return []views.MenuSection{
		{Label: "Competitions", Href: gymnastPath(token), Current: current == gymnastCompetitions},
		{Label: "Your details", Href: gymnastDetailsPath(token), Current: current == gymnastDetails},
	}
}

func (p *competitionPages) registerGymnasts(handle func(string, http.HandlerFunc)) {
	handle("GET /competitions/gymnast/{token}", p.gymnastHome)
	handle("GET /competitions/gymnast/{token}/details", p.gymnastDetails)
	handle("GET /competitions/gymnast/{token}/competitions/{id}", p.gymnastCompetition)
	handle("GET /competitions/gymnast/{token}/competitions/{id}/enter", p.gymnastEnter)
	handle("POST /competitions/gymnast/{token}/competitions/{id}/enter", p.gymnastEnter)
}

// gymnast is the individual a link opens, with the person's entries on their
// own, in every competition.
func (p *competitionPages) gymnast(w http.ResponseWriter, r *http.Request) (store.Individual, []store.Entry, bool) {
	ind, err := p.st.IndividualByLink(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return ind, nil, false
	}
	entries, err := p.st.IndividualEntries(r.Context(), ind.PersonID)
	if err != nil {
		failed(w, r, err)
		return ind, nil, false
	}
	return ind, entries, true
}

// gymnastPage is the part of an individual's page that every tab has.
func (p *competitionPages) gymnastPage(r *http.Request, ind store.Individual, tab string) views.GymnastPage {
	token := r.PathValue("token")
	page := views.GymnastPage{Name: ind.Name, Menu: gymnastMenu(token, tab), Home: gymnastPath(token), Notice: r.URL.Query().Get("notice")}
	if !store.IsVia(token) {
		page.Link = origin(r) + gymnastPath(token)
	}
	return page
}

// gymnastHome lists the competitions an individual entered.
func (p *competitionPages) gymnastHome(w http.ResponseWriter, r *http.Request) {
	ind, entries, ok := p.gymnast(w, r)
	if !ok {
		return
	}
	page := p.gymnastPage(r, ind, gymnastCompetitions)
	token := r.PathValue("token")
	count := map[string]int{}
	var order []string
	for _, e := range entries {
		if count[e.CompetitionID] == 0 {
			order = append(order, e.CompetitionID)
		}
		count[e.CompetitionID]++
	}
	for _, id := range order {
		c, err := p.st.Competition(r.Context(), id)
		if err != nil {
			failed(w, r, err)
			return
		}
		page.Competitions = append(page.Competitions, views.GymnastCompetition{
			Page: gymnastCompetitionPath(token, id), Competition: summary(c.Competition, p.now()), Status: views.Count(count[id], "entry", "entries"),
		})
	}
	render(w, r, views.GymnastHome(page))
}

// gymnastDetails is an individual's details: their link, and their other
// links to add and take off.
func (p *competitionPages) gymnastDetails(w http.ResponseWriter, r *http.Request) {
	ind, _, ok := p.gymnast(w, r)
	if !ok {
		return
	}
	page := p.gymnastPage(r, ind, gymnastDetails)
	var err error
	if page.Other, err = p.otherLinks(r, gymnastPath(r.PathValue("token"))); err != nil {
		failed(w, r, err)
		return
	}
	render(w, r, views.GymnastDetails(page))
}

// gymnastEntered is the competition in the URL, with the individual's
// entries in it: none, and it isn't theirs to see.
func (p *competitionPages) gymnastEntered(w http.ResponseWriter, r *http.Request, entries []store.Entry) (store.Competition, []store.Entry, bool) {
	var mine []store.Entry
	for _, e := range entries {
		if e.CompetitionID == r.PathValue("id") {
			mine = append(mine, e)
		}
	}
	if len(mine) == 0 {
		failed(w, r, store.ErrNotFound)
		return store.Competition{}, nil, false
	}
	c, err := p.st.Competition(r.Context(), r.PathValue("id"))
	if err != nil {
		failed(w, r, err)
		return c, nil, false
	}
	feedbackOn(r, c)
	return c, mine, true
}

// gymnastCompetition is an individual's page for one competition: a row for
// each discipline they entered, each opening to its card.
func (p *competitionPages) gymnastCompetition(w http.ResponseWriter, r *http.Request) {
	ind, entries, ok := p.gymnast(w, r)
	if !ok {
		return
	}
	c, mine, ok := p.gymnastEntered(w, r, entries)
	if !ok {
		return
	}
	token := r.PathValue("token")
	page := p.gymnastPage(r, ind, gymnastCompetitions)
	gc := views.GymnastCompetition{Competition: summary(c.Competition, p.now()), Status: views.Count(len(mine), "entry", "entries"), Day: entryPath(token, mine[0]) + "/day"}
	if gc.Competition.Open && c.Individuals {
		gc.Enter = gymnastCompetitionPath(token, c.ID) + "/enter"
	}
	for _, e := range mine {
		shown, err := card(c.Competition, e.Entry, clubOf(e))
		if err != nil {
			failed(w, r, err)
			return
		}
		withSignoff(&shown, storedSignoff(c.Competition, e))
		waiting := p.waitingOf(r.Context(), c.ID, e.ID)
		word, class := brief(individualState(e, waiting.Waiting), len(shown.Problems), waiting.Waiting, "Entered")
		gc.Events = append(gc.Events, views.GymnastEvent{
			Row:     views.MemberEvent{Discipline: e.Entry.Discipline, Name: competitions.DisciplineName(e.Entry.Discipline), Level: e.Entry.Level, Brief: word, BriefClass: class, Status: "Entered " + e.SentAt.In(local).Format("2 January, 15:04")},
			Gymnast: e.Entry.Gymnast, Open: entryPath(token, e), Placement: placement(c, e.ID), Card: &shown,
		})
	}
	page.Competition = &gc
	render(w, r, views.GymnastCompetitionPage(page))
}

// gymnastEnter enters another discipline of a competition the individual has
// entered, under the same link.
func (p *competitionPages) gymnastEnter(w http.ResponseWriter, r *http.Request) {
	ind, entries, ok := p.gymnast(w, r)
	if !ok {
		return
	}
	c, _, ok := p.gymnastEntered(w, r, entries)
	if !ok {
		return
	}
	if !c.Individuals {
		failed(w, r, store.ErrNotFound)
		return
	}
	if r.Method == http.MethodGet {
		e := competitions.Entry{Gymnast: ind.Name, Discipline: chosenDiscipline(c.Competition, r.URL.Query().Get("discipline"))}
		render(w, r, views.CompetitionEnter(enterPage(c, e, r.URL.Path, p.now())))
		return
	}
	e, problems := postedEntry(r, c.Competition, "")
	if len(problems) > 0 {
		page := enterPage(c, e, r.URL.Path, p.now())
		page.Form.Problems = problems
		w.WriteHeader(http.StatusUnprocessableEntity)
		render(w, r, views.CompetitionEnter(page))
		return
	}
	token := r.PathValue("token")
	added, _, err := p.st.AddIndividualEntry(r.Context(), c.ID, token, e)
	if err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, entryPath(token, added)+"?saved=1", http.StatusSeeOther)
}

// enterAs is the person an entry through a competition's individual link is
// for: the personal link posted as "as" (a member's, coach's or
// individual's, or an entry link), or "" for someone new. It says what's
// wrong with a link that isn't one.
func enterAs(r *http.Request) (string, string) {
	link := r.FormValue("as")
	if link == "" {
		return "", ""
	}
	kind, token, ok := pastedLink(link)
	switch {
	case !ok:
		return "", "That isn't a link from this site. Copy the whole link of your own page, or leave it blank."
	case kind != store.RoleMember && kind != store.RoleCoach && kind != store.RoleIndividual:
		return "", "That isn't the link of your own page. Give the link of a page you entered on your own, or your club member page, or leave it blank."
	}
	return token, ""
}

// asProblem is what's wrong with entering as a link, from the store.
func asProblem(err error) string {
	if errors.Is(err, store.ErrNotFound) {
		return "That link doesn't lead anywhere: it may have been replaced. Check you copied all of it, or leave it blank."
	}
	return ""
}
