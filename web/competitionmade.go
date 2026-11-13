package web

import (
	"context"
	"log"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The maintainer hears when anyone makes a competition: its name, when it
// was made and which site, never who made it or its links. The maintainer's
// page lists the competitions made (all but a sandbox copy's), and
// FEEDBACK_EMAIL, with email set up, gets an email for each one, the way
// feedback reaches them (feedback.go). The sandbox emails nothing (ADR
// 0016), so what's made there is only on its own maintainer's page.

// site is where the app is, as the maintainer is told.
func (p *competitionPages) site() string {
	if p.sandbox != nil {
		return "in the sandbox"
	}
	return "on the live site"
}

// madeItems are the competitions made, newest first, as the maintainer's
// page lists them.
func (p *competitionPages) madeItems(ctx context.Context) ([]views.MadeItem, error) {
	made, err := p.st.MadeCompetitions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]views.MadeItem, len(made))
	for i, c := range made {
		out[i] = views.MadeItem{Name: c.Name, Day: madeDay(c.Date), At: c.CreatedAt.In(local).Format("Mon 2 Jan, 15:04"), How: madeHow(c.MadeAs, c.CopiedFrom)}
		if c.SetupKept {
			out[i].How = strings.TrimPrefix(out[i].How+"; its set-up kept, its people deleted", "; ")
		}
	}
	return out, nil
}

// madeHow says how a competition was made, if not new: "Copy of ISTO
// 2027" or "Rehearsal copy of ISTO 2027".
func madeHow(madeAs, from string) string {
	from = strings.Join(strings.Fields(from), " ")
	switch madeAs {
	case store.MadeCopy:
		return "Copy of " + from
	case store.MadeRehearsal:
		return "Rehearsal copy of " + from
	}
	return ""
}

// madeDay is a competition's (first) day, e.g. "Sat 6 Mar 2027".
func madeDay(date string) string {
	day, err := competitions.Competition{Date: date}.Day()
	if err != nil {
		return date
	}
	return day.Format("Mon 2 Jan 2006")
}

// emailMade emails FEEDBACK_EMAIL that a competition was made, when it's
// set and email is, without waiting for it.
func (p *competitionPages) emailMade(c store.Competition) {
	if p.feedbackTo == "" || p.notify == nil || p.notify.mail == nil {
		return
	}
	subject, body := madeEmail(c, p.site(), p.notify.base)
	if err := p.notify.send.add(context.Background(), store.Round{}, []delivery{{about: "Emailing that a competition was made", who: "The maintainer",
		email: &email{to: p.feedbackTo, subject: subject, body: body}}}); err != nil {
		log.Printf("Emailing that a competition was made: %v", err)
	}
}

// madeEmail is the email saying a competition was made: its name, its day,
// when and where (site, at base). Never a link: they're secret.
func madeEmail(c store.Competition, site, base string) (subject, body string) {
	name := strings.Join(strings.Fields(c.Name), " ")
	var b strings.Builder
	b.WriteString("Someone made a competition " + site + ".\n\n")
	b.WriteString("Competition: " + name + "\n")
	if how := madeHow(c.MadeAs, c.CopiedFrom); how != "" {
		b.WriteString("Made as: " + how + "\n")
	}
	b.WriteString("Its day: " + madeDay(c.Date) + "\n")
	b.WriteString("Made: " + c.CreatedAt.In(local).Format("Mon 2 January 2006, 15:04") + "\n")
	b.WriteString("Site: " + strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://") + "\n")
	b.WriteString("\nIt's listed on your feedback page, under Competitions made.\n")
	subject = "Competition made: " + name
	if c.Rehearsal() {
		subject = "Rehearsal copy made: " + name
	}
	return subject, b.String()
}
