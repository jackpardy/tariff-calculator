package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Late requests (late changes, roadmap 2026-10-08; late requests,
// 2026-11-11): after the deadline a member, their comp sec or an individual
// asks to change an entry in the ways the organiser allows, or to enter
// late. The request reaches the organiser at once, with whether a coach has
// signed it off; the organiser sees whether it can be fitted in, replies to
// it if they like (a thread, which the asker answers), and accepts it
// (applying it, moving the gymnast in the draft timetable and charging its
// fee) or turns it down with a reason.

// lateRoots are the pages a late request is made from: a member's, a comp
// sec's for a member, and an individual's.
var lateRoots = []string{
	"/clubs/member/{token}/competitions/{id}/late",
	"/clubs/admin/{token}/members/{member}/competitions/{id}/late",
	"/competitions/entry/{token}/late",
}

func (p *competitionPages) registerLate(handle func(string, http.HandlerFunc)) {
	for _, path := range lateRoots {
		handle("GET "+path, p.lateRequest)
		handle("POST "+path, p.lateRequest)
		handle("POST "+path+"/reply", p.answerLate)
	}
	handle("GET /competitions/admin/{token}/late", p.lateAdmin)
	handle("POST /competitions/admin/{token}/late/settings", p.setLate)
	handle("POST /competitions/admin/{token}/late/{id}/accept", p.acceptLate)
	handle("POST /competitions/admin/{token}/late/{id}/reject", p.rejectLate)
	handle("POST /competitions/admin/{token}/late/{id}/reply", p.replyLate)
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

// signedOffForLate says whether a request has a sign-off that counts: a
// coach signed it off, and isn't a club's coach the competition hasn't
// approved, where it approves them.
func (p *competitionPages) signedOffForLate(ctx context.Context, c store.Competition, req store.LateRequest) bool {
	if !req.SignedOff() {
		return false
	}
	return !c.ApproveCoaches || req.SignedCoach == "" || p.approvedCoach(ctx, c, req.SignedCoach)
}

// lateBlocked is why the organiser can't accept a request yet: a coach must
// sign it off first; "" if they can.
func (p *competitionPages) lateBlocked(ctx context.Context, c store.Competition, req store.LateRequest) string {
	if !c.Signoff || !c.Late.SignoffFirst || p.signedOffForLate(ctx, c, req) {
		return ""
	}
	return "It can't be accepted until a coach signs it off: you can reply, or turn it down."
}

// openLateFor is the late request still to be decided for an entry, if any.
func (p *competitionPages) openLateFor(ctx context.Context, competitionID, entryID string) (store.LateRequest, bool, error) {
	r, ok, err := p.st.LatestLateRequest(ctx, competitionID, entryID)
	return r, ok && r.Open(), err
}

// lateEntries are a competition's entries and those asked for late, which
// late requests are about.
func (p *competitionPages) lateEntries(ctx context.Context, competitionID string) ([]store.Entry, error) {
	entries, err := p.entriesOf(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	asked, err := p.st.AskedEntries(ctx, competitionID)
	return append(slices.Clone(entries), asked...), err
}

// coachLateRequests are the late requests a club's coach can sign off at a
// competition: for members they see, still to be decided, not signed off.
func (p *competitionPages) coachLateRequests(ctx context.Context, c store.Competition, sees map[string]bool) ([]store.LateRequest, error) {
	requests, err := p.st.LateRequests(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	entries, err := p.lateEntries(ctx, c.ID)
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

// lateWhat is what a request asks for, e.g. "Level change to BUCS L4",
// "Late entry in BUCS L3".
func lateWhat(r store.LateRequest) string {
	if r.Kind == competitions.LateNew {
		return "Late entry in " + r.Entry.Event()
	}
	return competitions.LateKindName(r.Kind) + " to " + r.Entry.Event()
}

// lateSigned is the notice a coach sees, having signed a request off.
func lateSigned(c store.Competition) string {
	if c.Late.SignoffFirst {
		return "the organiser can accept it now."
	}
	return "the organiser sees you have."
}

// coachLate shows a club's coach a late request to sign off, and signs it.
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
					seeOther(w, r, back, "Signed off "+req.Entry.Gymnasts()+"'s late request: "+lateSigned(c))
					return
				}
			}
			p.renderLateSignoff(w, r, c, req, club, back, false, problems)
			return
		}
	}
	message(w, r, http.StatusNotFound, "Nothing to sign off", "That late request has been signed off or decided already, or isn't for one of your gymnasts.")
}

// individualCoachLate shows an individual's coach, through their sign-off
// link, the entry's late request to sign off, and signs it.
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
		message(w, r, http.StatusNotFound, "Nothing to sign off", "There's no late request for this entry waiting for you.")
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
			seeOther(w, r, back, "Signed off the late request: "+lateSigned(c))
			return
		}
	}
	p.renderLateSignoff(w, r, c, req, onTheirOwn, back, true, problems)
}

