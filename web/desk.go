package web

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"

	"tariffCalculator/store"
)

// Messages from the organisers' desk (roadmap 2026-10-09): told at once,
// without the wait changes have, to whoever has asked to hear and covers
// someone the message is for. Everyone else sees it on their own page.

// tellDesk tells a message to every address whose subscription covers
// someone it was sent to, once per address, on any topic: it's urgent. It
// hands them to the sender and returns how many are to be pushed and
// emailed; the sender keeps the counts with the message when it's done.
func (n *notifier) tellDesk(ctx context.Context, c store.Competition, m store.DeskMessage) (pushes, emails int) {
	if n == nil {
		return 0, 0
	}
	subs, err := n.st.CompetitionSubscriptions(ctx, c.ID)
	if err != nil || len(subs) == 0 {
		if err != nil {
			log.Printf("Desk: %s: %v", c.ID, err)
		}
		return 0, 0
	}
	entries, err := n.st.Entries(ctx, c.ID)
	if err != nil {
		log.Printf("Desk: %s: %v", c.ID, err)
		return 0, 0
	}
	// Who it reaches: its people, every member of its clubs, and the comp sec
	// and coaches of its clubs and staff clubs.
	members := map[string][]store.Member{}
	reached := map[string]bool{}
	for _, k := range m.People {
		reached[k] = true
	}
	for _, club := range m.Clubs {
		for _, mem := range n.members(ctx, club, members) {
			reached[store.MemberKey(mem.ID)] = true
		}
	}
	for _, e := range entries {
		if e.ClubID != "" {
			n.members(ctx, e.ClubID, members)
		}
	}
	// A club's coaches hear what is sent to the club, even with no members,
	// and what is sent to its staff.
	clubCoach := map[string]bool{}
	for _, club := range slices.Concat(m.Clubs, m.Staff) {
		coaches, err := n.st.Coaches(ctx, club)
		if err != nil {
			log.Printf("Desk: coaches of %s: %v", club, err)
		}
		for _, co := range coaches {
			clubCoach[co.ID] = true
		}
	}

	covers := func(sub store.Subscription) bool {
		switch sub.Kind {
		case store.NotifyMember:
			return reached[store.MemberKey(sub.OwnerID)]
		case store.NotifyIndividual:
			return slices.ContainsFunc(n.individualKeys(ctx, sub), func(k string) bool { return reached[k] })
		case store.NotifyClub:
			return slices.Contains(m.Clubs, sub.OwnerID) || slices.Contains(m.Staff, sub.OwnerID) || slices.ContainsFunc(n.members(ctx, sub.OwnerID, members), func(mem store.Member) bool { return reached[store.MemberKey(mem.ID)] })
		case store.NotifyCoach:
			if clubCoach[sub.OwnerID] {
				return true
			}
			for _, clubMembers := range members {
				if slices.ContainsFunc(clubMembers, func(mem store.Member) bool { return mem.CoachID == sub.OwnerID && reached[store.MemberKey(mem.ID)] }) {
					return true
				}
			}
		}
		return false
	}

	// One delivery per address.
	byAddress := map[string][]store.Subscription{}
	var addresses []string
	for _, sub := range subs {
		if !covers(sub) {
			continue
		}
		key := sub.Channel + " " + strings.ToLower(sub.Address)
		if _, ok := byAddress[key]; !ok {
			addresses = append(addresses, key)
		}
		byAddress[key] = append(byAddress[key], sub)
	}
	t := &tally{competitionID: c.ID, messageID: m.ID}
	var ds []delivery
	for _, key := range addresses {
		told := byAddress[key]
		switch told[0].Channel {
		case store.ByEmail:
			if n.mail == nil {
				continue
			}
			subject, body, headers := n.deskEmail(c, m, told)
			ds = append(ds, delivery{about: "Desk: emailing about " + c.ID, email: &email{told[0].Address, subject, body, headers}, tally: t})
			t.emails++
		case store.ByPush:
			ds = append(ds, delivery{about: "Desk: pushing about " + c.ID, sub: told[0], drop: told, tally: t,
				push: pushMessage{Title: c.Name, Body: m.Full(), URL: n.savedLinks()}})
			t.pushes++
		}
	}
	n.send.send(ds)
	return t.pushes, t.emails
}

// deskEmail is the email telling one address a message from the desk.
func (n *notifier) deskEmail(c store.Competition, m store.DeskMessage, subs []store.Subscription) (subject, body string, headers map[string]string) {
	subject = "Message from the organisers of " + c.Name
	if m.Come {
		subject = "Please come to the organisers' desk · " + c.Name
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n%s", m.Full(), n.seeYourPage("To see it"))
	// Any one off link stops every email to the address for the competition.
	fmt.Fprintf(&b, "\nTo stop these emails:\n%s\n", n.offLink(subs[0].Token))
	return subject, b.String(), map[string]string{
		"List-Unsubscribe":      "<" + n.offLink(subs[0].Token) + ">",
		"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
	}
}

// sendingNotice says how many a message is going to, by push and email,
// e.g. "Sending to 214 people: 180 by email, 34 by push."
func sendingNotice(pushes, emails int) string {
	switch total := pushes + emails; total {
	case 0:
		return "No one it reaches has asked to hear by push or email."
	case 1:
		return fmt.Sprintf("Sending to 1 person: %d by email, %d by push.", emails, pushes)
	default:
		return fmt.Sprintf("Sending to %d people: %d by email, %d by push.", total, emails, pushes)
	}
}

// deskProgress is how a desk message's sending is going, while it is.
func (n *notifier) deskProgress(messageID string) (tally, bool) {
	if n == nil {
		return tally{}, false
	}
	return n.send.progress(messageID)
}
