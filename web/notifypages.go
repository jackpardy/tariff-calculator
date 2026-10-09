package web

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"slices"
	"strings"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The pages for notifications (ADR 0008): opting in on a member's,
// individual's, comp sec's or coach's page for a competition; confirming
// an email and turning it off from a link; and the organiser's Notify now
// and No wait on the competition's days.

func (p *competitionPages) registerNotify(handle func(string, http.HandlerFunc)) {
	for _, path := range []string{
		"/clubs/member/{token}/competitions/{id}/notify",
		"/competitions/entry/{token}/notify",
		"/clubs/admin/{token}/competitions/{id}/notify",
		"/clubs/coach/{token}/competitions/{id}/notify",
	} {
		handle("GET "+path, p.notifyPage)
		handle("POST "+path, p.notifyPage)
	}
	handle("GET /notify/confirm/{token}", p.confirmNotify)
	handle("POST /notify/confirm/{token}", p.confirmNotify)
	handle("GET /notify/off/{token}", p.notifyOff)
	handle("POST /notify/off/{token}", p.notifyOff)
	handle("POST /competitions/admin/{token}/notify-now", p.notifyNow)
	handle("POST /competitions/admin/{token}/no-wait", p.noWait)
}

// notifyLink is a link to a notify page, or "" without storage. Push needs
// nothing set up, so there's always a way.
func (p *competitionPages) notifyLink(path string) string {
	if p.notify == nil {
		return ""
	}
	return path
}

// notifyTarget is whose page a notify page is on.
type notifyTarget struct {
	c           store.Competition
	kind, owner string
	page        string // their page for the competition
	who         string // whose changes, e.g. "you"
	yours       bool   // their own, rather than others'
}

// target is the person and competition a notify page's link is for.
func (p *competitionPages) target(w http.ResponseWriter, r *http.Request) (notifyTarget, bool) {
	token := r.PathValue("token")
	switch {
	case strings.HasPrefix(r.URL.Path, "/clubs/member/"):
		m, ok := p.member(w, r)
		if !ok {
			return notifyTarget{}, false
		}
		c, ok := p.memberCompetition(w, r, m)
		return notifyTarget{c, store.NotifyMember, m.ID, dayPath(memberPath(token), c.ID), "you", true}, ok
	case strings.HasPrefix(r.URL.Path, "/competitions/entry/"):
		e, c, ok := p.own(w, r)
		return notifyTarget{c, store.NotifyIndividual, e.ID, "/competitions/entry/" + token, "you", true}, ok
	case strings.HasPrefix(r.URL.Path, "/clubs/admin/"):
		club, ok := p.clubAdmin(w, r)
		if !ok {
			return notifyTarget{}, false
		}
		c, ok := p.enteredCompetition(w, r, club.ID)
		return notifyTarget{c, store.NotifyClub, club.ID, clubPath(token), club.Name + "'s members", false}, ok
	default:
		coach, _, ok := p.coach(w, r)
		if !ok {
			return notifyTarget{}, false
		}
		c, ok := p.enteredCompetition(w, r, coach.ClubID)
		return notifyTarget{c, store.NotifyCoach, coach.ID, coachPath(token), "the gymnasts you coach", false}, ok
	}
}

// topicLabels say what each topic is, for oneself or for others.
func topicLabels(yours bool) map[string]string {
	if yours {
		return map[string]string{
			store.AboutTimetable: "When or where you compete",
			store.AboutDuties:    "Your officiating",
			store.AboutCards:     "Your cards: checked, a note from the organiser, removed or on hold",
		}
	}
	return map[string]string{
		store.AboutTimetable: "When or where they compete",
		store.AboutDuties:    "Their officiating",
		store.AboutCards:     "Their cards: checked, a note from the organiser, removed or on hold",
	}
}

