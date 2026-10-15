package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/requirements"
	"tariffCalculator/views"
)

// Creating a competition and its settings: going live, deadlines,
// individuals, events and the order of levels, splitting men and women,
// video proof, sign-off, replacing the admin link and deleting it.

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
	if at, problem := postedLiveAt(r, p.now()); problem != "" {
		problems = append(problems, problem)
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
	backToSettings(w, r, notice)
}

// postedLiveAt is when a form says entries open: opens=now, opens=at with
// liveDate and liveTime still to come, or anything else for private (zero).
// problem says what's wrong with it, as a sentence to show, if anything.
func postedLiveAt(r *http.Request, now time.Time) (at time.Time, problem string) {
	switch r.FormValue("opens") {
	case "now":
		return now, ""
	case "at":
		at, err := time.ParseInLocation("2006-01-02 15:04", r.FormValue("liveDate")+" "+r.FormValue("liveTime"), local)
		switch {
		case err != nil:
			return time.Time{}, "Say the date and time entries open."
		case !at.After(now):
			return time.Time{}, "The time entries open has already passed: go live now instead."
		}
		return at.UTC(), ""
	}
	return time.Time{}, ""
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
		to := adminPath(r.PathValue("token"))
		if r.FormValue("back") == "settings" {
			to = settingsPath(r.PathValue("token"))
		}
		seeOther(w, r, to, notice)
	}
	at, problem := postedLiveAt(r, p.now())
	if problem != "" {
		back(problem)
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
			backToSettings(w, r, "Entries must close by the end of the competition date; the closing time wasn't changed.")
			return
		}
		notice = "Entries now close " + deadline.Format("Monday 2 January 2006, 15:04") + "."
	}
	if err := p.st.SetDeadline(r.Context(), c.ID, deadline); err != nil {
		failed(w, r, err)
		return
	}
	backToSettings(w, r, notice)
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
		backToSettings(w, r, "Give each date as a date and time; nothing was changed.")
		return
	case !changes.IsZero() && changes.Before(c.Deadline), !signoffs.IsZero() && signoffs.Before(c.Deadline):
		backToSettings(w, r, "Changes and sign-offs can't close before entries do; nothing was changed.")
		return
	case changed.Validate() != nil:
		backToSettings(w, r, "Changes and sign-offs must close by the end of the competition date; nothing was changed.")
		return
	}
	if err := p.st.SetChangesAndSignoffs(r.Context(), c.ID, changes, signoffs); err != nil {
		failed(w, r, err)
		return
	}
	backToSettings(w, r, "Saved.")
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
	notice := "Gymnasts can no longer enter on their own."
	if r.FormValue("on") == "1" {
		notice = "Gymnasts can now enter on their own, with the link for entering on their own."
	}
	backToSettings(w, r, notice)
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
	http.Redirect(w, r, settingsPath(admin)+"?new=replaced", http.StatusSeeOther)
}

func (p *competitionPages) delete(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if r.FormValue("confirm") != "1" {
		backToSettings(w, r, "Tick the box to delete the competition; nothing was deleted.")
		return
	}
	if err := p.st.DeleteCompetition(r.Context(), c.ID); err != nil {
		failed(w, r, err)
		return
	}
	message(w, r, http.StatusOK, "Competition deleted", c.Name+" and all its entries have been deleted.")
}

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
	backToSettings(w, r, notice)
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
	seeOther(w, r, settingsPath(r.PathValue("token"))+"#level-order", "Order of levels changed.")
}

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
	backToSettings(w, r, notice)
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
	backToSettings(w, r, notice)
}
