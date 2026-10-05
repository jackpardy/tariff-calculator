package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // the image has no zoneinfo; deadlines are Irish and UK time

	"tariffCalculator/competitions"
	"tariffCalculator/requirements"
	"tariffCalculator/skills"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The competition pages (ADR 0004 step 3): create a competition, the
// organiser's dashboard and each entry, and individual entry. They aren't
// linked from the calculator until storage is durable (Decision 8).

// local is the time zone deadlines are entered and shown in: Ireland and the
// UK keep the same clock.
var local = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Dublin")
	if err != nil {
		panic(err)
	}
	return loc
}()

// maxCompetitionsPerHour is how many competitions one address can create in an
// hour (ADR 0004 Decision 7).
const maxCompetitionsPerHour = 5

// competitionPages serves the competition pages from the store, which is nil
// when storage is off.
type competitionPages struct {
	st                                *store.Store
	limiter, clubLimiter, joinLimiter *limiter
	now                               func() time.Time
}

func newCompetitionPages(st *store.Store) *competitionPages {
	return &competitionPages{
		st:          st,
		limiter:     newLimiter(maxCompetitionsPerHour, time.Hour),
		clubLimiter: newLimiter(maxClubsPerHour, time.Hour),
		joinLimiter: newLimiter(maxJoinsPerHour, time.Hour),
		now:         time.Now,
	}
}

func (p *competitionPages) register(mux *http.ServeMux) {
	handle := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, p.secret(h)) }
	handle("GET /competitions/new", p.newForm)
	handle("POST /competitions", p.create)
	handle("GET /competitions/admin/{token}", p.dashboard)
	handle("GET /competitions/admin/{token}/entries/{id}", p.entry)
	handle("POST /competitions/admin/{token}/individuals", p.individuals)
	handle("POST /competitions/admin/{token}/replace-link", p.replaceLink)
	handle("POST /competitions/admin/{token}/delete", p.delete)
	handle("GET /competitions/enter/{token}", p.enterForm)
	handle("POST /competitions/enter/{token}", p.enter)
	handle("GET /competitions/entry/{token}", p.ownEntry)
	handle("POST /competitions/entry/{token}", p.replaceEntry)
	handle("POST /competitions/entry/{token}/withdraw", p.withdraw)
	p.registerClubs(mux)
}

// secret marks the pages as private: links in their URLs mustn't leak through
// referrers, caches or search engines. Without storage they say so.
func (p *competitionPages) secret(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex")
		if p.st == nil {
			message(w, r, http.StatusServiceUnavailable, "Not available yet", "Competition entries aren't available on this server yet.")
			return
		}
		h(w, r)
	})
}

// message renders a page that only says something.
func message(w http.ResponseWriter, r *http.Request, status int, title, text string) {
	w.WriteHeader(status)
	render(w, r, views.Message(title, text))
}

// failed reports a store error as a page: a link that leads nowhere, a
// deadline passed, a limit reached, or something unexpected.
func failed(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		message(w, r, http.StatusNotFound, "Link not found", "This link doesn't lead anywhere. It may have been replaced, or the competition deleted.")
	case errors.Is(err, store.ErrClosed):
		message(w, r, http.StatusConflict, "Entries have closed", "The deadline for this competition has passed, so entries can't be changed.")
	case errors.Is(err, store.ErrLimit):
		message(w, r, http.StatusConflict, "Competition full", "This competition has as many entries as it can take.")
	default:
		log.Printf("Competition storage: %s %s: %v", r.Method, r.URL.Path, err)
		message(w, r, http.StatusInternalServerError, "Something went wrong", "That didn't work. Please try again in a minute.")
	}
}

// --- Creating a competition ---

func (p *competitionPages) newForm(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.NewCompetition(views.CompetitionForm{
		DeadlineTime: "23:59", Individuals: true, Levels: map[string]bool{}, Groups: requirements.BuiltinGroups(),
	}))
}