// notifyPage shows and saves how a person hears about a competition.
func (p *competitionPages) notifyPage(w http.ResponseWriter, r *http.Request) {
	t, ok := p.target(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	subs, err := p.st.Subscriptions(ctx, t.c.ID, t.kind, t.owner)
	if err != nil {
		failed(w, r, err)
		return
	}
	var email *store.Subscription
	for i, s := range subs {
		if s.Channel == store.ByEmail {
			email = &subs[i]
		}
	}
	page := views.NotifyPage{Competition: summary(t.c.Competition, p.now()), Back: t.page, Action: r.URL.Path, For: t.who,
		EmailOn: p.notify != nil && p.notify.mail != nil, Notice: r.URL.Query().Get("notice")}
	if r.Method == http.MethodPost {
		switch r.FormValue("action") {
		case "remove":
			if err := p.st.RemoveSubscription(ctx, t.c.ID, t.kind, t.owner, r.FormValue("remove")); err != nil && !errors.Is(err, store.ErrNotFound) {
				failed(w, r, err)
				return
			}
			notice := "Turned off."
			if email != nil && email.ID == r.FormValue("remove") {
				notice = "No more emails about this competition."
			}
			http.Redirect(w, r, r.URL.Path+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
			return
		case "push":
			problems := p.savePush(r, t)
			if len(problems) == 0 {
				http.Redirect(w, r, r.URL.Path+"?notice="+url.QueryEscape("Notifications are on for this phone."), http.StatusSeeOther)
				return
			}
			page.Problems = problems
			w.WriteHeader(http.StatusUnprocessableEntity)
		default:
			notice, problems := p.saveEmail(r, t, email)
			if len(problems) == 0 {
				http.Redirect(w, r, r.URL.Path+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
				return
			}
			page.Problems = problems
			w.WriteHeader(http.StatusUnprocessableEntity)
		}
	}
	labels := topicLabels(t.yours)
	for _, topic := range store.Topics {
		on := email == nil || email.About(topic)
		if r.Method == http.MethodPost {
			on = slices.Contains(r.Form["topic"], topic)
		}
		page.Topics = append(page.Topics, views.NotifyTopic{Value: topic, Label: labels[topic], On: on})
	}
	if email != nil {
		page.Email = views.NotifyEmail{Address: email.Address, Confirmed: email.Confirmed, Remove: email.ID}
	}
	page.PushKey = p.notify.pushPublicKey(ctx)
	for _, topic := range store.Topics {
		page.PushTopics = append(page.PushTopics, views.NotifyTopic{Value: topic, Label: labels[topic], On: true})
	}
	for _, s := range subs {
		if s.Channel == store.ByPush {
			page.Phones = append(page.Phones, views.NotifyPhone{ID: s.ID, Endpoint: s.Address,
				Added: s.CreatedAt.In(local).Format("2 January"), Topics: strings.Join(s.Topics, ", ")})
		}
	}
	if r.Method == http.MethodPost {
		page.Email.Address = r.FormValue("email")
	}
	render(w, r, views.NotifyCompetition(page))
}

// saveEmail saves the email posted, sending a link to confirm it if it's
// new, and says what happened or what's wrong.
func (p *competitionPages) saveEmail(r *http.Request, t notifyTarget, was *store.Subscription) (string, []string) {
	if p.notify == nil || p.notify.mail == nil {
		return "", []string{"Emails can't be sent from here yet."}
	}
	address, ok := emailAddress(r.FormValue("email"))
	var problems []string
	if !ok {
		problems = append(problems, "Give an email address, like name@example.com.")
	}
	if r.FormValue("adult") != "1" {
		problems = append(problems, "Tick to say you're 18 or over, or it's a parent's email.")
	}
	topics := r.Form["topic"]
	if len(topics) == 0 {
		problems = append(problems, "Tick at least one thing to hear about.")
	}
	if len(problems) > 0 {
		return "", problems
	}
	same := was != nil && was.Confirmed && strings.EqualFold(was.Address, address)
	if !same && !p.emailLimiter.allow(clientIP(r), p.now()) {
		return "", []string{"Too many emails asked for from here just now. Try again in an hour."}
	}
	sub, token, err := p.st.Subscribe(r.Context(), store.Subscription{
		CompetitionID: t.c.ID, Kind: t.kind, OwnerID: t.owner, Channel: store.ByEmail, Address: address, Topics: topics, Page: t.page,
	})
	if err != nil {
		return "", []string{"That didn't work. Please try again in a minute."}
	}
	if token == "" {
		return "Saved.", nil
	}
	if err := p.notify.confirmEmail(t.c, sub, t.who); err != nil {
		p.st.RemoveSubscription(r.Context(), t.c.ID, t.kind, t.owner, sub.ID)
		return "", []string{"The email couldn't be sent. Check the address, or try again later."}
	}
	return "We've emailed " + address + " a link to confirm it. Nothing else is sent until it's tapped.", nil
}

// savePush saves this phone's push subscription, as push.js posts it:
// endpoint, p256dh, auth and each topic.
func (p *competitionPages) savePush(r *http.Request, t notifyTarget) []string {
	endpoint := r.FormValue("endpoint")
	keys := pushKeys{P256dh: r.FormValue("p256dh"), Auth: r.FormValue("auth")}
	if !pushEndpoint(endpoint) || len(endpoint) > 1024 || !validPushKeys(keys) {
		return []string{"This browser's notifications can't be used here."}
	}
	topics := r.Form["topic"]
	if len(topics) == 0 {
		return []string{"Tick at least one thing to hear about."}
	}
	k, _ := json.Marshal(keys)
	_, _, err := p.st.Subscribe(r.Context(), store.Subscription{
		CompetitionID: t.c.ID, Kind: t.kind, OwnerID: t.owner, Channel: store.ByPush, Address: endpoint, Keys: string(k), Topics: topics, Page: t.page,
	})
	switch {
	case errors.Is(err, store.ErrLimit):
		return []string{fmt.Sprintf("Notifications are already on for %d phones: turn one off first.", store.MaxPush)}
	case err != nil:
		return []string{"That didn't work. Please try again in a minute."}
	}
	return nil
}

// validPushKeys says whether a browser's push keys are the right shape: a
// P-256 public key and a 16-byte secret.
func validPushKeys(k pushKeys) bool {
	pub, err1 := base64.RawURLEncoding.DecodeString(strings.TrimRight(k.P256dh, "="))
	auth, err2 := base64.RawURLEncoding.DecodeString(strings.TrimRight(k.Auth, "="))
	if err1 != nil || err2 != nil || len(auth) != 16 {
		return false
	}
	_, err := ecdh.P256().NewPublicKey(pub)
	return err == nil
}

// emailAddress is an email address as given, if it is one: a plain
// address, nothing else.
func emailAddress(v string) (string, bool) {
	v = strings.TrimSpace(v)
	a, err := mail.ParseAddress(v)
	if err != nil || a.Name != "" || a.Address != v || len(v) > 254 || strings.ContainsAny(v, "\r\n<>") {
		return "", false
	}
	return v, true
}

// confirmNotify confirms an email from its link: a button, then done.
func (p *competitionPages) confirmNotify(w http.ResponseWriter, r *http.Request) {
	sub, c, ok := p.notifyLinkOf(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		render(w, r, views.NotifyLink("Confirm your email", "Confirm "+sub.Address+" to be emailed about changes at "+c.Name+".", r.URL.Path, "Confirm"))
		return
	}
	if err := p.st.ConfirmSubscription(r.Context(), r.PathValue("token")); err != nil {
		failed(w, r, err)
		return
	}
	render(w, r, views.NotifyLink("Email confirmed", "You'll be emailed about changes at "+c.Name+". Every email has a link to stop them.", "", ""))
}

// notifyOff turns an email off from its link: a button, or at once for a
// mail program's one-click unsubscribe (RFC 8058), which posts.
func (p *competitionPages) notifyOff(w http.ResponseWriter, r *http.Request) {
	sub, c, ok := p.notifyLinkOf(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		render(w, r, views.NotifyLink("Stop these emails", "Stop emailing "+sub.Address+" about changes at "+c.Name+"?", r.URL.Path, "Stop them"))
		return
	}
	if err := p.st.Unsubscribe(r.Context(), r.PathValue("token")); err != nil {
		failed(w, r, err)
		return
	}
	render(w, r, views.NotifyLink("Stopped", "No more emails about "+c.Name+".", "", ""))
}

// notifyLinkOf is the subscription and competition a confirm or off link is
// for.
func (p *competitionPages) notifyLinkOf(w http.ResponseWriter, r *http.Request) (store.Subscription, store.Competition, bool) {
	sub, err := p.st.SubscriptionByToken(r.Context(), r.PathValue("token"))
	if errors.Is(err, store.ErrNotFound) {
		message(w, r, http.StatusNotFound, "Link not found", "This link doesn't lead anywhere: the emails may already be stopped, or the competition deleted.")
		return store.Subscription{}, store.Competition{}, false
	}
	if err != nil {
		failed(w, r, err)
		return store.Subscription{}, store.Competition{}, false
	}
	c, err := p.st.Competition(r.Context(), sub.CompetitionID)
	if err != nil {
		failed(w, r, err)
		return store.Subscription{}, store.Competition{}, false
	}
	return sub, c, true
}

// notifyNow sends the changes waiting at once.
func (p *competitionPages) notifyNow(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := p.st.NotifyNow(r.Context(), c.ID); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"?notice="+url.QueryEscape("Telling everyone who asked, within a minute."), http.StatusSeeOther)
}

// noWait turns the wait off on the competition's days (on=1), or back on.
func (p *competitionPages) noWait(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	on := r.FormValue("on") == "1"
	if err := p.st.SetNoWait(r.Context(), c.ID, on); err != nil {
		failed(w, r, err)
		return
	}
	notice := "Changes wait 10 minutes before they're told, every day."
	if on {
		notice = "On the competition's days, changes are told as soon as they're published."
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}
