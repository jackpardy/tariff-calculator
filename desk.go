package main

import (
	"context"
	"errors"
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
// returns how many addresses were pushed and emailed.
func (n *notifier) tellDesk(ctx context.Context, c store.Competition, m store.DeskMessage) (pushed, emailed int) {
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
	keys := personKeys(entries)

	// Who it reaches: its people, and every member of its clubs.
	members := map[string][]store.Member{}
	reached := map[string]bool{}
	for _, k := range m.People {
		reached[k] = true
	}
	for _, club := range m.Clubs {
		for _, mem := range n.members(ctx, club, members) {
			reached["m:"+mem.ID] = true
		}
	}
	for _, e := range entries {
		if e.ClubID != "" {
			n.members(ctx, e.ClubID, members)
		}
	}
	// A club's coaches hear what is sent to the club, even with no members.
	clubCoach := map[string]bool{}
	for _, club := range m.Clubs {
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
			return reached["m:"+sub.OwnerID]
		case store.NotifyIndividual:
			return len(keys[sub.OwnerID]) > 0 && reached[keys[sub.OwnerID][0]]
		case store.NotifyClub:
			return slices.Contains(m.Clubs, sub.OwnerID) || slices.ContainsFunc(n.members(ctx, sub.OwnerID, members), func(mem store.Member) bool { return reached["m:"+mem.ID] })
		case store.NotifyCoach:
			if clubCoach[sub.OwnerID] {
				return true
			}
			for _, clubMembers := range members {
				if slices.ContainsFunc(clubMembers, func(mem store.Member) bool { return mem.CoachID == sub.OwnerID && reached["m:"+mem.ID] }) {
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
	for _, key := range addresses {
		told := byAddress[key]
		switch told[0].Channel {
		case store.ByEmail:
			if n.mail == nil {
				continue
			}
			subject, body, headers := n.deskEmail(c, m, told)
			if err := n.mail.send(told[0].Address, subject, body, headers); err != nil {
				log.Printf("Desk: emailing about %s: %v", c.ID, err)
				continue
			}
			emailed++
		case store.ByPush:
			err := n.push(ctx, told[0], pushMessage{Title: c.Name, Body: m.Full(), URL: n.base + told[0].Page})
			switch {
			case errors.Is(err, errGone):
				for _, s := range told {
					n.st.DropSubscription(ctx, s.ID)
				}
			case err != nil:
				log.Printf("Desk: pushing about %s: %v", c.ID, err)
			default:
				pushed++
			}
		}
	}
	return pushed, emailed
}

// deskEmail is the email telling one address a message from the desk.
func (n *notifier) deskEmail(c store.Competition, m store.DeskMessage, subs []store.Subscription) (subject, body string, headers map[string]string) {
	subject = "Message from the organisers of " + c.Name
	if m.Come {
		subject = "Please come to the organisers' desk · " + c.Name
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\nSee it on your page:\n", m.Full())
	for _, s := range subs {
		fmt.Fprintf(&b, "%s%s\n", n.base, s.Page)
	}
	b.WriteString("\nTo stop these emails:\n")
	for _, s := range subs {
		fmt.Fprintf(&b, "%s\n", n.offLink(s.Token))
	}
	return subject, b.String(), map[string]string{
		"List-Unsubscribe":      "<" + n.offLink(subs[0].Token) + ">",
		"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
	}
}