// create makes a competition from the form: name, date, deadlineDate,
// deadlineTime, individuals, each built-in level ticked (level), and each of
// the browser's own levels ticked (custom: {"level": ..., "sets": {id: set}}).
func (p *competitionPages) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	form := views.CompetitionForm{
		Name: strings.TrimSpace(r.FormValue("name")), Date: r.FormValue("date"),
		DeadlineDate: r.FormValue("deadlineDate"), DeadlineTime: r.FormValue("deadlineTime"),
		Individuals: r.FormValue("individuals") == "1", Levels: map[string]bool{}, Groups: requirements.BuiltinGroups(),
	}
	c := competitions.Competition{Name: form.Name, Date: form.Date, Individuals: form.Individuals}
	var problems []string
	deadline, err := time.ParseInLocation("2006-01-02 15:04", form.DeadlineDate+" "+form.DeadlineTime, local)
	switch {
	case err != nil:
		problems = append(problems, "Entries need a closing date and time.")
	case !deadline.After(p.now()):
		problems = append(problems, "The closing time has already passed.")
	default:
		c.Deadline = deadline.UTC()
	}
	for _, ref := range r.Form["level"] {
		form.Levels[ref] = true
		c.Levels = append(c.Levels, competitions.Level{Ref: ref})
	}
	for _, raw := range r.Form["custom"] {
		var own struct {
			Level requirements.Level          `json:"level"`
			Sets  map[string]requirements.Set `json:"sets"`
		}
		if err := json.Unmarshal([]byte(raw), &own); err != nil {
			problems = append(problems, "One of your own levels couldn't be read.")
			continue
		}
		c.Levels = append(c.Levels, competitions.Level{Custom: &own.Level, Sets: own.Sets})
	}
	if c.Deadline.IsZero() {
		c.Deadline = time.Now() // reported above; let Validate report the rest
	}
	if err := c.Validate(); err != nil {
		problems = append(problems, sentences(err)...)
	}
	if len(problems) > 0 {
		form.Problems = problems
		w.WriteHeader(http.StatusUnprocessableEntity)
		render(w, r, views.NewCompetition(form))
		return
	}
	if !p.limiter.allow(clientIP(r), p.now()) {
		message(w, r, http.StatusTooManyRequests, "Too many competitions", "This address has created several competitions in the last hour. Please try again later.")
		return
	}
	_, admin, err := p.st.CreateCompetition(r.Context(), c)
	if err != nil {
		failed(w, r, err)
		return
	}
	// The dashboard says to save the link; reloading it can't create another.
	http.Redirect(w, r, adminPath(admin)+"?new=created", http.StatusSeeOther)
}

// sentences splits a validation error into its problems, each a sentence.
func sentences(err error) []string {
	var out []string
	for _, line := range strings.Split(err.Error(), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, strings.ToUpper(line[:1])+line[1:]+".")
		}
	}
	return out
}

func adminPath(token string) string { return "/competitions/admin/" + token }

// origin is the scheme and host the request came to, for full links.
func origin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// summary is what the pages say about a competition.
func summary(c competitions.Competition, now time.Time) views.CompetitionSummary {
	out := views.CompetitionSummary{
		Name: c.Name, Date: c.Date, Deadline: c.Deadline.In(local).Format("Monday 2 January 2006, 15:04"),
		DeleteAfter: c.DeleteAfter().Format("2 January 2006"), Open: c.Open(now), Individuals: c.Individuals,
	}
	if day, err := c.Day(); err == nil {
		out.Date = day.Format("Monday 2 January 2006")
	}
	return out
}

// --- The organiser ---

// admin is the competition an admin link opens, or a page saying it doesn't.
func (p *competitionPages) admin(w http.ResponseWriter, r *http.Request) (store.Competition, bool) {
	c, err := p.st.CompetitionByAdmin(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return c, false
	}
	return c, true
}

