package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Late changes (roadmap 2026-10-08): after the deadline a member, their
// comp sec or an individual asks to change an entry in the ways the
// organiser allows; the organiser sees whether it can be fitted in, and
// accepts it (applying it, moving the gymnast in the draft timetable and
// charging its fee) or rejects it with a reason.

func (p *competitionPages) registerLate(handle func(string, http.HandlerFunc)) {
	for _, path := range []string{
		"/clubs/member/{token}/competitions/{id}/late",
		"/clubs/admin/{token}/members/{member}/competitions/{id}/late",
		"/competitions/entry/{token}/late",
	} {
		handle("GET "+path, p.lateRequest)
		handle("POST "+path, p.lateRequest)
	}
	handle("GET /competitions/admin/{token}/late", p.lateAdmin)
	handle("POST /competitions/admin/{token}/late/settings", p.setLate)
	handle("POST /competitions/admin/{token}/late/{id}/accept", p.acceptLate)
	handle("POST /competitions/admin/{token}/late/{id}/reject", p.rejectLate)
	handle("GET /clubs/coach/{token}/late/{id}", p.coachLate)
	handle("POST /clubs/coach/{token}/late/{id}", p.coachLate)
	handle("GET /competitions/signoff/{token}/late", p.individualCoachLate)
	handle("POST /competitions/signoff/{token}/late", p.individualCoachLate)
}

// approvedCoach says whether a club's coach is approved at a competition.
func (p *competitionPages) approvedCoach(ctx context.Context, c store.Competition, coachID string) bool {
	sent, err := p.st.CompetitionCoaches(ctx, c.ID)
	if err != nil {
		return false
	}
	for _, s := range sent {
		if s.CoachID == coachID {
			return s.Status == store.CoachApproved
		}
	}
	return false
}

// openLateFor is the late change still to be decided for an entry, if any.
func (p *competitionPages) openLateFor(ctx context.Context, competitionID, entryID string) (store.LateRequest, bool, error) {
	r, ok, err := p.st.LatestLateRequest(ctx, competitionID, entryID)
	return r, ok && r.Open(), err
}

