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
	"slices"
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
	emailLimiter                      *limiter // confirmation emails asked for, per address
	now                               func() time.Time
	notify                            *notifier // nil when storage is off
}

func newCompetitionPages(st *store.Store) *competitionPages {
	p := &competitionPages{
		st:           st,
		limiter:      newLimiter(maxCompetitionsPerHour, time.Hour),
		clubLimiter:  newLimiter(maxClubsPerHour, time.Hour),
		joinLimiter:  newLimiter(maxJoinsPerHour, time.Hour),
		emailLimiter: newLimiter(maxEmailsPerHour, time.Hour),
		now:          time.Now,
	}
	if st != nil {
		p.notify = newNotifier(st)
	}
	return p
}

func (p *competitionPages) register(mux *http.ServeMux) {
	handle := func(pattern string, h http.HandlerFunc) {
		if strings.Contains(pattern, " "+adminPrefix) {
			h = p.gate(pattern, h)
		}
		mux.Handle(pattern, p.secret(h))
	}
	p.registerAccess(handle)
	p.registerFees(handle)
	p.registerLimits(handle)
	p.registerLate(handle)
	handle("GET /competitions/new", p.newForm)
	handle("POST /competitions", p.create)
	handle("GET /competitions/admin/{token}", p.dashboard)
	handle("GET /competitions/admin/{token}/entries/{id}", p.entry)
	handle("POST /competitions/admin/{token}/entries/{id}/check", p.check)
	handle("GET /competitions/admin/{token}/cards", p.cards)
	handle("GET /competitions/admin/{token}/entries.csv", p.csvExport)
	handle("POST /competitions/admin/{token}/deadline", p.deadline)
	handle("POST /competitions/admin/{token}/later-deadlines", p.laterDeadlines)
	handle("POST /competitions/admin/{token}/live", p.live)
	handle("POST /competitions/admin/{token}/video", p.video)
	handle("POST /competitions/admin/{token}/signoff", p.setSignoff)
	handle("POST /competitions/admin/{token}/remove", p.removeEntries)
	handle("POST /competitions/admin/{token}/entries/{id}/restore", p.restoreEntry)
	handle("POST /competitions/admin/{token}/split", p.setSplit)
	handle("POST /competitions/admin/{token}/events", p.setEvents)
	handle("POST /competitions/admin/{token}/levels/move", p.moveLevel)
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
	p.registerTimetable(handle)
	p.registerPartners(handle)
	p.registerOfficials(handle)
	p.registerApprovals(handle)
	p.registerMine(handle)
	p.registerTimelines(handle)
	p.registerCalendars(handle)
	p.registerNotify(handle)
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
	case errors.Is(err, store.ErrNotOpen):
		message(w, r, http.StatusConflict, "Entries aren't open", "The organiser hasn't opened entries for this competition, so they can't be sent or changed just now.")
	case errors.Is(err, store.ErrClosed):
		message(w, r, http.StatusConflict, "Entries have closed", "The deadline for this competition has passed, so entries can't be changed.")
	case errors.Is(err, store.ErrLimit):
		message(w, r, http.StatusConflict, "Competition full", "This competition has as many entries as it can take.")
	case errors.Is(err, store.ErrRemoved):
		message(w, r, http.StatusConflict, "Entry removed", "The organiser has removed this entry, so it can't be changed. Ask them if you think it should be back in.")
	default:
		log.Printf("Competition storage: %s %s: %v", r.Method, r.URL.Path, err)
		message(w, r, http.StatusInternalServerError, "Something went wrong", "That didn't work. Please try again in a minute.")
	}
}

// notOpen is why a competition's entries aren't open: not live yet (or
// paused), or closed.
func notOpen(c competitions.Competition, now time.Time) error {
	if !c.Live(now) {
		return store.ErrNotOpen
	}
	return store.ErrClosed
}

// --- Creating a competition ---

