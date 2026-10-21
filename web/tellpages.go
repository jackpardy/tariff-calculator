package web

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Messages to the organiser (migration 48): a member, coach, comp sec or
// individual tells a competition's organiser something from their own page
// for it, named from their link (never typed), and about one of their
// entries if they choose. It joins the concerns on the organiser's
// dashboard. Feedback about the app is something else: it goes to the
// people who make the app (feedback.go).

// maxMessage is the most a message to the organiser can say, in characters.
const maxMessage = 1000

// tellRoots are the pages a message can be sent from: a person's or club's
// page for a competition, as whose knows them.
var tellRoots = []string{
	"/clubs/member/{token}/competitions/{id}",
	"/clubs/admin/{token}/competitions/{id}",
	"/clubs/coach/{token}/competitions/{id}",
	"/competitions/entry/{token}",
}

func (p *competitionPages) registerTell(handle func(string, http.HandlerFunc)) {
	for _, root := range tellRoots {
		handle("POST "+root+"/organiser", p.tellOrganiser)
		handle("POST "+root+"/organiser/{message}", p.addToMessage)
	}
	handle("POST "+adminPrefix+"/messages", p.messages)
}

// messages lets people send the organiser messages, or stops them.
func (p *competitionPages) messages(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	on := r.FormValue("on") == "1"
	if err := p.st.SetMessages(r.Context(), c.ID, on); err != nil {
		failed(w, r, err)
		return
	}
	notice := "No one can send you messages from their pages now. Those sent already stay on your dashboard."
	if on {
		notice = "Members, coaches, comp secs and gymnasts can send you messages from their pages."
	}
	backToSettings(w, r, notice)
}

// teller is who a message is from, and which of the competition's entries
// it can be about: theirs, their club's, or their coach's gymnasts'; and
// for a comp sec or coach (events), whole events of those.
type teller struct {
	from   store.Sender
	sees   func(store.Entry) bool
	events bool
}

// memberTeller is a club's member, whose entries are those they're in.
func memberTeller(m store.Member, club string) *teller {
	return &teller{from: store.Sender{Role: store.RoleMember, Name: m.Name, Club: club, ID: "member:" + m.ID}, sees: hasPerson(store.MemberKey(m.ID))}
}

// individualTeller is a gymnast entering on their own, known by an entry.
func individualTeller(e store.Entry) *teller {
	return &teller{from: store.Sender{Role: store.RoleIndividual, Name: e.Entry.Gymnast, ID: "individual:" + e.Key()}, sees: hasPerson(e.Key())}
}

// clubTeller is a club's comp sec: every entry of the club's.
func clubTeller(clubID, club string) *teller {
	return &teller{from: store.Sender{Role: store.RoleClub, Name: club, Club: club, ID: "club:" + clubID},
		sees: func(e store.Entry) bool { return e.ClubID == clubID }, events: true}
}

// coachTeller is a club's coach: the entries of the members they see
// (members, by id).
func coachTeller(coach store.Coach, club string, members map[string]bool) *teller {
	return &teller{from: store.Sender{Role: store.RoleCoach, Name: coach.Name, Club: club, ID: "coach:" + coach.ID},
		sees: func(e store.Entry) bool { return e.ClubID == coach.ClubID && members[e.MemberID] }, events: true}
}

// coachSees are the members whose entries a coach sees, by id.
func coachSees(entries []store.MemberEntry) map[string]bool {
	out := map[string]bool{}
	for _, e := range entries {
		out[e.MemberID] = true
	}
	return out
}

// hasPerson says whether an entry is the person's (key), or they're in it.
func hasPerson(key string) func(store.Entry) bool {
	return func(e store.Entry) bool { return slices.Contains(e.People(), key) }
}

