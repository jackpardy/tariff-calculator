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
// it can be about: theirs, their club's, or their coach's gymnasts'.
type teller struct {
	from store.Sender
	sees func(store.Entry) bool
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
		sees: func(e store.Entry) bool { return e.ClubID == clubID }}
}

// coachTeller is a club's coach: the entries of the members they see
// (members, by id).
func coachTeller(coach store.Coach, club string, members map[string]bool) *teller {
	return &teller{from: store.Sender{Role: store.RoleCoach, Name: coach.Name, Club: club, ID: "coach:" + coach.ID},
		sees: func(e store.Entry) bool { return e.ClubID == coach.ClubID && members[e.MemberID] }}
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

// tellForm is the form for telling the organiser, posting under base; nil
// while the organiser has messages off.
func (p *competitionPages) tellForm(ctx context.Context, c store.Competition, base string, t *teller) *views.TellOrganiser {
	if t == nil || !c.Messages {
		return nil
	}
	// The Feedback button points to it, as what reaches the organiser.
	if fb := views.FeedbackFormOf(ctx); fb != nil {
		fb.Organiser = true
	}
	f := &views.TellOrganiser{ID: "tell-organiser-" + c.ID, Action: base + "/organiser", From: senderName(t.from)}
	// Without the entries (the facts can't be read), it's about the
	// competition: the page still shows.
	about, _ := p.tellAbout(ctx, c, t)
	for _, e := range about {
		f.About = append(f.About, views.ListOption{Value: e.ID, Label: entryLabel(e)})
	}
	return f
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

// tellOrganiser keeps a message to the organiser (text, and entry for one
// about an entry of theirs), and goes back to the page it was sent from.
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
	text := strings.TrimSpace(r.FormValue("text"))
	if rs := []rune(text); len(rs) > maxMessage {
		text = string(rs[:maxMessage])
	}
	if text == "" {
		seeOther(w, r, back, "Say what you want to tell the organiser; nothing was sent.")
		return
	}
	entryID := r.FormValue("entry")
	if entryID != "" {
		about, err := p.tellAbout(r.Context(), a.c, t)
		if err != nil {
			failed(w, r, err)
			return
		}
		if !slices.ContainsFunc(about, func(e store.Entry) bool { return e.ID == entryID }) {
			failed(w, r, store.ErrNotFound)
			return
		}
	}
	_, err := p.st.TellOrganiser(r.Context(), a.c.ID, entryID, t.from, text)
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

// tellBack is the page a message goes back to: the one it was sent from,
// which may be My competition or an individual's page for the competition
// (the form's back) rather than the page it posts under.
func tellBack(p *competitionPages, r *http.Request, a asker) string {
	switch {
	case a.kind == store.NotifyCoach:
		return a.back
	case r.FormValue("back") == "day" && a.day != "":
		return a.day
	case r.FormValue("back") == "gymnast" && a.kind == store.NotifyIndividual && a.entry.IndividualID != "":
		return gymnastCompetitionPath(p.gymnastBase(r, a.entry), a.c.ID)
	}
	return strings.TrimSuffix(r.URL.Path, "/organiser")
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