func (p *competitionPages) newForm(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.NewCompetition(views.CompetitionForm{
		DeadlineTime: "23:59", Opens: "later", LiveTime: "09:00", Individuals: true, Levels: map[string]bool{}, Groups: requirements.BuiltinGroups(),
		Events: views.EventsForm{Groups: requirements.BuiltinGroups(), Synchro: map[string]bool{}},
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
		Opens: r.FormValue("opens"), LiveDate: r.FormValue("liveDate"), LiveTime: r.FormValue("liveTime"),
	}
	c := competitions.Competition{Name: form.Name, Date: form.Date, Individuals: form.Individuals}
	var problems []string
	c.Video, form.Video, problems = postedVideo(r)
	if at, err := postedLiveAt(r, p.now()); err != nil {
		problems = append(problems, err.Error())
	} else {
		c.LiveAt = at
	}
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
	var pairs [][]string
	c.Synchro, c.Tumbling, c.DMT, pairs, form.Events = postedEvents(r)
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
	// Easiest first: built-ins by rank, the coach's own levels after them, to
	// move where they belong.
	c.Levels, c.Synchro = competitions.OrderLevels(nil, c.Levels), competitions.OrderLevels(nil, c.Synchro)
	if paired, err := competitions.PairLevels(c.Synchro, pairs); err != nil {
		problems = append(problems, sentences(err)...)
	} else {
		c.Synchro = paired
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

// dateField and timeField are a later closing time for a form: "" if it
// isn't later than the deadline.
func dateField(t, deadline time.Time) string {
	if !t.After(deadline) {
		return ""
	}
	return t.In(local).Format("2006-01-02")
}

func timeField(t, deadline time.Time) string {
	if !t.After(deadline) {
		return ""
	}
	return t.In(local).Format("15:04")
}

// summary is what the pages say about a competition.
func summary(c competitions.Competition, now time.Time) views.CompetitionSummary {
	out := views.CompetitionSummary{
		Name: c.Name, Date: c.Date, Deadline: c.Deadline.In(local).Format("Monday 2 January 2006, 15:04"),
		DeleteAfter: c.DeleteAfter().Format("2 January 2006"), Open: c.Open(now), Individuals: c.Individuals,
		ChangesOpen: c.ChangesOpen(now), SignoffsOpen: c.SignoffsOpen(now),
		Video: c.Video.Describe(), Signoff: c.Signoff, Live: c.Live(now),
	}
	if !c.LiveAt.IsZero() && !out.Live {
		out.Opens = c.LiveAt.In(local).Format("Monday 2 January 2006, 15:04")
	}
	if c.ChangesClose().After(c.Deadline) {
		out.ChangesUntil = c.ChangesClose().In(local).Format("Monday 2 January 2006, 15:04")
	}
	if c.SignoffsClose().After(c.Deadline) {
		out.SignoffsUntil = c.SignoffsClose().In(local).Format("Monday 2 January 2006, 15:04")
	}
	if day, err := c.Day(); err == nil {
		out.Date = day.Format("Monday 2 January 2006")
	}
	return out
}

// --- The organiser ---

// admin is the competition an admin link opens, or a page saying it doesn't.
func (p *competitionPages) admin(w http.ResponseWriter, r *http.Request) (store.Competition, bool) {
	c, _, err := p.st.AdminLink(r.Context(), r.PathValue("token"))
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
	video    string // videoStatus
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
			if problem := storedSignoff(c, j.Entry).Problem(); problem != "" && !j.Withdrawn {
				j.problems = append(j.problems, problem)
			}
		}
		j.video = videoStatus(c, j)
		out[i] = j
	}
	for i, problem := range synchroLevels(c, entries) {
		out[i].problems = append([]string{problem}, out[i].problems...)
	}
	return out
}

// synchroLevels checks each synchro pair's level against their individual
// trampoline levels (by entry index): the same level, or the easier of two a
// level apart; further apart, they usually can't pair.
func synchroLevels(c competitions.Competition, entries []store.Entry) map[int]string {
	keys := personKeys(entries)
	individual := map[string]string{} // person → their trampoline level
	for _, e := range entries {
		if !e.Withdrawn && e.Entry.Discipline == competitions.Trampoline {
			individual[keys[e.ID][0]] = e.Entry.Level
		}
	}
	out := map[int]string{}
	for i, e := range entries {
		ks := keys[e.ID]
		if e.Withdrawn || e.Entry.Discipline != competitions.Synchro || len(ks) < 2 || e.Entry.Partner == nil {
			continue
		}
		a, b := individual[ks[0]], individual[ks[1]]
		if a == "" || b == "" {
			continue
		}
		pair := fmt.Sprintf("%s (%s) and %s (%s)", e.Entry.Gymnast, a, e.Entry.Partner.Name, b)
		switch want, ok := c.SynchroLevel(a, b); {
		case !ok:
			out[i] = pair + " compete more than a level apart individually, so usually can't pair in synchro"
		case want != e.Entry.DoesLevel():
			out[i] = fmt.Sprintf("%s compete individually, so as a pair they do %s in synchro, not %s", pair, want, e.Entry.DoesLevel())
		}
	}
	return out
}

// matches says whether an entry passes the dashboard's filter: club (a
// club's name, or "individual"), problems=1, unchecked=1, level, entry (an
// id), q (a name, gymnast or synchro partner), coach (their name, or "none"),
// checked and signedoff (yes or no), video (missing, provided, OK or need
// more) and category (Men or Women).
func (j judged) matches(q url.Values) bool {
	yesNo := func(key string, is bool) bool {
		return q.Get(key) == "" || (q.Get(key) == "yes") == is
	}
	coach := q.Get("coach")
	switch {
	case q.Get("club") != "" && q.Get("club") != j.club,
		q.Get("problems") == "1" && (len(j.problems) == 0 || j.Withdrawn),
		q.Get("unchecked") == "1" && (j.Checked() || j.Withdrawn),
		q.Get("level") != "" && q.Get("level") != j.Entry.Entry.Event(),
		q.Get("entry") != "" && q.Get("entry") != j.ID,
		!competitions.NameMatches(j.Entry.Entry.Gymnasts(), q.Get("q")),
		coach == "none" && j.CoachName() != "",
		coach != "" && coach != "none" && coach != j.CoachName(),
		!yesNo("checked", j.Checked()),
		!yesNo("signedoff", j.SignedOff()),
		q.Get("video") != "" && q.Get("video") != j.video,
		q.Get("category") != "" && q.Get("category") != j.Entry.Entry.Category:
		return false
	}
	return true
}

// filterKeys are the dashboard's filters, as matches reads them.
var filterKeys = []string{"club", "problems", "q", "coach", "checked", "signedoff", "video", "category"}

// dashboardFilter is the request's filters, without anything else.
func dashboardFilter(q url.Values) url.Values {
	out := url.Values{}
	for _, k := range filterKeys {
		if v := strings.TrimSpace(q.Get(k)); v != "" {
			out.Set(k, v)
		}
	}
	return out
}

// sortRows orders a level's rows by gymnast, club, sent (latest first) or
// coach; otherwise as they came (by club, then gymnast).
func sortRows(rows []views.DashboardRow, sentAt map[string]time.Time, by string) {
	key := map[string]func(r views.DashboardRow) string{
		"gymnast": func(r views.DashboardRow) string { return strings.ToLower(r.Gymnast) },
		"club":    func(r views.DashboardRow) string { return strings.ToLower(r.Club) },
		"coach":   func(r views.DashboardRow) string { return strings.ToLower(r.Signoff.By + "\x00" + r.Coach) },
	}[by]
	switch {
	case by == "sent":
		slices.SortStableFunc(rows, func(a, b views.DashboardRow) int { return sentAt[b.ID].Compare(sentAt[a.ID]) })
	case key != nil:
		slices.SortStableFunc(rows, func(a, b views.DashboardRow) int { return strings.Compare(key(a), key(b)) })
	}
}

// dashboard lists every entry by level, filtered by club (club: a club's name,
// or "individual") and to entries with problems (problems=1).
func (p *competitionPages) dashboard(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	all, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	entries, waiting := splitWaiting(all)
	clubs, err := p.st.CompetitionClubs(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	q := r.URL.Query()
	deadline := c.Deadline.In(local)
	liveAt := c.LiveAt.In(local)
	if c.LiveAt.IsZero() {
		today := p.now().In(local) // suggest tomorrow morning
		liveAt = time.Date(today.Year(), today.Month(), today.Day()+1, 9, 0, 0, 0, local)
	}
	d := views.Dashboard{
		Base: adminPath(r.PathValue("token")), Competition: summary(c.Competition, p.now()),
		Links: views.CompetitionLinks{Club: origin(r) + "/competitions/club/" + c.ClubLink, Individual: origin(r) + "/competitions/enter/" + c.IndividualLink},
		Club:  q.Get("club"), ProblemsOnly: q.Get("problems") == "1",
		Entries: len(entries), New: q.Get("new"), Notice: q.Get("notice"),
		DeadlineDate: deadline.Format("2006-01-02"), DeadlineTime: deadline.Format("15:04"),
		LiveDate: liveAt.Format("2006-01-02"), LiveTime: liveAt.Format("15:04"),
		ChangesDate: dateField(c.ChangesUntil, c.Deadline), ChangesTime: timeField(c.ChangesUntil, c.Deadline),
		SignoffsDate: dateField(c.SignoffsUntil, c.Deadline), SignoffsTime: timeField(c.SignoffsUntil, c.Deadline),
		NotifyOn: p.notifyLink("/") != "", NoWait: c.NoWait,
		Video: videoForm(c.Video), Split: splitForm(c.Split, c.EventNames()), Events: eventsForm(c.Competition),
		LevelOrder: levelOrderForm(c.Competition),
	}
	if !c.NotifyDue.IsZero() {
		d.NotifyAt = c.NotifyDue.In(local).Format("15:04")
	}
	d.Waiting, d.Limits = waitingLists(c, waiting, d.Base), limitFields(c, all)
	if late, err := p.st.LateRequests(r.Context(), c.ID); err == nil {
		for _, l := range late {
			if l.Status == store.LateWaiting {
				d.LateWaiting++
			}
		}
	}
	d.Access = access(r)
	if d.Access.Organiser {
		links, err := p.st.Links(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		d.LinkList = linkViews(links)
	}
	if d.Concerns, d.OpenConcerns, err = p.concernViews(r.Context(), c, d.Base); err != nil {
		failed(w, r, err)
		return
	}
	removed, err := p.st.RemovedEntries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	for _, e := range removed {
		d.Removed = append(d.Removed, views.RemovedRow{ID: e.ID, Gymnast: e.Entry.Gymnasts(), Club: clubOf(e), Level: e.Entry.Event(),
			Held: e.Removal == store.Held, Resent: e.Resent, Note: e.RemovalNote, To: reasonTo(e)})
	}
	if c.Signoff {
		sentCoaches, err := p.st.CompetitionCoaches(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		named, err := p.st.CompetitionEntryCoaches(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		d.CoachApproval = coachApprovalForm(c.Competition, sentCoaches, named)
	}
	if d.New != "" {
		d.Links.Admin = origin(r) + adminPath(r.PathValue("token"))
	}
	seen := map[string]bool{}
	for _, club := range clubs {
		d.Clubs, seen[club.Name] = append(d.Clubs, club.Name), true
	}
	byLevel := map[string]int{}
	for _, name := range c.EventNames() {
		byLevel[name] = len(d.Levels)
		d.Levels = append(d.Levels, views.DashboardLevel{Name: name})
	}
	filter := dashboardFilter(q)
	d.Filter, d.Search, d.Coach, d.CheckedFilter, d.SignedFilter, d.VideoFilter, d.Category, d.Sort =
		filter, filter.Get("q"), filter.Get("coach"), filter.Get("checked"), filter.Get("signedoff"), filter.Get("video"), filter.Get("category"), q.Get("sort")
	d.CanSplit = c.Split.Any()
	coaches := map[string]bool{}
	sentAt := map[string]time.Time{}
	for _, j := range judge(c.Competition, entries) {
		if name := j.CoachName(); name != "" && !coaches[name] {
			coaches[name] = true
			d.Coaches = append(d.Coaches, name)
		}

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
		d.Shown++
		sentAt[j.ID] = j.SentAt
		row := views.DashboardRow{
			ID: j.ID, Gymnast: j.Entry.Entry.Gymnasts(), Club: j.club, Problems: j.problems, Coach: j.CoachName(),
			Sent: j.SentAt.In(local).Format("2 Jan, 15:04"), Checked: j.Checked(), Note: j.Note, Withdrawn: j.Withdrawn,
			Video: videoStatus(c.Competition, j), Signoff: storedSignoff(c.Competition, j.Entry), Category: j.Entry.Entry.Category,
		}
		if j.err == nil && !j.card.Unchecked {
			for i, ex := range []requirements.Checked{j.card.First, j.card.Second} {
				if !j.card.Does(i) {
					continue
				}
				row.Met, row.Rules = row.Met+ex.Met(), row.Rules+len(ex.Results)
				row.Exercises[i] = exerciseSummary(j.Entry.Entry.Exercises[i], ex)
			}
		}
		n, ok := byLevel[j.Entry.Entry.Event()]
		if !ok {
			n, byLevel[j.Entry.Entry.Event()] = len(d.Levels), len(d.Levels)
			d.Levels = append(d.Levels, views.DashboardLevel{Name: j.Entry.Entry.Event()})
		}
		d.Levels[n].Rows = append(d.Levels[n].Rows, row)
	}
	slices.Sort(d.Coaches)
	totals := map[string]int{}
	for _, e := range entries {
		if !e.Withdrawn {
			totals[e.Entry.Event()]++
		}
	}
	for i := range d.Levels {
		d.Levels[i].Anchor, d.Levels[i].Total = "level-"+strconv.Itoa(i), totals[d.Levels[i].Name]
		if limit := c.Limits[d.Levels[i].Name]; limit > 0 {
			d.Levels[i].Places = fmt.Sprintf("%d of %d places", totals[d.Levels[i].Name], limit)
		}
		sortRows(d.Levels[i].Rows, sentAt, d.Sort)
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
	stored, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	entries, _ := splitWaiting(stored)
	page := views.CardsPage{Title: "Cards · " + c.Name, Back: adminPath(r.PathValue("token"))}
	q := r.URL.Query()
	all := judge(c.Competition, entries)
	for _, level := range levelOrder(c.Competition, all) {
		for _, j := range all {
			if j.Entry.Entry.Event() != level || !j.matches(q) || j.err != nil || j.card.Unchecked || (j.Withdrawn && q.Get("entry") == "") {
				continue
			}
			club := j.ClubName
			if j.Individual {
				club = ""
			}
			for i, ex := range []requirements.Checked{j.card.First, j.card.Second} {
				if !j.card.Does(i) {
					continue
				}
				round := [...]string{"1st exercise", "2nd exercise"}[i]
				if j.card.Only > 0 {
					round = "Routine"
				}
				page.Cards = append(page.Cards, views.PrintedCard{
					Validation: ex.Validation, Required: ex.Required, Checks: ex.Checks,
					Details: views.SheetDetails{
						"gymnast": j.Entry.Entry.Gymnasts(), "club": club, "category": strings.TrimSpace(j.Entry.Entry.Event() + " " + j.Entry.Entry.Category),
						"competition": c.Name, "round": round + " · " + ex.SetName,
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
	for _, name := range c.EventNames() {
		if !seen[name] {
			out, seen[name] = append(out, name), true
		}
	}
	for _, j := range entries {
		if name := j.Entry.Entry.Event(); !seen[name] {
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
	stored, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	entries, _ := splitWaiting(stored)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName(c.Name)+".csv"))
	out := csv.NewWriter(w)
	out.Write([]string{"Gymnast", "Club", "Level", "1st exercise", "1st difficulty", "2nd exercise", "2nd difficulty", "Problems", "Checked", "Note", "Sent", "Video", "Withdrawn", "Signed off by", "Category"})
	all := judge(c.Competition, entries)
	q := r.URL.Query()
	for _, level := range levelOrder(c.Competition, all) {
		for _, j := range all {
			if j.Entry.Entry.Event() != level || !j.matches(q) {
				continue
			}
			club := j.ClubName
			if j.Individual {
				club = "Individual"
			}
			row := []string{j.Entry.Entry.Gymnasts(), club, level, "", "", "", "", strconv.Itoa(len(j.problems)), "", j.Note, j.SentAt.In(local).Format("2006-01-02 15:04"), videoStatus(c.Competition, j), "", "", j.Entry.Entry.Category}
			if j.err == nil && !j.card.Unchecked {
				for i, ex := range []requirements.Checked{j.card.First, j.card.Second} {
					if !j.card.Does(i) {
						continue
					}
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
	p.notify.changing(r.Context(), c)
	if err := p.st.MarkChecked(r.Context(), c.ID, r.PathValue("id"), r.FormValue("checked") == "1", note); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"/entries/"+r.PathValue("id"), http.StatusSeeOther)
}

// postedLiveAt is when a form says entries open: opens=now, opens=at with
// liveDate and liveTime still to come, or anything else for private (zero).
func postedLiveAt(r *http.Request, now time.Time) (time.Time, error) {
	switch r.FormValue("opens") {
	case "now":
		return now, nil
	case "at":
		at, err := time.ParseInLocation("2006-01-02 15:04", r.FormValue("liveDate")+" "+r.FormValue("liveTime"), local)
		switch {
		case err != nil:
			return time.Time{}, errors.New("Say the date and time entries open.")
		case !at.After(now):
			return time.Time{}, errors.New("The time entries open has already passed: go live now instead.")
		}
		return at.UTC(), nil
	}
	return time.Time{}, nil
}

// live takes a competition live now (opens=now) or at a time (opens=at,
// liveDate, liveTime), or makes it private, pausing entries and keeping
// those made (opens=private).
func (p *competitionPages) live(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	back := func(notice string) {
		http.Redirect(w, r, adminPath(r.PathValue("token"))+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
	}
	at, err := postedLiveAt(r, p.now())
	if err != nil {
		back(err.Error())
		return
	}
	changed := c.Competition
	changed.LiveAt = at
	if !at.IsZero() && changed.Validate() != nil {
		back("Entries must open before they close (" + c.Deadline.In(local).Format("Monday 2 January 2006, 15:04") + "): change the closing time first.")
		return
	}
	if err := p.st.SetLive(r.Context(), c.ID, at); err != nil {
		failed(w, r, err)
		return
	}
	switch {
	case at.IsZero():
		back("The competition is private: nobody can send or change entries until you go live again. Entries already made are kept.")
	case at.After(p.now()):
		back("Entries open " + at.In(local).Format("Monday 2 January 2006, 15:04") + ".")
	default:
		back("The competition is live: clubs and gymnasts can enter.")
	}
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

// laterDeadlines sets when changes to entries (changesDate, changesTime)
// and coaches' sign-offs (signoffsDate, signoffsTime) close, if later than
// the deadline; empty: with it.
func (p *competitionPages) laterDeadlines(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	at := func(date, clock string) (time.Time, bool) {
		if strings.TrimSpace(date) == "" {
			return time.Time{}, true
		}
		if clock == "" {
			clock = "23:59"
		}
		t, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clock, local)
		return t.UTC(), err == nil
	}
	changes, ok1 := at(r.FormValue("changesDate"), r.FormValue("changesTime"))
	signoffs, ok2 := at(r.FormValue("signoffsDate"), r.FormValue("signoffsTime"))
	changed := c.Competition
	changed.ChangesUntil, changed.SignoffsUntil = changes, signoffs
	switch {
	case !ok1 || !ok2:
		backToDashboard(w, r, "Give each date as a date and time; nothing was changed.")
		return
	case !changes.IsZero() && changes.Before(c.Deadline), !signoffs.IsZero() && signoffs.Before(c.Deadline):
		backToDashboard(w, r, "Changes and sign-offs can't close before entries do; nothing was changed.")
		return
	case changed.Validate() != nil:
		backToDashboard(w, r, "Changes and sign-offs must close by the end of the competition date; nothing was changed.")
		return
	}
	if err := p.st.SetChangesAndSignoffs(r.Context(), c.ID, changes, signoffs); err != nil {
		failed(w, r, err)
		return
	}
	backToDashboard(w, r, "Saved.")
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
		Withdrawn: e.Withdrawn, Access: access(r), Notice: r.URL.Query().Get("notice"),
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
	out := views.EntryCard{Gymnast: e.Gymnasts(), Club: club, Level: e.Event(), Problems: checked.Problems(), Unchecked: checked.Unchecked}
	if checked.Unchecked {
		return out, nil
	}
	l, _, _ := c.EntryLevel(e)
	if e.Choice != "" {
		out.Level += " (doing " + e.Choice + ")"
	}
	for i, ex := range []requirements.Checked{checked.First, checked.Second} {
		if !checked.Does(i) {
			out.Exercises[i].Skip = true
			continue
		}
		title := [...]string{"First exercise", "Second exercise"}[i]
		if checked.Only > 0 {
			title = "Routine (the level's voluntary)"
		}
		card := views.ExerciseCard{
			Title: title, Requirements: ex.SetName,
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
		if !checked.Does(i) {
			continue
		}
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
	d := e.Discipline
	f := views.EntryForm{
		Action: action, Submit: submit, Gymnast: e.Gymnast, Level: e.DoesLevel(), Category: e.Category, Video: c.Video.Describe(),
		Discipline: d, Synchro: d == competitions.Synchro, Unchecked: !competitions.Checked(d),
	}
	if e.Partner != nil {
		f.PartnerName, f.PartnerClub = e.Partner.Name, e.Partner.Club
	}
	if f.Unchecked {
		f.Video = "" // no routine to film
		for _, name := range c.LevelNames(d) {
			f.Levels = append(f.Levels, views.EntryLevel{Name: name, Split: c.Split.Splits(competitions.EventName(d, name))})
		}
		return f
	}
	for _, l := range c.EntryLevels(d) {
		level, err := l.Resolve()
		if err != nil {
			continue
		}
		event := level.Name
		el := views.EntryLevel{Name: level.Name}
		if ev, paired, ok := c.SynchroEventOf(level.Name); d == competitions.Synchro && ok && paired {
			event, el.Label = ev, ev+" (doing "+level.Name+")"
		}
		el.Split = c.Split.Splits(competitions.EventName(d, event))
		only := competitions.Entry{Discipline: d}.Only(level)
		for n := range 2 {
			ex := &el.Exercises[n]
			if only > 0 && n+1 != only {
				ex.Skip = true
				continue
			}
			if only > 0 {
				ex.Title = "Routine: the level's voluntary"
			}
			for _, ref := range level.Exercise(n + 1).Options {
				o := views.EntryOption{Ref: ref, Label: ref}
				if set, err := l.Set(ref); err == nil {
					o.Label = set.Name
					_, o.SetRoutine = requirements.SetRoutine(set)
				}
				ex.Options = append(ex.Options, o)
			}
			if level.Name == e.DoesLevel() {
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
	e := competitions.Entry{Gymnast: gymnast, Discipline: r.FormValue("discipline"), Level: r.FormValue("level"), Category: r.FormValue("category")}
	if e.Discipline == competitions.Synchro {
		e.Partner = &competitions.Partner{Name: r.FormValue("partnerName"), Club: r.FormValue("partnerClub")}
		// A level paired into an event: the pair enters the event, doing it.
		if event, paired, ok := c.SynchroEventOf(e.Level); ok && paired {
			e.Level, e.Choice = event, e.Level
		}
	}
	var problems []string
	l, level, ok := c.EntryLevel(e)
	only := e.Only(level)
	for i := range e.Exercises {
		if !competitions.Checked(e.Discipline) {
			break
		}
		if ok && only > 0 && i+1 != only {
			continue // synchro does one routine
		}
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
	render(w, r, views.CompetitionEnter(enterPage(c, competitions.Entry{Discipline: chosenDiscipline(c.Competition, r.URL.Query().Get("discipline"))}, r.URL.Path, p.now())))
}

// chosenDiscipline is the discipline asked for, if the competition offers
// it, else its first.
func chosenDiscipline(c competitions.Competition, asked string) string {
	offered := c.Disciplines()
	if slices.Contains(offered, asked) || len(offered) == 0 {
		return asked
	}
	return offered[0]
}

// enterPage is the individual entry page, for one discipline, with a tab for
// each the competition offers.
func enterPage(c store.Competition, e competitions.Entry, path string, now time.Time) views.EnterPage {
	page := views.EnterPage{Competition: summary(c.Competition, now), Form: entryForm(c.Competition, e, path, "Enter")}
	for _, d := range c.Disciplines() {
		page.Disciplines = append(page.Disciplines, views.DisciplineTab{
			Name: competitions.DisciplineName(d), URL: path + "?discipline=" + d, Current: d == e.Discipline,
		})
	}
	return page
}

func (p *competitionPages) enter(w http.ResponseWriter, r *http.Request) {
	c, ok := p.individual(w, r)
	if !ok {
		return
	}
	e, problems := postedEntry(r, c.Competition, "")
	if len(problems) > 0 {
		page := enterPage(c, e, r.URL.Path, p.now())
		page.Form.Problems = problems
		w.WriteHeader(http.StatusUnprocessableEntity)
		render(w, r, views.CompetitionEnter(page))
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
		Placement:   placement(c, e.ID),
		Waiting:     waitingText(p.waitingOf(r.Context(), c.ID, e.ID)),
		Late:        p.lateStatusOf(r.Context(), c, e.ID),
		Duties:      dutiesOf(c, individualKey(e.Entry.Gymnast)),
		Day:         path + "/day",
		VideoReview: e.VideoReview, VideoNote: e.VideoNote,
		Form: entryForm(c.Competition, form, path, "Save changes"), Withdraw: path + "/withdraw",
	}
	if c.Signoff {
		page.SignoffLink = origin(r) + signoffPath(e.SignoffLink)
	}
	page.Notice = r.URL.Query().Get("notice")
	if published(c) {
		page.Timeline, page.Calendar = path+"/timeline", path+"/calendar.ics"
	}
	page.Notify = p.notifyLink(path + "/notify")
	page.Fees = p.feesSummary(r.Context(), c, entryPayer(e.ID), path+"/invoice")
	if lateOpen(c, p.now()) {
		page.LateAsk = path + "/late"
	}
	if len(page.Duties) > 0 {
		page.ScoreSheets = path + "/score-sheets"
	}
	switch {
	case e.Removal == store.Removed:
		page.Removal = strings.TrimSpace("The organiser removed this entry, so it can't be changed. " + e.RemovalNote)
		page.Competition.Open, page.Competition.ChangesOpen = false, false
	case e.Resent:
		page.Removal = "Changed and sent again: waiting for the organiser to accept it back in."
	case e.Removal == store.Held:
		page.Removal = strings.TrimSpace("The organiser asks for changes before taking this entry back in: change it below. " + e.RemovalNote)
	}
	if page.Coach, err = p.entryCoachView(r, c, e, path); err != nil {
		failed(w, r, err)
		return
	}
	seen := p.seenFor(r)
	if warn, err := seen.pairWarning(c.ID, e.ID); err != nil {
		failed(w, r, err)
		return
	} else if warn != "" {
		page.Card.Problems = append([]string{warn}, page.Card.Problems...)
	}
	pairs, err := p.st.IndividualPairEntries(r.Context(), e.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	if page.Pairs, err = seen.pairViews(pairs, nil); err != nil {
		failed(w, r, err)
		return
	}
	if e.Entry.Partner != nil {
		page.Partner = &views.PartnerView{Name: e.Entry.Partner.Name, Link: origin(r) + partnerPath(e.PartnerLink), Confirmed: e.PartnerConfirmed}
	}
	if offer, err := p.st.IndividualOffer(r.Context(), e.ID); err == nil {
		form := offerForm(c.Competition, offer, path+"/offer")
		page.Offer = &form
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

// --- Other disciplines (ADR 0005 Decision 1) ---

// postedEvents reads the synchro levels ticked (synchroLevel), which to pair
// into one event (synchroPairs: one event a line, levels joined by "+"), the
// tumbling and DMT levels, one a line (tumbling, dmt), and the form as posted.
func postedEvents(r *http.Request) ([]competitions.Level, []string, []string, [][]string, views.EventsForm) {
	form := views.EventsForm{Groups: requirements.BuiltinGroups(), Synchro: map[string]bool{}, Pairs: r.FormValue("synchroPairs"),
		Tumbling: r.FormValue("tumbling"), DMT: r.FormValue("dmt")}
	var synchro []competitions.Level
	for _, ref := range r.Form["synchroLevel"] {
		form.Synchro[ref] = true
		synchro = append(synchro, competitions.Level{Ref: ref})
	}
	var pairs [][]string
	for _, line := range lines(form.Pairs) {
		var names []string
		for _, n := range strings.Split(line, "+") {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}
		pairs = append(pairs, names)
	}
	return synchro, lines(form.Tumbling), lines(form.DMT), pairs, form
}

// lines are a text's non-blank lines, trimmed.
func lines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// eventsForm is a competition's other events as the form shows them.
func eventsForm(c competitions.Competition) views.EventsForm {
	f := views.EventsForm{Groups: requirements.BuiltinGroups(), Synchro: map[string]bool{},
		Tumbling: strings.Join(c.Tumbling, "\n"), DMT: strings.Join(c.DMT, "\n")}
	for _, l := range c.Synchro {
		for _, m := range l.Members() {
			f.Synchro[m.Ref] = true
		}
	}
	var pairs []string
	for _, names := range c.Pairings() {
		pairs = append(pairs, strings.Join(names, " + "))
	}
	f.Pairs = strings.Join(pairs, "\n")
	return f
}

// setEvents changes the synchro, tumbling and DMT levels. Entries already made
// for a level taken away stay, flagged as a level the competition doesn't offer.
func (p *competitionPages) setEvents(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	changed := c.Competition
	var pairs [][]string
	changed.Synchro, changed.Tumbling, changed.DMT, pairs, _ = postedEvents(r)
	var was []competitions.Level
	for _, l := range c.Synchro {
		was = append(was, l.Members()...)
	}
	changed.Synchro = competitions.OrderLevels(was, changed.Synchro)
	notice := "Events changed."
	var err error
	if changed.Synchro, err = competitions.PairLevels(changed.Synchro, pairs); err != nil {
		notice = "Events weren't changed: " + strings.Join(sentences(err), " ")
	} else if err := changed.Validate(); err != nil {
		notice = "Events weren't changed: " + strings.Join(sentences(err), " ")
	} else if err := p.st.SetEvents(r.Context(), c.ID, changed.Synchro, changed.Tumbling, changed.DMT); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

// levelOrderForm is each offered discipline's levels in order, to move.
func levelOrderForm(c competitions.Competition) []views.LevelOrder {
	var out []views.LevelOrder
	for _, d := range c.Disciplines() {
		out = append(out, views.LevelOrder{Discipline: d, Name: competitions.DisciplineName(d), Levels: c.LevelOrder(d)})
	}
	return out
}

// moveLevel moves a level one place easier or harder in its discipline's
// order: move is "<discipline>:<place>:<-1 or 1>".
func (p *competitionPages) moveLevel(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	var d string
	var at, by int
	if parts := strings.Split(r.FormValue("move"), ":"); len(parts) == 3 {
		d = parts[0]
		at, _ = strconv.Atoi(parts[1])
		by, _ = strconv.Atoi(parts[2])
	}
	changed := c.Competition
	changed.Levels, changed.Synchro = slices.Clone(c.Levels), slices.Clone(c.Synchro)
	changed.Tumbling, changed.DMT = slices.Clone(c.Tumbling), slices.Clone(c.DMT)
	if err := changed.MoveLevel(d, at, by); err != nil {
		badRequest(w, err)
		return
	}
	if err := p.st.SetLevelOrder(r.Context(), c.ID, changed); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"?notice="+url.QueryEscape("Order of levels changed.")+"#level-order", http.StatusSeeOther)
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
	return views.SignoffView{Required: c.Signoff, Signed: e.SignedOff(), By: e.SignedBy, Note: e.SignNote, Unapproved: e.Unapproved}
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
	if named, why, err := p.entryCoachFor(r, c, e); err != nil {
		failed(w, r, err)
		return
	} else if why != "" {
		page.Cannot = why
	} else if named != "" {
		page.AskName, page.Name = false, named
	}
	if req, open, err := p.openLateFor(r.Context(), c.ID, e.ID); err == nil && open && !req.SignedOff() && c.Signoff {
		page.Late = []views.CoachLate{{Member: req.Entry.Gymnasts(), What: competitions.LateKindName(req.Kind) + " to " + req.Entry.Event(), Link: r.URL.Path + "/late"}}
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
	if !c.SignoffsOpen(p.now()) {
		failed(w, r, notOpen(c.Competition, p.now()))
		return
	}
	name := strings.TrimSpace(r.FormValue("coach"))
	if named, why, err := p.entryCoachFor(r, c, e); err != nil {
		failed(w, r, err)
		return
	} else if why != "" {
		p.renderIndividualSignoff(w, r, e, c, name, []string{why})
		return
	} else if named != "" {
		name = named // the coach the organiser approved
	}
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

// reasonTo says who sees why an entry was removed or held.
func reasonTo(e store.Entry) string {
	switch {
	case e.Individual:
		return "the gymnast sees it"
	case e.RemovalTo == store.ToClub:
		return "the club sees it"
	case e.RemovalTo == store.ToMember:
		return "the gymnast sees it"
	}
	return "the club and gymnast see it"
}

// removeEntries removes (action=remove) or holds (action=hold) the entries
// ticked (entry: their ids), with a reason (note) for the club, the gymnast
// or both (to).
func (p *competitionPages) removeEntries(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	removal, done := store.Removed, "Removed"
	if r.FormValue("action") == "hold" {
		removal, done = store.Held, "Put on hold"
	}
	to := r.FormValue("to")
	if to != store.ToClub && to != store.ToMember {
		to = store.ToBoth
	}
	p.notify.changing(r.Context(), c)
	n, err := p.st.RemoveEntries(r.Context(), c.ID, r.Form["entry"], removal, limitNote(r.FormValue("note")), to)
	if err != nil {
		failed(w, r, err)
		return
	}
	notice := "Tick the entries to remove or hold first; nothing was changed."
	if n > 0 {
		notice = fmt.Sprintf("%s %s. They're listed at the end, to restore.", done, entriesWord(n))
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

// restoreEntry puts a removed or held entry back in the competition.
func (p *competitionPages) restoreEntry(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	p.notify.changing(r.Context(), c)
	if err := p.st.RestoreEntry(r.Context(), c.ID, r.PathValue("id")); err != nil {
		failed(w, r, err)
		return
	}
	http.Redirect(w, r, adminPath(r.PathValue("token"))+"?notice="+url.QueryEscape("Back in the competition."), http.StatusSeeOther)
}
