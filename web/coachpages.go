package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
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

// whoSignsOff says, on a member's entry waiting for a coach's sign-off
// (s), who can give it: the club's coaches who sign off routines and see
// them (their own coach, or any while they have none or the club lets every
// coach see every member). Where no one can, it says why, and what to do:
// on the comp sec's Coaches (coachesPage), or else asking the comp sec.
func whoSignsOff(s *views.SignoffView, m store.Member, coaches []store.Coach, seeAll bool, coachesPage string) {
	if !s.Required || s.Signed || s.By != "" {
		return
	}
	var names []string
	var theirs string
	for _, c := range coaches {
		if c.ID == m.CoachID {
			theirs = c.Name
		}
		if c.SignsOff && (seeAll || m.CoachID == "" || c.ID == m.CoachID) {
			names = append(names, c.Name)
		}
	}
	switch {
	case len(names) == 1:
		s.Waiting = names[0] + " signs it off, on their coach page."
	case len(names) > 1 && len(names) <= 3:
		s.Waiting = strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1] + " signs it off, on their coach page."
	case len(names) > 3:
		s.Waiting = fmt.Sprintf("Any of the club's %d coaches who sign off routines can sign it off, on their coach page.", len(names))
	default:
		whose := "Your coach"
		if coachesPage != "" {
			whose = m.Name + "'s coach"
		}
		why := "No coach of the club signs off routines yet, so no one can sign it off."
		if theirs != "" && slices.ContainsFunc(coaches, func(c store.Coach) bool { return c.SignsOff }) {
			why = whose + ", " + theirs + ", doesn't sign off routines, so no one can sign it off."
		}
		if coachesPage != "" {
			s.Waiting, s.Coaches = why+" It can still be sent: the organiser sees it isn't signed off.", coachesPage
		} else {
			s.Waiting = why + " Ask your comp sec to add a coach who does. It can still be sent: the organiser sees it isn't signed off."
		}
	}
}

// memberProblems are a member's entry's problems as the organiser sees them
// (entryProblems): the competition's copy's, while it's what the member has;
// otherwise the member's own, worked out the same way, with its sign-off as
// it stands. sent are the competition's entries, all those enteredAll.
func memberProblems(c competitions.Competition, e store.MemberEntry, sent []store.Entry, all []competitions.Entered) []string {
	people := []string{store.MemberKey(e.MemberID)}
	for _, s := range sent {
		if s.MemberID != e.MemberID || s.Entry.Discipline != e.Discipline {
			continue
		}
		if e.Sent() && !e.ChangedSinceSent() {
			return entryProblems(c, s, all)
		}
		people = s.People()
	}
	return c.EntryProblems(competitions.Entered{Entry: e.Entry, People: people, Signoff: memberSignoff(c, e).Problem()}, all)
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
		seeOther(w, r, clubCoachesPath(r.PathValue("token")), strings.Join(sentences(err), " "))
		return
	}
	// A name names one coach, so reloading the page showing the new link
	// doesn't add them again; a second coach of the same name needs telling apart.
	coaches, err := p.st.Coaches(r.Context(), club.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	if slices.ContainsFunc(coaches, func(c store.Coach) bool { return store.NameKey(c.Name) == store.NameKey(name) }) {
		seeOther(w, r, clubCoachesPath(r.PathValue("token")), name+" is already one of your coaches; nothing was changed. For a new link for them, use New link on their row; for another coach of that name, add something to tell them apart, e.g. a middle initial.")
		return
	}
	coach, token, err := p.st.CreateCoach(r.Context(), club.ID, name)
	if err != nil {
		failed(w, r, err)
		return
	}
	// Shown on this response only: it isn't kept, so it can't go in a redirect's URL.
	p.renderClub(w, r, club, clubCoaches, &views.MemberLink{Name: coach.Name, Link: origin(r) + coachPath(token), Coach: true})
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
	if p.askClub(w, r, club, clubCoaches, views.Ask{
		Title:  "Make a new link for " + coach.Name + "?",
		Text:   "Their old coach link stops working at once, wherever it's saved: in their browser, on their phone's home screen or in their bookmarks. Their sign-offs and notes stay. You see the new link once, here, to send them.",
		Button: "Make a new link", Back: clubCoachesPath(r.PathValue("token")) + "#" + coachAnchor(coach.ID),
	}) {
		return
	}
	token, err := p.st.ReplaceCoachLink(r.Context(), club.ID, coach.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	p.renderClub(w, r, club, clubCoaches, &views.MemberLink{Name: coach.Name, Link: origin(r) + coachPath(token), Coach: true})
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
	if p.askClub(w, r, club, clubCoaches, views.Ask{
		Remove: true, // takes something away: red
		Title:  "Remove " + coach.Name + " as a coach?",
		Text:   "Their coach link stops working at once. Members who chose them have no coach until one is chosen; their sign-offs stay.",
		Button: "Remove " + coach.Name, Back: clubCoachesPath(r.PathValue("token")) + "#" + coachAnchor(coach.ID),
	}) {
		return
	}
	if err := p.st.RemoveCoach(r.Context(), club.ID, coach.ID); err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, clubCoachesPath(r.PathValue("token")), "Removed "+coach.Name+" as a coach.")
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
	seeOther(w, r, clubCoachesPath(r.PathValue("token")), notice)
}

