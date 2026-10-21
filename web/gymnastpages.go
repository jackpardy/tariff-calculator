package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

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
	gymnastRequirements = "requirements"
	gymnastDetails      = "details"
)

// gymnastMenu is the menu of an individual's pages, with the tab shown
// current. A competition's page, and each entry's, is in Competitions.
func gymnastMenu(token, current string) []views.MenuSection {
	return []views.MenuSection{
		{Label: "Competitions", Href: gymnastPath(token), Current: current == gymnastCompetitions},
		{Label: "Requirements", Href: gymnastPath(token) + "/requirements", Current: current == gymnastRequirements},
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
	return p.gymnastPageOf(r, r.PathValue("token"), ind, tab)
}

// gymnastPageOf is that for their page at token, whatever the request's.
func (p *competitionPages) gymnastPageOf(r *http.Request, token string, ind store.Individual, tab string) views.GymnastPage {
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
	byComp := map[string][]store.Entry{}
	var order []string
	for _, e := range entries {
		if len(byComp[e.CompetitionID]) == 0 {
			order = append(order, e.CompetitionID)
		}
		byComp[e.CompetitionID] = append(byComp[e.CompetitionID], e)
	}
	for _, id := range order {
		c, err := p.st.Competition(r.Context(), id)
		if err != nil {
			failed(w, r, err)
			return
		}
		// As a member's card has: how many entries, their problems, and the
		// timetable and messages chips.
		mine, live, withdrawn := byComp[id], 0, 0
		gc := views.GymnastCompetition{Page: gymnastCompetitionPath(token, id), Competition: summary(c, p.now())}
		for _, e := range mine {
			if e.Removal == store.Withdrew {
				withdrawn++
				continue
			}
			live++
			if shown, err := card(c.Competition, e.Entry, onTheirOwn); err == nil {
				gc.Problems += len(shown.Problems)
			}
		}
		gc.Status = views.Count(live, "entry", "entries")
		if withdrawn > 0 {
			gc.Status += fmt.Sprintf(" · %d withdrawn", withdrawn)
		}
		gc.MyLinks = p.competitionLinks(r.Context(), c, linksFor{base: entryPath(token, mine[0]), key: mine[0].Key(), people: []string{mine[0].Key()}})
		page.Competitions = append(page.Competitions, gc)
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
	feedbackAbout(r, c)
	return c, mine, true
}

// gymnastCompetition is an individual's page for one competition: a row for
// each discipline they entered, each opening to all of that entry.
func (p *competitionPages) gymnastCompetition(w http.ResponseWriter, r *http.Request) {
	p.renderGymnastCompetition(w, r, r.PathValue("token"), r.PathValue("id"), nil)
}

// renderGymnastCompetition shows the page of the individual at token for a
// competition, with a change that came back with problems (posted) in its
// row; the row the address names (entry) opens, as does a lone entry's.
func (p *competitionPages) renderGymnastCompetition(w http.ResponseWriter, r *http.Request, token, competitionID string, posted *postedOwn) {
	ind, err := p.st.IndividualByLink(r.Context(), token)
	if err != nil {
		failed(w, r, err)
		return
	}
	entries, err := p.st.IndividualEntries(r.Context(), ind.PersonID)
	if err != nil {
		failed(w, r, err)
		return
	}
	var mine []store.Entry
	for _, e := range entries {
		if e.CompetitionID == competitionID {
			mine = append(mine, e)
		}
	}
	if len(mine) == 0 {
		failed(w, r, store.ErrNotFound)
		return
	}
	c, err := p.st.Competition(r.Context(), competitionID)
	if err != nil {
		failed(w, r, err)
		return
	}
	feedbackAbout(r, c)
	page := p.gymnastPageOf(r, token, ind, gymnastCompetitions)
	first := entryPath(token, mine[0])
	gc := views.GymnastCompetition{Competition: summary(c, p.now()), Status: views.Count(len(mine), "entry", "entries"), Day: first + "/day",
		Saved: r.URL.Query().Get("saved") == "1"}
	gc.MyLinks = p.competitionLinks(r.Context(), c, linksFor{base: first, key: mine[0].Key(), people: []string{mine[0].Key()}})
	// Another discipline: the first not entered yet, if there's one.
	if left := notEntered(c.Competition, mine); gc.Competition.Open && c.Individuals && len(left) > 0 {
		gc.Enter = gymnastCompetitionPath(token, c.ID) + "/enter?discipline=" + url.QueryEscape(left[0])
	}
	open := r.URL.Query().Get("entry")
	for _, e := range mine {
		form, problems := e.Entry, []string(nil)
		if posted != nil && posted.entry == e.ID {
			form, problems = posted.form, posted.problems
		}
		ev, err := p.entryView(r, e, c, entryPath(token, e), form, problems)
		if err != nil {
			failed(w, r, err)
			return
		}
		gc.Pairs = append(gc.Pairs, ev.Pairs...)
		waiting := p.waitingOf(r.Context(), c.ID, e.ID)
		word, state := brief(individualState(e, waiting.Waiting), len(ev.Card.Problems), waiting.Waiting, "Entered")
		gc.Events = append(gc.Events, views.GymnastEvent{
			Row:   views.MemberEvent{Discipline: e.Entry.Discipline, Name: competitions.DisciplineName(e.Entry.Discipline), Level: ev.Card.Level, Brief: word, BriefState: state},
			Open:  e.ID == open || len(mine) == 1 || len(problems) > 0,
			Entry: ev,
		})
	}
	page.Competition = &gc
	if posted != nil && len(posted.problems) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	render(w, r, views.GymnastCompetitionPage(page))
}

// gymnastEnter enters another discipline of a competition the individual has
// entered, under the same link.
func (p *competitionPages) gymnastEnter(w http.ResponseWriter, r *http.Request) {
	ind, entries, ok := p.gymnast(w, r)
	if !ok {
		return
	}
	c, mine, ok := p.gymnastEntered(w, r, entries)
	if !ok {
		return
	}
	if !c.Individuals {
		failed(w, r, store.ErrNotFound)
		return
	}
	if r.Method == http.MethodGet {
		asked := r.URL.Query().Get("discipline")
		if left := notEntered(c.Competition, mine); !r.URL.Query().Has("discipline") && len(left) > 0 {
			asked = left[0]
		}
		e := competitions.Entry{Gymnast: ind.Name, Discipline: chosenDiscipline(c.Competition, asked)}
		render(w, r, views.GymnastEnterPage(p.gymnastEnterPage(r, ind, c, mine, e, nil)))
		return
	}
	// Under their name: it's the same person's page.
	e, problems := postedEntry(r, c.Competition, ind.Name)
	// One entry a discipline: a second is a change to the first, which the
	// page links to in place of the form.
	if page := p.gymnastEnterPage(r, ind, c, mine, e, problems); page.Enter.Already != nil || len(problems) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		render(w, r, views.GymnastEnterPage(page))
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

// gymnastEnterPage is entering e's discipline of competition c on the
// individual's page, with the form's problems: tabs for the disciplines
// they haven't entered, links to those they have (mine), and in place of
// the form, a link to the entry if they've entered e's already.
func (p *competitionPages) gymnastEnterPage(r *http.Request, ind store.Individual, c store.Competition, mine []store.Entry, e competitions.Entry, problems []string) views.GymnastPage {
	back := gymnastCompetitionPath(r.PathValue("token"), c.ID)
	page := p.gymnastPage(r, ind, gymnastCompetitions)
	enter := &views.GymnastEnter{Back: back, Competition: summary(c, p.now())}
	for _, m := range mine {
		tab := views.DisciplineTab{Name: competitions.DisciplineName(m.Entry.Discipline), URL: back + "?entry=" + url.QueryEscape(m.ID)}
		enter.Entered = append(enter.Entered, tab)
		if m.Entry.Discipline == e.Discipline {
			enter.Already = &tab
		}
	}
	for _, d := range notEntered(c.Competition, mine) {
		enter.Disciplines = append(enter.Disciplines, views.DisciplineTab{
			Name: competitions.DisciplineName(d), URL: r.URL.Path + "?discipline=" + url.QueryEscape(d), Current: d == e.Discipline,
		})
	}
	if enter.Already == nil {
		form := entryForm(c.Competition, e, r.URL.Path, "Enter")
		form.Member, form.Problems = true, problems
		enter.Form = &form
	}
	page.Enter = enter
	return page
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

// notEntered are the disciplines a competition offers that these entries
// aren't in, in its order.
func notEntered(c competitions.Competition, mine []store.Entry) []string {
	var out []string
	for _, d := range c.Disciplines() {
		if !slices.ContainsFunc(mine, func(m store.Entry) bool { return m.Entry.Discipline == d }) {
			out = append(out, d)
		}
	}
	return out
}
