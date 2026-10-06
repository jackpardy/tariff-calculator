package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
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
	handle("POST /competitions/admin/{token}/entries/{id}/check", p.check)
	handle("GET /competitions/admin/{token}/cards", p.cards)
	handle("GET /competitions/admin/{token}/entries.csv", p.csvExport)
	handle("POST /competitions/admin/{token}/deadline", p.deadline)
	handle("POST /competitions/admin/{token}/video", p.video)
	handle("POST /competitions/admin/{token}/signoff", p.setSignoff)
	handle("POST /competitions/admin/{token}/split", p.setSplit)
	handle("GET /competitions/signoff/{token}", p.individualSignoff)
	handle("POST /competitions/signoff/{token}", p.signOffIndividual)
	handle("POST /competitions/admin/{token}/entries/{id}/video", p.reviewVideo)
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
	c.Video, form.Video, problems = postedVideo(r)
	c.Signoff = r.FormValue("signoff") == "1"
	form.Signoff = c.Signoff
	if r.FormValue("split") == competitions.SplitAll {
		c.Split, form.SplitAll = competitions.Split{Mode: competitions.SplitAll}, true
	}
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

// The video proof choices the form offers (views.videoFields), as matchers.
var (
	videoTriples = requirements.Matcher{Label: "any triple somersault", Rotation: &requirements.Range{Min: intPtr(12)}}
	videoDoubles = requirements.Matcher{Label: "any double somersault or more", Rotation: &requirements.Range{Min: intPtr(8)}}
)

func intPtr(n int) *int { return &n }

// videoTariff is the matcher for any skill of a tariff or more.
func videoTariff(min float64) requirements.Matcher {
	return requirements.Matcher{Label: fmt.Sprintf("any skill of tariff %.1f or more", min), Tariff: &requirements.TariffRange{Min: &min}}
}

// postedVideo reads the video proof chosen (video: "", "skills" or "routine";
// for skills, videoTriples, videoDoubles and videoTariff), as the form shows
// it, with any problems.
func postedVideo(r *http.Request) (competitions.Video, views.VideoForm, []string) {
	form := views.VideoForm{
		Need: r.FormValue("video"), Triples: r.FormValue("videoTriples") == "1", Doubles: r.FormValue("videoDoubles") == "1",
		Tariff: strings.TrimSpace(r.FormValue("videoTariff")),
	}
	// Ticking a skill without choosing "some skills" means some skills.
	if form.Need == competitions.VideoNone && (form.Triples || form.Doubles || form.Tariff != "") {
		form.Need = competitions.VideoSkills
	}
	v := competitions.Video{Need: form.Need}
	if form.Need != competitions.VideoSkills {
		return v, form, nil
	}
	var problems []string
	if form.Triples {
		v.Skills = append(v.Skills, videoTriples)
	}
	if form.Doubles {
		v.Skills = append(v.Skills, videoDoubles)
	}
	if form.Tariff != "" {
		if min, err := strconv.ParseFloat(form.Tariff, 64); err != nil || min <= 0 || min > 5 {
			problems = append(problems, "The tariff for video should be a number such as 1.5.")
		} else {
			v.Skills = append(v.Skills, videoTariff(min))
		}
	}
	return v, form, problems
}

// videoForm is a competition's video proof as the form shows it.
func videoForm(v competitions.Video) views.VideoForm {
	form := views.VideoForm{Need: v.Need}
	for _, m := range v.Skills {
		switch {
		case m.Label == videoTriples.Label:
			form.Triples = true
		case m.Label == videoDoubles.Label:
			form.Doubles = true
		case m.Tariff != nil && m.Tariff.Min != nil:
			form.Tariff = strconv.FormatFloat(*m.Tariff.Min, 'f', -1, 64)
		}
	}
	return form
}

// videoStatus is an entry's video for the dashboard: "" when none is needed or
// sent, else missing, provided, OK or need more.
func videoStatus(c competitions.Competition, j judged) string {
	if j.err != nil || c.Video.Need == competitions.VideoNone {
		return ""
	}
	needs := c.VideoNeeds(j.card)
	switch {
	case competitions.VideoMissing(j.Entry.Entry, needs):
		return "missing"
	case j.VideoReview == store.VideoOK:
		return "OK"
	case j.VideoReview == store.VideoMore:
		return "need more"
	case needs[0].Needed || needs[1].Needed || j.Entry.Entry.Exercises[0].Video != "" || j.Entry.Entry.Exercises[1].Video != "":
		return "provided"
	}
	return ""
}

