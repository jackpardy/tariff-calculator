package main

import (
	"errors"
	"fmt"
	"io"
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
	if problem := s.Problem(); problem != "" {
		card.Problems = append(card.Problems, problem)
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
	sentTo, err := p.st.CoachSentTo(ctx, coach.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	path := coachPath(r.PathValue("token"))
	page := views.CoachPage{Club: club, Coach: coach.Name, Link: origin(r) + path, Notice: r.URL.Query().Get("notice"), SeesAll: seesAll, SignsOff: coach.SignsOff}
	for _, c := range comps {
		cc := views.CoachCompetition{Competition: summary(c.Competition, p.now()), Cannot: notApproved(c, sentTo), Notify: p.notifyLink(path + "/competitions/" + c.ID + "/notify")}
		if published(c) {
			cc.Timeline = path + "/competitions/" + c.ID + "/timeline"
			cc.Calendar = path + "/competitions/" + c.ID + "/calendar.ics"
		}
		seen := map[string]bool{}
		for _, e := range entries {
			if e.CompetitionID == c.ID {
				seen[e.MemberID] = true
			}
		}
		if coach.SignsOff && c.Signoff {
			lates, err := p.coachLateRequests(ctx, c, seen)
			if err != nil {
				failed(w, r, err)
				return
			}
			for _, l := range lates {
				cc.Late = append(cc.Late, views.CoachLate{Member: l.Entry.Gymnasts(), What: competitions.LateKindName(l.Kind) + " to " + l.Entry.Event(), Link: path + "/late/" + l.ID})
			}
		}
		for _, e := range entries {
			if e.CompetitionID != c.ID {
				continue
			}
			row := views.CoachRow{Member: e.MemberName, Level: e.Entry.Event(), Signoff: memberSignoff(c.Competition, e),
				Open: path + "/members/" + e.MemberID + "/competitions/" + c.ID + "?discipline=" + e.Discipline}
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
	pairs, err := p.st.CoachPairEntries(ctx, coach)
	if err != nil {
		failed(w, r, err)
		return
	}
	sees := map[string]bool{} // members whose own entries they see already
	for _, e := range entries {
		sees[e.MemberID] = true
	}
	if page.Pairs, err = p.seenFor(r).pairViews(pairs, func(pe store.PairEntry) bool { return sees[pe.EntrantMemberID] }); err != nil {
		failed(w, r, err)
		return
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
		if e.MemberID == r.PathValue("member") && e.CompetitionID == r.PathValue("id") && e.Discipline == r.FormValue("discipline") {
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
	page := views.SignoffPage{
		Back: coachPath(r.PathValue("token")), Competition: summary(c.Competition, p.now()),
		Card: shown, Signoff: shown.Signoff, Action: r.URL.Path + "?discipline=" + e.Discipline,
	}
	if !coach.SignsOff {
		page.Cannot = "Your club hasn't set you to sign off routines, so you can see this entry but not sign it off."
	} else if why, err := p.notApprovedFor(r, c, coach); err != nil {
		failed(w, r, err)
		return
	} else if why != "" {
		page.Cannot = why
	}
	render(w, r, views.SignoffEntry(page))
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
	if !c.SignoffsOpen(p.now()) {
		failed(w, r, notOpen(c.Competition, p.now()))
		return
	}
	if !coach.SignsOff {
		http.Redirect(w, r, coachPath(r.PathValue("token"))+"?notice="+urlQuery("Your club hasn't set you to sign off routines; nothing was changed."), http.StatusSeeOther)
		return
	}
	if why, err := p.notApprovedFor(r, c, coach); err != nil {
		failed(w, r, err)
		return
	} else if why != "" {
		http.Redirect(w, r, coachPath(r.PathValue("token"))+"?notice="+urlQuery(why+" Nothing was changed."), http.StatusSeeOther)
		return
	}
	signed := r.FormValue("signed") == "1"
	if err := p.st.SignOff(r.Context(), coach, e.MemberID, c.ID, e.Discipline, signed, limitNote(r.FormValue("note"))); err != nil {
		failed(w, r, err)
		return
	}
	notice := fmt.Sprintf("Signed off %s's %s entry for %s.", e.MemberName, e.Entry.Event(), c.Name)
	if !signed {
		notice = fmt.Sprintf("Told %s their %s entry for %s isn't ready yet.", e.MemberName, e.Entry.Event(), c.Name)
	}
	http.Redirect(w, r, coachPath(r.PathValue("token"))+"?notice="+urlQuery(notice), http.StatusSeeOther)
}

// maxCertificateBytes is the largest certificate (ADR 0007 Decision 4).
const maxCertificateBytes = 10 << 20

// uploads says whether a path takes an upload, for its larger size limit.
func uploads(path string) bool {
	return strings.HasPrefix(path, "/clubs/admin/") && strings.HasSuffix(path, "/qualifications") ||
		strings.HasPrefix(path, "/competitions/entry/") && strings.HasSuffix(path, "/coach")
}

// certificateType is the media type of a certificate from its own bytes:
// a JPEG, PNG or WebP photo, or a PDF; "" for anything else.
func certificateType(data []byte) string {
	switch t := http.DetectContentType(data); t {
	case "image/jpeg", "image/png", "image/webp", "application/pdf":
		return t
	}
	return ""
}

// qualificationName names a qualification by its key, or says it's no
// longer on the list.
func qualificationName(key string) string {
	if q, ok := competitions.QualificationByKey(key); ok {
		return q.Name()
	}
	return "A qualification no longer on the list"
}

// coachSignsOff says whether a coach signs off routines (on=1) or not.
func (p *competitionPages) coachSignsOff(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	coach, err := p.clubCoach(r, club)
	if err != nil {
		failed(w, r, err)
		return
	}
	on := r.FormValue("on") == "1"
	if err := p.st.SetCoachSignsOff(r.Context(), club.ID, coach.ID, on); err != nil {
		failed(w, r, err)
		return
	}
	notice := coach.Name + " signs off routines."
	if !on {
		notice = coach.Name + " doesn't sign off routines, but still sees their members' entries."
	}
	http.Redirect(w, r, clubPath(r.PathValue("token"))+"?notice="+urlQuery(notice), http.StatusSeeOther)
}

// addQualification gives a coach a qualification (qualification: its key)
// with its certificate (a file).
func (p *competitionPages) addQualification(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	coach, err := p.clubCoach(r, club)
	if err != nil {
		failed(w, r, err)
		return
	}
	back := func(notice string) {
		http.Redirect(w, r, clubPath(r.PathValue("token"))+"?notice="+urlQuery(notice), http.StatusSeeOther)
	}
	q, data, certType, problem := postedCertificate(r)
	if problem != "" {
		back("Nothing was added: " + problem)
		return
	}
	if _, err := p.st.AddQualification(r.Context(), club.ID, coach.ID, q.Key, certType, data); errors.Is(err, store.ErrLimit) {
		back(fmt.Sprintf("Nothing was added: a coach can have %d qualifications at most. Remove one first.", store.MaxQualifications))
		return
	} else if err != nil {
		failed(w, r, err)
		return
	}
	back("Added " + q.Name() + " for " + coach.Name + ".")
}

// postedCertificate is the qualification (its key) and certificate (a file)
// a form uploads, or what's wrong with them.
func postedCertificate(r *http.Request) (q competitions.Qualification, data []byte, certType, problem string) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return q, nil, "", "the certificate is over 10 MB. A photo or a smaller PDF will do."
		}
		return q, nil, "", "choose a qualification and its certificate."
	}
	q, ok := competitions.QualificationByKey(r.FormValue("qualification"))
	if !ok {
		return q, nil, "", "choose the coach's qualification."
	}
	file, _, err := r.FormFile("certificate")
	if err != nil {
		return q, nil, "", "add the certificate, a photo or PDF of it."
	}
	defer file.Close()
	data, err = io.ReadAll(io.LimitReader(file, maxCertificateBytes+1))
	if err != nil || len(data) > maxCertificateBytes {
		return q, nil, "", "the certificate is over 10 MB. A photo or a smaller PDF will do."
	}
	if certType = certificateType(data); certType == "" {
		return q, nil, "", "the certificate should be a photo (JPEG, PNG or WebP) or a PDF."
	}
	return q, data, certType, ""
}

// removeQualification removes a coach's qualification and its certificate.
func (p *competitionPages) removeQualification(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	coach, err := p.clubCoach(r, club)
	if err != nil {
		failed(w, r, err)
		return
	}
	if err := p.st.RemoveQualification(r.Context(), club.ID, r.PathValue("q")); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, clubPath(r.PathValue("token"))+"?notice="+urlQuery("Removed a qualification of "+coach.Name+"'s, and its certificate."), http.StatusSeeOther)
}

// clubCertificate shows the comp sec a coach's certificate.
func (p *competitionPages) clubCertificate(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	certType, data, err := p.st.Certificate(r.Context(), club.ID, r.PathValue("q"))
	if err != nil {
		failed(w, r, err)
		return
	}
	serveCertificate(w, certType, data)
}

// serveCertificate writes a certificate: as its own type only, never
// cached, shown in the browser.
func serveCertificate(w http.ResponseWriter, certType string, data []byte) {
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "application/pdf": ".pdf"}[certType]
	w.Header().Set("Content-Type", certType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", `inline; filename="certificate`+ext+`"`)
	w.Write(data)
}

// notApproved says why a coach can't sign off at a competition that approves
// coaches (ADR 0007 Decision 7), given where they've been sent; "" if they
// can.
func notApproved(c store.Competition, sentTo map[string]store.SentCoach) string {
	if !c.Signoff || !c.ApproveCoaches {
		return ""
	}
	s, ok := sentTo[c.ID]
	switch {
	case !ok:
		return "This competition approves coaches: your club hasn't sent you yet, so you can't sign off for it."
	case s.Status == store.CoachWaiting:
		return "Waiting for the organiser to approve you before you can sign off."
	case s.Status == store.CoachRefused:
		return strings.TrimSpace("The organiser hasn't approved you, so you can't sign off. " + s.Note)
	case s.Status == store.CoachWithdrawn:
		return strings.TrimSpace("The organiser withdrew your approval, so you can't sign off. " + s.Note)
	}
	return ""
}

// notApprovedFor is notApproved, looking up where the coach has been sent.
func (p *competitionPages) notApprovedFor(r *http.Request, c store.Competition, coach store.Coach) (string, error) {
	if !c.Signoff || !c.ApproveCoaches {
		return "", nil
	}
	sentTo, err := p.st.CoachSentTo(r.Context(), coach.ID)
	if err != nil {
		return "", err
	}
	return notApproved(c, sentTo), nil
}
