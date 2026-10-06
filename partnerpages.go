package main

import (
	"net/http"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Synchro partners (ADR 0005 Decision 3): a synchro entry's partner link
// opens a page where the partner confirms the pair, saying who they are by
// their own link (a club member page, or their own individual entry), so the
// timetable can check their clashes.

func partnerPath(token string) string { return "/competitions/partner/" + token }

func (p *competitionPages) registerPartners(handle func(string, http.HandlerFunc)) {
	handle("GET /competitions/partner/{token}", p.partner)
	handle("POST /competitions/partner/{token}", p.confirmPartner)
}

// invite is the synchro entry a partner link belongs to, with its competition.
func (p *competitionPages) invite(w http.ResponseWriter, r *http.Request) (store.PartnerInvite, store.Competition, bool) {
	inv, err := p.st.PartnerByLink(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return inv, store.Competition{}, false
	}
	c, err := p.st.Competition(r.Context(), inv.CompetitionID)
	if err != nil {
		failed(w, r, err)
		return inv, c, false
	}
	return inv, c, true
}

func (p *competitionPages) renderPartner(w http.ResponseWriter, r *http.Request, inv store.PartnerInvite, c store.Competition, problems []string) {
	page := views.PartnerPage{
		Competition: summary(c.Competition, p.now()), Gymnast: inv.Entry.Gymnast, Club: inv.Club,
		Event: inv.Entry.Event(), Confirmed: inv.Confirmed, Action: r.URL.Path, Problems: problems,
	}
	if inv.Entry.Partner != nil {
		page.Partner = inv.Entry.Partner.Name
	}
	if len(problems) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	render(w, r, views.PartnerConfirm(page))
}

func (p *competitionPages) partner(w http.ResponseWriter, r *http.Request) {
	inv, c, ok := p.invite(w, r)
	if !ok {
		return
	}
	p.renderPartner(w, r, inv, c, nil)
}

// confirmPartner confirms the pair as the partner whose own link is posted
// (link: a club member page or an individual entry in this competition, or
// nothing for a partner with no other entry).
func (p *competitionPages) confirmPartner(w http.ResponseWriter, r *http.Request) {
	inv, c, ok := p.invite(w, r)
	if !ok {
		return
	}
	link := strings.TrimSpace(r.FormValue("link"))
	token := func(prefix string) string {
		i := strings.LastIndex(link, prefix)
		if i < 0 {
			return ""
		}
		t, _, _ := strings.Cut(link[i+len(prefix):], "?")
		return t
	}
	var memberID, entryID string
	switch {
	case link == "":
	case token("/clubs/member/") != "":
		m, err := p.st.MemberByLink(r.Context(), token("/clubs/member/"))
		if err != nil {
			p.renderPartner(w, r, inv, c, []string{"That member link doesn't lead anywhere. Copy it from your club member page."})
			return
		}
		memberID = m.ID
	case token("/competitions/entry/") != "":
		e, err := p.st.IndividualEntry(r.Context(), token("/competitions/entry/"))
		if err != nil || e.CompetitionID != c.ID {
			p.renderPartner(w, r, inv, c, []string{"That isn't your entry in this competition. Copy the link from your own entry's page."})
			return
		}
		if e.Entry.Discipline == competitions.Synchro && e.PartnerLink == r.PathValue("token") {
			p.renderPartner(w, r, inv, c, []string{"That's this synchro entry itself: give your own link instead."})
			return
		}
		entryID = e.ID
	default:
		p.renderPartner(w, r, inv, c, []string{"That isn't a member page or entry link. Leave it blank if you have no other entry."})
		return
	}
	if err := p.st.ConfirmPartner(r.Context(), r.PathValue("token"), memberID, entryID); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
}