// assignCoach chooses a member's coach for them (coach: an id, "" for none).
func (p *competitionPages) assignCoach(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	m, err := p.st.Member(r.Context(), club.ID, r.PathValue("member"))
	if err != nil {
		failed(w, r, err)
		return
	}
	if err := p.st.SetMemberCoach(r.Context(), club.ID, m.ID, r.FormValue("coach")); err != nil {
		failed(w, r, err)
		return
	}
	notice := m.Name + " has no coach chosen now."
	if id := r.FormValue("coach"); id != "" {
		coaches, err := p.st.Coaches(r.Context(), club.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		if i := slices.IndexFunc(coaches, func(c store.Coach) bool { return c.ID == id }); i >= 0 {
			notice = m.Name + "'s coach is now " + coaches[i].Name + "."
		}
	}
	// Back to their row, which says it too, as the page opens there: the
	// list's search and filters are kept for the tab.
	seeOther(w, r, clubMembersPath(r.PathValue("token"))+"?at="+url.QueryEscape(m.ID)+"#"+views.MemberAnchor(m.ID), notice)
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
	seeOther(w, r, memberDetailsPath(r.PathValue("token")), "Coach saved.")
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

// coachDetails is a coach's details: their link, and their other links to
// add and take off; 404 through another of their links, whose own page has
// them.
func (p *competitionPages) coachDetails(w http.ResponseWriter, r *http.Request) {
	coach, club, ok := p.coach(w, r)
	if !ok {
		return
	}
	token := r.PathValue("token")
	if store.IsVia(token) {
		failed(w, r, store.ErrNotFound)
		return
	}
	other, err := p.otherLinks(r, coachPath(token))
	if err != nil {
		failed(w, r, err)
		return
	}
	render(w, r, views.CoachDetails(views.CoachDetailsPage{
		Frame: views.CoachFrame{Club: club, Coach: coach.Name, Notice: r.URL.Query().Get("notice"), Menu: coachMenu(token, coachDetailsTab)},
		Link:  origin(r) + coachPath(token), Other: other,
	}))
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
	clubMembers, err := p.st.Members(ctx, coach.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	var coached []store.Member // the members who chose this coach
	for _, m := range clubMembers {
		if m.CoachID == coach.ID {
			coached = append(coached, m)
		}
	}
	path := coachPath(r.PathValue("token"))
	page := views.CoachPage{Club: club, Coach: coach.Name, Link: origin(r) + path, Notice: r.URL.Query().Get("notice"), SeesAll: seesAll, SignsOff: coach.SignsOff,
		Menu: coachMenu(r.PathValue("token"), coachEntriesTab)}
	if store.IsVia(r.PathValue("token")) {
		page.Link = "" // opened through another of their links, which is the one to keep
	}
	var levels []string
	var competitionOptions []views.ListOption
	tell := coachTeller(coach, club, coachSees(entries))
	for _, c := range comps {
		cc := views.CoachCompetition{Competition: summary(c, p.now()), Cannot: notApproved(c, sentTo),
			MyLinks: p.competitionLinks(ctx, c, linksFor{base: path + "/competitions/" + c.ID,
				people: memberKeys(coached), clubs: clubIDs(coach.ClubID), staff: clubIDs(coach.ClubID), tell: tell})}
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
		sent, err := p.entriesOf(ctx, c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		all := enteredAll(c.Competition, sent)
		for _, e := range entries {
			if e.CompetitionID != c.ID {
				continue
			}
			row := views.CoachRow{Member: e.MemberName, Level: e.Entry.Event(), Signoff: memberSignoff(c.Competition, e),
				Open: path + "/members/" + e.MemberID + "/competitions/" + c.ID + "?discipline=" + e.Discipline}
			row.Problems = len(memberProblems(c.Competition, e, sent, all))
			if !e.SignedOff() {
				cc.Waiting++
			}
			row.List = views.ListRow{Name: e.MemberName, Values: map[string][]string{
				"competition": {c.ID}, "level": {row.Level}, "problems": {hasValue(row.Problems > 0)}, "signoff": {coachSignoffValue(row.Signoff)},
			}}
			cc.Rows = append(cc.Rows, row)
			levels = append(levels, row.Level)
		}
		page.Tools.Total += len(cc.Rows)
		competitionOptions = append(competitionOptions, views.ListOption{Value: c.ID, Label: c.Name})
		page.Competitions = append(page.Competitions, cc)
	}
	page.Tools.ID, page.Tools.Noun = "coach-entries", "entries"
	page.Tools.Filters = coachEntryFilters(competitionOptions, levels)
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
			feedbackAbout(r, c)
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
	frame := coachFrame(r, coach, club, coachEntriesTab, nil)
	page := views.SignoffPage{
		Frame: &frame, Back: coachPath(r.PathValue("token")), Competition: summary(c, p.now()),
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
		seeOther(w, r, coachPath(r.PathValue("token")), "Your club hasn't set you to sign off routines; nothing was changed.")
		return
	}
	if why, err := p.notApprovedFor(r, c, coach); err != nil {
		failed(w, r, err)
		return
	} else if why != "" {
		seeOther(w, r, coachPath(r.PathValue("token")), why+" Nothing was changed.")
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
	// On to the next entry at the competition waiting for a sign-off, after
	// this one in the list (by name); back to the list once there's none.
	entries, err := p.st.CoachEntries(r.Context(), coach)
	if err != nil {
		failed(w, r, err)
		return
	}
	var waiting []store.MemberEntry
	for _, o := range entries {
		if o.CompetitionID == c.ID && !o.SignedOff() && o.SignedBy == "" && (o.MemberID != e.MemberID || o.Discipline != e.Discipline) {
			waiting = append(waiting, o)
		}
	}
	if len(waiting) == 0 {
		seeOther(w, r, coachPath(r.PathValue("token")), notice+" Nothing else is waiting for you there.")
		return
	}
	next := waiting[0]
	for _, o := range waiting {
		if competitions.CompareNames(o.MemberName, e.MemberName) > 0 {
			next = o
			break
		}
	}
	seeOther(w, r, coachPath(r.PathValue("token"))+"/members/"+next.MemberID+"/competitions/"+c.ID+"?discipline="+next.Discipline,
		fmt.Sprintf("%s Next waiting: %s (%d left).", notice, next.MemberName, len(waiting)))
}

// maxCertificateBytes is the largest certificate (ADR 0007 Decision 4).
const maxCertificateBytes = store.MaxCertificateBytes

// uploads says whether a path takes an upload, for its larger size limit.
func uploads(path string) bool {
	return strings.HasPrefix(path, "/clubs/admin/") && strings.HasSuffix(path, "/qualifications") ||
		strings.HasPrefix(path, "/competitions/entry/") && strings.HasSuffix(path, "/coach") ||
		(strings.HasPrefix(path, "/clubs/") || strings.HasPrefix(path, "/competitions/entry/")) && strings.HasSuffix(path, "/judging")
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
	seeOther(w, r, clubCoachesPath(r.PathValue("token")), notice)
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
		seeOther(w, r, clubCoachesPath(r.PathValue("token")), notice)
	}
	q, data, certType, problem := postedCertificate(r)
	if problem != "" {
		back("Nothing was added: " + problem)
		return
	}
	_, err = p.st.AddQualification(r.Context(), club.ID, coach.ID, q.Key, certType, data)
	switch {
	case errors.Is(err, store.ErrLimit):
		back(fmt.Sprintf("Nothing was added: a coach can have %d qualifications at most. Remove one first.", store.MaxQualifications))
	case errors.Is(err, store.ErrCertificatesFull):
		back(fmt.Sprintf("Nothing was added: your coaches' certificates already take up the %d MB a club has for them. Remove one, or add a smaller photo or PDF.",
			store.MaxClubCertificateBytes>>20))
	case err != nil && certificateRefused(err) != "":
		back("Nothing was added: " + certificateRefused(err))
	case err != nil:
		failed(w, r, err)
	default:
		back("Added " + q.Name() + " for " + coach.Name + ".")
	}
}

// certificateRefused says why a certificate the store wouldn't keep was
// refused, for the reasons any upload can meet; "" for other errors.
func certificateRefused(err error) string {
	switch {
	case errors.Is(err, store.ErrCertificateTooBig):
		return "the certificate is over 3 MB. A photo or a smaller PDF will do."
	case errors.Is(err, store.ErrDiskFull):
		return "the server is short of space just now. Please try again later."
	}
	return ""
}

// postedCertificate is the qualification (its key) and certificate (a file)
// a form uploads, or what's wrong with them.
func postedCertificate(r *http.Request) (q competitions.Qualification, data []byte, certType, problem string) {
	if problem := parseUpload(r); problem != "" {
		return q, nil, "", problem
	}
	q, ok := competitions.QualificationByKey(r.FormValue("qualification"))
	if !ok {
		return q, nil, "", "choose the coach's qualification."
	}
	data, certType, problem = postedFile(r)
	return q, data, certType, problem
}

// parseUpload reads a form uploading a certificate, saying what's wrong
// with it if it can't be read.
func parseUpload(r *http.Request) string {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return "the certificate is over 3 MB. A photo or a smaller PDF will do."
		}
		return "choose a qualification and its certificate."
	}
	return ""
}

// postedFile is the certificate an upload form sends (certificate), and its
// type from its own bytes, or what's wrong with it.
func postedFile(r *http.Request) (data []byte, certType, problem string) {
	file, _, err := r.FormFile("certificate")
	if err != nil {
		return nil, "", "add the certificate, a photo or PDF of it."
	}
	defer file.Close()
	data, err = io.ReadAll(io.LimitReader(file, maxCertificateBytes+1))
	if err != nil || len(data) > maxCertificateBytes {
		return nil, "", "the certificate is over 3 MB. A photo or a smaller PDF will do."
	}
	if certType = certificateType(data); certType == "" {
		return nil, "", "the certificate should be a photo (JPEG, PNG or WebP) or a PDF."
	}
	return data, certType, ""
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
	seeOther(w, r, clubCoachesPath(r.PathValue("token")), "Removed a qualification of "+coach.Name+"'s, and its certificate.")
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
