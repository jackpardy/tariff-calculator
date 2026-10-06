package main

import (
	"fmt"
	"net/http"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Coach sign-off for clubs (ADR 0004 Decision 11): the comp sec adds coaches,
// members choose theirs (or the comp sec does), and each coach signs off the
// entries they see on their own page.

func coachPath(token string) string { return "/clubs/coach/" + token }

// memberSignoff is a member's entry's sign-off, as the pages show it.
func memberSignoff(c competitions.Competition, e store.MemberEntry) views.SignoffView {
	return views.SignoffView{Required: c.Signoff, Signed: e.SignedOff(), By: e.SignedBy, Note: e.SignNote}
}

// withSignoff adds an entry's sign-off to its card, and a missing one to its
// problems, as the organiser's dashboard counts it.
func withSignoff(card *views.EntryCard, s views.SignoffView) {
	card.Signoff = s
	if s.Required && !s.Signed {
		card.Problems = append(card.Problems, "Not signed off by a coach")
	}
}

// --- The comp sec ---

func (p *competitionPages) addCoach(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if err := competitions.CheckName("coach", name); err != nil {
		http.Redirect(w, r, clubPath(r.PathValue("token"))+"?notice="+urlQuery(strings.Join(sentences(err), " ")), http.StatusSeeOther)
		return
	}
	coach, token, err := p.st.CreateCoach(r.Context(), club.ID, name)
	if err != nil {
		failed(w, r, err)
		return
	}
	// Shown on this response only: it isn't kept, so it can't go in a redirect's URL.
	p.renderClub(w, r, club, &views.MemberLink{Name: coach.Name, Link: origin(r) + coachPath(token), Coach: true})
}

func (p *competitionPages) newCoachLink(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	coach, err := p.clubCoach(r, club)
	if err != nil {
		failed(w, r, err)
		return
	}
	token, err := p.st.ReplaceCoachLink(r.Context(), club.ID, coach.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	p.renderClub(w, r, club, &views.MemberLink{Name: coach.Name, Link: origin(r) + coachPath(token), Coach: true})
}

// clubCoach is the club's coach the URL names.
func (p *competitionPages) clubCoach(r *http.Request, club store.Club) (store.Coach, error) {
	coaches, err := p.st.Coaches(r.Context(), club.ID)
	if err != nil {
		return store.Coach{}, err
	}
	for _, c := range coaches {
		if c.ID == r.PathValue("coach") {
			return c, nil
		}
	}
	return store.Coach{}, store.ErrNotFound
}

func (p *competitionPages) removeCoach(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	coach, err := p.clubCoach(r, club)
	if err != nil {
		failed(w, r, err)
		return
	}
	if err := p.st.RemoveCoach(r.Context(), club.ID, coach.ID); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, clubPath(r.PathValue("token"))+"?notice="+urlQuery("Removed "+coach.Name+" as a coach."), http.StatusSeeOther)
}

func (p *competitionPages) coachesSeeAll(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	on := r.FormValue("on") == "1"
	if err := p.st.SetCoachesSeeAll(r.Context(), club.ID, on); err != nil {
		failed(w, r, err)
		return
	}
	notice := "Each coach sees the members who chose them, and those without a coach."
	if on {
		notice = "Every coach now sees every member."
	}
	http.Redirect(w, r, clubPath(r.PathValue("token"))+"?notice="+urlQuery(notice), http.StatusSeeOther)
}

// assignCoach chooses a member's coach for them (coach: an id, "" for none).
func (p *competitionPages) assignCoach(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	if err := p.st.SetMemberCoach(r.Context(), club.ID, r.PathValue("member"), r.FormValue("coach")); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, clubPath(r.PathValue("token")), http.StatusSeeOther)
}

// --- Members ---

// chooseOwnCoach is a member choosing their coach (coach: an id, "" for none).
func (p *competitionPages) chooseOwnCoach(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	if err := p.st.SetMemberCoach(r.Context(), m.ClubID, m.ID, r.FormValue("coach")); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, memberPath(r.PathValue("token"))+"?notice="+urlQuery("Coach saved."), http.StatusSeeOther)
}

// --- Coaches ---