// video changes the video proof a competition asks for.
func (p *competitionPages) video(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	v, _, problems := postedVideo(r)
	changed := c.Competition
	changed.Video = v
	if err := changed.Validate(); err != nil {
		problems = append(problems, sentences(err)...)
	}
	notice := "Video proof changed."
	if len(problems) > 0 {
		notice = "Video proof wasn't changed: " + strings.Join(problems, " ")
	} else if err := p.st.SetVideo(r.Context(), c.ID, v); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

// reviewVideo records the organiser's review of an entry's videos (review:
// "ok", "more" or "" to clear) with a note of what more is needed.
func (p *competitionPages) reviewVideo(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if len([]rune(note)) > 300 {
		note = string([]rune(note)[:300])
	}
	if err := p.st.ReviewVideo(r.Context(), c.ID, r.PathValue("id"), r.FormValue("review"), note); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			failed(w, r, err)
		} else {
			badRequest(w, err)
		}
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"/entries/"+r.PathValue("id"), http.StatusSeeOther)
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
		Video: c.Video.Describe(), Signoff: c.Signoff,
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

// judged is a stored entry checked, with who sent it ("individual" for an
// individual) and its problems.
type judged struct {
	store.Entry
	club     string
	card     competitions.Card
	err      error // the entry couldn't be checked (its level is gone)
	problems []string
}

// judge checks every entry.
func judge(c competitions.Competition, entries []store.Entry) []judged {
	out := make([]judged, len(entries))
	for i, e := range entries {
		j := judged{Entry: e, club: e.ClubName}
		if e.Individual {
			j.club = "individual"
		}
		if j.card, j.err = c.Check(e.Entry); j.err != nil {
			j.problems = []string{j.err.Error()}
		} else {
			j.problems = j.card.Problems()
			if c.Signoff && !j.SignedOff() && !j.Withdrawn {
				j.problems = append(j.problems, "Not signed off by a coach")
			}
		}
		out[i] = j
	}
	return out
}