// dashboard lists every entry by level, filtered by club (club: a club's name,
// or "individual") and to entries with problems (problems=1).
func (p *competitionPages) dashboard(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	entries, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	clubs, err := p.st.CompetitionClubs(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	d := views.Dashboard{
		Base: adminPath(r.PathValue("token")), Competition: summary(c.Competition, p.now()),
		Links: views.CompetitionLinks{Club: origin(r) + "/competitions/club/" + c.ClubLink, Individual: origin(r) + "/competitions/enter/" + c.IndividualLink},
		Club:  r.URL.Query().Get("club"), ProblemsOnly: r.URL.Query().Get("problems") == "1",
		Entries: len(entries), New: r.URL.Query().Get("new"),
	}
	if d.New != "" {
		d.Links.Admin = origin(r) + adminPath(r.PathValue("token"))
	}
	seen := map[string]bool{}
	for _, club := range clubs {
		d.Clubs, seen[club.Name] = append(d.Clubs, club.Name), true
	}
	byLevel := map[string]int{}
	for _, l := range c.Levels {
		if level, err := l.Resolve(); err == nil {
			byLevel[level.Name] = len(d.Levels)
			d.Levels = append(d.Levels, views.DashboardLevel{Name: level.Name})
		}
	}
	for _, e := range entries {
		club := e.ClubName
		if e.Individual {
			club = "individual"
		} else if !seen[club] {
			d.Clubs, seen[club] = append(d.Clubs, club), true // a club deleted since it sent
		}
		row := views.DashboardRow{ID: e.ID, Gymnast: e.Entry.Gymnast, Club: club, Sent: e.SentAt.In(local).Format("2 Jan, 15:04")}
		card, err := c.Check(e.Entry)
		if err != nil {
			row.Problems = []string{err.Error()}
		} else {
			row.Problems = card.Problems()
			for i, ex := range []requirements.Checked{card.First, card.Second} {
				row.Met, row.Rules = row.Met+ex.Met(), row.Rules+len(ex.Results)
				row.Exercises[i] = exerciseSummary(e.Entry.Exercises[i], ex)
			}
		}
		if len(row.Problems) > 0 {
			d.WithProblems++
		}
		if (d.Club != "" && d.Club != club) || (d.ProblemsOnly && len(row.Problems) == 0) {
			continue
		}
		n, ok := byLevel[e.Entry.Level]
		if !ok {
			n, byLevel[e.Entry.Level] = len(d.Levels), len(d.Levels)
			d.Levels = append(d.Levels, views.DashboardLevel{Name: e.Entry.Level})
		}
		d.Levels[n].Rows = append(d.Levels[n].Rows, row)
	}
	render(w, r, views.CompetitionDashboard(d))
}

// exerciseSummary is an exercise in a dashboard row: a set routine performed as
// written by its name, anything else by its difficulty.
func exerciseSummary(ex competitions.Exercise, c requirements.Checked) string {
	if len(ex.Skills) == 0 && !c.Checks.ScoreDifficulty {
		return c.SetName
	}
	if !c.Checks.ScoreDifficulty {
		return fmt.Sprintf("%d skills", len(c.Validation.Skills))
	}
	return fmt.Sprintf("%.1f", c.Validation.TotalTariff)
}

// entry shows one entry, checked.
func (p *competitionPages) entry(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	e, err := p.st.CompetitionEntry(r.Context(), c.ID, r.PathValue("id"))
	if err != nil {
		failed(w, r, err)
		return
	}
	shown, err := card(c.Competition, e.Entry, clubOf(e))
	if err != nil {
		failed(w, r, err)
		return
	}
	render(w, r, views.CompetitionEntry(views.EntryDetail{
		Base: adminPath(r.PathValue("token")), Competition: summary(c.Competition, p.now()),
		Card: shown, Sent: e.SentAt.In(local).Format("Monday 2 January, 15:04"),
	}))
}

// clubOf is who sent a stored entry, as the pages say it.
func clubOf(e store.Entry) string {
	if e.Individual {
		return "Individual"
	}
	return e.ClubName
}

// card is an entry checked, as the pages show it.
func card(c competitions.Competition, e competitions.Entry, club string) (views.EntryCard, error) {
	checked, err := c.Check(e)
	if err != nil {
		return views.EntryCard{}, err
	}
	out := views.EntryCard{Gymnast: e.Gymnast, Club: club, Level: checked.Level.Name, Problems: checked.Problems()}
	l, _, _ := c.Level(e.Level)
	for i, ex := range []requirements.Checked{checked.First, checked.Second} {
		card := views.ExerciseCard{
			Title: [...]string{"First exercise", "Second exercise"}[i], Requirements: ex.SetName,
			Validation: ex.Validation, Checks: ex.Checks, Results: ex.Results, Required: ex.Required,
		}
		if ex.SetErr != nil {
			card.SetErr = ex.SetErr.Error()
		}
		if set, err := l.Set(e.Exercises[i].Option); err == nil && len(e.Exercises[i].Skills) == 0 {
			_, card.SetRoutine = requirements.SetRoutine(set)
		}
		out.Exercises[i] = card
	}
	out.Exercises[0].Carried, out.Exercises[1].Repeated = checked.Carried, checked.Repeated
	return out, nil
}

func (p *competitionPages) individuals(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := p.st.SetIndividuals(r.Context(), c.ID, r.FormValue("on") == "1"); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token")), http.StatusSeeOther)
}

func (p *competitionPages) replaceLink(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	admin, err := p.st.ReplaceCompetitionAdmin(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(admin)+"?new=replaced", http.StatusSeeOther)
}

func (p *competitionPages) delete(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if r.FormValue("confirm") != "1" {
		http.Redirect(w, r, adminPath(r.PathValue("token")), http.StatusSeeOther)
		return
	}
	if err := p.st.DeleteCompetition(r.Context(), c.ID); err != nil {
		failed(w, r, err)
		return
	}
	message(w, r, http.StatusOK, "Competition deleted", c.Name+" and all its entries have been deleted.")
}

// --- Individuals ---

// entryForm is the form for entering a competition, starting from e.
func entryForm(c competitions.Competition, e competitions.Entry, action, submit string) views.EntryForm {
	f := views.EntryForm{Action: action, Submit: submit, Gymnast: e.Gymnast, Level: e.Level}
	for _, l := range c.Levels {
		level, err := l.Resolve()
		if err != nil {
			continue
		}
		el := views.EntryLevel{Name: level.Name}
		for n := range 2 {
			ex := &el.Exercises[n]
			for _, ref := range level.Exercise(n + 1).Options {
				o := views.EntryOption{Ref: ref, Label: ref}
				if set, err := l.Set(ref); err == nil {
					o.Label = set.Name
					_, o.SetRoutine = requirements.SetRoutine(set)
				}
				ex.Options = append(ex.Options, o)
			}
			if level.Name == e.Level {
				ex.Chosen = e.Exercises[n].Option
				if len(e.Exercises[n].Skills) > 0 {
					data, _ := json.Marshal(e.Exercises[n].Skills)
					ex.Skills, ex.Count = string(data), len(e.Exercises[n].Skills)
				}
			}
		}
		f.Levels = append(f.Levels, el)
	}
	return f
}

// postedEntry reads an entry from the form: gymnast (unless given, as for a
// club's member), level, and for each exercise its option (ex1Option,
// ex2Option) and routine as JSON (ex1Skills, ex2Skills). A set routine's
// routine is left out: it's performed as written.
func postedEntry(r *http.Request, c competitions.Competition, gymnast string) (competitions.Entry, []string) {
	if gymnast == "" {
		gymnast = r.FormValue("gymnast")
	}
	e := competitions.Entry{Gymnast: gymnast, Level: r.FormValue("level")}
	var problems []string
	l, _, ok := c.Level(e.Level)
	for i := range e.Exercises {
		ex := &e.Exercises[i]
		ex.Option = r.FormValue(fmt.Sprintf("ex%dOption", i+1))
		if ok {
			if set, err := l.Set(ex.Option); err == nil {
				if _, isSet := requirements.SetRoutine(set); isSet {
					continue
				}
			}
		}
		raw := strings.TrimSpace(r.FormValue(fmt.Sprintf("ex%dSkills", i+1)))
		if raw == "" {
			problems = append(problems, fmt.Sprintf("Choose a routine for the %s exercise.", [...]string{"first", "second"}[i]))
			continue
		}
		var routine []skills.TrampolineSkill
		if err := json.Unmarshal([]byte(raw), &routine); err != nil {
			problems = append(problems, fmt.Sprintf("The %s exercise's routine couldn't be read.", [...]string{"first", "second"}[i]))
			continue
		}
		ex.Skills = routine
	}
	if err := c.ValidateEntry(&e); err != nil {
		problems = append(problems, sentences(err)...)
	}
	return e, problems
}

// individual is the competition an individual entry link enters.
func (p *competitionPages) individual(w http.ResponseWriter, r *http.Request) (store.Competition, bool) {
	c, err := p.st.CompetitionByIndividualLink(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return c, false
	}
	return c, true
}

func (p *competitionPages) enterForm(w http.ResponseWriter, r *http.Request) {
	c, ok := p.individual(w, r)
	if !ok {
		return
	}
	render(w, r, views.CompetitionEnter(views.EnterPage{
		Competition: summary(c.Competition, p.now()),
		Form:        entryForm(c.Competition, competitions.Entry{}, r.URL.Path, "Enter"),
	}))
}

func (p *competitionPages) enter(w http.ResponseWriter, r *http.Request) {
	c, ok := p.individual(w, r)
	if !ok {
		return
	}
	e, problems := postedEntry(r, c.Competition, "")
	if len(problems) > 0 {
		form := entryForm(c.Competition, e, r.URL.Path, "Enter")
		form.Problems = problems
		w.WriteHeader(http.StatusUnprocessableEntity)
		render(w, r, views.CompetitionEnter(views.EnterPage{Competition: summary(c.Competition, p.now()), Form: form}))
		return
	}
	_, token, err := p.st.AddIndividualEntry(r.Context(), c.ID, e)
	if err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, "/competitions/entry/"+token+"?saved=1", http.StatusSeeOther)
}