// renderLateSignoff shows a late request's card, to sign off.
func (p *competitionPages) renderLateSignoff(w http.ResponseWriter, r *http.Request, c store.Competition, req store.LateRequest, club, back string, askName bool, problems []string) {
	shown, err := card(c.Competition, req.Entry, club)
	if err != nil {
		failed(w, r, err)
		return
	}
	if len(problems) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	backTo := "Entries" // a coach's page
	if strings.HasPrefix(back, "/competitions/signoff/") {
		backTo = "The entry to sign off"
	}
	render(w, r, views.LateSignoff(views.LateSignoffPage{Competition: summary(c, p.now()), Back: back, BackTo: backTo, Action: r.URL.Path,
		What: lateWhat(req), Card: shown, AskName: askName, Problems: problems, Note: req.Note}))
}

// lateOpen says whether late changes can be asked for now: the competition
// is live, changes to entries have closed, and the organiser allows some.
func lateOpen(c store.Competition, now time.Time) bool {
	return c.Live(now) && !c.ChangesOpen(now) && c.Late.Any()
}

// lateNewOpen says whether entering late can be asked for now: the
// competition is live, new entries have closed, and the organiser allows
// it (as they do unless they turn it off).
func lateNewOpen(c store.Competition, now time.Time) bool {
	return c.Live(now) && !c.Open(now) && c.Late.Allowed(competitions.LateNew)
}

// lateTarget is whose entry a late request page is for.
type lateTarget struct {
	c       store.Competition
	entry   store.Entry // the competition's copy (Asked, for a late entry); ID "" for none yet
	saved   *store.MemberEntry
	member  string // a club member's id; "" for an individual
	who     string // who's asking, e.g. "UCD (competition secretary)"
	gymnast string // fixed for a club's member
	from    store.Sender
	back    string
	backTo  string // the page back is, for its link
	// A member asking themselves, and what their club lets them send
	// (2026-11-11): what it doesn't waits for the comp sec to send.
	byMember bool
	club     store.Club
}

// heldFor says whether a request of this kind, asked now, waits for the
// club to send it: a member's, where the club doesn't let members ask for
// it themselves.
func (t lateTarget) heldFor(kind string) bool {
	if !t.byMember {
		return false
	}
	ms := memberSends(t.club, t.c)
	if kind == competitions.LateNew {
		return !ms.LateEntries
	}
	return !ms.LateChanges
}

// clubPoster is who of a club is acting, as other people see them: the
// person a role's link is for, or the club's competition secretary for the
// club admin's link (who is the link's holder, club the club's name).
func clubPoster(club, who string) string {
	if who == "" || who == club || who == "Club admin" {
		return club + " competition secretary"
	}
	return who
}

// late says whether the target is asking to enter late (or asked): the
// competition has no copy, or only the one asked for.
func (t lateTarget) late() bool { return t.entry.ID == "" || t.entry.Removal == store.Asked }

// lateTargetOf is the entry a late request link is for, or a page saying
// why there isn't one.
func (p *competitionPages) lateTargetOf(w http.ResponseWriter, r *http.Request) (lateTarget, bool) {
	a, ok := p.whose(w, r)
	if !ok {
		return lateTarget{}, false
	}
	ctx := r.Context()
	t := lateTarget{c: a.c, entry: a.entry, who: a.name, gymnast: a.name, back: a.back, backTo: a.backTo()}
	switch a.kind {
	case store.NotifyIndividual:
		t.from = individualTeller(a.entry).from
		return t, true
	case store.NotifyClub:
		m, err := p.st.Member(ctx, a.club, r.PathValue("member"))
		if err != nil {
			failed(w, r, err)
			return t, false
		}
		poster := clubPoster(a.name, clubLinkOf(r).Who())
		t.who, t.gymnast, t.member = poster, m.Name, m.ID
		t.from = clubTeller(a.club, a.name, poster).from
	default:
		club, err := p.st.ClubByID(ctx, a.club)
		if err != nil {
			failed(w, r, err)
			return t, false
		}
		t.member, t.byMember, t.club = a.owner, true, club
		t.from = memberTeller(store.Member{ID: a.owner, Name: a.name}, club.Name).from
	}
	entries, err := p.lateEntries(ctx, t.c.ID)
	if err != nil {
		failed(w, r, err)
		return t, false
	}
	t.entry = store.Entry{}
	for _, e := range entries {
		if e.MemberID == t.member && e.Entry.Discipline == r.FormValue("discipline") {
			t.entry = e
		}
	}
	if t.saved, err = p.memberEntry(r, t.member, t.c.ID); err != nil {
		failed(w, r, err)
		return t, false
	}
	if t.entry.ID == "" && !lateNewOpen(t.c, p.now()) {
		message(w, r, http.StatusNotFound, "Not entered", "This entry hasn't reached the competition, so there's nothing to change.")
		return t, false
	}
	return t, true
}