// matches says whether an entry passes the dashboard's filter: club (a
// club's name, or "individual"), problems=1, unchecked=1, level and entry (an id).
func (j judged) matches(q url.Values) bool {
	switch {
	case q.Get("club") != "" && q.Get("club") != j.club,
		q.Get("problems") == "1" && (len(j.problems) == 0 || j.Withdrawn),
		q.Get("unchecked") == "1" && (j.Checked() || j.Withdrawn),
		q.Get("level") != "" && q.Get("level") != j.Entry.Entry.Level,
		q.Get("entry") != "" && q.Get("entry") != j.ID:
		return false
	}
	return true
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
	q := r.URL.Query()
	deadline := c.Deadline.In(local)
	d := views.Dashboard{
		Base: adminPath(r.PathValue("token")), Competition: summary(c.Competition, p.now()),
		Links: views.CompetitionLinks{Club: origin(r) + "/competitions/club/" + c.ClubLink, Individual: origin(r) + "/competitions/enter/" + c.IndividualLink},
		Club:  q.Get("club"), ProblemsOnly: q.Get("problems") == "1",
		Entries: len(entries), New: q.Get("new"), Notice: q.Get("notice"),
		DeadlineDate: deadline.Format("2006-01-02"), DeadlineTime: deadline.Format("15:04"),
		Video: videoForm(c.Video), Split: splitForm(c.Split, c.LevelNames()),
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
	filter := url.Values{"club": {d.Club}}
	if d.ProblemsOnly {
		filter.Set("problems", "1")
	}
	for _, j := range judge(c.Competition, entries) {
		if !j.Individual && !seen[j.club] {
			d.Clubs, seen[j.club] = append(d.Clubs, j.club), true // a club deleted since it sent
		}
		// A withdrawn entry isn't live: it's listed, marked, but not counted.
		switch {
		case j.Withdrawn:
			d.Entries--
			d.Withdrawn++
		case len(j.problems) > 0:
			d.WithProblems++
		}
		if !j.Checked() && !j.Withdrawn {
			d.Unchecked++
		}
		if !j.matches(filter) {
			continue
		}
		row := views.DashboardRow{
			ID: j.ID, Gymnast: j.Entry.Entry.Gymnast, Club: j.club, Problems: j.problems,
			Sent: j.SentAt.In(local).Format("2 Jan, 15:04"), Checked: j.Checked(), Note: j.Note, Withdrawn: j.Withdrawn,
			Video: videoStatus(c.Competition, j), Signoff: storedSignoff(c.Competition, j.Entry), Category: j.Entry.Entry.Category,
		}
		if j.err == nil {
			for i, ex := range []requirements.Checked{j.card.First, j.card.Second} {
				row.Met, row.Rules = row.Met+ex.Met(), row.Rules+len(ex.Results)
				row.Exercises[i] = exerciseSummary(j.Entry.Entry.Exercises[i], ex)
			}
		}
		n, ok := byLevel[j.Entry.Entry.Level]
		if !ok {
			n, byLevel[j.Entry.Entry.Level] = len(d.Levels), len(d.Levels)
			d.Levels = append(d.Levels, views.DashboardLevel{Name: j.Entry.Entry.Level})
		}
		d.Levels[n].Rows = append(d.Levels[n].Rows, row)
	}
	render(w, r, views.CompetitionDashboard(d))
}

// cards prints the competition cards of the entries the dashboard's filter
// selects (club, problems, unchecked, level, entry), one exercise per page, by
// level then as listed.
func (p *competitionPages) cards(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	entries, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	page := views.CardsPage{Title: "Cards · " + c.Name, Back: adminPath(r.PathValue("token"))}
	q := r.URL.Query()
	all := judge(c.Competition, entries)
	for _, level := range levelOrder(c.Competition, all) {
		for _, j := range all {
			if j.Entry.Entry.Level != level || !j.matches(q) || j.err != nil || (j.Withdrawn && q.Get("entry") == "") {
				continue
			}
			club := j.ClubName
			if j.Individual {
				club = ""
			}
			for i, ex := range []requirements.Checked{j.card.First, j.card.Second} {
				page.Cards = append(page.Cards, views.PrintedCard{
					Validation: ex.Validation, Required: ex.Required, Checks: ex.Checks,
					Details: views.SheetDetails{
						"gymnast": j.Entry.Entry.Gymnast, "club": club, "category": strings.TrimSpace(j.card.Level.Name + " " + j.Entry.Entry.Category),
						"competition": c.Name, "round": [...]string{"1st exercise", "2nd exercise"}[i] + " · " + ex.SetName,
						"coach": j.CoachName(),
					},
				})
			}
		}
	}
	render(w, r, views.CompetitionCards(page))
}

// levelOrder is the competition's levels in order, then any level entries
// name that it no longer offers.
func levelOrder(c competitions.Competition, entries []judged) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range c.Levels {
		if level, err := l.Resolve(); err == nil && !seen[level.Name] {
			out, seen[level.Name] = append(out, level.Name), true
		}
	}
	for _, j := range entries {
		if name := j.Entry.Entry.Level; !seen[name] {
			out, seen[name] = append(out, name), true
		}
	}
	return out
}

// csvExport is every entry as CSV, for a scoring system or a spreadsheet: one
// row per gymnast, with each exercise's requirements and difficulty.
func (p *competitionPages) csvExport(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	entries, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName(c.Name)+".csv"))
	out := csv.NewWriter(w)
	out.Write([]string{"Gymnast", "Club", "Level", "1st exercise", "1st difficulty", "2nd exercise", "2nd difficulty", "Problems", "Checked", "Note", "Sent", "Video", "Withdrawn", "Signed off by", "Category"})
	all := judge(c.Competition, entries)
	for _, level := range levelOrder(c.Competition, all) {
		for _, j := range all {
			if j.Entry.Entry.Level != level {
				continue
			}
			club := j.ClubName
			if j.Individual {
				club = "Individual"
			}
			row := []string{j.Entry.Entry.Gymnast, club, level, "", "", "", "", strconv.Itoa(len(j.problems)), "", j.Note, j.SentAt.In(local).Format("2006-01-02 15:04"), videoStatus(c.Competition, j), "", "", j.Entry.Entry.Category}
			if j.err == nil {
				for i, ex := range []requirements.Checked{j.card.First, j.card.Second} {
					row[3+2*i] = ex.SetName
					if ex.Checks.ScoreDifficulty {
						row[4+2*i] = fmt.Sprintf("%.1f", ex.Validation.TotalTariff)
					}
				}
			}
			if j.Checked() {
				row[8] = "yes"
			}
			if j.Withdrawn {
				row[12] = "yes"
			}
			if j.SignedOff() {
				row[13] = j.SignedBy
			}
			for i := range row {
				row[i] = csvSafe(row[i])
			}
			out.Write(row)
		}
	}
	out.Flush()
}

// csvSafe stops a spreadsheet reading a cell as a formula.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// fileName is a name made safe for a downloaded file.
func fileName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "entries"
	}
	return b.String()
}

