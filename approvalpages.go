package main

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Approving coaches, for the organiser (ADR 0007 Decisions 2, 6 and 7): the
// dashboard's setting, the page of coaches clubs have sent, their
// certificates, and the organiser's decisions.

func (p *competitionPages) registerApprovals(handle func(string, http.HandlerFunc)) {
	handle("POST /competitions/admin/{token}/approve-coaches", p.setApproveCoaches)
	handle("GET /competitions/admin/{token}/coaches", p.approvals)
	handle("POST /competitions/admin/{token}/coaches/{coach}", p.decideCoach)
	handle("GET /competitions/admin/{token}/coaches/{coach}/certificates/{q}", p.competitionCertificate)
}

// coachDisciplines are the disciplines a competition's coaches are asked
// about: trampoline (for synchro too), tumbling and DMT, those it offers.
func coachDisciplines(c competitions.Competition) []string {
	var out []string
	for _, d := range c.Disciplines() {
		if d == competitions.Synchro {
			d = competitions.Trampoline
		}
		if !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}

// coachApprovalForm is the dashboard's setting for approving coaches.
func coachApprovalForm(c competitions.Competition, sent []store.SentCoach) views.CoachApprovalForm {
	f := views.CoachApprovalForm{On: c.ApproveCoaches}
	for _, d := range coachDisciplines(c) {
		f.Levels = append(f.Levels, views.CoachLevelField{Key: offerKey(d), Name: competitions.DisciplineName(d), Level: c.CoachLevel(d)})
	}
	for _, s := range sent {
		if s.Status == store.CoachWaiting {
			f.Waiting++
		}
	}
	return f
}

// setApproveCoaches says whether coaches must be approved (on=1), and the
// lowest level accepted in each discipline (level-<discipline>).
func (p *competitionPages) setApproveCoaches(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	on := r.FormValue("on") == "1"
	levels := map[string]int{}
	for _, d := range coachDisciplines(c.Competition) {
		if n, err := strconv.Atoi(r.FormValue("level-" + offerKey(d))); err == nil && n >= 1 && n <= 4 && n != competitions.DefaultCoachLevel {
			levels[d] = n
		}
	}
	if err := p.st.SetApproveCoaches(r.Context(), c.ID, on, levels); err != nil {
		failed(w, r, err)
		return
	}
	notice := "Any coach can sign off."
	if on {
		notice = "Only coaches you approve can sign off: clubs send theirs, and you approve them on the Coaches page."
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

// approvals is the page of coaches clubs have sent, by club.
func (p *competitionPages) approvals(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	sent, err := p.st.CompetitionCoaches(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	base := adminPath(r.PathValue("token"))
	page := views.ApprovalsPage{Base: base, Competition: summary(c.Competition, p.now()), Notice: r.URL.Query().Get("notice")}
	var asked []string
	for _, d := range coachDisciplines(c.Competition) {
		asked = append(asked, competitions.DisciplineName(d)+" Level "+strconv.Itoa(c.CoachLevel(d)))
	}
	page.Asked = joinAnd(asked)
	for _, s := range sent {
		row := views.ApprovalRow{ID: s.CoachID, Name: s.Name, Status: coachStatus(s.Status), Approved: s.Approved(),
			Withdrawn: s.Status == store.CoachWithdrawn, Note: s.Note, Changed: s.Changed}
		if s.Status == store.CoachWaiting {
			row.Status = "Waiting"
		}
		var quals []competitions.Qualification
		for _, q := range s.Qualifications {
			row.Qualifications = append(row.Qualifications, views.ApprovalQualification{Name: qualificationName(q.Qualification),
				Certificate: base + "/coaches/" + s.CoachID + "/certificates/" + q.ID})
			if known, ok := competitions.QualificationByKey(q.Qualification); ok {
				quals = append(quals, known)
			}
		}
		for _, d := range coachDisciplines(c.Competition) {
			meets := slices.ContainsFunc(quals, func(q competitions.Qualification) bool { return q.Meets(d, c.CoachLevel(d)) })
			mark := " ✗"
			if meets {
				mark = " ✓"
			}
			row.Meets = append(row.Meets, competitions.DisciplineName(d)+" Level "+strconv.Itoa(c.CoachLevel(d))+mark)
		}
		club := s.ClubName
		if i := len(page.Clubs) - 1; i >= 0 && page.Clubs[i].Club == club {
			page.Clubs[i].Coaches = append(page.Clubs[i].Coaches, row)
		} else {
			page.Clubs = append(page.Clubs, views.ApprovalClub{Club: club, Coaches: []views.ApprovalRow{row}})
		}
	}
	render(w, r, views.CompetitionApprovals(page))
}

// joinAnd joins words as "a, b and c".
func joinAnd(words []string) string {
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}

// decideCoach approves a coach (action=approve; afresh, counting only their
// sign-offs from now on), says they're not approved (refuse), or withdraws
// their approval (withdraw), with a note for the club.
func (p *competitionPages) decideCoach(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	sent, err := p.st.CompetitionCoaches(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	i := slices.IndexFunc(sent, func(s store.SentCoach) bool { return s.CoachID == r.PathValue("coach") })
	if i < 0 {
		failed(w, r, store.ErrNotFound)
		return
	}
	coach, note := sent[i], limitNote(r.FormValue("note"))
	status, afresh, notice := "", false, ""
	switch r.FormValue("action") {
	case "approve":
		status, notice = store.CoachApproved, "Approved "+coach.Name+"."
	case "afresh":
		status, afresh, notice = store.CoachApproved, true, "Approved "+coach.Name+": their sign-offs from now on count."
	case "refuse":
		status, notice = store.CoachRefused, coach.Name+" isn't approved."
	case "withdraw":
		status, notice = store.CoachWithdrawn, "Withdrew "+coach.Name+"'s approval: their sign-offs don't count."
	default:
		http.Redirect(w, r, adminPath(r.PathValue("token"))+"/coaches", http.StatusSeeOther)
		return
	}
	if err := p.st.DecideCoach(r.Context(), c.ID, coach.CoachID, status, note, afresh); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"/coaches?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

// competitionCertificate shows the organiser a certificate of a coach sent
// to them.
func (p *competitionPages) competitionCertificate(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	certType, data, err := p.st.CompetitionCertificate(r.Context(), c.ID, r.PathValue("coach"), r.PathValue("q"))
	if err != nil {
		failed(w, r, err)
		return
	}
	serveCertificate(w, certType, data)
}