// memberCopy is the competition's copy of the member's entry (the path's
// member, the posted discipline), asked for late or not; not ok for none.
func (p *competitionPages) memberCopy(r *http.Request) (store.Entry, bool) {
	entries, err := p.lateEntries(r.Context(), r.PathValue("id"))
	if err != nil {
		return store.Entry{}, false
	}
	for _, e := range entries {
		if e.MemberID == r.PathValue("member") && e.Entry.Discipline == r.FormValue("discipline") {
			return e, true
		}
	}
	return store.Entry{}, false
}

// lateRequest shows and sends a request to change an entry after the
// deadline, or to enter late; and shows where it stands, with its thread.
func (p *competitionPages) lateRequest(w http.ResponseWriter, r *http.Request) {
	t, ok := p.lateTargetOf(w, r)
	if !ok {
		return
	}
	c, ctx := t.c, r.Context()
	var latest store.LateRequest
	has := false
	if t.entry.ID != "" {
		var err error
		if latest, has, err = p.st.LatestLateRequest(ctx, c.ID, t.entry.ID); err != nil {
			failed(w, r, err)
			return
		}
	}
	// What can be asked: to enter late, while that's open and the entry
	// isn't in (asked again while the request is still to be decided, or
	// after a turn-down, as a new request); otherwise a change, while those
	// are open.
	canAsk := lateOpen(c, p.now())
	if t.late() {
		canAsk = lateNewOpen(c, p.now()) && (t.entry.ID == "" || has)
	}
	if !canAsk && !has {
		noteRefused(w)
		message(w, r, http.StatusConflict, "No late changes", "Late changes can be asked for only after changes to entries close, where the organiser allows them. Until then, change the entry as usual.")
		return
	}
	action := r.URL.Path
	if d := r.FormValue("discipline"); d != "" {
		action += "?discipline=" + url.QueryEscape(d)
	}
	start := t.entry.Entry
	switch {
	case has && latest.Open():
		start = latest.Entry
	case t.entry.ID == "" && t.saved != nil:
		start = t.saved.Entry
	case t.entry.ID == "":
		start = competitions.Entry{Gymnast: t.gymnast, Discipline: chosenDiscipline(c.Competition, r.FormValue("discipline"))}
	}
	var problems []string
	if r.Method == http.MethodPost {
		if !canAsk {
			refuse(w, r, t.back, "That can't be asked for now; nothing was sent.")
			return
		}
		note := limitNote(strings.TrimSpace(r.FormValue("note")))
		if t.late() {
			p.askLate(w, r, t, note)
			return
		}
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
			held := t.heldFor(kind)
			_, err := p.st.RequestLateChange(ctx, c.ID, t.entry.ID, changed, t.who, note, t.from, held)
			switch {
			case errors.Is(err, store.ErrNotAllowed):
				problems = []string{err.Error()}
			case err != nil:
				failed(w, r, err)
				return
			case held:
				seeOther(w, r, t.back, "Late change saved. "+clubSendsLate)
				return
			default:
				p.memberSent(ctx, t, "Asked for a late change: "+changed.Gymnasts()+" · "+changed.Event())
				seeOther(w, r, t.back, "Your late change has gone to the organiser."+lateSignoffNote(c))
				return
			}
		}
		noteRefused(w)
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	page := views.LateRequestPage{Competition: summary(c, p.now()), Back: t.back, BackTo: t.backTo, Gymnast: t.gymnast,
		Current: t.entry.Entry.Event(), Problems: problems, New: t.late(), Notice: r.URL.Query().Get("notice")}
	if canAsk {
		submit := "Ask the organiser"
		if t.late() {
			submit = "Ask to enter late"
		}
		page.Form = entryForm(c.Competition, start, action, submit)
		page.Form.Problems, page.Form.Member, page.Form.Note, page.Form.NoteText = nil, true, true, r.FormValue("note")
		page.Ask = true
	}
	for _, kind := range competitions.LateKinds {
		if rule := c.Late.Rule(kind); rule.On && (kind == competitions.LateNew) == t.late() {
			allowed := competitions.LateKindName(kind)
			if rule.Fee > 0 {
				allowed += " (" + c.Fees.Money(rule.Fee) + " if accepted)"
			}
			page.Allowed = append(page.Allowed, allowed)
		}
	}
	if has {
		page.Status = p.lateStatus(ctx, c, latest)
		if posts, ok := p.lateThreadOf(ctx, c, latest, t.from); ok {
			page.Thread = posts
			if latest.Open() && latest.Sent() {
				page.Answer = strings.TrimSuffix(r.URL.Path, "/") + "/reply"
				page.Discipline = r.FormValue("discipline")
			}
		}
	}
	page.Signoff = c.Signoff
	page.SignoffFirst = c.Signoff && c.Late.SignoffFirst
	render(w, r, views.LateRequest(page))
}