// own is the entry a personal link opens, with its competition.
func (p *competitionPages) own(w http.ResponseWriter, r *http.Request) (store.Entry, store.Competition, bool) {
	e, err := p.st.IndividualEntry(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return e, store.Competition{}, false
	}
	c, err := p.st.Competition(r.Context(), e.CompetitionID)
	if err != nil {
		failed(w, r, err)
		return e, c, false
	}
	return e, c, true
}

func (p *competitionPages) ownEntry(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.own(w, r)
	if !ok {
		return
	}
	p.renderOwn(w, r, e, c, e.Entry, nil)
}

// renderOwn shows an individual their entry, and the form to change it
// (starting from form, with any problems).
func (p *competitionPages) renderOwn(w http.ResponseWriter, r *http.Request, e store.Entry, c store.Competition, form competitions.Entry, problems []string) {
	shown, err := card(c.Competition, e.Entry, clubOf(e))
	if err != nil {
		failed(w, r, err)
		return
	}
	path := "/competitions/entry/" + r.PathValue("token")
	page := views.EntryPage{
		Competition: summary(c.Competition, p.now()), Link: origin(r) + path, JustSaved: r.URL.Query().Get("saved") == "1",
		Sent: e.SentAt.In(local).Format("Monday 2 January, 15:04"), Card: shown,
		Form: entryForm(c.Competition, form, path, "Save changes"), Withdraw: path + "/withdraw",
	}
	page.Form.Problems = problems
	if len(problems) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	render(w, r, views.CompetitionEntryPage(page))
}

