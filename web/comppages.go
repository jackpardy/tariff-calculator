package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
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
//
// This file has what they share: the routes, the organiser's link, an
// entry's card and the entry form. The rest is in compsettings.go,
// dashboardpages.go, individualpages.go, helpers.go and a file a feature.

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
	facts                             factCache // what the pages work out from each competition's entries
	fixes                             fixCache  // changes that might make each timetable fit
	feedbackTo                        string    // where feedback is emailed (FEEDBACK_EMAIL); "" for nowhere
}

func newCompetitionPages(st *store.Store) *competitionPages {
	p := &competitionPages{
		st:           st,
		limiter:      newLimiter(maxCompetitionsPerHour, time.Hour),
		clubLimiter:  newLimiter(maxClubsPerHour, time.Hour),
		joinLimiter:  newLimiter(maxJoinsPerHour, time.Hour),
		emailLimiter: newLimiter(maxEmailsPerHour, time.Hour),
		now:          time.Now,
		feedbackTo:   os.Getenv("FEEDBACK_EMAIL"),
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
	p.registerFeedback(handle)
	p.registerFees(handle)
	p.registerLimits(handle)
	p.registerLate(handle)
	p.registerDesk(handle)
	p.registerDay(handle)
	p.registerScreen(handle)
	p.registerScreenNotices(handle)
	handle("GET /competitions/new", p.newForm)
	handle("POST /competitions", p.create)
	handle("GET /competitions/admin/{token}", p.dashboard)
	handle("GET /competitions/admin/{token}/settings", p.settings)
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
	handle("POST /competitions/entry/{token}/restore", p.restore)
	p.registerClubs(mux)
	p.registerTimetable(handle)
	p.registerPartners(handle)
	p.registerOfficials(handle)
	p.registerApprovals(handle)
	p.registerPeople(handle)
	p.registerRoles(handle)
	p.registerGymnasts(handle)
	p.registerRoutines(handle)
	p.registerSkills(handle)
	p.registerLevels(handle)
	p.registerKept(handle)
	p.registerJudging(handle)
	p.registerLevelSharing(handle)
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
		h(w, withFeedback(p.withRoles(r)))
	})
}

// notOpen is why a competition's entries aren't open: not live yet (or
// paused), or closed.
func notOpen(c competitions.Competition, now time.Time) error {
	if !c.Live(now) {
		return store.ErrNotOpen
	}
	return store.ErrClosed
}

func adminPath(token string) string { return "/competitions/admin/" + token }

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
func summary(sc store.Competition, now time.Time) views.CompetitionSummary {
	c := sc.Competition
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
		out.Date = dateRange(day, len(schedule(sc).Setup.Days))
	}
	return out
}

// dateRange is a competition's dates: its first day, or over several days
// "Friday 26 – Sunday 28 February 2027".
func dateRange(first time.Time, days int) string {
	if days <= 1 {
		return first.Format("Monday 2 January 2006")
	}
	last := first.AddDate(0, 0, days-1)
	switch {
	case first.Year() != last.Year():
		return first.Format("Monday 2 January 2006") + " – " + last.Format("Monday 2 January 2006")
	case first.Month() != last.Month():
		return first.Format("Monday 2 January") + " – " + last.Format("Monday 2 January 2006")
	}
	return first.Format("Monday 2") + " – " + last.Format("Monday 2 January 2006")
}

// admin is the competition an admin link opens, or a page saying it doesn't.
func (p *competitionPages) admin(w http.ResponseWriter, r *http.Request) (store.Competition, bool) {
	c, _, err := p.st.AdminLink(r.Context(), r.PathValue("token"))
	if err != nil {
		failed(w, r, err)
		return c, false
	}
	return c, true
}

// onTheirOwn is what pages call a gymnast's club when they entered without
// one: the same words everywhere, never "individual".
const onTheirOwn = "On their own"

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

// entryForm is the form for entering a competition, starting from e.
func entryForm(c competitions.Competition, e competitions.Entry, action, submit string) views.EntryForm {
	d := e.Discipline
	f := views.EntryForm{
		Action: action, Submit: submit, Gymnast: e.Gymnast, Level: e.DoesLevel(), Category: e.Category, Video: c.Video.Describe(),
		Discipline: d, Synchro: d == competitions.Synchro, Unchecked: !competitions.Checked(d), Builder: routineBuilder(action, d),
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

// routineBuilder opens the Routine Builder from an entry form posting to
// action, with a way back to the form, at its discipline: a GET of the
// form's address shows it.
func routineBuilder(action, discipline string) string {
	sep := "?"
	if strings.Contains(action, "?") {
		sep = "&"
	}
	return "/?back=" + url.QueryEscape(action+sep+"discipline="+url.QueryEscape(discipline))
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
			problems = append(problems, fmt.Sprintf("Choose a routine for the %s exercise. None to choose? Build one with the Routine Builder link under it.", [...]string{"first", "second"}[i]))
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