// clubSendsLate is what a member is told of a late request their club
// sends for them, in the words a saved entry has.
const clubSendsLate = "Your club's competition secretary sends it to the organiser."

// memberSent records in the club's history what a member sent the
// competition themselves (what), where their club lets them.
func (p *competitionPages) memberSent(ctx context.Context, t lateTarget, what string) {
	if !t.byMember {
		return
	}
	if err := p.st.RecordClub(ctx, t.club.ID, t.gymnast, what+" (sent themselves)"); err != nil {
		log.Printf("Club history: %s: %v", t.club.ID, err)
	}
}

// askLate asks to enter late: from the member's saved entry (use=saved,
// one tap from their page), or the entry posted. An individual's entry is
// asked for when they enter; asked again here, after a turn-down or while
// it's still to be decided.
func (p *competitionPages) askLate(w http.ResponseWriter, r *http.Request, t lateTarget, note string) {
	c := t.c
	if t.member == "" {
		e, problems := postedEntry(r, c.Competition, t.gymnast)
		if len(problems) > 0 {
			noteRefused(w)
			w.WriteHeader(http.StatusUnprocessableEntity)
			page := views.LateRequestPage{Competition: summary(c, p.now()), Back: t.back, BackTo: t.backTo, Gymnast: t.gymnast, New: true, Ask: true,
				Problems: problems, Form: entryForm(c.Competition, e, r.URL.Path, "Ask to enter late"),
				Signoff: c.Signoff, SignoffFirst: c.Signoff && c.Late.SignoffFirst}
			page.Form.Member, page.Form.Note, page.Form.NoteText = true, true, note
			render(w, r, views.LateRequest(page))
			return
		}
		switch _, err := p.st.AskAgainOnOwn(r.Context(), c.ID, t.entry.ID, e, note, t.from); {
		case errors.Is(err, store.ErrNotAllowed), errors.Is(err, store.ErrEntered):
			refuse(w, r, t.back, "That can't be asked for now; nothing was sent.")
			return
		case err != nil:
			failed(w, r, err)
			return
		}
		seeOther(w, r, t.back, "Your request to enter late has gone to the organiser."+lateSignoffNote(c))
		return
	}
	var posted *competitions.Entry
	discipline := r.FormValue("discipline")
	if r.FormValue("use") != "saved" {
		e, problems := postedEntry(r, c.Competition, t.gymnast)
		if len(problems) > 0 {
			noteRefused(w)
			w.WriteHeader(http.StatusUnprocessableEntity)
			page := views.LateRequestPage{Competition: summary(c, p.now()), Back: t.back, BackTo: t.backTo, Gymnast: t.gymnast, New: true, Ask: true,
				Problems: problems, Form: entryForm(c.Competition, e, r.URL.Path+"?discipline="+url.QueryEscape(e.Discipline), "Ask to enter late"),
				Signoff: c.Signoff, SignoffFirst: c.Signoff && c.Late.SignoffFirst}
			page.Form.Member, page.Form.Note, page.Form.NoteText = true, true, note
			render(w, r, views.LateRequest(page))
			return
		}
		posted, discipline = &e, e.Discipline
	} else if t.saved == nil {
		refuse(w, r, t.back, "There's no saved entry to ask with; nothing was sent.")
		return
	}
	held := t.heldFor(competitions.LateNew)
	asked, err := p.st.AskToEnterLate(r.Context(), c.ID, t.member, discipline, posted, t.who, note, t.from, held)
	switch {
	case errors.Is(err, store.ErrNotAllowed):
		refuse(w, r, t.back, "The organiser isn't taking late entries; nothing was sent.")
		return
	case errors.Is(err, store.ErrEntered):
		refuse(w, r, t.back, "That entry is in the competition already; nothing was sent.")
		return
	case err != nil:
		failed(w, r, err)
		return
	case held:
		seeOther(w, r, t.back, "Request to enter late saved. "+clubSendsLate)
		return
	}
	p.memberSent(r.Context(), t, "Asked to enter "+asked.Entry.Gymnasts()+" · "+asked.Entry.Event()+" late")
	seeOther(w, r, t.back, "Your request to enter late has gone to the organiser."+lateSignoffNote(c))
}

// lateSignoffNote is what a notice adds, having asked: a coach signs it off
// too, where the competition needs it.
func lateSignoffNote(c store.Competition) string {
	switch {
	case !c.Signoff:
		return ""
	case c.Late.SignoffFirst:
		return " A coach needs to sign it off before the organiser can accept it."
	}
	return " A coach can sign it off too: the organiser sees whether they have."
}

