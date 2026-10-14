package web

import (
	"net/http"
	"net/url"
	"time"

	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The organiser's Settings page (roadmap 2026-10-14): the links and every
// setting that used to fold away on the entries page. The forms post to their
// own routes, which come back here.

// settingsPath is the settings page of the admin link with this token.
func settingsPath(token string) string { return adminPath(token) + "/settings" }

// backToSettings goes back to the settings page, saying what happened ("" to
// say nothing).
func backToSettings(w http.ResponseWriter, r *http.Request, notice string) {
	to := settingsPath(r.PathValue("token"))
	if notice != "" {
		to += "?notice=" + url.QueryEscape(notice)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// liveFields are the date and time to suggest for going live: when it's due
// to, else tomorrow morning.
func (p *competitionPages) liveFields(c store.Competition) (date, clock string) {
	liveAt := c.LiveAt.In(local)
	if c.LiveAt.IsZero() {
		today := p.now().In(local)
		liveAt = time.Date(today.Year(), today.Month(), today.Day()+1, 9, 0, 0, 0, local)
	}
	return liveAt.Format("2006-01-02"), liveAt.Format("15:04")
}

// settings shows the competition's links and settings.
func (p *competitionPages) settings(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	entries, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	q := r.URL.Query()
	deadline := c.Deadline.In(local)
	liveDate, liveTime := p.liveFields(c)
	page := views.SettingsPage{
		Base: adminPath(r.PathValue("token")), Notice: q.Get("notice"), New: q.Get("new"),
		Menu: p.menuFor(r, c, menuSettings), Access: access(r), Competition: summary(c.Competition, p.now()),
		Links:        views.CompetitionLinks{Club: origin(r) + "/competitions/club/" + c.ClubLink, Individual: origin(r) + "/competitions/enter/" + c.IndividualLink},
		DeadlineDate: deadline.Format("2006-01-02"), DeadlineTime: deadline.Format("15:04"),
		ChangesDate: dateField(c.ChangesUntil, c.Deadline), ChangesTime: timeField(c.ChangesUntil, c.Deadline),
		SignoffsDate: dateField(c.SignoffsUntil, c.Deadline), SignoffsTime: timeField(c.SignoffsUntil, c.Deadline),
		LiveDate: liveDate, LiveTime: liveTime,
		NotifyOn: p.notifyLink("/") != "", NoWait: c.NoWait,
		Limits: limitFields(c, entries), Video: videoForm(c.Video), Split: splitForm(c.Split, c.EventNames()),
		Events: eventsForm(c.Competition), LevelOrder: levelOrderForm(c.Competition),
	}
	if page.New != "" && !store.IsVia(r.PathValue("token")) {
		page.Links.Admin = origin(r) + adminPath(r.PathValue("token"))
	}
	if page.Access.Organiser {
		links, err := p.st.Links(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		page.LinkList = linkViews(links)
		ties, err := p.st.CompetitionTies(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		names := map[string]string{}
		for _, l := range links {
			names[l.ID] = l.Name
		}
		for _, t := range ties {
			what := "Admin link"
			if t.Kind == store.RoleHelper {
				what = names[t.Target]
			}
			page.Ties = append(page.Ties, views.TieView{ID: t.ID, Name: t.Name, What: what, Since: t.At.In(local).Format("2 January 2006")})
		}
	}
	if c.Signoff {
		sent, err := p.st.CompetitionCoaches(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		named, err := p.st.CompetitionEntryCoaches(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		page.CoachApproval = coachApprovalForm(c.Competition, sent, named)
	}
	render(w, r, views.CompetitionSettings(page))
}