func (p *competitionPages) replaceEntry(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.own(w, r)
	if !ok {
		return
	}
	changed, problems := postedEntry(r, c.Competition, "")
	if len(problems) > 0 {
		p.renderOwn(w, r, e, c, changed, problems)
		return
	}
	if err := p.st.ReplaceIndividualEntry(r.Context(), r.PathValue("token"), changed); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, "/competitions/entry/"+r.PathValue("token")+"?saved=1", http.StatusSeeOther)
}

func (p *competitionPages) withdraw(w http.ResponseWriter, r *http.Request) {
	_, c, ok := p.own(w, r)
	if !ok {
		return
	}
	if r.FormValue("confirm") != "1" {
		http.Redirect(w, r, "/competitions/entry/"+r.PathValue("token"), http.StatusSeeOther)
		return
	}
	if err := p.st.WithdrawIndividualEntry(r.Context(), r.PathValue("token")); err != nil {
		failed(w, r, err)
		return
	}
	message(w, r, http.StatusOK, "Entry withdrawn", "Your entry for "+c.Name+" has been withdrawn and deleted.")
}

// --- Abuse limits ---

// limiter allows each address n actions per window.
type limiter struct {
	mu     sync.Mutex
	n      int
	window time.Duration
	seen   map[string][]time.Time
}

func newLimiter(n int, window time.Duration) *limiter {
	return &limiter{n: n, window: window, seen: map[string][]time.Time{}}
}

// allow records an action by ip at now, unless it has had n in the window.
func (l *limiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.seen[ip][:0]
	for _, t := range l.seen[ip] {
		if now.Sub(t) < l.window {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.n {
		l.seen[ip] = recent
		return false
	}
	l.seen[ip] = append(recent, now)
	// Forget addresses with nothing recent, so the map doesn't grow forever.
	for k, ts := range l.seen {
		if len(ts) == 0 || now.Sub(ts[len(ts)-1]) >= l.window {
			delete(l.seen, k)
		}
	}
	return true
}

// clientIP is the address a request came from. Behind Caddy, that's the
// rightmost X-Forwarded-For value, the one Caddy added (ADR 0004 Decision 7).
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