// answerLate adds the asker's words (text) to their latest request's
// thread, while it's still to be decided.
func (p *competitionPages) answerLate(w http.ResponseWriter, r *http.Request) {
	t, ok := p.lateTargetOf(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	back := strings.TrimSuffix(r.URL.Path, "/reply")
	if d := r.FormValue("discipline"); d != "" {
		back += "?discipline=" + url.QueryEscape(d)
	}
	text := limitMessage(r.FormValue("text"))
	if text == "" {
		refuse(w, r, back, "Write what to say first; nothing was sent.")
		return
	}
	var req store.LateRequest
	open := false
	if t.entry.ID != "" {
		var err error
		if req, open, err = p.openLateFor(ctx, t.c.ID, t.entry.ID); err != nil {
			failed(w, r, err)
			return
		}
	}
	if !open {
		refuse(w, r, back, "That request has been decided; nothing was sent.")
		return
	}
	if !req.Sent() {
		refuse(w, r, back, "That request hasn't gone to the organiser yet; nothing was sent.")
		return
	}
	thread, err := p.st.LateThread(ctx, t.c.ID, req.ID, t.from)
	if err != nil {
		failed(w, r, err)
		return
	}
	k, err := p.st.Concern(ctx, t.c.ID, thread)
	if err != nil {
		failed(w, r, err)
		return
	}
	// Whose thread it is adds as it; anyone else (the comp sec on a
	// member's, the member on their comp sec's) by name.
	who := t.who
	if k.Sender == t.from.ID {
		who = ""
		if t.from.Role == store.RoleClub {
			who = t.from.Name
		}
	}
	_, err = p.st.AddToMessage(ctx, t.c.ID, thread, k.Sender, who, text)
	switch {
	case errors.Is(err, store.ErrLimit):
		refuse(w, r, back, "That request's thread is as long as it can be; speak to the organiser.")
		return
	case err != nil:
		failed(w, r, err)
		return
	}
	seeOther(w, r, back, "Sent, for the organiser to see.")
}

// lateThreadOf is a request's thread as the asker sees it; not ok for
// none yet.
func (p *competitionPages) lateThreadOf(ctx context.Context, c store.Competition, req store.LateRequest, viewer store.Sender) ([]views.SentPost, bool) {
	if req.Thread == "" {
		return nil, req.Open() // one from before threads gets one when first added to
	}
	k, err := p.st.Concern(ctx, c.ID, req.Thread)
	if err != nil {
		return nil, false
	}
	var out []views.SentPost
	for _, post := range k.Thread {
		v := views.SentPost{Text: post.Text, At: post.At.In(local).Format("Mon 2 Jan, 15:04"), Organiser: !post.BySender}
		// The comp sec and the member both see a member's request: say who
		// added what, but the asker's own as theirs.
		if post.BySender && (post.Who != k.Who || k.Sender != viewer.ID) {
			v.By = post.Who
		}
		out = append(out, v)
	}
	return out, true
}

// sameEntry says whether two entries are the same.
func sameEntry(a, b competitions.Entry) bool {
	return fmt.Sprint(a) == fmt.Sprint(b)
}

// lateStatusText says where a late request stands. signed says whether a
// coach's sign-off counts (signedOffForLate).
func lateStatusText(c store.Competition, r store.LateRequest, signed bool) string {
	what := lateWhat(r)
	switch r.Status {
	case store.LateAccepted:
		s := what + ": accepted"
		if r.Fee > 0 {
			s += " (" + c.Fees.Money(r.Fee) + ", on the invoice)"
		}
		if r.Reason != "" {
			s += ". The organiser's note: " + r.Reason
		}
		return s
	case store.LateRejected:
		return what + ": turned down. " + r.Reason
	}
	if !r.Sent() {
		return what + ", saved " + r.At.In(local).Format("2 Jan, 15:04") + ": not sent yet. " + clubSendsLate
	}
	s := what + ", asked " + r.At.In(local).Format("2 Jan, 15:04")
	switch {
	case !c.Signoff:
	case signed:
		s += ", signed off by " + r.SignedBy
	case r.SignedOff():
		s += ", signed off by " + r.SignedBy + ", who isn't an approved coach"
	}
	s += ": waiting for the organiser"
	if c.Signoff && c.Late.SignoffFirst && !signed {
		s += ", who can't accept it until a coach signs it off"
	}
	return s + "."
}

// lateState is a late request's state, as a chip's words and colour:
// accepted, turned down, or waiting (for sign-off, or signed off, where the
// competition needs it; asked otherwise).
func lateState(c store.Competition, r store.LateRequest, signed bool) (string, views.State) {
	switch {
	case r.Status == store.LateAccepted:
		return "Accepted", views.Done
	case r.Status == store.LateRejected:
		return "Turned down", views.Stopped
	case !r.Sent():
		return "Not sent yet", views.NotYet
	case !c.Signoff:
		return "Asked", views.InProgress
	case signed:
		return "Signed off", views.InProgress
	}
	return "Waiting for sign-off", views.InProgress
}

// lateStatus is where a late request stands, for the asker's pages.
func (p *competitionPages) lateStatus(ctx context.Context, c store.Competition, r store.LateRequest) views.LateStatus {
	signed := p.signedOffForLate(ctx, c, r)
	label, state := lateState(c, r, signed)
	return views.LateStatus{Text: lateStatusText(c, r, signed), Label: label, State: state}
}

// lateStatusOf is where the latest late request for an entry stands, none
// for no entry or request.
func (p *competitionPages) lateStatusOf(ctx context.Context, c store.Competition, entryID string) views.LateStatus {
	if entryID == "" {
		return views.LateStatus{}
	}
	r, ok, err := p.st.LatestLateRequest(ctx, c.ID, entryID)
	if err != nil || !ok {
		return views.LateStatus{}
	}
	return p.lateStatus(ctx, c, r)
}

// lateAdmin is the organiser's page: which late requests are allowed, and
// each request, with its sign-off, whether it can be fitted in, and its
// thread.
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
	entries, err := p.entriesOf(ctx, c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	all, err := p.lateEntries(ctx, c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	byID := entriesByID(all)
	f, err := p.liveRota(ctx, c)
	if err != nil {
		failed(w, r, err)
		return
	}
	base := adminPath(r.PathValue("token"))
	page := views.LateAdminPage{Base: base, Competition: summary(c, p.now()), Menu: p.menuFor(r, c, menuLate), Notice: r.URL.Query().Get("notice"),
		Signoff: c.Signoff, SignoffFirst: c.Late.SignoffFirst}
	var allowed []string
	for _, kind := range competitions.LateKinds {
		rule := c.Late.Rule(kind)
		page.Kinds = append(page.Kinds, views.LateKindField{Kind: kind, Name: competitions.LateKindName(kind), On: rule.On, Fee: amountField(rule.Fee)})
		if rule.On {
			fee := "no fee"
			if rule.Fee > 0 {
				fee = c.Fees.Money(rule.Fee)
			}
			allowed = append(allowed, fmt.Sprintf("%s (%s)", lateKindPlural(kind), fee))
		}
	}
	page.Now = lateNow(allowed, c.Signoff && c.Late.SignoffFirst)
	turnedDown := map[string][]store.LateRequest{} // by entry
	for _, req := range requests {
		if req.Status == store.LateRejected {
			turnedDown[req.EntryID] = append(turnedDown[req.EntryID], req)
		}
	}
	for _, req := range requests {
		if !req.Sent() {
			continue // a member's, for their club to send
		}
		e := byID[req.EntryID]
		signed := p.signedOffForLate(ctx, c, req)
		v := views.LateRequestView{ID: req.ID, Who: req.Who, Kind: competitions.LateKindName(req.Kind), Gymnast: req.Entry.Gymnasts(),
			From: e.Entry.Event(), To: req.Entry.Event(), Note: req.Note, At: req.At.In(local).Format("2 Jan, 15:04"),
			Waiting: req.Open(), Status: lateStatusText(c, req, signed), Link: base + "/entries/" + req.EntryID}
		if req.Kind == competitions.LateNew {
			v.From = ""
		}
		// Asked again after a turn-down: what was turned down before, and why.
		for _, before := range turnedDown[req.EntryID] {
			if before.ID != req.ID && before.DecidedAt.Before(req.At.Add(time.Second)) && req.Open() {
				v.Before = append(v.Before, fmt.Sprintf("Asked before for %s, turned down %s: %s", before.Entry.Event(), before.DecidedAt.In(local).Format("2 Jan"), before.Reason))
			}
		}
		if fee := c.Late.Rule(req.Kind).Fee; fee > 0 {
			v.Fee = c.Fees.Money(fee)
		}
		if c.Signoff {
			v.Signoff = &views.SignoffView{Required: true, Signed: req.SignedOff(), By: req.SignedBy, Note: req.SignNote,
				Unapproved: req.SignedOff() && !signed}
		}
		if req.Thread != "" {
			if k, err := p.st.Concern(ctx, c.ID, req.Thread); err == nil {
				for _, post := range k.Thread {
					v.Thread = append(v.Thread, views.ThreadPost{Who: post.Who, Text: post.Text, At: post.At.In(local).Format("Mon 2 Jan, 15:04"), Organiser: !post.BySender})
				}
			}
		}
		if v.Waiting {
			v.Blocked = p.lateBlocked(ctx, c, req)
			v.Checks = p.lateChecks(ctx, c, req, e, entries, f)
			// The card as it would be, to see before deciding.
			if shown, err := card(c.Competition, req.Entry, clubOf(e)); err == nil {
				v.NewCard = &shown
			}
		}
		page.Requests = append(page.Requests, v)
	}
	// A request accepted lands in the draft timetable: the way there to publish it.
	if c.Published != nil && c.Timetable != nil && !sameTimetable(*c.Timetable, *c.Published) {
		page.Timetable = base + "/timetable"
	}
	render(w, r, views.LateAdmin(page))
}

// lateKindPlural is a kind of late request in a sentence, e.g. "level
// changes".
func lateKindPlural(kind string) string {
	if kind == competitions.LateNew {
		return "late entries"
	}
	return strings.ToLower(competitions.LateKindName(kind)) + "s"
}

// lateChecks say whether a request can be fitted in: the card's problems,
// the level's limit, and where it would go in the draft timetable and what
// that does.
func (p *competitionPages) lateChecks(ctx context.Context, c store.Competition, req store.LateRequest, e store.Entry, entries []store.Entry, f rotaFacts) []string {
	var out []string
	shown, err := card(c.Competition, req.Entry, clubOf(e))
	which := "new card"
	if req.Kind == competitions.LateNew {
		which = "card"
	}
	switch {
	case err != nil:
		out = append(out, "The card can't be checked: "+err.Error())
	case len(shown.Problems) > 0:
		out = append(out, fmt.Sprintf("The %s has %s: %s", which, views.Count(len(shown.Problems), "problem", "problems"), strings.Join(shown.Problems, "; ")))
	default:
		out = append(out, "The "+which+" meets its requirements.")
	}
	if c.Signoff {
		switch {
		case req.SignedOff() && !p.signedOffForLate(ctx, c, req):
			out = append(out, "Signed off by "+req.SignedBy+", who isn't an approved coach: accepted, it would need signing off again.")
		case req.SignedOff():
			out = append(out, "Signed off by "+req.SignedBy+".")
		default:
			out = append(out, "Not signed off by a coach: accepted, it would need signing off.")
		}
	}
	if req.Kind == competitions.LateRoutines {
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
	_, place := c.Timetable.PlaceChange(e.ID, event, req.Entry.Category, f.one, f.oneNames)
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

// setLate saves which late requests are allowed (late-<kind>=1) and their
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
			refuse(w, r, back, competitions.LateKindName(kind)+" fee: "+err.Error()+". Nothing was changed.")
			return
		}
		on := r.FormValue("late-"+kind) == "1"
		switch {
		case kind == competitions.LateNew && (!on || fee > 0):
			late.Rules[kind] = competitions.LateRule{Off: !on, Fee: fee}
		case kind != competitions.LateNew && (on || fee > 0):
			late.Rules[kind] = competitions.LateRule{On: on, Fee: fee}
		}
	}
	if err := p.st.SetLate(r.Context(), c.ID, late); err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, back, "Saved.")
}

