package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// More admin links, each able to do less; the change history; and concerns
// flagged for the organiser (ADR 0009).

const adminPrefix = "/competitions/admin/{token}"

// linkKey is the context key of the admin link a request came by.
type linkKey struct{}

// linkOf is the admin link a request came by (the organiser's, outside the
// gate).
func linkOf(r *http.Request) store.Link {
	if l, ok := r.Context().Value(linkKey{}).(store.Link); ok {
		return l
	}
	return store.Link{Name: "Organiser", Kind: store.LinkOrganiser}
}

// allowed says whether a kind of admin link can use a route, by its
// pattern, e.g. "POST /competitions/admin/{token}/entries/{id}/check".
func allowed(kind, pattern string) bool {
	method, path, _ := strings.Cut(pattern, " ")
	path = strings.TrimPrefix(path, adminPrefix)
	get, post := method == http.MethodGet, method == http.MethodPost
	switch kind {
	case store.LinkOrganiser:
		return true
	case store.LinkEverything:
		return !post || !(path == "/links" || path == "/links/{id}/remove" || path == "/replace-link" || path == "/delete")
	case store.LinkScreen:
		return get && path == "/screen" // nothing else, not even flagging a concern
	}
	if post && path == "/concerns" {
		return true // anyone can flag a concern
	}
	viewing := get && (path == "" || path == "/entries/{id}" || path == "/cards" || path == "/entries.csv" || path == "/history")
	switch kind {
	case store.LinkCards:
		return viewing || post && (path == "/entries/{id}/check" || path == "/entries/{id}/video")
	case store.LinkChair:
		return viewing || get && (path == "/timetable/print" || path == "/timetable/timeline.csv" || path == "/day" || path == "/screen" || path == "/desk") || post && (path == "/day/flight" || path == "/day/checkin" || path == "/desk")
	case store.LinkTimetable:
		return viewing || get && (strings.HasPrefix(path, "/timetable") || path == "/officials" || path == "/day" || path == "/screen" || path == "/desk") || post && (path == "/day/flight" || path == "/day/checkin" || path == "/desk") ||
			post && (strings.HasPrefix(path, "/timetable/") || strings.HasPrefix(path, "/officials/") || path == "/notify-now")
	}
	return false
}

// can says whether the request's link can use a route of the admin pages,
// e.g. can(r, "POST", "/remove").
func can(r *http.Request, method, path string) bool {
	return allowed(linkOf(r).Kind, method+" "+adminPrefix+path)
}

// access is what a page's controls may offer, for the request's link.
func access(r *http.Request) views.Access {
	l := linkOf(r)
	return views.Access{
		Who: l.Name, Organiser: l.Kind == store.LinkOrganiser,
		Settings:  can(r, "POST", "/deadline"),
		Remove:    can(r, "POST", "/remove"),
		Check:     can(r, "POST", "/entries/{id}/check"),
		Timetable: can(r, "GET", "/timetable"),
		Sheets:    can(r, "GET", "/timetable/print"),
		Officials: can(r, "GET", "/officials"),
		Coaches:   can(r, "GET", "/coaches"),
		NotifyNow: can(r, "POST", "/notify-now"),
		Resolve:   can(r, "POST", "/concerns/{id}/resolve"),
		Day:       can(r, "GET", "/day"),
		Desk:      can(r, "GET", "/desk"),
	}
}

// statusRecorder notes a response's status.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// gate lets a request through to an admin route only by a link that can use
// it, and records each change it makes in the competition's history.
func (p *competitionPages) gate(pattern string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, link, err := p.st.AdminLink(r.Context(), r.PathValue("token"))
		if err != nil {
			failed(w, r, err)
			return
		}
		if !allowed(link.Kind, pattern) {
			message(w, r, http.StatusForbidden, "Not with this link", "This link can't do that. Ask the organiser.")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), linkKey{}, link))
		if r.Method != http.MethodPost {
			h(w, r)
			return
		}
		r.ParseForm() // for describing it after, whatever the handler reads
		what := p.describe(r, c, pattern)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h(rec, r)
		if rec.status < 400 && what != "" && !strings.HasSuffix(pattern, "/delete") {
			p.st.Record(context.WithoutCancel(r.Context()), c.ID, link.Name, what)
		}
	}
}

