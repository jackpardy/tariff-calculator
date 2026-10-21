package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The organiser's answers to messages (migration 49): they say they've
// received one, reply (the sender can add to it too: a thread, tellpages.go),
// and resolve it, with a reply and a note of their own if they like. The
// sender sees each on their page (tellForm's Sent), and hears of it by push
// or email if they've asked to hear about changes.

func (p *competitionPages) registerReplies(handle func(string, http.HandlerFunc)) {
	handle("POST "+adminPrefix+"/concerns/{id}/received", p.receivedMessage)
	handle("POST "+adminPrefix+"/concerns/{id}/reply", p.replyToMessage)
}

// concernsBack is the dashboard's messages and concerns.
func concernsBack(r *http.Request) string { return adminPath(r.PathValue("token")) + "#concerns" }

// receivedMessage tells a message's sender the organiser has it.
func (p *competitionPages) receivedMessage(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	err := p.st.Acknowledge(r.Context(), c.ID, r.PathValue("id"), linkOf(r).Name)
	if errors.Is(err, store.ErrNotFound) {
		seeOther(w, r, concernsBack(r), "That was marked received already; nothing was changed.")
		return
	}
	if err != nil {
		failed(w, r, err)
		return
	}
	k, err := p.st.Concern(r.Context(), c.ID, r.PathValue("id"))
	if err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, concernsBack(r), "Marked received. "+p.notify.tellSender(r.Context(), c, k, senderReceived, ""))
}

// replyToMessage keeps the organiser's reply (reply), which the sender sees.
func (p *competitionPages) replyToMessage(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	reply := limitMessage(r.FormValue("reply"))
	if reply == "" {
		seeOther(w, r, concernsBack(r), "Write the reply first; nothing was sent.")
		return
	}
	k, ok := p.reply(w, r, c, reply)
	if !ok {
		return
	}
	seeOther(w, r, concernsBack(r), "Reply sent. "+p.notify.tellSender(r.Context(), c, k, senderReplied, reply))
}

// reply keeps a reply to a message and returns the message as it now is;
// not ok, having answered, when it can't.
func (p *competitionPages) reply(w http.ResponseWriter, r *http.Request, c store.Competition, reply string) (store.Concern, bool) {
	_, err := p.st.ReplyTo(r.Context(), c.ID, r.PathValue("id"), linkOf(r).Name, reply)
	switch {
	case errors.Is(err, store.ErrLimit):
		seeOther(w, r, concernsBack(r), fmt.Sprintf("A message takes %d replies and additions; nothing was sent. Speak to them instead.", store.MaxPosts))
		return store.Concern{}, false
	case err != nil:
		failed(w, r, err)
		return store.Concern{}, false
	}
	k, err := p.st.Concern(r.Context(), c.ID, r.PathValue("id"))
	if err != nil {
		failed(w, r, err)
		return store.Concern{}, false
	}
	return k, true
}

// limitMessage is what was written, trimmed to maxMessage characters.
func limitMessage(text string) string {
	text = strings.TrimSpace(text)
	if rs := []rune(text); len(rs) > maxMessage {
		text = string(rs[:maxMessage])
	}
	return text
}

// What the sender is told.
const (
	senderReceived = "received"
	senderReplied  = "replied"
	senderResolved = "resolved"
)

// tellSender tells a message's sender, at every address they've asked to
// hear on for the competition, that the organiser has received it, replied
// (reply) or resolved it (with reply, if any), and says how many it's
// sending to, for the organiser's notice.
func (n *notifier) tellSender(ctx context.Context, c store.Competition, k store.Concern, what, reply string) string {
	if n == nil || k.Sender == "" {
		return ""
	}
	if n.sandbox {
		return "They see it on their page. " + views.NotificationsOff
	}
	kind, owner, _ := strings.Cut(k.Sender, ":")
	subs, err := n.st.CompetitionSubscriptions(ctx, c.ID)
	if err != nil {
		log.Printf("Replies: %s: %v", c.ID, err)
		return ""
	}
	theirs := func(sub store.Subscription) bool {
		if sub.Kind != kind {
			return false
		}
		if kind == store.NotifyIndividual {
			// An individual is known by their person key, from any of their entries.
			return slices.Contains(n.individualKeys(ctx, sub), owner)
		}
		return sub.OwnerID == owner
	}
	byAddress := map[string][]store.Subscription{}
	var addresses []string
	for _, sub := range subs {
		if !theirs(sub) {
			continue
		}
		key := sub.Channel + " " + strings.ToLower(sub.Address)
		if _, ok := byAddress[key]; !ok {
			addresses = append(addresses, key)
		}
		byAddress[key] = append(byAddress[key], sub)
	}
	subject, line := senderNews(c, what, reply)
	var ds []delivery
	for _, key := range addresses {
		told := byAddress[key]
		switch told[0].Channel {
		case store.ByEmail:
			if n.mail == nil {
				continue
			}
			body := line + "\n\nYour message:\n" + quoted(k.Text) + "\n\n" + n.seeYourPage("To see it") +
				"\nTo stop these emails:\n" + n.offLink(told[0].Token) + "\n"
			ds = append(ds, delivery{about: "Replies: emailing about " + c.ID, email: &email{told[0].Address, subject, body, map[string]string{
				"List-Unsubscribe":      "<" + n.offLink(told[0].Token) + ">",
				"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
			}}})
		case store.ByPush:
			ds = append(ds, delivery{about: "Replies: pushing about " + c.ID, sub: told[0], drop: told,
				push: pushMessage{Title: c.Name, Body: line, URL: n.savedLinks()}})
		}
	}
	n.send.send(ds)
	switch len(ds) {
	case 0:
		return "They see it on their page."
	case 1:
		return "They see it on their page, and are told by push or email."
	}
	return fmt.Sprintf("They see it on their page, and are told at %d addresses.", len(ds))
}

// senderNews is what the sender is told: an email's subject, and the line
// that's the push's body and starts the email.
func senderNews(c store.Competition, what, reply string) (subject, line string) {
	switch {
	case reply != "" && what == senderResolved:
		return "Reply from the organiser of " + c.Name, "The organiser of " + c.Name + " replied, and marked your message resolved:\n\n" + reply
	case reply != "":
		return "Reply from the organiser of " + c.Name, "The organiser of " + c.Name + " replied:\n\n" + reply
	case what == senderResolved:
		return "Your message to the organiser of " + c.Name + " is resolved", "The organiser of " + c.Name + " marked your message resolved."
	}
	return "The organiser of " + c.Name + " has your message", "The organiser of " + c.Name + " has received your message."
}

// quoted is text as a quote in an email.
func quoted(text string) string {
	return "> " + strings.ReplaceAll(text, "\n", "\n> ")
}