// lateBack is the organiser's late requests, at one of them.
func lateBack(r *http.Request) string {
	return adminPath(r.PathValue("token")) + "/late#late-" + r.PathValue("id")
}

// decidable is the request a decision is about, still to be decided; not
// ok, having said why, when it isn't.
func (p *competitionPages) decidable(w http.ResponseWriter, r *http.Request, c store.Competition) (store.LateRequest, bool) {
	req, err := p.st.LateRequest(r.Context(), c.ID, r.PathValue("id"))
	switch {
	case errors.Is(err, store.ErrNotFound), err == nil && !req.Sent():
		// A member's not sent by their club yet isn't the organiser's.
		refuse(w, r, adminPath(r.PathValue("token"))+"/late", "That request isn't there any more; nothing was changed.")
		return req, false
	case err != nil:
		failed(w, r, err)
		return req, false
	case !req.Open():
		refuse(w, r, lateBack(r), "That request has been decided already.")
		return req, false
	}
	return req, true
}

// acceptLate applies a request, with an optional note: the entry changes
// (or comes in, entered late), a level change or late entry goes into the
// draft timetable (or out of it, if they'd wait), and the fee is charged.
// Where a coach must sign it off first, it waits until they have.
func (p *competitionPages) acceptLate(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	back := lateBack(r)
	req, ok := p.decidable(w, r, c)
	if !ok {
		return
	}
	if why := p.lateBlocked(ctx, c, req); why != "" {
		refuse(w, r, back, why)
		return
	}
	note := limitNote(strings.TrimSpace(r.FormValue("note")))
	p.notify.changing(ctx, c)
	came := p.notify.watching(ctx, c, cameFree)
	accepted, err := p.st.AcceptLateChange(ctx, c.ID, req.ID, note, linkOf(r).Name, c.Late.Rule(req.Kind).Fee)
	if err == nil {
		came(ctx)
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		refuse(w, r, back, "That request has been decided already, or its entry was withdrawn; nothing was changed.")
		return
	case err != nil:
		failed(w, r, err)
		return
	}
	notice := "Accepted: " + accepted.Entry.Gymnasts() + " is in " + accepted.Entry.Event() + "."
	entries, err := p.entriesOf(ctx, c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	waiting := false
	for _, e := range entries {
		if e.ID == accepted.EntryID && e.Waiting > 0 && accepted.Kind != competitions.LateRoutines {
			waiting = true
			notice = fmt.Sprintf("Accepted: %s is %s on %s's waiting list.", accepted.Entry.Gymnasts(), waitingWord(e.Waiting), accepted.Entry.Event())
		}
	}
	if accepted.Kind != competitions.LateRoutines && c.Timetable != nil && c.Timetable.Planned {
		f, err := p.liveRota(ctx, c)
		if err != nil {
			failed(w, r, err)
			return
		}
		people, names := f.one, f.oneNames
		// The draft as it is now, not as it was when the page was read: an
		// edit made since stays.
		err = p.st.UpdateTimetable(ctx, c.ID, func(draft *competitions.Schedule) error {
			if waiting {
				draft.RemoveEntry(accepted.EntryID)
				draft.Retime()
				return nil
			}
			var place competitions.ChangePlace
			*draft, place = draft.PlaceChange(accepted.EntryID, accepted.Entry.Event(), accepted.Entry.Category, people, names)
			if place.Flight >= 0 {
				notice += " The draft timetable has them in " + place.Name + ": publish it when you're ready."
			} else {
				notice += " The draft timetable has no flight of it yet: place them by hand."
			}
			return nil
		})
		if err != nil {
			failed(w, r, err)
			return
		}
	}
	if k, err := p.st.Concern(ctx, c.ID, accepted.Thread); err == nil {
		notice += " " + p.notify.tellSender(ctx, c, k, senderAccepted, note)
	}
	seeOther(w, r, back, strings.TrimSpace(notice))
}

// rejectLate turns a request down, with a reason for whoever asked.
func (p *competitionPages) rejectLate(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	back := lateBack(r)
	reason := limitNote(strings.TrimSpace(r.FormValue("reason")))
	if reason == "" {
		refuse(w, r, back, "Give a reason: whoever asked sees it.")
		return
	}
	req, ok := p.decidable(w, r, c)
	if !ok {
		return
	}
	p.notify.changing(ctx, c)
	switch err := p.st.RejectLateChange(ctx, c.ID, req.ID, reason, linkOf(r).Name); {
	case errors.Is(err, store.ErrNotFound):
		// Decided in the meantime, by someone else.
		refuse(w, r, back, "That request has been decided already.")
		return
	case err != nil:
		failed(w, r, err)
		return
	}
	notice := "Turned down; they'll see your reason."
	if k, err := p.st.Concern(ctx, c.ID, req.Thread); err == nil {
		notice = "Turned down. " + p.notify.tellSender(ctx, c, k, senderTurnedDown, reason)
	}
	seeOther(w, r, back, notice)
}

// replyLate keeps the organiser's reply (reply) to a request still to be
// decided, which the asker sees with it and is told of.
func (p *competitionPages) replyLate(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	back := lateBack(r)
	reply := limitMessage(r.FormValue("reply"))
	if reply == "" {
		refuse(w, r, back, "Write the reply first; nothing was sent.")
		return
	}
	req, ok := p.decidable(w, r, c)
	if !ok {
		return
	}
	// A request from before threads gets one, from whoever the entry is.
	e, err := p.st.CompetitionEntry(ctx, c.ID, req.EntryID)
	if err != nil {
		failed(w, r, err)
		return
	}
	from := individualTeller(e).from
	if e.MemberID != "" {
		from = memberTeller(store.Member{ID: e.MemberID, Name: e.Entry.Gymnast}, e.ClubName).from
	}
	thread, err := p.st.LateThread(ctx, c.ID, req.ID, from)
	if err != nil {
		failed(w, r, err)
		return
	}
	_, err = p.st.ReplyTo(ctx, c.ID, thread, linkOf(r).Name, reply)
	switch {
	case errors.Is(err, store.ErrLimit):
		refuse(w, r, back, fmt.Sprintf("A request takes %d replies and answers; nothing was sent. Speak to them instead.", store.MaxPosts))
		return
	case err != nil:
		failed(w, r, err)
		return
	}
	k, err := p.st.Concern(ctx, c.ID, thread)
	if err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, back, strings.TrimSpace("Reply sent. "+p.notify.tellSender(ctx, c, k, senderReplied, reply)))
}

// lateNow says, in a line, what can be asked for after the deadline:
// allowed are the kinds, each with its fee; signoffFirst if a request can't
// be accepted until a coach signs it off.
func lateNow(allowed []string, signoffFirst bool) string {
	if len(allowed) == 0 {
		return "Nothing can be asked for after the deadline"
	}
	now := strings.Join(allowed, " and ") + " can be asked for after the deadline"
	now = strings.ToUpper(now[:1]) + now[1:]
	if signoffFirst {
		now += "; a coach signs each off before you can accept it"
	}
	return now
}
