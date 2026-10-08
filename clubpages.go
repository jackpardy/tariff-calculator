package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The club pages (ADR 0004 step 4): a comp sec creates a club, members join
// it and keep their entries, and the comp sec enters the club in competitions
// and sends its members' entries.

const (
	// maxClubsPerHour is how many clubs one address can create in an hour.
	maxClubsPerHour = 5
	// maxJoinsPerHour is how many times one address can join clubs in an hour.
	maxJoinsPerHour = 30
	// maxEmailsPerHour is how many confirmation emails one address can have
	// sent in an hour (ADR 0008 Decision 2).
	maxEmailsPerHour = 10
)

func (p *competitionPages) registerClubs(mux *http.ServeMux) {
	handle := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, p.secret(h)) }
	handle("GET /competitions", p.hub)
	handle("GET /clubs/new", p.newClub)
	handle("POST /clubs", p.createClub)
	handle("GET /clubs/admin/{token}", p.club)
	handle("POST /clubs/admin/{token}/competitions/{id}/send", p.send)
	handle("POST /clubs/admin/{token}/competitions/{id}/coaches", p.sendCoaches)
	handle("GET /clubs/admin/{token}/members/{member}/competitions/{id}", p.editMemberEntry)
	handle("POST /clubs/admin/{token}/members/{member}/competitions/{id}", p.saveMemberEntryForMember)
	handle("POST /clubs/admin/{token}/members/{member}/new-link", p.newMemberLink)
	handle("POST /clubs/admin/{token}/members/{member}/remove", p.removeMember)
	handle("POST /clubs/admin/{token}/replace-link", p.replaceClubLink)
	handle("POST /clubs/admin/{token}/delete", p.deleteClub)
	handle("GET /clubs/join/{token}", p.joinForm)
	handle("POST /clubs/join/{token}", p.join)
	handle("GET /clubs/member/{token}", p.memberHome)
	handle("POST /clubs/member/{token}/competitions/{id}", p.saveOwnMemberEntry)
	handle("POST /clubs/member/{token}/competitions/{id}/withdraw", p.withdrawMemberEntry)
	handle("POST /clubs/member/{token}/coach", p.chooseOwnCoach)
	handle("POST /clubs/admin/{token}/coaches", p.addCoach)
	handle("POST /clubs/admin/{token}/coaches/{coach}/new-link", p.newCoachLink)
	handle("POST /clubs/admin/{token}/coaches/{coach}/remove", p.removeCoach)
	handle("POST /clubs/admin/{token}/coaches/{coach}/signs-off", p.coachSignsOff)
	handle("POST /clubs/admin/{token}/coaches/{coach}/qualifications", p.addQualification)
	handle("POST /clubs/admin/{token}/coaches/{coach}/qualifications/{q}/remove", p.removeQualification)
	handle("GET /clubs/admin/{token}/coaches/{coach}/qualifications/{q}/certificate", p.clubCertificate)
	handle("POST /clubs/admin/{token}/coaches-see-all", p.coachesSeeAll)
	handle("POST /clubs/admin/{token}/members/{member}/coach", p.assignCoach)
	handle("GET /clubs/coach/{token}", p.coachHome)
	handle("GET /clubs/coach/{token}/members/{member}/competitions/{id}", p.coachEntry)
	handle("POST /clubs/coach/{token}/members/{member}/competitions/{id}", p.signOff)
	handle("GET /competitions/club/{token}", p.clubLink)
	handle("POST /competitions/club/{token}", p.enterClub)
}

func (p *competitionPages) hub(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.Hub())
}

func clubPath(token string) string   { return "/clubs/admin/" + token }
func memberPath(token string) string { return "/clubs/member/" + token }

// --- The comp sec ---

func (p *competitionPages) newClub(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.NewClub("", nil))
}

