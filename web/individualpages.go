package web

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Entering on one's own: the entry link's form, the individual's entry
// page, changing and withdrawing it, and their coach's sign-off (ADR 0004
// Decision 11).

// individual is the competition an individual entry link enters.
func (p *competitionPages) individual(w http.ResponseWriter, r *http.Request) (store.Competition, bool) {
	c, err := p.st.CompetitionByIndividualLink(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return c, false
	}
	return c, true
}

func (p *competitionPages) enterForm(w http.ResponseWriter, r *http.Request) {
	c, ok := p.individual(w, r)
	if !ok {
		return
	}
	page := enterPage(c, competitions.Entry{Discipline: chosenDiscipline(c.Competition, r.URL.Query().Get("discipline"))}, r.URL.Path, p.now())
	page.Form.As = &views.EnterAs{}
	render(w, r, views.CompetitionEnter(page))
}

// chosenDiscipline is the discipline asked for, if the competition offers
// it, else its first.
func chosenDiscipline(c competitions.Competition, asked string) string {
	offered := c.Disciplines()
	if slices.Contains(offered, asked) || len(offered) == 0 {
		return asked
	}
	return offered[0]
}

// enterPage is the individual entry page, for one discipline, with a tab for
// each the competition offers.
func enterPage(c store.Competition, e competitions.Entry, path string, now time.Time) views.EnterPage {
	page := views.EnterPage{Competition: summary(c.Competition, now), Form: entryForm(c.Competition, e, path, "Enter")}
	for _, d := range c.Disciplines() {
		page.Disciplines = append(page.Disciplines, views.DisciplineTab{
			Name: competitions.DisciplineName(d), URL: path + "?discipline=" + d, Current: d == e.Discipline,
		})
	}
	return page
}

func (p *competitionPages) enter(w http.ResponseWriter, r *http.Request) {
	c, ok := p.individual(w, r)
	if !ok {
		return
	}
	e, problems := postedEntry(r, c.Competition, "")
	as, problem := enterAs(r)
	if problem != "" {
		problems = append(problems, problem)
	}
	again := func(problems []string) {
		page := enterPage(c, e, r.URL.Path, p.now())
		page.Form.Problems, page.Form.As = problems, &views.EnterAs{Link: r.FormValue("as")}
		w.WriteHeader(http.StatusUnprocessableEntity)
		render(w, r, views.CompetitionEnter(page))
	}
	if len(problems) > 0 {
		again(problems)
		return
	}
	// As the person whose page's link was given, or someone new, with a
	// personal link of their own (ADR 0010).
	added, personal, err := p.st.AddIndividualEntry(r.Context(), c.ID, as, e)
	if problem := asProblem(err); problem != "" {
		again([]string{problem})
		return
	}
	if err != nil {
		failed(w, r, err)
		return
	}
	if personal == "" {
		personal = store.Personal(as)
	}
	http.Redirect(w, r, "/competitions/entry/"+store.Via(personal, added.ID)+"?saved=1", http.StatusSeeOther)
}

// own is the entry a personal link opens, with its competition.
func (p *competitionPages) own(w http.ResponseWriter, r *http.Request) (store.Entry, store.Competition, bool) {
	e, err := p.st.IndividualEntry(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return e, store.Competition{}, false
	}
	c, err := p.st.Competition(r.Context(), e.CompetitionID)
	if err != nil {
		failed(w, r, err)
		return e, c, false
	}
	return e, c, true
}

func (p *competitionPages) ownEntry(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.own(w, r)
	if !ok {
		return
	}
	p.renderOwn(w, r, e, c, e.Entry, nil)
}

// gymnastBase is the token of the gymnast's own page an entry opened with
// the request's token is on (ADR 0010): their personal link's, if it's
// theirs; otherwise the entry's own.
func (p *competitionPages) gymnastBase(r *http.Request, e store.Entry) string {
	token := r.PathValue("token")
	if !store.IsVia(token) {
		return token
	}
	if h, err := p.st.HolderOf(r.Context(), token); err == nil && h.Kind == store.RoleIndividual {
		return store.Personal(token)
	}
	return store.Via(store.Personal(token), e.IndividualID)
}