// check marks an entry checked (checked=1) or not, with a note for the club
// or gymnast.
func (p *competitionPages) check(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if len([]rune(note)) > 500 {
		note = string([]rune(note)[:500])
	}
	if err := p.st.MarkChecked(r.Context(), c.ID, r.PathValue("id"), r.FormValue("checked") == "1", note); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"/entries/"+r.PathValue("id"), http.StatusSeeOther)
}

// deadline closes entries now (close=1) or changes when they close
// (deadlineDate, deadlineTime), up to the end of the competition date.
func (p *competitionPages) deadline(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	notice := "Entries are closed."
	deadline := p.now()
	if r.FormValue("close") != "1" {
		var err error
		deadline, err = time.ParseInLocation("2006-01-02 15:04", r.FormValue("deadlineDate")+" "+r.FormValue("deadlineTime"), local)
		changed := c.Competition
		changed.Deadline = deadline
		if err != nil || changed.Validate() != nil {
			http.Redirect(w, r, adminPath(r.PathValue("token"))+"?notice="+url.QueryEscape("Entries must close by the end of the competition date; the closing time wasn't changed."), http.StatusSeeOther)
			return
		}
		notice = "Entries now close " + deadline.Format("Monday 2 January 2006, 15:04") + "."
	}
	if err := p.st.SetDeadline(r.Context(), c.ID, deadline); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
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
	withSignoff(&shown, storedSignoff(c.Competition, e))
	render(w, r, views.CompetitionEntry(views.EntryDetail{
		Base: adminPath(r.PathValue("token")), Competition: summary(c.Competition, p.now()),
		Card: shown, Sent: e.SentAt.In(local).Format("Monday 2 January, 15:04"),
		ID: e.ID, Checked: checkedText(e), Note: e.Note, VideoReview: e.VideoReview, VideoNote: e.VideoNote,
		Withdrawn: e.Withdrawn,
	}))
}