// tellAbout are the competition's entries a message can be about.
func (p *competitionPages) tellAbout(ctx context.Context, c store.Competition, t *teller) ([]store.Entry, error) {
	entries, err := p.entriesOf(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	var out []store.Entry
	for _, e := range entries {
		if t.sees(e) {
			out = append(out, e)
		}
	}
	return out, nil
}

// maxAboutPlain is how many entries the About list shows without a
// search box.
const maxAboutPlain = 8

// tellForm is the form for telling the organiser, posting under base, and
// the messages they've sent; nil with neither (the organiser has messages
// off and they've sent none).
func (p *competitionPages) tellForm(ctx context.Context, c store.Competition, base string, t *teller) *views.TellOrganiser {
	if t == nil {
		return nil
	}
	// Without the entries (the facts can't be read), it's about the
	// competition: the page still shows.
	about, _ := p.tellAbout(ctx, c, t)
	sent, _ := p.st.SentBy(ctx, c.ID, t.from.ID)
	if !c.Messages && len(sent) == 0 {
		return nil
	}
	f := &views.TellOrganiser{ID: "tell-organiser-" + c.ID, From: senderName(t.from), Events: t.events, Search: len(about) > maxAboutPlain}
	labels := map[string]string{}
	for _, e := range about {
		labels[e.ID] = entryLabel(e)
	}
	for _, k := range sent {
		m := sentView(k, labels)
		if c.Messages {
			m.Add = base + "/organiser/" + k.ID
		}
		f.Sent = append(f.Sent, m)
	}
	if !c.Messages {
		return f
	}
	// The Feedback button points to it, as what reaches the organiser.
	if fb := views.FeedbackFormOf(ctx); fb != nil {
		fb.Organiser = true
	}
	f.Action = base + "/organiser"
	for _, group := range byEvent(about) {
		g := views.AboutGroup{Event: group[0].Entry.Event()}
		for _, e := range group {
			label := entryLabel(e)
			if t.events {
				label = e.Entry.Gymnasts() // under its event
			}
			g.Entries = append(g.Entries, views.ListOption{Value: e.ID, Label: label})
		}
		f.About = append(f.About, g)
	}
	return f
}

// byEvent are entries grouped by event, in the order the events first come
// in, each event's by gymnast.
func byEvent(entries []store.Entry) [][]store.Entry {
	var out [][]store.Entry
	at := map[string]int{}
	for _, e := range entries {
		ev := e.Entry.Event()
		i, ok := at[ev]
		if !ok {
			i = len(out)
			at[ev] = i
			out = append(out, nil)
		}
		out[i] = append(out[i], e)
	}
	for _, group := range out {
		slices.SortStableFunc(group, func(a, b store.Entry) int { return strings.Compare(a.Entry.Gymnasts(), b.Entry.Gymnasts()) })
	}
	return out
}

// aboutText is what a message is about, in words: its entries (by label,
// those still there) and events; "" for the competition generally.
func aboutText(k store.Concern, labels map[string]string) string {
	var parts []string
	for _, ev := range k.Events {
		parts = append(parts, "all of "+ev)
	}
	for _, id := range k.Entries {
		if l, ok := labels[id]; ok {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, "; ")
}

// sentView is a message as its sender sees it again.
func sentView(k store.Concern, labels map[string]string) views.SentMessage {
	m := views.SentMessage{ID: k.ID, Text: k.Text, About: aboutText(k, labels), At: k.At.In(local).Format("Mon 2 Jan, 15:04"), Status: "Sent", StatusState: views.NotYet}
	switch {
	case !k.Open():
		m.Status, m.StatusState = "Resolved", views.Done
	case k.Acknowledged():
		m.Status, m.StatusState = "Received", views.InProgress
	}
	for _, p := range k.Thread {
		m.Thread = append(m.Thread, views.SentPost{Text: p.Text, At: p.At.In(local).Format("Mon 2 Jan, 15:04"), Organiser: !p.BySender})
	}
	return m
}

// entryLabel names an entry in a list: its gymnasts and event.
func entryLabel(e store.Entry) string { return e.Entry.Gymnasts() + " · " + e.Entry.Event() }

// senderName is who sent a message, as the organiser and the sender see it.
func senderName(s store.Sender) string {
	switch s.Role {
	case store.RoleMember:
		return s.Name + ", a member of " + s.Club
	case store.RoleCoach:
		return s.Name + ", a coach at " + s.Club
	case store.RoleClub:
		return s.Club + "'s comp sec"
	case store.RoleIndividual:
		return s.Name + ", entered on their own"
	}
	return s.Name
}

// tellOrganiser keeps a message to the organiser (text; entry, any number,
// for entries of theirs; event for a comp sec's or coach's whole events),
// and goes back to the page it was sent from.
func (p *competitionPages) tellOrganiser(w http.ResponseWriter, r *http.Request) {
	a, ok := p.whose(w, r)
	if !ok {
		return
	}
	t, ok := p.tellerOf(w, r, a)
	if !ok {
		return
	}
	back := tellBack(p, r, a)
	if !a.c.Messages {
		seeOther(w, r, back, "The organiser isn't taking messages here just now; nothing was sent.")
		return
	}
	text := limitMessage(r.FormValue("text"))
	if text == "" {
		seeOther(w, r, back, "Say what you want to tell the organiser; nothing was sent.")
		return
	}
	about, ok := p.aboutPosted(w, r, a, t)
	if !ok {
		return
	}
	_, err := p.st.TellOrganiser(r.Context(), a.c.ID, about, t.from, text)
	switch {
	case errors.Is(err, store.ErrLimit):
		seeOther(w, r, back, "You've sent the organiser a lot today, so this wasn't sent. Try again tomorrow, or speak to them.")
		return
	case err != nil:
		failed(w, r, err)
		return
	}
	seeOther(w, r, back, "Sent to the organiser of "+a.c.Name+", with your name.")
}

// addToMessage adds the sender's words (text) to a message they sent, which
// opens it again if it was resolved, and goes back to it.
func (p *competitionPages) addToMessage(w http.ResponseWriter, r *http.Request) {
	a, ok := p.whose(w, r)
	if !ok {
		return
	}
	t, ok := p.tellerOf(w, r, a)
	if !ok {
		return
	}
	back := tellBackFrom(p, r, a, "/organiser/"+r.PathValue("message"))
	if !a.c.Messages {
		seeOther(w, r, back, "The organiser isn't taking messages here just now; nothing was sent.")
		return
	}
	text := limitMessage(r.FormValue("text"))
	if text == "" {
		seeOther(w, r, back, "Write what to add first; nothing was sent.")
		return
	}
	_, err := p.st.AddToMessage(r.Context(), a.c.ID, r.PathValue("message"), t.from.ID, text)
	switch {
	case errors.Is(err, store.ErrLimit):
		seeOther(w, r, back, "That message is as long as it can be; send a new one, or speak to the organiser.")
		return
	case err != nil:
		failed(w, r, err)
		return
	}
	seeOther(w, r, back+"#message-"+r.PathValue("message"), "Added, for the organiser to see.")
}

// aboutPosted is what a posted message is about: entries of theirs, and
// (a comp sec or coach) events of those, without the entries an event
// covers. Anything else is a page that isn't there.
func (p *competitionPages) aboutPosted(w http.ResponseWriter, r *http.Request, a asker, t *teller) (store.About, bool) {
	var about store.About
	entries, events := r.Form["entry"], r.Form["event"]
	if len(entries) == 0 && len(events) == 0 {
		return about, true
	}
	if len(events) > 0 && !t.events {
		failed(w, r, store.ErrNotFound)
		return about, false
	}
	theirs, err := p.tellAbout(r.Context(), a.c, t)
	if err != nil {
		failed(w, r, err)
		return about, false
	}
	byID := map[string]store.Entry{}
	evs := map[string]bool{}
	for _, e := range theirs {
		byID[e.ID] = e
		evs[e.Entry.Event()] = true
	}
	for _, ev := range events {
		if !evs[ev] {
			failed(w, r, store.ErrNotFound)
			return about, false
		}
		if !slices.Contains(about.Events, ev) {
			about.Events = append(about.Events, ev)
		}
	}
	for _, id := range entries {
		e, ok := byID[id]
		if !ok {
			failed(w, r, store.ErrNotFound)
			return about, false
		}
		if !slices.Contains(about.Events, e.Entry.Event()) && !slices.Contains(about.Entries, id) {
			about.Entries = append(about.Entries, id)
		}
	}
	return about, true
}

// tellBack is the page a message goes back to: the one it was sent from,
// which may be My competition or an individual's page for the competition
// (the form's back) rather than the page it posts under.
func tellBack(p *competitionPages, r *http.Request, a asker) string {
	return tellBackFrom(p, r, a, "/organiser")
}

// tellBackFrom is tellBack for a route under the page (route), as
// "/organiser" or "/organiser/{message}" is.
func tellBackFrom(p *competitionPages, r *http.Request, a asker, route string) string {
	switch {
	case a.kind == store.NotifyCoach:
		return a.back
	case r.FormValue("back") == "day" && a.day != "":
		return a.day
	case r.FormValue("back") == "gymnast" && a.kind == store.NotifyIndividual && a.entry.IndividualID != "":
		return gymnastCompetitionPath(p.gymnastBase(r, a.entry), a.c.ID)
	}
	return strings.TrimSuffix(r.URL.Path, route)
}

// tellerOf is who the asker is, as a message's sender.
func (p *competitionPages) tellerOf(w http.ResponseWriter, r *http.Request, a asker) (*teller, bool) {
	ctx := r.Context()
	if a.kind == store.NotifyCoach {
		coach, club, ok := p.coach(w, r)
		if !ok {
			return nil, false
		}
		entries, err := p.st.CoachEntries(ctx, coach)
		if err != nil {
			failed(w, r, err)
			return nil, false
		}
		return coachTeller(coach, club, coachSees(entries)), true
	}
	club := ""
	if a.kind == store.NotifyMember {
		var err error
		if club, err = p.st.ClubName(ctx, a.club); err != nil {
			failed(w, r, err)
			return nil, false
		}
	}
	return tellerFor(a, club), true
}

// tellerFor is who a member (with their club's name), individual or comp sec
// asking is, as a message's sender.
func tellerFor(a asker, club string) *teller {
	switch a.kind {
	case store.NotifyMember:
		return memberTeller(store.Member{ID: a.owner, Name: a.name}, club)
	case store.NotifyIndividual:
		return individualTeller(a.entry)
	}
	return clubTeller(a.owner, a.name)
}
