package web

import (
	"cmp"
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
	feedbackAbout(r, c)
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
	page := views.EnterPage{Competition: summary(c, now), Form: entryForm(c.Competition, e, path, "Enter")}
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
	feedbackAbout(r, c)
	return e, c, true
}

// ownEntry opens an entry: on its person's page for the competition, at its
// row (ADR 0010), carrying what the address says (just saved, a notice);
// an entry on no one's page shows on its own.
func (p *competitionPages) ownEntry(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.own(w, r)
	if !ok {
		return
	}
	if e.IndividualID != "" {
		q := r.URL.Query()
		q.Set("entry", e.ID)
		http.Redirect(w, r, gymnastCompetitionPath(p.gymnastBase(r, e), c.ID)+"?"+q.Encode(), http.StatusSeeOther)
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
// (starting from form, with any problems): on their page for the
// competition, the entry's row open, or on its own for an entry on no one's
// page.
func (p *competitionPages) renderOwn(w http.ResponseWriter, r *http.Request, e store.Entry, c store.Competition, form competitions.Entry, problems []string) {
	if e.IndividualID != "" {
		p.renderGymnastCompetition(w, r, p.gymnastBase(r, e), c.ID, &postedOwn{entry: e.ID, form: form, problems: problems})
		return
	}
	page, err := p.entryView(r, e, c, "/competitions/entry/"+r.PathValue("token"), form, problems)
	if err != nil {
		failed(w, r, err)
		return
	}
	page.Link = origin(r) + "/competitions/entry/" + r.PathValue("token")
	page.JustSaved = r.URL.Query().Get("saved") == "1"
	if len(problems) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	render(w, r, views.CompetitionEntryPage(page))
}

// postedOwn is a change to one of an individual's entries that came back
// with problems, to show in its row.
type postedOwn struct {
	entry    string
	form     competitions.Entry
	problems []string
}

// entryView is one of an individual's entries, as their pages show it, its
// forms posting under path (the entry's own address), the change form
// starting from form, with any problems.
func (p *competitionPages) entryView(r *http.Request, e store.Entry, c store.Competition, path string, form competitions.Entry, problems []string) (views.EntryPage, error) {
	shown, err := card(c.Competition, e.Entry, clubOf(e))
	if err != nil {
		return views.EntryPage{}, err
	}
	withSignoff(&shown, storedSignoff(c.Competition, e))
	page := views.EntryPage{
		Name:        e.Entry.Gymnast,
		Competition: summary(c, p.now()),
		Sent:        e.SentAt.In(local).Format("Monday 2 January, 15:04"), Card: shown, Checked: e.Checked(), Note: e.Note,
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
	page.Notice = r.URL.Query().Get("notice")
	page.MyLinks = p.competitionLinks(r.Context(), c, linksFor{base: path, key: e.Key(), people: []string{e.Key()}, tell: individualTeller(e)})
	page.Fees = p.feesSummary(r.Context(), c, entryPayer(e.ID), path+"/invoice")
	if lateOpen(c, p.now()) {
		page.LateAsk = path + "/late"
	}
	switch {
	case e.Removal == store.Withdrew:
		page.Removal = "You withdrew this entry: it isn't in the competition."
		if page.Competition.ChangesOpen {
			page.Removal += " You can put it back until " + cmp.Or(page.Competition.ChangesUntil, page.Competition.Deadline) + "."
			page.Restore = path + "/restore"
		}
		page.Competition.Open, page.Competition.ChangesOpen = false, false
		page.SignoffLink = ""
	case e.Removal == store.Removed:
		page.Removal = strings.TrimSpace("The organiser removed this entry, so it can't be changed. " + e.RemovalNote)
		page.Competition.Open, page.Competition.ChangesOpen = false, false
	case e.Resent:
		page.Removal = "Changed and sent again: waiting for the organiser to accept it back in."
	case e.Removal == store.Held:
		page.Removal = strings.TrimSpace("The organiser asks for changes before taking this entry back in: change it below. " + e.RemovalNote)
	}
	if page.Coach, err = p.entryCoachView(r, c, e, path); err != nil {
		return page, err
	}
	seen := p.seenFor(r)
	if warn, err := seen.pairWarning(c.ID, e.ID); err != nil {
		return page, err
	} else if warn != "" {
		page.Card.Problems = append([]string{warn}, page.Card.Problems...)
	}
	pairs, err := p.st.IndividualPairEntries(r.Context(), e.ID)
	if err != nil {
		return page, err
	}
	if page.Pairs, err = seen.pairViews(pairs, nil); err != nil {
		return page, err
	}
	if e.Entry.Partner != nil {
		page.Partner = &views.PartnerView{Name: e.Entry.Partner.Name, Link: origin(r) + partnerPath(e.PartnerLink), Confirmed: e.PartnerConfirmed}
	}
	if offer, err := p.st.IndividualOffer(r.Context(), e.ID); err == nil {
		form := offerForm(c.Competition, offer, path+"/offer")
		form.Note = "One offer for the competition: it's the same on each of your entries."
		if form.Judging, err = p.entryJudging(r.Context(), c, e, path+"/judging", r.URL.Path); err != nil {
			return page, err
		}
		page.Offer = &form
	}
	page.Form.Problems = problems
	page.Form.Key = "-" + e.ID // its own field ids: their competition page shows each entry's form
	return page, nil
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
	seeOther(w, r, "/competitions/entry/"+r.PathValue("token"), "Withdrawn from "+c.Name+".")
}

// restore puts back an entry its gymnast withdrew.
func (p *competitionPages) restore(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := p.own(w, r); !ok {
		return
	}
	if err := p.st.RestoreIndividualEntry(r.Context(), r.PathValue("token")); err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, "/competitions/entry/"+r.PathValue("token"), "Put back: you're entered again.")
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
	feedbackAbout(r, c)
	if e.Removal == store.Withdrew {
		message(w, r, http.StatusOK, "Withdrawn", e.Entry.Gymnasts()+" withdrew this entry from "+c.Name+", so there's nothing to sign off.")
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
		Competition: summary(c, p.now()), Card: shown, Signoff: shown.Signoff,
		Action: r.URL.Path, AskName: true, Name: name, Problems: problems,
	}
	switch r.URL.Query().Get("done") {
	case "signed":
		page.Notice = "Signed off. Thank you: " + e.Entry.Gymnasts() + " sees it on their entry."
	case "notyet":
		page.Notice = "Saved: " + e.Entry.Gymnasts() + " sees it isn't ready yet, with your note."
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
	done := "notyet"
	if r.FormValue("signed") == "1" {
		done = "signed"
	}
	http.Redirect(w, r, r.URL.Path+"?done="+done, http.StatusSeeOther)
}