// describe says what a change does, for the history; "" for one not worth
// recording.
func (p *competitionPages) describe(r *http.Request, c store.Competition, pattern string) string {
	path := strings.TrimPrefix(strings.TrimPrefix(pattern, "POST "), adminPrefix)
	f := r.Form.Get
	entry := func() string {
		id := r.PathValue("id")
		e, err := p.st.CompetitionEntry(r.Context(), c.ID, id)
		if err != nil {
			return ""
		}
		return ": " + e.Entry.Gymnasts() + " · " + e.Entry.Event()
	}
	switch path {
	case "/entries/{id}/check":
		what := "Marked a card checked"
		if f("checked") != "1" {
			what = "Marked a card not checked"
		}
		if note := strings.TrimSpace(f("note")); note != "" {
			return what + entry() + ` (note: "` + note + `")`
		}
		return what + entry()
	case "/entries/{id}/video":
		return "Reviewed the videos" + entry()
	case "/entries/{id}/restore":
		return "Put an entry back in" + entry()
	case "/remove":
		n := len(r.Form["entry"])
		if f("action") == "hold" {
			return fmt.Sprintf("Put %s on hold", entriesWord(n))
		}
		return "Removed " + entriesWord(n)
	case "/later-deadlines":
		return "Changed when changes and sign-offs close"
	case "/deadline":
		if f("close") == "1" {
			return "Closed entries"
		}
		return "Changed when entries close"
	case "/live":
		switch f("opens") {
		case "now":
			return "Went live"
		case "at":
			return "Set when it goes live"
		}
		return "Made it private"
	case "/links":
		return "Made a link for " + strings.TrimSpace(f("name")) + " (" + linkKinds[f("kind")] + ")"
	case "/links/{id}/remove":
		return "Removed a link"
	case "/replace-link":
		return "Replaced the admin link"
	case "/concerns":
		return "Flagged a concern"
	case "/concerns/{id}/resolve":
		return "Resolved a concern"
	case "/late/settings":
		return "Changed the late changes allowed"
	case "/late/{id}/accept":
		return "Accepted a late change"
	case "/late/{id}/reject":
		return "Turned down a late change"
	case "/limits":
		return "Changed the limits"
	case "/entries/{id}/let-in":
		if f("on") == "0" {
			return "Put an entry back on the waiting list" + entry()
		}
		return "Let an entry in from the waiting list" + entry()
	case "/fees/settings":
		return "Changed the fees"
	case "/fees/payments":
		who := "someone"
		if a, ok, err := p.accountOf(r.Context(), c, f("payer")); err == nil && ok {
			who = a.name
		}
		amount := f("amount")
		if cents, err := competitions.ParseMoney(amount); err == nil {
			amount = c.Fees.Money(cents)
		}
		return "Recorded a payment of " + amount + " from " + who
	case "/fees/payments/{id}/remove":
		return "Removed a payment"
	case "/notify-now":
		return "Sent the changes waiting (Notify now)"
	case "/timetable/plan":
		return "Planned the timetable"
	case "/timetable/publish":
		switch f("action") {
		case "unpublish":
			return "Unpublished the timetable"
		case "discard":
			return "Discarded the draft timetable's changes"
		}
		return "Published the timetable"
	case "/timetable/flight":
		return "Changed a flight"
	case "/timetable/entry":
		return "Moved a gymnast"
	case "/timetable/officials":
		return "Changed the officials' seats"
	case "/timetable/delay":
		return "Kept a delay in the draft timetable"
	case "/timetable/leave":
		return "Kept an official leaving in the draft timetable"
	case "/timetable/simulate", "/timetable/simulate/delete":
		return "" // simulations change nothing
	case "/day/flight":
		return p.describeFlightMark(r, c)
	case "/day/checkin":
		return p.describeCheckin(r, c)
	case "/desk":
		return p.describeDesk(r, c)
	}
	switch {
	case strings.HasPrefix(path, "/timetable/setup/"):
		return "Changed the timetable's setup (" + strings.TrimPrefix(path, "/timetable/setup/") + ")"
	case strings.HasPrefix(path, "/officials"):
		return "Changed the officials"
	case strings.HasPrefix(path, "/coaches/") || strings.HasPrefix(path, "/entry-coaches/"):
		return "Decided about a coach (" + f("action") + ")"
	}
	return "Changed the settings (" + strings.TrimPrefix(path, "/") + ")"
}

// linkKinds name the kinds of extra link.
var linkKinds = map[string]string{
	store.LinkEverything: "everything but links and deleting",
	store.LinkCards:      "checking cards",
	store.LinkTimetable:  "timetable and officials",
	store.LinkChair:      "chairs of judges",
	store.LinkScreen:     "venue screen",
}

func (p *competitionPages) registerAccess(handle func(string, http.HandlerFunc)) {
	handle("POST /competitions/admin/{token}/links", p.addLink)
	handle("POST /competitions/admin/{token}/links/{id}/remove", p.removeLink)
	handle("GET /competitions/admin/{token}/history", p.history)
	handle("POST /competitions/admin/{token}/concerns", p.flagConcern)
	handle("POST /competitions/admin/{token}/concerns/{id}/resolve", p.resolveConcern)
}