// coachLateRequests are the late changes a club's coach can sign off at a
// competition: for members they see, still to be decided, not signed off.
func (p *competitionPages) coachLateRequests(ctx context.Context, c store.Competition, sees map[string]bool) ([]store.LateRequest, error) {
	requests, err := p.st.LateRequests(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	entries, err := p.st.Entries(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	memberOf := map[string]string{}
	for _, e := range entries {
		memberOf[e.ID] = e.MemberID
	}
	var out []store.LateRequest
	for _, r := range requests {
		if r.Open() && !r.SignedOff() && sees[memberOf[r.EntryID]] {
			out = append(out, r)
		}
	}
	return out, nil
}

// coachLate shows a club's coach a late change to sign off, and signs it.
func (p *competitionPages) coachLate(w http.ResponseWriter, r *http.Request) {
	coach, club, ok := p.coach(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	entries, err := p.st.CoachEntries(ctx, coach)
	if err != nil {
		failed(w, r, err)
		return
	}
	sees := map[string]bool{}
	comps := map[string]bool{}
	for _, e := range entries {
		sees[e.MemberID], comps[e.CompetitionID] = true, true
	}
	back := coachPath(r.PathValue("token"))
	for id := range comps {
		c, err := p.st.Competition(ctx, id)
		if err != nil {
			continue
		}
		requests, err := p.coachLateRequests(ctx, c, sees)
		if err != nil {
			failed(w, r, err)
			return
		}
		for _, req := range requests {
			if req.ID != r.PathValue("id") {
				continue
			}
			var problems []string
			if r.Method == http.MethodPost {
				sentTo, err := p.st.CoachSentTo(ctx, coach.ID)
				if err != nil {
					failed(w, r, err)
					return
				}
				switch why := notApproved(c, sentTo); {
				case !coach.SignsOff:
					problems = []string{"Your club hasn't set you to sign off routines."}
				case why != "":
					problems = []string{why}
				default:
					if err := p.st.SignLateRequest(ctx, c.ID, req.ID, coach.Name, coach.ID, limitNote(strings.TrimSpace(r.FormValue("note")))); err != nil {
						failed(w, r, err)
						return
					}
					http.Redirect(w, r, back+"?notice="+url.QueryEscape("Signed off "+req.Entry.Gymnasts()+"'s late change; it's gone to the organiser."), http.StatusSeeOther)
					return
				}
			}
			p.renderLateSignoff(w, r, c, req, club, back, false, problems)
			return
		}
	}
	message(w, r, http.StatusNotFound, "Nothing to sign off", "That late change has been signed off or decided already, or isn't for one of your gymnasts.")
}

// individualCoachLate shows an individual's coach, through their sign-off
// link, the entry's late change to sign off, and signs it.
func (p *competitionPages) individualCoachLate(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.signoffEntry(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	req, open, err := p.openLateFor(ctx, c.ID, e.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	back := "/competitions/signoff/" + r.PathValue("token")
	if !open || req.SignedOff() {
		message(w, r, http.StatusNotFound, "Nothing to sign off", "There's no late change for this entry waiting for you.")
		return
	}
	var problems []string
	if r.Method == http.MethodPost {
		name := strings.TrimSpace(r.FormValue("coach"))
		if named, why, err := p.entryCoachFor(r, c, e); err != nil {
			failed(w, r, err)
			return
		} else if why != "" {
			problems = []string{why}
		} else if named != "" {
			name = named
		}
		if err := competitions.CheckName("coach", name); err != nil && len(problems) == 0 {
			problems = sentences(err)
		}
		if len(problems) == 0 {
			if err := p.st.SignLateRequest(ctx, c.ID, req.ID, name, "", limitNote(strings.TrimSpace(r.FormValue("note")))); err != nil {
				failed(w, r, err)
				return
			}
			http.Redirect(w, r, back+"?notice="+url.QueryEscape("Signed off; the late change has gone to the organiser."), http.StatusSeeOther)
			return
		}
	}
	p.renderLateSignoff(w, r, c, req, "Individual", back, true, problems)
}

// renderLateSignoff shows a late change's new card, to sign off.
func (p *competitionPages) renderLateSignoff(w http.ResponseWriter, r *http.Request, c store.Competition, req store.LateRequest, club, back string, askName bool, problems []string) {
	shown, err := card(c.Competition, req.Entry, club)
	if err != nil {
		failed(w, r, err)
		return
	}
	if len(problems) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	render(w, r, views.LateSignoff(views.LateSignoffPage{Competition: summary(c.Competition, p.now()), Back: back, Action: r.URL.Path,
		What: competitions.LateKindName(req.Kind) + " to " + req.Entry.Event(), Card: shown, AskName: askName, Problems: problems, Note: req.Note}))
}

// lateOpen says whether late changes can be asked for now: the competition
// is live, changes to entries have closed, and the organiser allows some.
func lateOpen(c store.Competition, now time.Time) bool {
	return c.Live(now) && !c.ChangesOpen(now) && c.Late.Any()
}

// lateTarget is whose entry a late change page is for.
type lateTarget struct {
	c       store.Competition
	entry   store.Entry // the competition's copy
	who     string      // who's asking, e.g. "UCD (comp sec)"
	gymnast string      // fixed for a club's member
	back    string
}

// lateTargetOf is the entry a late change link is for, or a page saying why
// there isn't one.
func (p *competitionPages) lateTargetOf(w http.ResponseWriter, r *http.Request) (lateTarget, bool) {
	ctx, token := r.Context(), r.PathValue("token")
	var t lateTarget
	var memberID string
	switch {
	case strings.HasPrefix(r.URL.Path, "/competitions/entry/"):
		e, c, ok := p.own(w, r)
		if !ok {
			return t, false
		}
		t = lateTarget{c: c, entry: e, who: e.Entry.Gymnast, gymnast: e.Entry.Gymnast, back: "/competitions/entry/" + token}
	case strings.HasPrefix(r.URL.Path, "/clubs/admin/"):
		club, ok := p.clubAdmin(w, r)
		if !ok {
			return t, false
		}
		m, c, ok := p.memberOfClub(w, r, club)
		if !ok {
			return t, false
		}
		t, memberID = lateTarget{c: c, who: club.Name + " (comp sec)", gymnast: m.Name, back: clubPath(token)}, m.ID
	default:
		m, ok := p.member(w, r)
		if !ok {
			return t, false
		}
		c, ok := p.memberCompetition(w, r, m)
		if !ok {
			return t, false
		}
		t, memberID = lateTarget{c: c, who: m.Name, gymnast: m.Name, back: memberPath(token)}, m.ID
	}
	if memberID != "" {
		entries, err := p.st.Entries(ctx, t.c.ID)
		if err != nil {
			failed(w, r, err)
			return t, false
		}
		found := false
		for _, e := range entries {
			if e.MemberID == memberID && e.Entry.Discipline == r.FormValue("discipline") {
				t.entry, found = e, true
			}
		}
		if !found {
			message(w, r, http.StatusNotFound, "Not entered", "This entry hasn't reached the competition, so there's nothing to change.")
			return t, false
		}
	}
	return t, true
}

// lateRequest shows and sends a request to change an entry after the
// deadline.
func (p *competitionPages) lateRequest(w http.ResponseWriter, r *http.Request) {
	t, ok := p.lateTargetOf(w, r)
	if !ok {
		return
	}
	c, ctx := t.c, r.Context()
	if !lateOpen(c, p.now()) {
		message(w, r, http.StatusConflict, "No late changes", "Late changes can be asked for only after changes to entries close, where the organiser allows them. Until then, change the entry as usual.")
		return
	}
	action := r.URL.Path
	if d := r.FormValue("discipline"); d != "" {
		action += "?discipline=" + url.QueryEscape(d)
	}
	start := t.entry.Entry
	latest, has, err := p.st.LatestLateRequest(ctx, c.ID, t.entry.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	if has && latest.Open() {
		start = latest.Entry
	}
	var problems []string
	if r.Method == http.MethodPost {
		changed, posted := postedEntry(r, c.Competition, t.gymnast)
		start, problems = changed, posted
		kind := competitions.LateKind(t.entry.Entry, changed)
		switch {
		case len(problems) > 0:
		case sameEntry(t.entry.Entry, changed):
			problems = []string{"That's the entry as it is: change it first."}
		case !c.Late.Allowed(kind):
			problems = []string{competitions.LateKindName(kind) + "s aren't allowed after the deadline."}
		}
		if len(problems) == 0 {
			coachFirst := c.Signoff && c.Late.SignoffFirst
			_, err := p.st.RequestLateChange(ctx, c.ID, t.entry.ID, changed, t.who, limitNote(strings.TrimSpace(r.FormValue("note"))), coachFirst)
			switch {
			case errors.Is(err, store.ErrNotAllowed):
				problems = []string{err.Error()}
			case err != nil:
				failed(w, r, err)
				return
			default:
				notice := "Your late change has gone to the organiser."
				if coachFirst {
					notice = "Your late change has gone to your coach to sign off; then it goes to the organiser."
				}
				http.Redirect(w, r, t.back+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
				return
			}
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	page := views.LateRequestPage{Competition: summary(c.Competition, p.now()), Back: t.back, Gymnast: t.gymnast,
		Current: t.entry.Entry.Event(), Problems: problems, Form: entryForm(c.Competition, start, action, "Ask the organiser")}
	page.Form.Problems, page.Form.Member, page.Form.Note, page.Form.NoteText = nil, true, true, r.FormValue("note")
	for _, kind := range competitions.LateKinds {
		if rule := c.Late.Rule(kind); rule.On {
			allowed := competitions.LateKindName(kind)
			if rule.Fee > 0 {
				allowed += " (" + c.Fees.Money(rule.Fee) + " if accepted)"
			}
			page.Allowed = append(page.Allowed, allowed)
		}
	}
	if has {
		page.Status = lateStatusText(c, latest)
	}
	page.CoachFirst = c.Signoff && c.Late.SignoffFirst
	render(w, r, views.LateRequest(page))
}

// sameEntry says whether two entries are the same.
func sameEntry(a, b competitions.Entry) bool {
	return fmt.Sprint(a) == fmt.Sprint(b)
}

// lateStatusText says where a late change request stands.
func lateStatusText(c store.Competition, r store.LateRequest) string {
	what := competitions.LateKindName(r.Kind) + " to " + r.Entry.Event()
	switch r.Status {
	case store.LateAccepted:
		s := what + ": accepted"
		if r.Fee > 0 {
			s += " (" + c.Fees.Money(r.Fee) + ", on the invoice)"
		}
		if r.Reason != "" {
			s += ". " + r.Reason
		}
		return s
	case store.LateRejected:
		return what + ": not accepted. " + r.Reason
	case store.LateCoach:
		return what + ": asked " + r.At.In(local).Format("2 Jan, 15:04") + ", waiting for a coach to sign it off."
	}
	s := what + ": asked " + r.At.In(local).Format("2 Jan, 15:04")
	if r.SignedOff() {
		s += ", signed off by " + r.SignedBy
	}
	return s + ", waiting for the organiser."
}

// lateStatusOf is where the latest late change for an entry stands, "" for
// none.
func (p *competitionPages) lateStatusOf(ctx context.Context, c store.Competition, entryID string) string {
	if entryID == "" {
		return ""
	}
	r, ok, err := p.st.LatestLateRequest(ctx, c.ID, entryID)
	if err != nil || !ok {
		return ""
	}
	return lateStatusText(c, r)
}

// lateAdmin is the organiser's page: which late changes are allowed, and
// each request, with whether it can be fitted in.
func (p *competitionPages) lateAdmin(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	requests, err := p.st.LateRequests(ctx, c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	entries, err := p.st.Entries(ctx, c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	byID := map[string]store.Entry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	base := adminPath(r.PathValue("token"))
	page := views.LateAdminPage{Base: base, Competition: summary(c.Competition, p.now()), Notice: r.URL.Query().Get("notice"),
		Signoff: c.Signoff, SignoffFirst: c.Late.SignoffFirst}
	for _, kind := range competitions.LateKinds {
		rule := c.Late.Rule(kind)
		page.Kinds = append(page.Kinds, views.LateKindField{Kind: kind, Name: competitions.LateKindName(kind), On: rule.On, Fee: amountField(rule.Fee)})
	}
	for _, req := range requests {
		e := byID[req.EntryID]
		v := views.LateRequestView{ID: req.ID, Who: req.Who, Kind: competitions.LateKindName(req.Kind), Gymnast: req.Entry.Gymnasts(),
			From: e.Entry.Event(), To: req.Entry.Event(), Note: req.Note, At: req.At.In(local).Format("2 Jan, 15:04"),
			Waiting: req.Status == store.LateWaiting, ForCoach: req.Status == store.LateCoach, Status: lateStatusText(c, req), Link: base + "/entries/" + req.EntryID}
		if fee := c.Late.Rule(req.Kind).Fee; fee > 0 {
			v.Fee = c.Fees.Money(fee)
		}
		if v.Waiting {
			v.Checks = p.lateChecks(ctx, c, req, e, entries)
		}
		page.Requests = append(page.Requests, v)
	}
	render(w, r, views.LateAdmin(page))
}

// lateChecks say whether a change can be fitted in: the new card's
// problems, the new level's limit, and where it would go in the draft
// timetable and what that does.
func (p *competitionPages) lateChecks(ctx context.Context, c store.Competition, req store.LateRequest, e store.Entry, entries []store.Entry) []string {
	var out []string
	shown, err := card(c.Competition, req.Entry, clubOf(e))
	switch {
	case err != nil:
		out = append(out, "The card can't be checked: "+err.Error())
	case len(shown.Problems) > 0:
		out = append(out, fmt.Sprintf("The new card has %s: %s", problemsWord(len(shown.Problems)), strings.Join(shown.Problems, "; ")))
	default:
		out = append(out, "The new card meets its requirements.")
	}
	if c.Signoff {
		switch {
		case req.SignedOff() && c.ApproveCoaches && req.SignedCoach != "" && !p.approvedCoach(ctx, c, req.SignedCoach):
			out = append(out, "Signed off by "+req.SignedBy+", who isn't an approved coach: accepted, it would need signing off again.")
		case req.SignedOff():
			out = append(out, "Signed off by "+req.SignedBy+".")
		default:
			out = append(out, "Not signed off by a coach: accepted, it would need signing off.")
		}
	}
	if req.Kind != competitions.LateLevel {
		return out
	}
	event := req.Entry.Event()
	if limit := c.Limits[event]; limit > 0 {
		in, waiting := 0, 0
		for _, o := range entries {
			if o.ID != e.ID && o.Entry.Event() == event && !o.Withdrawn {
				if o.Waiting > 0 {
					waiting++
				} else {
					in++
				}
			}
		}
		if in >= limit {
			out = append(out, fmt.Sprintf("%s is full (%d of %d): accepted, they'd be %s on its waiting list, unless you let them in.", event, in, limit, waitingWord(waiting+1)))
		} else {
			out = append(out, fmt.Sprintf("%s has room: %d of %d places taken.", event, in, limit))
		}
	}
	if c.Timetable == nil || !c.Timetable.Planned {
		return out
	}
	people, names := peopleOf(live(entries))
	_, place := c.Timetable.PlaceChange(e.ID, event, req.Entry.Category, people, names)
	if place.Flight < 0 {
		return append(out, "The draft timetable has no flight of "+event+" yet: accepted, they'd be out of the timetable until you place them.")
	}
	line := fmt.Sprintf("In the draft timetable: into %s (%s, warm-up %s)", place.Name, place.Area, place.Start)
	if place.Later > 0 {
		line += fmt.Sprintf(", which finishes %d min later", place.Later)
	}
	if place.Moves > 0 {
		line += fmt.Sprintf("; %d later flights move", place.Moves)
	}
	out = append(out, line+".")
	for _, problem := range place.Problems {
		out = append(out, "It would cause: "+problem+".")
	}
	return out
}

// setLate saves which late changes are allowed (late-<kind>=1) and their
// fees (fee-<kind>).
func (p *competitionPages) setLate(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	late := competitions.LateChanges{Rules: map[string]competitions.LateRule{}, SignoffFirst: r.FormValue("signoffFirst") == "1"}
	back := adminPath(r.PathValue("token")) + "/late"
	for _, kind := range competitions.LateKinds {
		fee, err := competitions.ParseMoney(r.FormValue("fee-" + kind))
		if err != nil {
			http.Redirect(w, r, back+"?notice="+url.QueryEscape(competitions.LateKindName(kind)+" fee: "+err.Error()+". Nothing was changed."), http.StatusSeeOther)
			return
		}
		if on := r.FormValue("late-"+kind) == "1"; on || fee > 0 {
			late.Rules[kind] = competitions.LateRule{On: on, Fee: fee}
		}
	}
	if err := p.st.SetLate(r.Context(), c.ID, late); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, back+"?notice="+url.QueryEscape("Saved."), http.StatusSeeOther)
}

// acceptLate applies a request, with an optional note: the entry changes,
// a level change moves the gymnast in the draft timetable (or out of it,
// if they'd wait), and the fee is charged.
func (p *competitionPages) acceptLate(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	back := adminPath(r.PathValue("token")) + "/late"
	requests, err := p.st.LateRequests(ctx, c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	var req *store.LateRequest
	for i := range requests {
		if requests[i].ID == r.PathValue("id") && requests[i].Status == store.LateWaiting {
			req = &requests[i]
		}
	}
	if req == nil {
		http.Redirect(w, r, back+"?notice="+url.QueryEscape("That request has been decided already."), http.StatusSeeOther)
		return
	}
	p.notify.changing(ctx, c)
	accepted, err := p.st.AcceptLateChange(ctx, c.ID, req.ID, limitNote(strings.TrimSpace(r.FormValue("note"))), linkOf(r).Name, c.Late.Rule(req.Kind).Fee)
	if err != nil {
		failed(w, r, err)
		return
	}
	notice := "Accepted: " + accepted.Entry.Gymnasts() + " is in " + accepted.Entry.Event() + "."
	if accepted.Kind == competitions.LateLevel && c.Timetable != nil && c.Timetable.Planned {
		entries, err := p.st.Entries(ctx, c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		draft := *c.Timetable
		waiting := false
		for _, e := range entries {
			if e.ID == accepted.EntryID && e.Waiting > 0 {
				waiting = true
				notice = fmt.Sprintf("Accepted: %s is %s on %s's waiting list.", accepted.Entry.Gymnasts(), waitingWord(e.Waiting), accepted.Entry.Event())
			}
		}
		if waiting {
			draft.RemoveEntry(accepted.EntryID)
			draft.Retime()
		} else {
			people, names := peopleOf(live(entries))
			var place competitions.ChangePlace
			draft, place = draft.PlaceChange(accepted.EntryID, accepted.Entry.Event(), accepted.Entry.Category, people, names)
			if place.Flight >= 0 {
				notice += " The draft timetable has them in " + place.Name + ": publish it when you're ready."
			} else {
				notice += " The draft timetable has no flight of it yet: place them by hand."
			}
		}
		if err := p.st.SetTimetable(ctx, c.ID, &draft); err != nil {
			failed(w, r, err)
			return
		}
	}
	http.Redirect(w, r, back+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

// rejectLate turns a request down, with a reason for whoever asked.
func (p *competitionPages) rejectLate(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	back := adminPath(r.PathValue("token")) + "/late"
	reason := limitNote(strings.TrimSpace(r.FormValue("reason")))
	if reason == "" {
		http.Redirect(w, r, back+"?notice="+url.QueryEscape("Give a reason: whoever asked sees it."), http.StatusSeeOther)
		return
	}
	p.notify.changing(r.Context(), c)
	if err := p.st.RejectLateChange(r.Context(), c.ID, r.PathValue("id"), reason, linkOf(r).Name); err != nil && !errors.Is(err, store.ErrNotFound) {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, back+"?notice="+url.QueryEscape("Not accepted; they'll see your reason."), http.StatusSeeOther)
}
