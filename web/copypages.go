package web

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Copying a competition (2026-11-13): its set-up for another competition,
// or with its entries as a rehearsal copy nobody else sees, which sends
// nothing and is deleted 30 days after it's made; and the page of a
// finished competition's kept set-up.

func (p *competitionPages) registerCopy(handle func(string, http.HandlerFunc)) {
	handle("GET /competitions/admin/{token}/copy", p.copyForm)
	handle("POST /competitions/admin/{token}/copy", p.copyCompetition)
	handle("POST /competitions/admin/{token}/keep-longer", p.keepLonger)
}

// keptRoutes are the only routes a kept set-up's admin link opens: its page,
// copying it and deleting it.
var keptRoutes = map[string]bool{"GET " + adminPrefix: true, "GET " + adminPrefix + "/copy": true,
	"POST " + adminPrefix + "/copy": true, "POST " + adminPrefix + "/delete": true}

// lastDay is a competition's last day, at midnight UTC: its first, or later
// on a timetable of several days.
func lastDay(c store.Competition) (time.Time, bool) {
	day, err := c.Day()
	if err != nil {
		return time.Time{}, false
	}
	if n := len(schedule(c).Setup.Days); n > 1 {
		day = day.AddDate(0, 0, n-1)
	}
	return day, true
}

// daysOver says whether a competition's days are over at now.
func daysOver(c store.Competition, now time.Time) bool {
	last, ok := lastDay(c)
	today := now.In(local)
	return ok && time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC).After(last)
}

// canRehearse says whether a rehearsal copy can be made of a competition:
// one with its people, not over, not a rehearsal copy itself.
func canRehearse(c store.Competition, now time.Time) bool {
	return !c.Rehearsal() && !c.SetupKept() && !daysOver(c, now)
}

// rehearsalLine is what every page of a rehearsal copy says.
func rehearsalLine(c store.Competition) string {
	return "Rehearsal copy of " + c.CopiedFrom + ", made " + c.CreatedAt.In(local).Format("2 January") +
		": nobody can see it but you, nothing is sent, and it's deleted on " + c.DeletesAt.In(local).Format("2 January") + "."
}

// copyForm asks before copying (as=setup or as=rehearsal), with the copy's
// name and first day.
func (p *competitionPages) copyForm(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	rehearsal := r.URL.Query().Get("as") == "rehearsal"
	if rehearsal && !canRehearse(c, p.now()) {
		seeOther(w, r, settingsPath(r.PathValue("token"))+"#copy", "A rehearsal copy can be made only of a competition with its entries, before its days are over.")
		return
	}
	page := p.copyPage(r, c, rehearsal)
	page.Name = c.Name + " (copy)"
	page.Date = ""
	if day, err := c.Day(); err == nil && !rehearsal {
		page.Date = day.AddDate(1, 0, 0).Format(competitions.DateLayout) // next year's, to change
	}
	if rehearsal {
		page.Name = c.Name + " (rehearsal)"
		page.Date = p.now().In(local).Format(competitions.DateLayout)
	}
	render(w, r, views.CopyCompetition(page))
}

// copyPage is the page asking before copying, but for its fields.
func (p *competitionPages) copyPage(r *http.Request, c store.Competition, rehearsal bool) views.CopyPage {
	page := views.CopyPage{Base: adminPath(r.PathValue("token")), Competition: summary(c, p.now()), Rehearsal: rehearsal,
		Days: max(1, len(schedule(c).Setup.Days)), Kept: c.SetupKept(),
		Deletes: p.now().In(local).AddDate(0, 0, store.RehearsalDays).Format("Monday 2 January 2006")}
	if !page.Kept {
		page.Menu = p.menuFor(r, c, menuSettings)
	}
	return page
}

// copyCompetition makes the copy (as, name, date) and opens it with its
// new admin link, shown once.
func (p *competitionPages) copyCompetition(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	rehearsal := r.FormValue("as") == "rehearsal"
	if rehearsal && !canRehearse(c, p.now()) {
		refuse(w, r, settingsPath(r.PathValue("token"))+"#copy", "A rehearsal copy can be made only of a competition with its entries, before its days are over; nothing was copied.")
		return
	}
	page := p.copyPage(r, c, rehearsal)
	page.Name, page.Date = strings.Join(strings.Fields(r.FormValue("name")), " "), r.FormValue("date")
	if err := competitions.CheckName("competition", page.Name); err != nil {
		page.Problems = append(page.Problems, sentences(err)...)
	}
	if _, err := time.Parse(competitions.DateLayout, page.Date); err != nil {
		page.Problems = append(page.Problems, "Give the copy's first day.")
	}
	if len(page.Problems) > 0 {
		noteRefused(w)
		w.WriteHeader(http.StatusUnprocessableEntity)
		render(w, r, views.CopyCompetition(page))
		return
	}
	if !p.limiter.allow(clientIP(r), p.now()) {
		noteRefused(w)
		message(w, r, http.StatusTooManyRequests, "Too many competitions", "This address has created several competitions in the last hour. Please try again later.")
		return
	}
	copied, admin, err := p.st.CopyCompetition(r.Context(), c.ID, page.Name, page.Date, rehearsal)
	if err != nil {
		failed(w, r, err)
		return
	}
	p.emailMade(copied)
	if rehearsal {
		http.Redirect(w, r, adminPath(admin)+"?new=rehearsal", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, settingsPath(admin)+"?new=copied", http.StatusSeeOther)
}

// keepLonger keeps a rehearsal copy 30 days longer, once.
func (p *competitionPages) keepLonger(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	switch err := p.st.ExtendRehearsal(r.Context(), c.ID); {
	case errors.Is(err, store.ErrExtended), errors.Is(err, store.ErrNotFound):
		refuse(w, r, settingsPath(r.PathValue("token"))+"#rehearsal", "It has been kept longer already; nothing was changed.")
		return
	case err != nil:
		failed(w, r, err)
		return
	}
	c, _, err := p.st.AdminLink(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return
	}
	seeOther(w, r, settingsPath(r.PathValue("token"))+"#rehearsal", "Kept 30 days longer: it's deleted on "+c.DeletesAt.In(local).Format("Monday 2 January 2006")+".")
}

// keptSetup is the page a kept set-up's admin link opens.
func (p *competitionPages) keptSetup(w http.ResponseWriter, r *http.Request, c store.Competition) {
	when := c.Date
	if first, err := c.Day(); err == nil {
		last, _ := lastDay(c)
		when = dayRange(first, last)
	}
	render(w, r, views.KeptSetup(views.KeptSetupPage{
		Base: adminPath(r.PathValue("token")), Name: c.Name, Notice: r.URL.Query().Get("notice"),
		Says: c.Name + " was on " + when + ". Its entries and everything about the people in it were deleted on " +
			c.SetupKeptAt.In(local).Format("2 January 2006") + "; its set-up is kept for you to copy.",
		Deletes: c.DeletesAt.Format("2 January 2006"),
	}))
}

// dayRange is a competition's days in a sentence: "3 April 2027", "3 to 5
// April 2027", "30 April to 2 May 2027".
func dayRange(first, last time.Time) string {
	switch {
	case !last.After(first):
		return first.Format("2 January 2006")
	case first.Year() != last.Year():
		return first.Format("2 January 2006") + " to " + last.Format("2 January 2006")
	case first.Month() != last.Month():
		return first.Format("2 January") + " to " + last.Format("2 January 2006")
	}
	return first.Format("2") + " to " + last.Format("2 January 2006")
}