// addLink makes an extra admin link (name, kind) and shows it, once.
func (p *competitionPages) addLink(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" || len([]rune(name)) > 60 || linkKinds[r.FormValue("kind")] == "" {
		backToDashboard(w, r, "Give the link a name (who it's for) and say what it can do.")
		return
	}
	l, token, err := p.st.AddLink(r.Context(), c.ID, name, r.FormValue("kind"))
	if err != nil {
		if err == store.ErrLimit {
			backToDashboard(w, r, fmt.Sprintf("A competition can have %d links: remove one first.", store.MaxLinks))
			return
		}
		failed(w, r, err)
		return
	}
	render(w, r, views.NewLink(views.NewLinkPage{
		Competition: summary(c.Competition, p.now()), Back: adminPath(r.PathValue("token")),
		Name: l.Name, Kind: linkKinds[l.Kind], Link: origin(r) + adminPath(token),
	}))
}

// removeLink stops an extra link working.
func (p *competitionPages) removeLink(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := p.st.RemoveLink(r.Context(), c.ID, r.PathValue("id")); err != nil {
		failed(w, r, err)
		return
	}
	backToDashboard(w, r, "The link has stopped working.")
}

// backToDashboard goes back to the dashboard, saying what happened.
func backToDashboard(w http.ResponseWriter, r *http.Request, notice string) {
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

// history shows the latest changes, and who made them.
func (p *competitionPages) history(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	changes, err := p.st.History(r.Context(), c.ID, 500)
	if err != nil {
		failed(w, r, err)
		return
	}
	page := views.HistoryPage{Competition: summary(c.Competition, p.now()), Back: adminPath(r.PathValue("token"))}
	for _, ch := range changes {
		page.Changes = append(page.Changes, views.HistoryChange{At: ch.At.In(local).Format("Mon 2 Jan, 15:04"), Who: ch.Who, What: ch.What})
	}
	render(w, r, views.History(page))
}

// flagConcern records a concern (text, and entry for one about an entry).
func (p *competitionPages) flagConcern(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	text := limitNote(strings.TrimSpace(r.FormValue("text")))
	back := adminPath(r.PathValue("token"))
	entryID := r.FormValue("entry")
	if entryID != "" {
		if _, err := p.st.CompetitionEntry(r.Context(), c.ID, entryID); err != nil {
			failed(w, r, err)
			return
		}
		back += "/entries/" + entryID
	}
	if text == "" {
		http.Redirect(w, r, back+"?notice="+url.QueryEscape("Say what the concern is."), http.StatusSeeOther)
		return
	}
	if _, err := p.st.FlagConcern(r.Context(), c.ID, entryID, linkOf(r).Name, text); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, back+"?notice="+url.QueryEscape("Flagged for the organiser."), http.StatusSeeOther)
}

// resolveConcern marks a concern dealt with (note optional).
func (p *competitionPages) resolveConcern(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := p.st.ResolveConcern(r.Context(), c.ID, r.PathValue("id"), linkOf(r).Name, limitNote(strings.TrimSpace(r.FormValue("note")))); err != nil && err != store.ErrNotFound {
		failed(w, r, err)
		return
	}
	backToDashboard(w, r, "Concern resolved.")
}

// concernViews are a competition's concerns, for the dashboard, with the
// entries they're about.
func (p *competitionPages) concernViews(ctx context.Context, c store.Competition, base string) ([]views.ConcernView, int, error) {
	concerns, err := p.st.Concerns(ctx, c.ID)
	if err != nil {
		return nil, 0, err
	}
	var out []views.ConcernView
	open := 0
	for _, k := range concerns {
		v := views.ConcernView{ID: k.ID, Who: k.Who, Text: k.Text, At: k.At.In(local).Format("Mon 2 Jan, 15:04"), Open: k.Open(),
			ResolvedBy: k.ResolvedBy, Resolution: k.Resolution}
		if k.Open() {
			open++
		}
		if k.EntryID != "" {
			if e, err := p.st.CompetitionEntry(ctx, c.ID, k.EntryID); err == nil {
				v.About, v.AboutLink = e.Entry.Gymnasts()+" · "+e.Entry.Event(), base+"/entries/"+k.EntryID
			}
		}
		out = append(out, v)
	}
	return out, open, nil
}

// linkViews are the extra links, for the organiser.
func linkViews(links []store.Link) []views.LinkView {
	var out []views.LinkView
	for _, l := range links {
		out = append(out, views.LinkView{ID: l.ID, Name: l.Name, Kind: linkKinds[l.Kind], Made: l.CreatedAt.In(local).Format("2 January")})
	}
	return out
}