// renderOwn shows an individual their entry, and the form to change it
// (starting from form, with any problems).
func (p *competitionPages) renderOwn(w http.ResponseWriter, r *http.Request, e store.Entry, c store.Competition, form competitions.Entry, problems []string) {
	shown, err := card(c.Competition, e.Entry, clubOf(e))
	if err != nil {
		failed(w, r, err)
		return
	}
	withSignoff(&shown, storedSignoff(c.Competition, e))
	token := r.PathValue("token")
	path := "/competitions/entry/" + token
	page := views.EntryPage{
		Name:        e.Entry.Gymnast,
		Competition: summary(c.Competition, p.now()), Link: origin(r) + path, JustSaved: r.URL.Query().Get("saved") == "1",
		Sent: e.SentAt.In(local).Format("Monday 2 January, 15:04"), Card: shown, Checked: e.Checked(), Note: e.Note,
		Placement:   placement(c, e.ID),
		Waiting:     waitingText(p.waitingOf(r.Context(), c.ID, e.ID)),
		Late:        p.lateStatusOf(r.Context(), c, e.ID),
		Day:         path + "/day",
		VideoReview: e.VideoReview, VideoNote: e.VideoNote,
		Form: entryForm(c.Competition, form, path, "Save changes"), Withdraw: path + "/withdraw",
	}
	if c.Signoff {
		page.SignoffLink = origin(r) + signoffPath(e.SignoffLink)
	}
	// The entry is on its person's page (ADR 0010): opened through their
	// personal link, that's the link to keep; through an entry link from
	// before, that entry link.
	base := p.gymnastBase(r, e)
	if store.IsVia(token) {
		page.Link = ""
		if base == store.Personal(token) {
			page.Link, page.Personal = origin(r)+gymnastPath(base), true
		}
	}
	if e.IndividualID != "" {
		page.Menu, page.Back = gymnastMenu(base, gymnastCompetitions), gymnastCompetitionPath(base, c.ID)
	}
	page.Notice = r.URL.Query().Get("notice")
	page.MyLinks = p.competitionLinks(r.Context(), c, linksFor{base: path, key: e.Key(), people: []string{e.Key()}})
	page.Fees = p.feesSummary(r.Context(), c, entryPayer(e.ID), path+"/invoice")
	if lateOpen(c, p.now()) {
		page.LateAsk = path + "/late"
	}
	switch {
	case e.Removal == store.Removed:
		page.Removal = strings.TrimSpace("The organiser removed this entry, so it can't be changed. " + e.RemovalNote)
		page.Competition.Open, page.Competition.ChangesOpen = false, false
	case e.Resent:
		page.Removal = "Changed and sent again: waiting for the organiser to accept it back in."
	case e.Removal == store.Held:
		page.Removal = strings.TrimSpace("The organiser asks for changes before taking this entry back in: change it below. " + e.RemovalNote)
	}
	if page.Coach, err = p.entryCoachView(r, c, e, path); err != nil {
		failed(w, r, err)
		return
	}
	seen := p.seenFor(r)
	if warn, err := seen.pairWarning(c.ID, e.ID); err != nil {
		failed(w, r, err)
		return
	} else if warn != "" {
		page.Card.Problems = append([]string{warn}, page.Card.Problems...)
	}
	pairs, err := p.st.IndividualPairEntries(r.Context(), e.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	if page.Pairs, err = seen.pairViews(pairs, nil); err != nil {
		failed(w, r, err)
		return
	}
	if e.Entry.Partner != nil {
		page.Partner = &views.PartnerView{Name: e.Entry.Partner.Name, Link: origin(r) + partnerPath(e.PartnerLink), Confirmed: e.PartnerConfirmed}
	}
	if offer, err := p.st.IndividualOffer(r.Context(), e.ID); err == nil {
		form := offerForm(c.Competition, offer, path+"/offer")
		page.Offer = &form
	}
	page.Form.Problems = problems
	if len(problems) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	render(w, r, views.CompetitionEntryPage(page))
}

func (p *competitionPages) replaceEntry(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.own(w, r)
	if !ok {
		return
	}
	changed, problems := postedEntry(r, c.Competition, "")
	if len(problems) > 0 {
		p.renderOwn(w, r, e, c, changed, problems)
		return
	}
	if err := p.st.ReplaceIndividualEntry(r.Context(), r.PathValue("token"), changed); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, "/competitions/entry/"+r.PathValue("token")+"?saved=1", http.StatusSeeOther)
}

