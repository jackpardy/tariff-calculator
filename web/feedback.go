package web

import (
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Feedback about the app (roadmap, ready for the dry run): a Feedback button
// at the foot of every page someone uses for a competition, posting to their
// own link's feedback route, so it's saved with their role, the page (its
// route pattern: never the path, which holds the link) and the time. The
// organiser reads it on the Feedback page and turns the button off in
// Settings; FEEDBACK_EMAIL, with email set up, sends each one to the
// maintainer too.

// feedbackPages are the pages with a Feedback button, by route pattern, with
// what the Feedback page calls them. Not the venue screen, printouts,
// downloads or pages about more than one competition.
var feedbackPages = map[string]string{
	adminPrefix:                                                     "Entries",
	adminPrefix + "/entries/{id}":                                   "An entry",
	adminPrefix + "/people":                                         "People",
	adminPrefix + "/people/{key}":                                   "A person",
	adminPrefix + "/late":                                           "Late changes",
	adminPrefix + "/coaches":                                        "Coaches",
	adminPrefix + "/fees":                                           "Fees",
	adminPrefix + "/timetable":                                      "Timetable",
	adminPrefix + "/officials":                                      "Officials",
	adminPrefix + "/timetable/simulate":                             "Simulate",
	adminPrefix + "/timetable/delay":                                "What if there's a delay",
	adminPrefix + "/timetable/leave":                                "What if an official leaves",
	adminPrefix + "/day":                                            "On the day",
	adminPrefix + "/desk":                                           "Desk messages",
	adminPrefix + "/settings":                                       "Settings",
	adminPrefix + "/history":                                        "History",
	adminPrefix + "/feedback":                                       "Feedback",
	"/competitions/club/{token}":                                    "Entering a club",
	"/clubs/admin/{token}/members/{member}/competitions/{id}":       "A member's entry",
	"/clubs/admin/{token}/members/{member}/competitions/{id}/offer": "A member's offer to help",
	"/clubs/admin/{token}/members/{member}/competitions/{id}/late":  "A late change",
	"/clubs/admin/{token}/competitions/{id}/notify":                 "Hearing about changes",
	"/clubs/admin/{token}/competitions/{id}/timeline":               "The club's timetable",
	"/clubs/member/{token}/competitions/{id}":                       "Their entries",
	"/clubs/member/{token}/competitions/{id}/day":                   "My competition",
	"/clubs/member/{token}/competitions/{id}/late":                  "A late change",
	"/clubs/member/{token}/competitions/{id}/notify":                "Hearing about changes",
	"/clubs/member/{token}/competitions/{id}/timeline":              "Their timetable",
	"/clubs/member/{token}/competitions/{id}/club-timeline":         "The club's timetable",
	"/clubs/coach/{token}/members/{member}/competitions/{id}":       "Signing off an entry",
	"/clubs/coach/{token}/competitions/{id}/notify":                 "Hearing about changes",
	"/clubs/coach/{token}/competitions/{id}/timeline":               "The club's timetable",
	"/competitions/enter/{token}":                                   "Entering on their own",
	"/competitions/entry/{token}":                                   "Their entry",
	"/competitions/entry/{token}/day":                               "My competition",
	"/competitions/entry/{token}/late":                              "A late change",
	"/competitions/entry/{token}/notify":                            "Hearing about changes",
	"/competitions/entry/{token}/timeline":                          "Their timetable",
	"/competitions/gymnast/{token}/competitions/{id}":               "Their entries",
	"/competitions/gymnast/{token}/competitions/{id}/enter":         "Entering on their own",
	"/competitions/partner/{token}":                                 "Confirming a synchro partner",
	"/competitions/signoff/{token}":                                 "Signing off an entry",
	"/competitions/signoff/{token}/late":                            "Signing off a late change",
}

// feedbackRoots are the links feedback is posted under, each with the role
// it's saved with (an admin link's is the link's own).
var feedbackRoots = map[string]string{
	adminPrefix:                     "",
	"/clubs/admin/{token}":          "Comp sec",
	"/competitions/club/{token}":    "Comp sec",
	"/clubs/member/{token}":         "Member",
	"/clubs/coach/{token}":          "Coach",
	"/competitions/enter/{token}":   "Individual",
	"/competitions/entry/{token}":   "Individual",
	"/competitions/gymnast/{token}": "Individual",
	"/competitions/partner/{token}": "Synchro partner",
	"/competitions/signoff/{token}": "An individual's coach",
}

// Feedback's limits: what's said, and the name or email to reply to.
const (
	maxFeedbackText  = 2000
	maxFeedbackReply = 200
)

func (p *competitionPages) registerFeedback(handle func(string, http.HandlerFunc)) {
	for root := range feedbackRoots {
		handle("POST "+root+"/feedback", p.sendFeedback)
	}
	handle("GET "+adminPrefix+"/feedback", p.feedbackPage)
	handle("POST "+adminPrefix+"/feedback/{id}/done", p.markFeedback)
	handle("POST "+adminPrefix+"/feedback-button", p.feedbackButton)
}

// withFeedback keeps a Feedback button for a page that can have one, which
// the page's competition then turns on (feedbackOn).
func withFeedback(r *http.Request) *http.Request {
	_, pattern, _ := strings.Cut(r.Pattern, " ")
	if _, ok := feedbackPages[pattern]; !ok {
		return r
	}
	parts := strings.SplitN(r.URL.Path, "/", 5) // "", the section, the kind of link, its token, the rest
	if len(parts) < 4 {
		return r
	}
	q := r.URL.Query()
	q.Del("notice")
	back := r.URL.Path
	if len(q) > 0 {
		back += "?" + q.Encode()
	}
	f := &views.FeedbackForm{Action: strings.Join(parts[:4], "/") + "/feedback", Page: pattern, Back: back}
	return r.WithContext(views.WithFeedback(r.Context(), f))
}

// feedbackOn shows the page's Feedback button if its competition has it on.
func feedbackOn(r *http.Request, c store.Competition) {
	if f := views.FeedbackFormOf(r.Context()); f != nil {
		f.Competition, f.On = c.ID, c.FeedbackButton
	}
}

// feedbackRoot is the link a feedback route is under, e.g.
// "/clubs/member/{token}".
func feedbackRoot(r *http.Request) string {
	_, pattern, _ := strings.Cut(r.Pattern, " ")
	return strings.TrimSuffix(pattern, "/feedback")
}

// feedbackCompetition is the competition feedback is about, if the link
// it's posted under can see it: the link's own, or for a club's, member's,
// coach's or individual's link the one posted, where they're entered.
func (p *competitionPages) feedbackCompetition(w http.ResponseWriter, r *http.Request) (store.Competition, string, bool) {
	ctx, id := r.Context(), r.FormValue("competition")
	root := feedbackRoot(r)
	role := feedbackRoots[root]
	entered := func(clubID string) bool {
		in, err := p.st.ClubEntered(ctx, clubID, id)
		return err == nil && in
	}
	var c store.Competition
	ok := false
	switch root {
	case adminPrefix:
		c, ok = p.admin(w, r)
		if l := linkOf(r); l.Kind == store.LinkOrganiser {
			role = "Organiser"
		} else {
			role = "Helper link: " + l.Name + " (" + linkKinds[l.Kind] + ")"
		}
		return c, role, ok
	case "/competitions/club/{token}":
		c, ok = p.clubLinkCompetition(w, r)
		return c, role, ok
	case "/competitions/enter/{token}":
		c, ok = p.individual(w, r)
		return c, role, ok
	case "/competitions/entry/{token}":
		_, c, ok = p.own(w, r)
		return c, role, ok
	case "/competitions/partner/{token}":
		_, c, ok = p.invite(w, r)
		return c, role, ok
	case "/competitions/signoff/{token}":
		_, c, ok = p.signoffEntry(w, r)
		return c, role, ok
	case "/clubs/admin/{token}":
		club, found := p.clubAdmin(w, r)
		if !found {
			return c, role, false
		}
		ok = entered(club.ID)
	case "/clubs/member/{token}":
		m, found := p.member(w, r)
		if !found {
			return c, role, false
		}
		ok = entered(m.ClubID)
	case "/clubs/coach/{token}":
		coach, _, found := p.coach(w, r)
		if !found {
			return c, role, false
		}
		ok = entered(coach.ClubID)
		if !ok {
			// A synchro partner's coach sees the pair's entry from another club.
			entries, err := p.st.CoachEntries(ctx, coach)
			ok = err == nil && slices.ContainsFunc(entries, func(e store.MemberEntry) bool { return e.CompetitionID == id })
		}
	case "/competitions/gymnast/{token}":
		_, entries, found := p.gymnast(w, r)
		if !found {
			return c, role, false
		}
		ok = slices.ContainsFunc(entries, func(e store.Entry) bool { return e.CompetitionID == id })
	}
	if !ok {
		failed(w, r, store.ErrNotFound)
		return c, role, false
	}
	c, err := p.st.Competition(ctx, id)
	if err != nil {
		failed(w, r, err)
		return c, role, false
	}
	return c, role, true
}

// sendFeedback keeps feedback about the app (text, optional reply and the
// page's pattern), emails it to FEEDBACK_EMAIL if set, and goes back to the
// page. The page's script sends it in the background (X-Feedback), and gets
// back what to say.
func (p *competitionPages) sendFeedback(w http.ResponseWriter, r *http.Request) {
	c, role, ok := p.feedbackCompetition(w, r)
	if !ok {
		return
	}
	fetched := r.Header.Get("X-Feedback") != ""
	refuse := func(status int, why string) {
		if fetched {
			http.Error(w, why, status)
			return
		}
		message(w, r, status, "Feedback not sent", why)
	}
	text := strings.TrimSpace(r.FormValue("text"))
	reply := strings.Join(strings.Fields(r.FormValue("reply")), " ") // one line: it may be an email header
	page := r.FormValue("page")
	switch {
	case !c.FeedbackButton:
		refuse(http.StatusForbidden, "The organiser has turned feedback off for this competition.")
		return
	case text == "":
		refuse(http.StatusUnprocessableEntity, "Say what happened, or what you think, then send it.")
		return
	case len([]rune(text)) > maxFeedbackText:
		refuse(http.StatusUnprocessableEntity, fmt.Sprintf("That's too long to send: keep it under %d characters.", maxFeedbackText))
		return
	case len([]rune(reply)) > maxFeedbackReply:
		refuse(http.StatusUnprocessableEntity, fmt.Sprintf("The name or email is too long: keep it under %d characters.", maxFeedbackReply))
		return
	}
	if _, known := feedbackPages[page]; !known {
		page = "" // never anything but a known page's pattern
	}
	f, err := p.st.AddFeedback(r.Context(), c.ID, store.Feedback{Text: text, ReplyTo: reply, Role: role, Page: page})
	if err == store.ErrLimit {
		refuse(http.StatusTooManyRequests, "This competition has had a lot of feedback today: please try again tomorrow.")
		return
	}
	if err != nil {
		failed(w, r, err)
		return
	}
	p.emailFeedback(c, f)
	if fetched {
		fmt.Fprint(w, "Thanks: your feedback is sent.")
		return
	}
	// Back to the page, only ever one of the same link's.
	link, back := strings.TrimSuffix(r.URL.Path, "/feedback"), r.FormValue("back")
	if back != link && !strings.HasPrefix(back, link+"/") && !strings.HasPrefix(back, link+"?") {
		back = link
	}
	back, _, _ = strings.Cut(back, "#")
	http.Redirect(w, r, back+"#feedback-sent", http.StatusSeeOther)
}

// emailFeedback emails feedback to FEEDBACK_EMAIL, when it's set and email
// is, without waiting for it.
func (p *competitionPages) emailFeedback(c store.Competition, f store.Feedback) {
	if p.feedbackTo == "" || p.notify == nil || p.notify.mail == nil {
		return
	}
	mail, to := p.notify.mail, p.feedbackTo
	subject, body, headers := feedbackEmail(c, f)
	go func() {
		if err := mail.send(to, subject, body, headers); err != nil {
			log.Printf("Emailing feedback about competition %s: %v", c.ID, err)
		}
	}()
}

// feedbackEmail is the email of a piece of feedback: what was said, who by
// (their role), on which page and when, replying to them if they gave an
// email. Never a link: they're secret.
func feedbackEmail(c store.Competition, f store.Feedback) (subject, body string, headers map[string]string) {
	page := feedbackPages[f.Page]
	if page == "" {
		page = "not known"
	} else {
		page += " (" + f.Page + ")"
	}
	body = f.Text + "\n\n" +
		"Competition: " + c.Name + "\n" +
		"From: " + f.Role + "\n" +
		"Page: " + page + "\n" +
		"At: " + f.At.In(local).Format("Mon 2 January 2006, 15:04") + "\n"
	if f.ReplyTo != "" {
		body += "Reply to: " + f.ReplyTo + "\n"
		if a, ok := emailAddress(f.ReplyTo); ok {
			headers = map[string]string{"Reply-To": a}
		}
	}
	return "Feedback: " + c.Name, body, headers
}

// feedbackPage lists the competition's feedback, newest first.
func (p *competitionPages) feedbackPage(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	all, err := p.st.FeedbackFor(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	page := views.FeedbackPage{Competition: summary(c.Competition, p.now()), Menu: p.menuFor(r, c, menuFeedback),
		Base: adminPath(r.PathValue("token")), Notice: r.URL.Query().Get("notice"), On: c.FeedbackButton}
	for _, f := range all {
		name := feedbackPages[f.Page]
		if name == "" {
			name = "Page not known"
		}
		page.Items = append(page.Items, views.FeedbackItem{ID: f.ID, At: f.At.In(local).Format("Mon 2 Jan, 15:04"), Role: f.Role, Page: name,
			Text: f.Text, ReplyTo: f.ReplyTo, Done: f.Done(), DoneBy: f.DoneBy})
		if !f.Done() {
			page.Waiting++
		}
	}
	render(w, r, views.Feedback(page))
}

// markFeedback marks feedback dealt with (done=1), or not.
func (p *competitionPages) markFeedback(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := p.st.MarkFeedback(r.Context(), c.ID, r.PathValue("id"), linkOf(r).Name, r.FormValue("done") == "1"); err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, adminPath(r.PathValue("token"))+"/feedback", "")
}

// feedbackButton turns the Feedback button on every page on (on=1) or off.
func (p *competitionPages) feedbackButton(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	on := r.FormValue("on") == "1"
	if err := p.st.SetFeedbackButton(r.Context(), c.ID, on); err != nil {
		failed(w, r, err)
		return
	}
	notice := "The Feedback button is off."
	if on {
		notice = "The Feedback button is on every page."
	}
	backToSettings(w, r, notice)
}
