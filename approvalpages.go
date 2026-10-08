package main

import (
	"errors"
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
	handle("POST /competitions/admin/{token}/entry-coaches/{entry}", p.decideEntryCoach)
	handle("GET /competitions/admin/{token}/entry-coaches/{entry}/certificate", p.entryCertificate)
	handle("POST /competitions/entry/{token}/coach", p.nameEntryCoach)
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

// coachApprovalForm is the dashboard's setting for approving coaches, with
// how many coaches sent, and named by individuals, are waiting.
func coachApprovalForm(c competitions.Competition, sent []store.SentCoach, named []store.EntryCoach) views.CoachApprovalForm {
	f := views.CoachApprovalForm{On: c.ApproveCoaches}
	for _, d := range coachDisciplines(c) {
		f.Levels = append(f.Levels, views.CoachLevelField{Key: offerKey(d), Name: competitions.DisciplineName(d), Level: c.CoachLevel(d)})
	}
	for _, s := range sent {
		if s.Status == store.CoachWaiting {
			f.Waiting++
		}
	}
	for _, n := range named {
		if n.Status == store.CoachWaiting {
			f.Waiting++
		}
	}
	return f
}

// askedLevels says the levels a competition asks for, e.g. "Trampoline Level
// 2 and Tumbling Level 1".
func askedLevels(c competitions.Competition) string {
	var asked []string
	for _, d := range coachDisciplines(c) {
		asked = append(asked, competitions.DisciplineName(d)+" Level "+strconv.Itoa(c.CoachLevel(d)))
	}
	return joinAnd(asked)
}

// meets says, for each discipline asked about, whether the qualifications
// (by key) meet its level, e.g. "Trampoline Level 2 ✓".
func meets(c competitions.Competition, keys []string) []string {
	var quals []competitions.Qualification
	for _, k := range keys {
		if q, ok := competitions.QualificationByKey(k); ok {
			quals = append(quals, q)
		}
	}
	var out []string
	for _, d := range coachDisciplines(c) {
		mark := " ✗"
		if slices.ContainsFunc(quals, func(q competitions.Qualification) bool { return q.Meets(d, c.CoachLevel(d)) }) {
			mark = " ✓"
		}
		out = append(out, competitions.DisciplineName(d)+" Level "+strconv.Itoa(c.CoachLevel(d))+mark)
	}
	return out
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
	named, err := p.st.CompetitionEntryCoaches(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	page := views.ApprovalsPage{Base: base, Competition: summary(c.Competition, p.now()), Notice: r.URL.Query().Get("notice"), Asked: askedLevels(c.Competition)}
	for _, s := range sent {
		row := views.ApprovalRow{ID: s.CoachID, Name: s.Name, Action: base + "/coaches/" + s.CoachID, Status: coachStatus(s.Status), Approved: s.Approved(),
			Withdrawn: s.Status == store.CoachWithdrawn, Note: s.Note, Changed: s.Changed}
		if s.Status == store.CoachWaiting {
			row.Status = "Waiting"
		}
		var keys []string
		for _, q := range s.Qualifications {
			row.Qualifications = append(row.Qualifications, views.ApprovalQualification{Name: qualificationName(q.Qualification),
				Certificate: base + "/coaches/" + s.CoachID + "/certificates/" + q.ID})
			keys = append(keys, q.Qualification)
		}
		row.Meets = meets(c.Competition, keys)
		club := s.ClubName
		if i := len(page.Clubs) - 1; i >= 0 && page.Clubs[i].Club == club {
			page.Clubs[i].Coaches = append(page.Clubs[i].Coaches, row)
		} else {
			page.Clubs = append(page.Clubs, views.ApprovalClub{Club: club, Coaches: []views.ApprovalRow{row}})
		}
	}
	if len(named) > 0 {
		individuals := views.ApprovalClub{Club: "Individuals"}
		for _, n := range named {
			row := views.ApprovalRow{ID: n.EntryID, Name: n.Name + ", coach of " + n.Gymnast, Action: base + "/entry-coaches/" + n.EntryID,
				Status: coachStatus(n.Status), Approved: n.Approved(), Withdrawn: n.Status == store.CoachWithdrawn, Note: n.Note,
				Qualifications: []views.ApprovalQualification{{Name: qualificationName(n.Qualification), Certificate: base + "/entry-coaches/" + n.EntryID + "/certificate"}},
				Meets:          meets(c.Competition, []string{n.Qualification})}
			if n.Status == store.CoachWaiting {
				row.Status = "Waiting"
			}
			individuals.Coaches = append(individuals.Coaches, row)
		}
		page.Clubs = append(page.Clubs, individuals)
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
	status, afresh, notice, ok := decision(r, coach.Name)
	if !ok {
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

// decision is what the organiser decided about a coach (action: approve,
// afresh, refuse or withdraw), and what to tell them it did.
func decision(r *http.Request, name string) (status string, afresh bool, notice string, ok bool) {
	switch r.FormValue("action") {
	case "approve":
		return store.CoachApproved, false, "Approved " + name + ".", true
	case "afresh":
		return store.CoachApproved, true, "Approved " + name + ": their sign-offs from now on count.", true
	case "refuse":
		return store.CoachRefused, false, name + " isn't approved.", true
	case "withdraw":
		return store.CoachWithdrawn, false, "Withdrew " + name + "'s approval: their sign-offs don't count.", true
	}
	return "", false, "", false
}

// decideEntryCoach is decideCoach for a coach an individual named.
func (p *competitionPages) decideEntryCoach(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	named, err := p.st.EntryCoachOf(r.Context(), r.PathValue("entry"))
	if err != nil || named.CompetitionID != c.ID {
		failed(w, r, store.ErrNotFound)
		return
	}
	status, afresh, notice, ok := decision(r, named.Name)
	if !ok {
		http.Redirect(w, r, adminPath(r.PathValue("token"))+"/coaches", http.StatusSeeOther)
		return
	}
	if err := p.st.DecideEntryCoach(r.Context(), c.ID, named.EntryID, status, limitNote(r.FormValue("note")), afresh); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"/coaches?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

// entryCertificate shows the organiser the certificate of a coach an
// individual named.
func (p *competitionPages) entryCertificate(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	certType, data, err := p.st.EntryCertificate(r.Context(), c.ID, r.PathValue("entry"))
	if err != nil {
		failed(w, r, err)
		return
	}
	serveCertificate(w, certType, data)
}

// entryCoachView is an individual's coach as their entry page shows it.
func (p *competitionPages) entryCoachView(r *http.Request, c store.Competition, e store.Entry, path string) (*views.EntryCoachView, error) {
	if !c.Signoff || !c.ApproveCoaches {
		return nil, nil
	}
	v := &views.EntryCoachView{Action: path + "/coach", Asked: askedLevels(c.Competition)}
	for _, q := range competitions.Qualifications {
		v.Options = append(v.Options, views.CoachOption{ID: q.Key, Name: q.Name()})
	}
	named, err := p.st.EntryCoachOf(r.Context(), e.ID)
	if errors.Is(err, store.ErrNotFound) {
		return v, nil
	} else if err != nil {
		return nil, err
	}
	v.Name, v.Qualification, v.Status, v.Approved, v.Note = named.Name, qualificationName(named.Qualification), coachStatus(named.Status), named.Approved(), named.Note
	return v, nil
}

// nameEntryCoach names an individual's coach (name, qualification, and a
// certificate file), for the organiser to approve.
func (p *competitionPages) nameEntryCoach(w http.ResponseWriter, r *http.Request) {
	_, c, ok := p.own(w, r)
	if !ok {
		return
	}
	path := "/competitions/entry/" + r.PathValue("token")
	back := func(notice string) {
		http.Redirect(w, r, path+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
	}
	if !c.Signoff || !c.ApproveCoaches {
		back("This competition doesn't approve coaches; nothing was changed.")
		return
	}
	q, data, certType, problem := postedCertificate(r)
	if problem != "" {
		back("Nothing was sent: " + problem)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if err := competitions.CheckName("coach", name); err != nil {
		back("Nothing was sent: " + strings.Join(sentences(err), " "))
		return
	}
	if err := p.st.SetEntryCoach(r.Context(), r.PathValue("token"), name, q.Key, certType, data); err != nil {
		failed(w, r, err)
		return
	}
	back("Sent " + name + " to the organiser to approve.")
}

// entryCoachFor is, at a competition that approves coaches, the name of the
// approved coach an individual named, who signs off their entry, or why no
// one can yet.
func (p *competitionPages) entryCoachFor(r *http.Request, c store.Competition, e store.Entry) (name, why string, err error) {
	if !c.Signoff || !c.ApproveCoaches {
		return "", "", nil
	}
	named, err := p.st.EntryCoachOf(r.Context(), e.ID)
	if errors.Is(err, store.ErrNotFound) {
		return "", e.Entry.Gymnasts() + " hasn't named their coach yet: this competition approves coaches before they sign off.", nil
	} else if err != nil {
		return "", "", err
	}
	switch named.Status {
	case store.CoachApproved:
		return named.Name, "", nil
	case store.CoachWaiting:
		return "", "Waiting for the organiser to approve " + named.Name + " as " + e.Entry.Gymnasts() + "'s coach.", nil
	case store.CoachRefused:
		return "", strings.TrimSpace("The organiser hasn't approved " + named.Name + " as a coach. " + named.Note), nil
	}
	return "", strings.TrimSpace("The organiser withdrew " + named.Name + "'s approval as a coach. " + named.Note), nil
}
