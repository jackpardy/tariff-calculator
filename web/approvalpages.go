package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	return competitions.JoinList(asked)
}

// meets says, for each discipline asked about that the coach has a
// qualification in, whether it meets the level, e.g. "Trampoline Level 2 ✓";
// with none in any of them, each is a ✗.
func meets(c competitions.Competition, keys []string) []string {
	var quals []competitions.Qualification
	for _, k := range keys {
		if q, ok := competitions.QualificationByKey(k); ok {
			quals = append(quals, q)
		}
	}
	var out []string
	has := func(d string) bool {
		return slices.ContainsFunc(quals, func(q competitions.Qualification) bool { return q.Meets(d, 0) })
	}
	coaches := slices.ContainsFunc(coachDisciplines(c), has)
	for _, d := range coachDisciplines(c) {
		if coaches && !has(d) {
			continue // a discipline they don't coach
		}
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
	// Turning it on stops sign-offs by coaches not approved counting: it
	// asks first, saying how many entries that leaves waiting.
	waiting := 0
	if on && !c.ApproveCoaches && c.Signoff {
		var err error
		if waiting, err = p.waitingOnApproval(r.Context(), c); err != nil {
			failed(w, r, err)
			return
		}
		if waiting > 0 && !asked(r) {
			p.askBefore(w, r, c, menuSettings, views.AskFirstPage{
				What: "Turn on coach approval",
				Says: fmt.Sprintf("This makes %s wait for a coach's sign-off.", views.Count(waiting, "entry", "entries")),
				Then: "Sign-offs by coaches you haven't approved stop counting. They count again once you approve those coaches on the Coaches page, after clubs send them.",
				Do:   "Turn it on", Back: settingsPath(r.PathValue("token")) + "#signoff",
			})
			return
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
	if waiting > 0 {
		notice += fmt.Sprintf(" Waiting for a coach's sign-off until then: %s.", views.Count(waiting, "entry", "entries"))
	}
	backToSettings(w, r, notice)
}

// waitingOnApproval is how many of a competition's entries, signed off now,
// would wait for a coach's sign-off once it approves coaches: those (in, not
// withdrawn) signed off by a coach it hasn't approved.
func (p *competitionPages) waitingOnApproval(ctx context.Context, c store.Competition) (int, error) {
	unapproved, err := p.st.UnapprovedSignoffs(ctx, c.ID)
	if err != nil || len(unapproved) == 0 {
		return 0, err
	}
	entries, err := p.entriesOf(ctx, c.ID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range live(entries) {
		if e.SignedOff() && unapproved[e.ID] {
			n++
		}
	}
	return n, nil
}

// individualsClub is the heading of the coaches individuals name, who have
// no club.
const individualsClub = "Gymnasts on their own"

// approvals is the page of coaches clubs have sent, by club.
func (p *competitionPages) approvals(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	p.renderApprovals(w, r, c, nil)
}

// renderApprovals shows the Coaches page, asking before a change (ask). After
// a coach's approval is withdrawn (undo: their row's anchor), its notice
// offers Undo.
func (p *competitionPages) renderApprovals(w http.ResponseWriter, r *http.Request, c store.Competition, ask *views.Ask) {
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
	page := views.ApprovalsPage{Base: base, Competition: summary(c, p.now()), Menu: p.menuFor(r, c, menuCoaches), Notice: r.URL.Query().Get("notice"), Ask: ask, Asked: askedLevels(c.Competition)}
	var clubs []string
	for _, s := range sent {
		row := views.ApprovalRow{ID: s.CoachID, Anchor: "coach-" + s.CoachID, Name: s.Name, Person: personPath(base, store.CoachKey(s.CoachID)), Action: base + "/coaches/" + s.CoachID, Status: coachStatus(s.Status), Approved: s.Approved(),
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
		row.List = views.ListRow{Name: s.Name, Values: map[string][]string{"club": {clubValue(s.ClubName)}, "status": {coachStatusValue(s.Status)}}}
		clubs = append(clubs, s.ClubName)
		club := s.ClubName
		if i := len(page.Clubs) - 1; i >= 0 && page.Clubs[i].Club == club {
			page.Clubs[i].Coaches = append(page.Clubs[i].Coaches, row)
		} else {
			page.Clubs = append(page.Clubs, views.ApprovalClub{Club: club, Coaches: []views.ApprovalRow{row}})
		}
	}
	if len(named) > 0 {
		individuals := views.ApprovalClub{Club: individualsClub}
		clubs = append(clubs, individualsClub)
		for _, n := range named {
			row := views.ApprovalRow{ID: n.EntryID, Anchor: "named-" + n.EntryID, Name: n.Name + ", coach of " + n.Gymnast, Person: personPath(base, store.NamedCoachKey(n.Name)), Action: base + "/entry-coaches/" + n.EntryID,
				Status: coachStatus(n.Status), Approved: n.Approved(), Withdrawn: n.Status == store.CoachWithdrawn, Note: n.Note,
				Qualifications: []views.ApprovalQualification{{Name: qualificationName(n.Qualification), Certificate: base + "/entry-coaches/" + n.EntryID + "/certificate"}},
				Meets:          meets(c.Competition, []string{n.Qualification})}
			row.List = views.ListRow{Name: row.Name, Values: map[string][]string{"club": {individualsClub}, "status": {coachStatusValue(n.Status)}}}
			if n.Status == store.CoachWaiting {
				row.Status = "Waiting"
			}
			individuals.Coaches = append(individuals.Coaches, row)
		}
		page.Clubs = append(page.Clubs, individuals)
	}
	for _, club := range page.Clubs {
		page.Total += len(club.Coaches)
	}
	page.Tools = views.ListTools{ID: "coaches", Noun: "coaches", Total: page.Total, Filters: []views.ListFilter{clubFilter(clubs), coachStatusFilter()}}
	if undo := r.URL.Query().Get("undo"); undo != "" && page.Notice != "" {
		for _, club := range page.Clubs {
			for _, k := range club.Coaches {
				if k.Anchor == undo && k.Withdrawn {
					page.Undo = &views.Undo{Action: k.Action, Fields: []views.FormValue{{Name: "action", Value: "approve"}, {Name: "anyway", Value: "1"}}}
				}
			}
		}
	}
	render(w, r, views.CompetitionApprovals(page))
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
	// Approving a coach with nothing to check asks first.
	if status == store.CoachApproved && len(coach.Qualifications) == 0 && !asked(r) {
		noteAsked(w)
		p.renderApprovals(w, r, c, inPlace(r, views.Ask{
			Title:  "Approve " + coach.Name + "?",
			Text:   "Their club hasn't sent any qualifications, so there's nothing to check. Once approved, their sign-offs count.",
			Button: "Approve " + coach.Name, Back: adminPath(r.PathValue("token")) + "/coaches#coach-" + coach.CoachID,
		}))
		return
	}
	if err := p.st.DecideCoach(r.Context(), c.ID, coach.CoachID, status, note, afresh); err != nil {
		failed(w, r, err)
		return
	}
	if status == store.CoachApproved && len(coach.Qualifications) == 0 {
		notice = "Approved " + coach.Name + ", though their club hasn't sent any qualifications."
	}
	// Back to the coach, where it shows.
	seeOther(w, r, decidedPath(r, "coach-"+coach.CoachID, status), notice)
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

// decidedPath is the Coaches page at the coach decided on (anchor). A
// withdrawal is easily put back, so it's made without asking, and the page
// offers Undo.
func decidedPath(r *http.Request, anchor, status string) string {
	to := adminPath(r.PathValue("token")) + "/coaches"
	if status == store.CoachWithdrawn {
		to += "?undo=" + anchor
	}
	return to + "#" + anchor
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
	seeOther(w, r, decidedPath(r, "named-"+named.EntryID, status), notice)
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
	// The chip has its own mark, so the status loses its tick.
	v.Status, v.State = strings.TrimSuffix(v.Status, " ✓"), coachState(named.Status)
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
		seeOther(w, r, path, notice)
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
	err := p.st.SetEntryCoach(r.Context(), r.PathValue("token"), name, q.Key, certType, data)
	switch {
	case errors.Is(err, store.ErrCertificatesFull):
		back("Nothing was sent: this competition has no room for more coaches' certificates. Ask the organiser.")
		return
	case err != nil && certificateRefused(err) != "":
		back("Nothing was sent: " + certificateRefused(err))
		return
	case err != nil:
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