// coach is the coach a link belongs to, and their club.
func (p *competitionPages) coach(w http.ResponseWriter, r *http.Request) (store.Coach, string, bool) {
	c, err := p.st.CoachByLink(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return c, "", false
	}
	club, err := p.st.ClubName(r.Context(), c.ClubID)
	if err != nil {
		failed(w, r, err)
		return c, "", false
	}
	return c, club, true
}

// coachHome lists every entry a coach sees, by competition.
func (p *competitionPages) coachHome(w http.ResponseWriter, r *http.Request) {
	coach, club, ok := p.coach(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	comps, err := p.st.ClubCompetitions(ctx, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	entries, err := p.st.CoachEntries(ctx, coach)
	if err != nil {
		failed(w, r, err)
		return
	}
	seesAll, err := p.st.CoachesSeeAll(ctx, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	path := coachPath(r.PathValue("token"))
	page := views.CoachPage{Club: club, Coach: coach.Name, Link: origin(r) + path, Notice: r.URL.Query().Get("notice"), SeesAll: seesAll}
	for _, c := range comps {
		cc := views.CoachCompetition{Competition: summary(c.Competition, p.now())}
		for _, e := range entries {
			if e.CompetitionID != c.ID {
				continue
			}
			row := views.CoachRow{Member: e.MemberName, Level: e.Entry.Event(), Signoff: memberSignoff(c.Competition, e),
				Open: path + "/members/" + e.MemberID + "/competitions/" + c.ID}
			if checked, err := c.Check(e.Entry); err != nil {
				row.Problems = 1
			} else {
				row.Problems = len(checked.Problems())
			}
			if !e.SignedOff() {
				cc.Waiting++
			}
			cc.Rows = append(cc.Rows, row)
		}
		page.Competitions = append(page.Competitions, cc)
	}
	render(w, r, views.CoachHome(page))
}

// coachSees is the entry the URL names, if the coach sees it, with its competition.
func (p *competitionPages) coachSees(w http.ResponseWriter, r *http.Request, coach store.Coach) (store.MemberEntry, store.Competition, bool) {
	entries, err := p.st.CoachEntries(r.Context(), coach)
	if err != nil {
		failed(w, r, err)
		return store.MemberEntry{}, store.Competition{}, false
	}
	for _, e := range entries {
		if e.MemberID == r.PathValue("member") && e.CompetitionID == r.PathValue("id") {
			c, err := p.st.Competition(r.Context(), e.CompetitionID)
			if err != nil {
				failed(w, r, err)
				return e, c, false
			}
			return e, c, true
		}
	}
	failed(w, r, store.ErrNotFound)
	return store.MemberEntry{}, store.Competition{}, false
}

// coachEntry shows a coach one of their members' entries, to sign it off.
func (p *competitionPages) coachEntry(w http.ResponseWriter, r *http.Request) {
	coach, club, ok := p.coach(w, r)
	if !ok {
		return
	}
	e, c, ok := p.coachSees(w, r, coach)
	if !ok {
		return
	}
	shown, err := card(c.Competition, e.Entry, club)
	if err != nil {
		failed(w, r, err)
		return
	}
	withSignoff(&shown, memberSignoff(c.Competition, e))
	render(w, r, views.SignoffEntry(views.SignoffPage{
		Back: coachPath(r.PathValue("token")), Competition: summary(c.Competition, p.now()),
		Card: shown, Signoff: shown.Signoff, Action: r.URL.Path,
	}))
}

// signOff records the coach's sign-off (signed=1) or not yet, with a note.
func (p *competitionPages) signOff(w http.ResponseWriter, r *http.Request) {
	coach, _, ok := p.coach(w, r)
	if !ok {
		return
	}
	e, c, ok := p.coachSees(w, r, coach)
	if !ok {
		return
	}
	if !c.Open(p.now()) {
		failed(w, r, store.ErrClosed)
		return
	}
	signed := r.FormValue("signed") == "1"
	if err := p.st.SignOff(r.Context(), coach, e.MemberID, c.ID, signed, limitNote(r.FormValue("note"))); err != nil {
		failed(w, r, err)
		return
	}
	notice := fmt.Sprintf("Signed off %s's entry for %s.", e.MemberName, c.Name)
	if !signed {
		notice = fmt.Sprintf("Told %s their entry for %s isn't ready yet.", e.MemberName, c.Name)
	}
	http.Redirect(w, r, coachPath(r.PathValue("token"))+"?notice="+urlQuery(notice), http.StatusSeeOther)
}