func (p *competitionPages) withdraw(w http.ResponseWriter, r *http.Request) {
	_, c, ok := p.own(w, r)
	if !ok {
		return
	}
	if r.FormValue("confirm") != "1" {
		http.Redirect(w, r, "/competitions/entry/"+r.PathValue("token"), http.StatusSeeOther)
		return
	}
	if err := p.st.WithdrawIndividualEntry(r.Context(), r.PathValue("token")); err != nil {
		failed(w, r, err)
		return
	}
	message(w, r, http.StatusOK, "Entry withdrawn", "Your entry for "+c.Name+" has been withdrawn and deleted.")
}

func signoffPath(token string) string { return "/competitions/signoff/" + token }

// storedSignoff is a competition's entry's sign-off, as the pages show it.
func storedSignoff(c competitions.Competition, e store.Entry) views.SignoffView {
	return views.SignoffView{Required: c.Signoff, Signed: e.SignedOff(), By: e.SignedBy, Note: e.SignNote, Unapproved: e.Unapproved}
}

// signoffEntry is the individual's entry a sign-off link opens, with its competition.
func (p *competitionPages) signoffEntry(w http.ResponseWriter, r *http.Request) (store.Entry, store.Competition, bool) {
	e, err := p.st.EntryBySignoffLink(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return e, store.Competition{}, false
	}
	c, err := p.st.Competition(r.Context(), e.CompetitionID)
	if err != nil {
		failed(w, r, err)
		return e, c, false
	}
	return e, c, true
}

// individualSignoff is the page an individual's coach signs off their entry on.
func (p *competitionPages) individualSignoff(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.signoffEntry(w, r)
	if !ok {
		return
	}
	p.renderIndividualSignoff(w, r, e, c, e.SignedBy, nil)
}

func (p *competitionPages) renderIndividualSignoff(w http.ResponseWriter, r *http.Request, e store.Entry, c store.Competition, name string, problems []string) {
	shown, err := card(c.Competition, e.Entry, clubOf(e))
	if err != nil {
		failed(w, r, err)
		return
	}
	withSignoff(&shown, storedSignoff(c.Competition, e))
	page := views.SignoffPage{
		Competition: summary(c.Competition, p.now()), Card: shown, Signoff: shown.Signoff,
		Action: r.URL.Path, AskName: true, Name: name, Problems: problems,
	}
	if named, why, err := p.entryCoachFor(r, c, e); err != nil {
		failed(w, r, err)
		return
	} else if why != "" {
		page.Cannot = why
	} else if named != "" {
		page.AskName, page.Name = false, named
	}
	if req, open, err := p.openLateFor(r.Context(), c.ID, e.ID); err == nil && open && !req.SignedOff() && c.Signoff {
		page.Late = []views.CoachLate{{Member: req.Entry.Gymnasts(), What: competitions.LateKindName(req.Kind) + " to " + req.Entry.Event(), Link: r.URL.Path + "/late"}}
	}
	if len(problems) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	render(w, r, views.SignoffEntry(page))
}

// signOffIndividual records the coach's sign-off (signed=1) or not yet, under
// their name (coach), with a note.
func (p *competitionPages) signOffIndividual(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.signoffEntry(w, r)
	if !ok {
		return
	}
	if !c.SignoffsOpen(p.now()) {
		failed(w, r, notOpen(c.Competition, p.now()))
		return
	}
	name := strings.TrimSpace(r.FormValue("coach"))
	if named, why, err := p.entryCoachFor(r, c, e); err != nil {
		failed(w, r, err)
		return
	} else if why != "" {
		p.renderIndividualSignoff(w, r, e, c, name, []string{why})
		return
	} else if named != "" {
		name = named // the coach the organiser approved
	}
	if err := competitions.CheckName("coach", name); err != nil {
		p.renderIndividualSignoff(w, r, e, c, name, sentences(err))
		return
	}
	if err := p.st.SignOffIndividual(r.Context(), r.PathValue("token"), name, r.FormValue("signed") == "1", limitNote(r.FormValue("note"))); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
}