// checkedText says when an entry was marked checked, "" if it isn't.
func checkedText(e store.Entry) string {
	if !e.Checked() {
		return ""
	}
	return e.CheckedAt.In(local).Format("Monday 2 January, 15:04")
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
	for i, need := range c.VideoNeeds(checked) {
		out.Exercises[i].Video = views.VideoView{Needed: need.Needed, Skills: need.Skills, Link: e.Exercises[i].Video, Note: e.Exercises[i].VideoNote}
	}
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
	f := views.EntryForm{Action: action, Submit: submit, Gymnast: e.Gymnast, Level: e.Level, Category: e.Category, Video: c.Video.Describe()}
	for _, l := range c.Levels {
		level, err := l.Resolve()
		if err != nil {
			continue
		}
		el := views.EntryLevel{Name: level.Name, Split: c.Split.Splits(level.Name)}
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
				ex.Video, ex.Note = e.Exercises[n].Video, e.Exercises[n].VideoNote
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
	e := competitions.Entry{Gymnast: gymnast, Level: r.FormValue("level"), Category: r.FormValue("category")}
	var problems []string
	l, _, ok := c.Level(e.Level)
	for i := range e.Exercises {
		ex := &e.Exercises[i]
		ex.Option = r.FormValue(fmt.Sprintf("ex%dOption", i+1))
		if c.Video.Need != competitions.VideoNone {
			ex.Video, ex.VideoNote = r.FormValue(fmt.Sprintf("ex%dVideo", i+1)), r.FormValue(fmt.Sprintf("ex%dVideoNote", i+1))
		}
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
	withSignoff(&shown, storedSignoff(c.Competition, e))
	path := "/competitions/entry/" + r.PathValue("token")
	page := views.EntryPage{
		Competition: summary(c.Competition, p.now()), Link: origin(r) + path, JustSaved: r.URL.Query().Get("saved") == "1",
		Sent: e.SentAt.In(local).Format("Monday 2 January, 15:04"), Card: shown, Checked: e.Checked(), Note: e.Note,
		VideoReview: e.VideoReview, VideoNote: e.VideoNote,
		Form: entryForm(c.Competition, form, path, "Save changes"), Withdraw: path + "/withdraw",
	}
	if c.Signoff {
		page.SignoffLink = origin(r) + signoffPath(e.SignoffLink)
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

// deleteExpired deletes competitions 120 days after their date and clubs
// unused for 120 days (ADR 0004 Decision 6), now and every few hours.
func deleteExpired(st *store.Store) {
	for {
		comps, clubs, err := st.DeleteExpired(context.Background())
		switch {
		case err != nil:
			log.Printf("Deleting expired competitions and clubs: %v", err)
		case comps+clubs > 0:
			log.Printf("Deleted %d expired competitions and %d unused clubs", comps, clubs)
		}
		time.Sleep(6 * time.Hour)
	}
}

// --- Men and women ---

// postedSplit reads a choice of levels to split: name ("", "all" or "some")
// and, with "some", each level ticked (levelName).
func postedSplit(r *http.Request, name, levelName string) competitions.Split {
	s := competitions.Split{Mode: r.FormValue(name)}
	if s.Mode == competitions.SplitSome {
		s.Levels = r.Form[levelName]
	}
	return s
}

// splitForm is a split as the form shows it.
func splitForm(s competitions.Split, levels []string) views.SplitForm {
	f := views.SplitForm{Mode: s.Mode, Levels: levels, Chosen: map[string]bool{}}
	for _, l := range s.Levels {
		f.Chosen[l] = true
	}
	return f
}

// setSplit changes which levels rank men and women separately. Entries made
// before keep their category; entries for a newly split level need one.
func (p *competitionPages) setSplit(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	split := postedSplit(r, "split", "splitLevel")
	changed := c.Competition
	changed.Split = split
	notice := "Men and women changed."
	if err := changed.Validate(); err != nil {
		notice = "Men and women weren't changed: " + strings.Join(sentences(err), " ")
	} else if err := p.st.SetSplit(r.Context(), c.ID, split); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

// --- Coach sign-off (ADR 0004 Decision 11) ---

func signoffPath(token string) string { return "/competitions/signoff/" + token }

// storedSignoff is a competition's entry's sign-off, as the pages show it.
func storedSignoff(c competitions.Competition, e store.Entry) views.SignoffView {
	return views.SignoffView{Required: c.Signoff, Signed: e.SignedOff(), By: e.SignedBy, Note: e.SignNote}
}

// setSignoff turns on or off whether entries need a coach's sign-off.
func (p *competitionPages) setSignoff(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	on := r.FormValue("on") == "1"
	if err := p.st.SetSignoff(r.Context(), c.ID, on); err != nil {
		failed(w, r, err)
		return
	}
	notice := "Entries no longer need a coach's sign-off."
	if on {
		notice = "Entries now need a coach's sign-off: those without one are flagged."
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

// signoffEntry is the individual's entry a sign-off link opens, with its competition.
func (p *competitionPages) signoffEntry(w http.ResponseWriter, r *http.Request) (store.Entry, store.Competition, bool) {
	e, err := p.st.EntryBySignoffLink(r.Context(), r.PathValue("token"))
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

// individualSignoff is the page an individual's coach signs off their entry on.
func (p *competitionPages) individualSignoff(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.signoffEntry(w, r)
	if !ok {
		return
	}
	p.renderIndividualSignoff(w, r, e, c, e.SignedBy, nil)
}

func (p *competitionPages) renderIndividualSignoff(w http.ResponseWriter, r *http.Request, e store.Entry, c store.Competition, name string, problems []string) {
	shown, err := card(c.Competition, e.Entry, clubOf(e))
	if err != nil {
		failed(w, r, err)
		return
	}
	withSignoff(&shown, storedSignoff(c.Competition, e))
	page := views.SignoffPage{
		Competition: summary(c.Competition, p.now()), Card: shown, Signoff: shown.Signoff,
		Action: r.URL.Path, AskName: true, Name: name, Problems: problems,
	}
	if len(problems) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	render(w, r, views.SignoffEntry(page))
}

// signOffIndividual records the coach's sign-off (signed=1) or not yet, under
// their name (coach), with a note.
func (p *competitionPages) signOffIndividual(w http.ResponseWriter, r *http.Request) {
	e, c, ok := p.signoffEntry(w, r)
	if !ok {
		return
	}
	if !c.Open(p.now()) {
		failed(w, r, store.ErrClosed)
		return
	}
	name := strings.TrimSpace(r.FormValue("coach"))
	if err := competitions.CheckName("coach", name); err != nil {
		p.renderIndividualSignoff(w, r, e, c, name, sentences(err))
		return
	}
	if err := p.st.SignOffIndividual(r.Context(), r.PathValue("token"), name, r.FormValue("signed") == "1", limitNote(r.FormValue("note"))); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
}

// limitNote is a note trimmed to at most 300 characters.
func limitNote(note string) string {
	note = strings.TrimSpace(note)
	if r := []rune(note); len(r) > 300 {
		note = string(r[:300])
	}
	return note
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