func (p *competitionPages) createClub(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if err := competitions.CheckName("club", name); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		render(w, r, views.NewClub(name, sentences(err)))
		return
	}
	if !p.clubLimiter.allow(clientIP(r), p.now()) {
		message(w, r, http.StatusTooManyRequests, "Too many clubs", "This address has created several clubs in the last hour. Please try again later.")
		return
	}
	_, admin, err := p.st.CreateClub(r.Context(), name)
	if err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, clubPath(admin)+"?new=created", http.StatusSeeOther)
}

// clubAdmin is the club an admin link opens, or a page saying it doesn't.
func (p *competitionPages) clubAdmin(w http.ResponseWriter, r *http.Request) (store.Club, bool) {
	c, err := p.st.ClubByAdmin(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return c, false
	}
	return c, true
}

// club is the comp sec's page: the join link, each competition the club is
// entered in with its members' entries, and the members.
func (p *competitionPages) club(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	p.renderClub(w, r, club, nil)
}

// renderClub shows the comp sec's page, with a member's new link if one was
// just made.
func (p *competitionPages) renderClub(w http.ResponseWriter, r *http.Request, club store.Club, newLink *views.MemberLink) {
	ctx := r.Context()
	base := clubPath(r.PathValue("token"))
	page := views.ClubPage{
		Base: base, Name: club.Name, New: r.URL.Query().Get("new"), NewLink: newLink, Notice: r.URL.Query().Get("notice"),
		JoinLink: origin(r) + "/clubs/join/" + club.JoinLink, CoachesSeeAll: club.CoachesSeeAll,
	}
	if page.New != "" {
		page.AdminLink = origin(r) + base
	}
	coaches, err := p.st.Coaches(ctx, club.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	quals, err := p.st.Qualifications(ctx, club.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	for _, q := range competitions.Qualifications {
		page.QualificationOptions = append(page.QualificationOptions, views.CoachOption{ID: q.Key, Name: q.Name()})
	}
	members, err := p.st.Members(ctx, club.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	comps, err := p.st.ClubCompetitions(ctx, club.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	entered := map[string]int{}
	for _, c := range comps {
		mine, err := p.st.ClubEntries(ctx, club.ID, c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		sent, err := p.st.Entries(ctx, c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		cc := views.ClubCompetition{ID: c.ID, Competition: summary(c.Competition, p.now()), ApproveCoaches: c.ApproveCoaches && c.Signoff,
			Notify: p.notifyLink(base + "/competitions/" + c.ID + "/notify")}
		cc.Fees = p.feesSummary(ctx, c, clubPayer(club.ID), base+"/competitions/"+c.ID+"/invoice")
		if published(c) {
			cc.Timeline = base + "/competitions/" + c.ID + "/timeline"
		}
		sentID := map[string]string{} // member → the competition's copy of their entry
		waitingPlace := map[string]int{} // member and discipline → place on a waiting list
		for _, e := range sent {
			if e.ClubID == club.ID {
				sentID[e.MemberID] = e.ID
				waitingPlace[e.MemberID+"/"+e.Entry.Discipline] = e.Waiting
			}
		}
		removals, err := p.st.ClubRemovals(ctx, club.ID, c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		has := map[string]bool{}
		for _, e := range mine {
			has[e.MemberID] = true
			entered[e.MemberID]++
			row := views.ClubEntryRow{Member: e.MemberName, Level: e.Entry.Event(), Status: "Sent", Note: e.Note, Signoff: memberSignoff(c.Competition, e)}
			rem, removed := removals[e.MemberID+"/"+e.Discipline]
			switch {
			case removed:
				row.Status = removalStatus(rem, e.ChangedSinceSent())
				if rem.ForClub() && rem.RemovalNote != "" {
					row.Note = strings.TrimSpace(row.Note + " " + rem.RemovalNote)
				}
				if rem.Removal == store.Held && e.ChangedSinceSent() && !rem.Resent {
					cc.ToSend++
				}
			case !e.Sent():
				row.Status = "Not sent"
				cc.ToSend++
			case waitingPlace[e.MemberID+"/"+e.Discipline] > 0 && !e.ChangedSinceSent():
				row.Status = "Sent · waiting list, " + waitingWord(waitingPlace[e.MemberID+"/"+e.Discipline])
			case e.ChangedSinceSent():
				row.Status = "Changed since sent"
				cc.ToSend++
			case e.Checked:
				row.Status = "Sent · checked ✓"
			}
			if checked, err := c.Check(e.Entry); err != nil {
				row.Problems = 1
			} else {
				row.Problems = len(checked.Problems())
				if competitions.VideoMissing(e.Entry, c.VideoNeeds(checked)) {
					row.Video = "missing"
				}
			}
			if row.Video == "" && e.VideoReview == store.VideoMore && !e.ChangedSinceSent() {
				row.Video, row.Note = "need more", strings.TrimSpace(row.Note+" "+e.VideoNote)
			}
			if cc.Competition.Open {
				row.Edit = base + "/members/" + e.MemberID + "/competitions/" + c.ID + "?discipline=" + e.Discipline
			}
			if at := placement(c, sentID[e.MemberID]); at != nil {
				row.Flight = fmt.Sprintf("%s · %s · warm-up %s", at.Flight, at.Panel, at.Time)
			}
			cc.Rows = append(cc.Rows, row)
		}
		// Entries the club sent for members who've since withdrawn or left.
		for _, e := range sent {
			if e.ClubID == club.ID && !has[e.MemberID] {
				cc.Rows = append(cc.Rows, views.ClubEntryRow{Member: e.Entry.Gymnasts(), Level: e.Entry.Event(), Status: "Withdrawn (still sent)"})
			}
		}
		if cc.ApproveCoaches {
			if cc.Coaches, err = p.clubSendCoaches(r, club, c.ID, coaches, quals, members, mine); err != nil {
				failed(w, r, err)
				return
			}
		}
		if cc.Offers, err = p.clubOffers(r, base, club, c, members); err != nil {
			failed(w, r, err)
			return
		}
		page.Competitions = append(page.Competitions, cc)
	}
	coached := map[string]int{}
	for _, m := range members {
		page.Members = append(page.Members, views.ClubMember{ID: m.ID, Name: m.Name, Entries: entered[m.ID], CoachID: m.CoachID})
		coached[m.CoachID]++
	}
	for _, c := range coaches {
		cv := views.ClubCoach{ID: c.ID, Name: c.Name, Members: coached[c.ID], SignsOff: c.SignsOff}
		for _, q := range quals[c.ID] {
			cv.Qualifications = append(cv.Qualifications, views.QualificationView{ID: q.ID, Name: qualificationName(q.Qualification),
				Certificate: base + "/coaches/" + c.ID + "/qualifications/" + q.ID + "/certificate"})
		}
		page.Coaches = append(page.Coaches, cv)
		page.CoachOptions = append(page.CoachOptions, views.CoachOption{ID: c.ID, Name: c.Name})
	}
	pairs, err := p.st.ClubPairEntries(ctx, club.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	ours := map[string]bool{}
	for _, m := range members {
		ours[m.ID] = true
	}
	if page.Pairs, err = p.seenFor(r).pairViews(pairs, func(pe store.PairEntry) bool { return ours[pe.EntrantMemberID] }); err != nil {
		failed(w, r, err)
		return
	}
	render(w, r, views.ClubAdmin(page))
}

// send sends the club's entries for a competition: which=changed sends those
// never sent or changed since, which=all sends everyone's again.
func (p *competitionPages) send(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	compID := r.PathValue("id")
	var members []string
	if r.FormValue("which") != "all" {
		mine, err := p.st.ClubEntries(r.Context(), club.ID, compID)
		if err != nil {
			failed(w, r, err)
			return
		}
		members = []string{}
		for _, e := range mine {
			if !e.Sent() || e.ChangedSinceSent() {
				members = append(members, e.MemberID)
			}
		}
	}
	n, err := p.st.Send(r.Context(), club.ID, compID, members)
	if err != nil {
		failed(w, r, err)
		return
	}
	c, err := p.st.Competition(r.Context(), compID)
	if err != nil {
		failed(w, r, err)
		return
	}
	notice := fmt.Sprintf("Sent %s to %s.", entriesWord(n), c.Name)
	http.Redirect(w, r, clubPath(r.PathValue("token"))+"?notice="+urlQuery(notice), http.StatusSeeOther)
}

// urlQuery escapes a notice for a redirect's query.
func urlQuery(s string) string { return url.QueryEscape(s) }

func entriesWord(n int) string {
	if n == 1 {
		return "1 entry"
	}
	return fmt.Sprintf("%d entries", n)
}

// memberOfClub is the member and competition a comp sec's member entry URL
// names, checking the member is the club's and the club is entered.
func (p *competitionPages) memberOfClub(w http.ResponseWriter, r *http.Request, club store.Club) (store.Member, store.Competition, bool) {
	m, err := p.st.Member(r.Context(), club.ID, r.PathValue("member"))
	if err != nil {
		failed(w, r, err)
		return m, store.Competition{}, false
	}
	c, ok := p.enteredCompetition(w, r, club.ID)
	return m, c, ok
}

// enteredCompetition is the competition in the URL, if the club is entered in it.
func (p *competitionPages) enteredCompetition(w http.ResponseWriter, r *http.Request, clubID string) (store.Competition, bool) {
	id := r.PathValue("id")
	if in, err := p.st.ClubEntered(r.Context(), clubID, id); err != nil || !in {
		failed(w, r, cmpErr(err, store.ErrNotFound))
		return store.Competition{}, false
	}
	c, err := p.st.Competition(r.Context(), id)
	if err != nil {
		failed(w, r, err)
		return c, false
	}
	return c, true
}

// cmpErr is err, or fallback when there's none.
func cmpErr(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}

// memberEntry is a member's entry for a competition, if they have one.
// memberEntry is a member's entry for a competition, in the discipline the
// request names (discipline), if they have one.
func (p *competitionPages) memberEntry(r *http.Request, memberID, competitionID string) (*store.MemberEntry, error) {
	entries, err := p.st.MemberEntries(r.Context(), memberID)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.CompetitionID == competitionID && e.Discipline == r.FormValue("discipline") {
			return &e, nil
		}
	}
	return nil, nil
}

func (p *competitionPages) editMemberEntry(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	m, c, ok := p.memberOfClub(w, r, club)
	if !ok {
		return
	}
	existing, err := p.memberEntry(r, m.ID, c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	start := competitions.Entry{Gymnast: m.Name, Discipline: r.FormValue("discipline")}
	if existing != nil {
		start = existing.Entry
	}
	p.renderMemberEdit(w, r, club, m, c, existing, start, nil)
}

func (p *competitionPages) renderMemberEdit(w http.ResponseWriter, r *http.Request, club store.Club, m store.Member, c store.Competition, existing *store.MemberEntry, form competitions.Entry, problems []string) {
	page := views.ClubEntryEdit{
		Base: clubPath(r.PathValue("token")), Club: club.Name, Member: m.Name, Competition: summary(c.Competition, p.now()),
		Form: entryForm(c.Competition, form, r.URL.Path, "Save"),
	}
	page.Form.Member, page.Form.Problems = true, problems
	if existing != nil {
		shown, err := card(c.Competition, existing.Entry, club.Name)
		withSignoff(&shown, memberSignoff(c.Competition, *existing))
		if err != nil {
			failed(w, r, err)
			return
		}
		page.Card = &shown
	}
	if len(problems) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	render(w, r, views.ClubEntryEditPage(page))
}

func (p *competitionPages) saveMemberEntryForMember(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	m, c, ok := p.memberOfClub(w, r, club)
	if !ok {
		return
	}
	e, problems := postedEntry(r, c.Competition, m.Name)
	if len(problems) > 0 {
		existing, err := p.memberEntry(r, m.ID, c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		p.renderMemberEdit(w, r, club, m, c, existing, e, problems)
		return
	}
	if err := p.st.SaveMemberEntry(r.Context(), m.ID, c.ID, e); err != nil {
		failed(w, r, err)
		return
	}
	notice := fmt.Sprintf("Saved %s's entry for %s. Send it when you're ready.", m.Name, c.Name)
	http.Redirect(w, r, clubPath(r.PathValue("token"))+"?notice="+urlQuery(notice), http.StatusSeeOther)
}

func (p *competitionPages) newMemberLink(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	m, err := p.st.Member(r.Context(), club.ID, r.PathValue("member"))
	if err != nil {
		failed(w, r, err)
		return
	}
	token, err := p.st.ReplaceMemberLink(r.Context(), club.ID, m.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	// Shown on this response only: it isn't kept, so it can't go in a redirect's URL.
	p.renderClub(w, r, club, &views.MemberLink{Name: m.Name, Link: origin(r) + memberPath(token)})
}

func (p *competitionPages) removeMember(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	m, err := p.st.Member(r.Context(), club.ID, r.PathValue("member"))
	if err != nil {
		failed(w, r, err)
		return
	}
	if err := p.st.RemoveMember(r.Context(), club.ID, m.ID); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, clubPath(r.PathValue("token"))+"?notice="+urlQuery("Removed "+m.Name+"."), http.StatusSeeOther)
}

func (p *competitionPages) replaceClubLink(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	admin, err := p.st.ReplaceClubAdmin(r.Context(), club.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, clubPath(admin)+"?new=replaced", http.StatusSeeOther)
}

func (p *competitionPages) deleteClub(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	if r.FormValue("confirm") != "1" {
		http.Redirect(w, r, clubPath(r.PathValue("token")), http.StatusSeeOther)
		return
	}
	if err := p.st.DeleteClub(r.Context(), club.ID); err != nil {
		failed(w, r, err)
		return
	}
	message(w, r, http.StatusOK, "Club deleted", club.Name+", its members and their entries have been deleted. Entries already sent stay with each competition.")
}

// --- Entering a club in a competition ---

func (p *competitionPages) clubLinkCompetition(w http.ResponseWriter, r *http.Request) (store.Competition, bool) {
	c, err := p.st.CompetitionByClubLink(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return c, false
	}
	return c, true
}

func (p *competitionPages) clubLink(w http.ResponseWriter, r *http.Request) {
	c, ok := p.clubLinkCompetition(w, r)
	if !ok {
		return
	}
	render(w, r, views.ClubLink(views.ClubLinkPage{Competition: summary(c.Competition, p.now()), Action: r.URL.Path}))
}

// enterClub enters the club whose admin link is posted (clubAdmin: the link,
// or just its token) in the competition.
func (p *competitionPages) enterClub(w http.ResponseWriter, r *http.Request) {
	c, ok := p.clubLinkCompetition(w, r)
	if !ok {
		return
	}
	token := strings.TrimSpace(r.FormValue("clubAdmin"))
	if i := strings.LastIndex(token, "/clubs/admin/"); i >= 0 {
		token = token[i+len("/clubs/admin/"):]
	}
	token, _, _ = strings.Cut(token, "?")
	club, err := p.st.ClubByAdmin(r.Context(), token)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		render(w, r, views.ClubLink(views.ClubLinkPage{
			Competition: summary(c.Competition, p.now()), Action: r.URL.Path,
			Problems: []string{"That isn't a club's admin link. Copy it from your club's page, or create the club first."},
		}))
		return
	}
	if err := p.st.AttachClub(r.Context(), club.ID, c.ID); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, clubPath(token)+"?notice="+urlQuery(club.Name+" is entered in "+c.Name+". Members can now add their entries."), http.StatusSeeOther)
}

// --- Members ---

func (p *competitionPages) joinClub(w http.ResponseWriter, r *http.Request) (store.Club, bool) {
	c, err := p.st.ClubByJoinLink(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return c, false
	}
	return c, true
}

func (p *competitionPages) joinForm(w http.ResponseWriter, r *http.Request) {
	club, ok := p.joinClub(w, r)
	if !ok {
		return
	}
	render(w, r, views.ClubJoin(views.JoinPage{Club: club.Name}))
}

func (p *competitionPages) join(w http.ResponseWriter, r *http.Request) {
	club, ok := p.joinClub(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if err := competitions.CheckName("member", name); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		render(w, r, views.ClubJoin(views.JoinPage{Club: club.Name, Name: name, Problems: sentences(err)}))
		return
	}
	if !p.joinLimiter.allow(clientIP(r), p.now()) {
		message(w, r, http.StatusTooManyRequests, "Too many joins", "This address has joined clubs many times in the last hour. Please try again later.")
		return
	}
	_, token, err := p.st.Join(r.Context(), club.ID, name)
	if err != nil {
		if errors.Is(err, store.ErrLimit) {
			message(w, r, http.StatusConflict, "Club full", club.Name+" has as many members as it can take.")
			return
		}
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, memberPath(token)+"?new=joined", http.StatusSeeOther)
}

// member is the member a personal link belongs to, and their club.
func (p *competitionPages) member(w http.ResponseWriter, r *http.Request) (store.Member, bool) {
	m, err := p.st.MemberByLink(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return m, false
	}
	return m, true
}

func (p *competitionPages) memberHome(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	p.renderMember(w, r, m, "", nil, nil)
}

// renderMember shows a member their page. A form just posted with problems
// (for failedID: a competition's id and the discipline, as "<id>/<discipline>")
// is shown open, as posted.
func (p *competitionPages) renderMember(w http.ResponseWriter, r *http.Request, m store.Member, failedID string, posted *competitions.Entry, problems []string) {
	ctx := r.Context()
	club, err := p.memberClub(r, m)
	if err != nil {
		failed(w, r, err)
		return
	}
	comps, err := p.st.ClubCompetitions(ctx, m.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	entries, err := p.st.MemberEntries(ctx, m.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	path := memberPath(r.PathValue("token"))
	page := views.MemberPage{
		Club: club, Member: m.Name, Link: origin(r) + path, New: r.URL.Query().Get("new") == "joined", Notice: r.URL.Query().Get("notice"),
		CoachID: m.CoachID, CoachAction: path + "/coach",
	}
	coaches, err := p.st.Coaches(ctx, m.ClubID)
	if err != nil {
		failed(w, r, err)
		return
	}
	for _, c := range coaches {
		page.Coaches = append(page.Coaches, views.CoachOption{ID: c.ID, Name: c.Name})
	}
	now := p.now()
	seen := p.seenFor(r)
	removals, err := p.st.MemberRemovals(ctx, m.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	for _, c := range comps {
		mc := views.MemberCompetition{ID: c.ID, Competition: summary(c.Competition, now), Duties: dutiesOf(c, "m:"+m.ID), Day: dayPath(path, c.ID),
			Notify: p.notifyLink(path + "/competitions/" + c.ID + "/notify")}
		if len(mc.Duties) > 0 {
			mc.ScoreSheets = path + "/competitions/" + c.ID + "/score-sheets"
		}
		if published(c) {
			mc.Timeline, mc.ClubTimeline = path+"/competitions/"+c.ID+"/timeline", path+"/competitions/"+c.ID+"/club-timeline"
		}
		var sent []store.Entry
		disciplines := c.Disciplines()
		entered := 0
		for _, d := range disciplines {
			ev := views.MemberEvent{
				Discipline: d, Name: competitions.DisciplineName(d), Status: "Not entered",
				Withdraw: path + "/competitions/" + c.ID + "/withdraw", Several: len(disciplines) > 1,
			}
			start := competitions.Entry{Gymnast: m.Name, Discipline: d}
			for _, e := range entries {
				if e.CompetitionID != c.ID || e.Discipline != d {
					continue
				}
				entered++
				start = e.Entry
				if e.Sent() {
					if sent == nil {
						sent, _ = p.st.Entries(ctx, c.ID)
					}
				}
				copyID := ""
				for _, s := range sent {
					if s.MemberID == m.ID && s.Entry.Discipline == d {
						ev.Placement, copyID, ev.Waiting = placement(c, s.ID), s.ID, waitingText(s)
					}
				}
				shown, err := card(c.Competition, e.Entry, club)
				if err != nil {
					failed(w, r, err)
					return
				}
				withSignoff(&shown, memberSignoff(c.Competition, e))
				if warn, err := seen.pairWarning(c.ID, copyID); err != nil {
					failed(w, r, err)
					return
				} else if warn != "" {
					shown.Problems = append([]string{warn}, shown.Problems...)
				}
				ev.Card = &shown
				ev.Note, ev.VideoReview, ev.VideoNote = e.Note, e.VideoReview, e.VideoNote
				if e.Entry.Partner != nil {
					ev.Partner = &views.PartnerView{Name: e.Entry.Partner.Name, Link: origin(r) + partnerPath(e.PartnerLink), Confirmed: e.PartnerConfirmed}
				}
				rem, removed := removals[c.ID+"/"+d]
				switch {
				case removed:
					ev.Status = removalStatus(rem, e.ChangedSinceSent())
					if rem.ForMember() && rem.RemovalNote != "" {
						ev.Note = strings.TrimSpace(ev.Note + " " + rem.RemovalNote)
					}
				case !e.Sent():
					ev.Status = "Saved, not sent yet"
				case e.ChangedSinceSent():
					ev.Status = "Changed since your club sent it"
				case e.Checked:
					ev.Status = "Sent by your club · checked by the organiser ✓"
				default:
					ev.Status = "Sent by your club"
				}
			}
			key := c.ID + "/" + d
			if key == failedID && posted != nil {
				start = *posted
			}
			ev.Form = entryForm(c.Competition, start, path+"/competitions/"+c.ID, "Save")
			ev.Form.Member, ev.Form.Key = true, "-"+c.ID+"-"+d
			if key == failedID {
				ev.Form.Problems = problems
				mc.Open = true
			}
			mc.Events = append(mc.Events, ev)
		}
		switch {
		case len(disciplines) == 1:
			mc.Status = mc.Events[0].Status
		case entered == 1:
			mc.Status = "1 entry"
		default:
			mc.Status = fmt.Sprintf("%d entries", entered)
		}
		offer, err := p.st.MemberOffer(ctx, m.ID, c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		form := offerForm(c.Competition, offer, path+"/competitions/"+c.ID+"/offer")
		mc.Offer = &form
		// Open what needs doing: a competition still open, or one with problems just posted.
		mc.Open = mc.Open || (mc.Competition.Open && len(comps) == 1)
		page.Competitions = append(page.Competitions, mc)
	}
	pairs, err := p.st.MemberPairEntries(ctx, m.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	if page.Pairs, err = seen.pairViews(pairs, nil); err != nil {
		failed(w, r, err)
		return
	}
	if len(problems) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	render(w, r, views.MemberHome(page))
}

// memberClub is the name of a member's club.
func (p *competitionPages) memberClub(r *http.Request, m store.Member) (string, error) {
	return p.st.ClubName(r.Context(), m.ClubID)
}

// memberCompetition is the competition in the URL, if the member's club is entered.
func (p *competitionPages) memberCompetition(w http.ResponseWriter, r *http.Request, m store.Member) (store.Competition, bool) {
	return p.enteredCompetition(w, r, m.ClubID)
}

func (p *competitionPages) saveOwnMemberEntry(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	c, ok := p.memberCompetition(w, r, m)
	if !ok {
		return
	}
	e, problems := postedEntry(r, c.Competition, m.Name)
	if len(problems) > 0 {
		p.renderMember(w, r, m, c.ID+"/"+e.Discipline, &e, problems)
		return
	}
	if err := p.st.SaveMemberEntry(r.Context(), m.ID, c.ID, e); err != nil {
		failed(w, r, err)
		return
	}
	notice := "Entry saved for " + c.Name + ". Your club's competition secretary sends it to the competition."
	http.Redirect(w, r, memberPath(r.PathValue("token"))+"?notice="+urlQuery(notice), http.StatusSeeOther)
}

func (p *competitionPages) withdrawMemberEntry(w http.ResponseWriter, r *http.Request) {
	m, ok := p.member(w, r)
	if !ok {
		return
	}
	c, ok := p.memberCompetition(w, r, m)
	if !ok {
		return
	}
	if r.FormValue("confirm") != "1" {
		http.Redirect(w, r, memberPath(r.PathValue("token")), http.StatusSeeOther)
		return
	}
	if err := p.st.WithdrawMemberEntry(r.Context(), m.ID, c.ID, r.FormValue("discipline")); err != nil {
		failed(w, r, err)
		return
	}
	notice := "Withdrawn from " + c.Name + ". If your club already sent it, it stays with the competition until the club sends again."
	http.Redirect(w, r, memberPath(r.PathValue("token"))+"?notice="+urlQuery(notice), http.StatusSeeOther)
}

// clubSendCoaches are the club's coaches who sign off, to send to a
// competition that approves coaches: those sent are ticked, or before any
// are, the coaches of members entered.
func (p *competitionPages) clubSendCoaches(r *http.Request, club store.Club, competitionID string, coaches []store.Coach,
	quals map[string][]store.CoachQualification, members []store.Member, mine []store.MemberEntry) ([]views.ClubSendCoach, error) {
	sent, err := p.st.ClubSentCoaches(r.Context(), club.ID, competitionID)
	if err != nil {
		return nil, err
	}
	coachOf := map[string]string{}
	for _, m := range members {
		coachOf[m.ID] = m.CoachID
	}
	entered := map[string]bool{}
	for _, e := range mine {
		entered[coachOf[e.MemberID]] = true
	}
	var out []views.ClubSendCoach
	for _, c := range coaches {
		if !c.SignsOff {
			continue
		}
		k := views.ClubSendCoach{ID: c.ID, Name: c.Name, Ticked: entered[c.ID] && len(sent) == 0, NoQualifications: len(quals[c.ID]) == 0}
		if s, ok := sent[c.ID]; ok {
			k.Ticked, k.Status, k.Approved, k.Note, k.Changed = true, coachStatus(s.Status), s.Approved(), s.Note, s.Changed
		}
		out = append(out, k)
	}
	return out, nil
}

// coachStatus says what an organiser has decided about a coach.
func coachStatus(status string) string {
	switch status {
	case store.CoachApproved:
		return "Approved ✓"
	case store.CoachRefused:
		return "Not approved"
	case store.CoachWithdrawn:
		return "Approval withdrawn"
	}
	return "Waiting for the organiser"
}

// sendCoaches sends the coaches ticked (coach: their ids) to a competition.
func (p *competitionPages) sendCoaches(w http.ResponseWriter, r *http.Request) {
	club, ok := p.clubAdmin(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	if err := p.st.SendCoaches(r.Context(), club.ID, r.PathValue("id"), r.Form["coach"]); err != nil {
		failed(w, r, err)
		return
	}
	notice := "Coaches sent: the organiser approves them before they can sign off."
	if len(r.Form["coach"]) == 0 {
		notice = "No coaches sent."
	}
	http.Redirect(w, r, clubPath(r.PathValue("token"))+"?notice="+urlQuery(notice), http.StatusSeeOther)
}

// removalStatus says what the organiser did with an entry: removed, or held
// for changes (changed since, to send again; or sent again, waiting).
func removalStatus(e store.Entry, changed bool) string {
	switch {
	case e.Removal == store.Removed:
		return "Removed by the organiser"
	case e.Resent:
		return "Changed and sent again: waiting for the organiser"
	case changed:
		return "On hold: changed, send it again"
	}
	return "On hold: the organiser asks for changes"
}
