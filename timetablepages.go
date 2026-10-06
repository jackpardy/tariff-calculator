package main

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/requirements"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The timetable (roadmap: competitions 3): the organiser plans flights on
// panels from the entries, adjusts them, prints marshal and chair of judges
// sheets, and publishes it so clubs and gymnasts see their flight and time.

func (p *competitionPages) registerTimetable(handle func(string, http.HandlerFunc)) {
	handle("GET /competitions/admin/{token}/timetable", p.timetable)
	handle("POST /competitions/admin/{token}/timetable/plan", p.planTimetable)
	handle("POST /competitions/admin/{token}/timetable/flight", p.editFlight)
	handle("POST /competitions/admin/{token}/timetable/entry", p.moveEntry)
	handle("POST /competitions/admin/{token}/timetable/publish", p.publishTimetable)
	handle("GET /competitions/admin/{token}/timetable/print", p.printTimetable)
}

// live are a competition's entries that count: not withdrawn.
func live(entries []store.Entry) []store.Entry {
	var out []store.Entry
	for _, e := range entries {
		if !e.Withdrawn {
			out = append(out, e)
		}
	}
	return out
}

// planEntries are entries as the planner needs them.
func planEntries(entries []store.Entry) []competitions.PlanEntry {
	out := make([]competitions.PlanEntry, len(entries))
	for i, e := range entries {
		club := e.ClubName
		if e.Individual {
			club = ""
		}
		out[i] = competitions.PlanEntry{ID: e.ID, Level: e.Entry.Event(), Category: e.Entry.Category, Club: club}
	}
	return out
}

// day is the competition's date, in Irish and UK time.
func day(c competitions.Competition) time.Time {
	d, err := c.Day()
	if err != nil {
		return time.Now().In(local)
	}
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, local)
}

// timetablePath is the timetable page's path for an admin link.
func timetablePath(r *http.Request) string {
	return adminPath(r.PathValue("token")) + "/timetable"
}

// back redirects to the timetable page with a notice.
func back(w http.ResponseWriter, r *http.Request, notice string) {
	to := timetablePath(r)
	if notice != "" {
		to += "?notice=" + url.QueryEscape(notice)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// timetableOf is the competition's timetable and its live entries.
func (p *competitionPages) timetableOf(w http.ResponseWriter, r *http.Request) (store.Competition, []store.Entry, bool) {
	c, ok := p.admin(w, r)
	if !ok {
		return c, nil, false
	}
	entries, err := p.st.Entries(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return c, nil, false
	}
	return c, live(entries), true
}

// panelsView are the timetable's panels as the pages show them: each
// flight's times and the gymnasts still entered, flagged if they've changed
// level since.
func panelsView(c competitions.Competition, t competitions.Timetable, entries []store.Entry) []views.PanelView {
	byID := map[string]store.Entry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	slots := t.Slots(day(c))
	var out []views.PanelView
	for pi, flights := range t.Panels {
		pv := views.PanelView{Number: pi + 1, Finish: start(c, t).Format("15:04")}
		for fi, f := range flights {
			fv := views.FlightView{
				Panel: pi, Index: fi, Name: f.Name(), First: fi == 0, Last: fi == len(flights)-1,
				Start: slots[pi][fi].Start.Format("15:04"), End: slots[pi][fi].End.Format("15:04"),
			}
			for _, id := range f.Entries {
				e, ok := byID[id]
				if !ok {
					continue // withdrawn since
				}
				g := views.TimetableGymnast{ID: id, Name: e.Entry.Gymnasts(), Club: clubOf(e), Category: e.Entry.Category}
				if e.Entry.Event() != f.Level || (f.Category != "" && e.Entry.Category != f.Category) {
					g.Moved = strings.TrimSpace(e.Entry.Event() + " " + e.Entry.Category)
				}
				fv.Gymnasts = append(fv.Gymnasts, g)
			}
			pv.Flights = append(pv.Flights, fv)
			pv.Finish = fv.End
		}
		out = append(out, pv)
	}
	return out
}

// start is when the timetable starts on the competition's day.
func start(c competitions.Competition, t competitions.Timetable) time.Time {
	at, _ := time.Parse("15:04", t.Settings.Start)
	d := day(c)
	return time.Date(d.Year(), d.Month(), d.Day(), at.Hour(), at.Minute(), 0, 0, local)
}

// planForm is the settings as the form shows them.
func planForm(c competitions.Competition, s competitions.PlanSettings) views.PlanForm {
	var split []string
	for _, l := range c.EventNames() {
		if c.Split.Splits(l) {
			split = append(split, l)
		}
	}
	return views.PlanForm{
		Panels: strconv.Itoa(s.Panels), Start: s.Start, Between: strconv.Itoa(s.MinutesBetween),
		PerGymnast: strconv.FormatFloat(s.MinutesPerGymnast, 'f', -1, 64), MaxFlight: strconv.Itoa(s.MaxFlight),
		FinishBy: s.FinishBy, Separate: splitForm(s.Separate, split), CanSeparate: len(split) > 0,
	}
}

func (p *competitionPages) timetable(w http.ResponseWriter, r *http.Request) {
	c, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	settings := competitions.DefaultSettings
	if c.Timetable != nil {
		settings = c.Timetable.Settings
	}
	page := views.TimetablePage{
		Base: adminPath(r.PathValue("token")), Competition: summary(c.Competition, p.now()),
		Notice: r.URL.Query().Get("notice"), Settings: planForm(c.Competition, settings), Entries: len(entries),
	}
	if settings.FinishBy != "" {
		page.Needed = competitions.PanelsNeeded(planEntries(entries), c.EventNames(), settings)
		switch {
		case page.Needed > 0:
			page.NeededText = fmt.Sprintf("To finish by %s you need %s.", settings.FinishBy, panelsWord(page.Needed))
		default:
			page.NeededText = fmt.Sprintf("Even 20 panels can't finish by %s: one level takes longer than that. Try fewer minutes per gymnast, or an earlier start.", settings.FinishBy)
		}
	}
	if t := c.Timetable; t != nil {
		page.Planned, page.Published = true, t.Published
		page.Finish = t.Finish(day(c.Competition)).Format("15:04")
		page.Panels = panelsView(c.Competition, *t, entries)
		for _, panel := range page.Panels {
			for _, f := range panel.Flights {
				page.Moves = append(page.Moves, views.FlightOption{
					Value: fmt.Sprintf("%d:%d", f.Panel, f.Index), Label: fmt.Sprintf("Panel %d · %s", panel.Number, f.Name),
				})
			}
		}
		for _, e := range entries {
			if _, placed := t.Find(e.ID); !placed {
				page.Unplaced = append(page.Unplaced, views.TimetableGymnast{
					ID: e.ID, Name: e.Entry.Gymnasts(), Club: clubOf(e), Category: strings.TrimSpace(e.Entry.Event() + " " + e.Entry.Category),
				})
			}
		}
	}
	render(w, r, views.CompetitionTimetable(page))
}

func panelsWord(n int) string {
	if n == 1 {
		return "1 panel"
	}
	return fmt.Sprintf("%d panels", n)
}

// postedSettings reads the planning form.
func postedSettings(r *http.Request) (competitions.PlanSettings, error) {
	var s competitions.PlanSettings
	var err error
	number := func(name string) float64 {
		v, e := strconv.ParseFloat(strings.TrimSpace(r.FormValue(name)), 64)
		if e != nil && err == nil {
			err = fmt.Errorf("%s should be a number", name)
		}
		return v
	}
	s.Panels = int(number("panels"))
	s.MinutesPerGymnast = number("perGymnast")
	s.MinutesBetween = int(number("between"))
	s.MaxFlight = int(number("maxFlight"))
	s.Start, s.FinishBy = strings.TrimSpace(r.FormValue("start")), strings.TrimSpace(r.FormValue("finishBy"))
	s.Separate = postedSplit(r, "separate", "separateLevel")
	if r.FormValue("separate") == "" && !r.Form.Has("separate") {
		s.Separate = competitions.Split{Mode: competitions.SplitAll} // no split levels: nothing to choose
	}
	if err != nil {
		return s, err
	}
	return s, s.Check()
}

// planTimetable plans the timetable afresh (action=plan), or keeps its
// flights and updates only the settings that set the times (action=times).
func (p *competitionPages) planTimetable(w http.ResponseWriter, r *http.Request) {
	c, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	s, err := postedSettings(r)
	if err != nil {
		back(w, r, "Nothing was changed: "+sentences(err)[0])
		return
	}
	var t competitions.Timetable
	notice := ""
	if r.FormValue("action") == "times" && c.Timetable != nil {
		t = *c.Timetable
		s.Panels, s.MaxFlight, s.Separate = t.Settings.Panels, t.Settings.MaxFlight, t.Settings.Separate
		t.Settings = s
		notice = "Times updated; the flights are as they were. Plan again to change the panels, flight size or men's and women's flights."
	} else {
		t = competitions.Plan(planEntries(entries), c.EventNames(), s, rand.New(rand.NewPCG(uint64(p.now().UnixNano()), 0)))
		if c.Timetable != nil {
			t.Published = c.Timetable.Published
		}
		notice = fmt.Sprintf("Planned %s on %s.", gymnastsWord(len(entries)), panelsWord(s.Panels))
	}
	if err := p.st.SetTimetable(r.Context(), c.ID, &t); err != nil {
		failed(w, r, err)
		return
	}
	back(w, r, notice)
}

func gymnastsWord(n int) string {
	if n == 1 {
		return "1 gymnast"
	}
	return fmt.Sprintf("%d gymnasts", n)
}

// editFlight moves a flight up or down its panel, to another panel (to), or
// redraws its running order.
func (p *competitionPages) editFlight(w http.ResponseWriter, r *http.Request) {
	c, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if c.Timetable == nil {
		back(w, r, "Plan the timetable first.")
		return
	}
	t := *c.Timetable
	panel, _ := strconv.Atoi(r.FormValue("panel"))
	flight, _ := strconv.Atoi(r.FormValue("flight"))
	done := false
	switch r.FormValue("action") {
	case "up":
		done = t.ShiftFlight(panel, flight, -1)
	case "down":
		done = t.ShiftFlight(panel, flight, 1)
	case "move":
		to, _ := strconv.Atoi(r.FormValue("to"))
		done = t.MoveFlight(panel, flight, to)
	case "redraw":
		clubs := map[string]string{}
		for _, pe := range planEntries(entries) {
			clubs[pe.ID] = pe.Club
		}
		done = t.Redraw(panel, flight, clubs, rand.New(rand.NewPCG(uint64(p.now().UnixNano()), 1)))
	}
	if !done {
		back(w, r, "That flight has moved since; nothing was changed.")
		return
	}
	if err := p.st.SetTimetable(r.Context(), c.ID, &t); err != nil {
		failed(w, r, err)
		return
	}
	back(w, r, "")
}

// moveEntry moves a gymnast to a flight (to: "panel:flight"), or out of the
// timetable (to: "").
func (p *competitionPages) moveEntry(w http.ResponseWriter, r *http.Request) {
	c, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if c.Timetable == nil {
		back(w, r, "Plan the timetable first.")
		return
	}
	id := r.FormValue("entry")
	known := false
	for _, e := range entries {
		known = known || e.ID == id
	}
	t := *c.Timetable
	to := r.FormValue("to")
	switch {
	case to == "":
		t.RemoveEntry(id)
	case !known:
		back(w, r, "That gymnast isn't entered any more.")
		return
	default:
		var panel, flight int
		if _, err := fmt.Sscanf(to, "%d:%d", &panel, &flight); err != nil || !t.MoveEntry(id, panel, flight) {
			back(w, r, "That flight has moved since; nothing was changed.")
			return
		}
	}
	if err := p.st.SetTimetable(r.Context(), c.ID, &t); err != nil {
		failed(w, r, err)
		return
	}
	back(w, r, "")
}

func (p *competitionPages) publishTimetable(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if c.Timetable == nil {
		back(w, r, "Plan the timetable first.")
		return
	}
	t := *c.Timetable
	t.Published = r.FormValue("on") == "1"
	if err := p.st.SetTimetable(r.Context(), c.ID, &t); err != nil {
		failed(w, r, err)
		return
	}
	notice := "Unpublished: only you see the timetable."
	if t.Published {
		notice = "Published: clubs, members and gymnasts entering on their own see their flight, panel and time."
	}
	back(w, r, notice)
}

// printTimetable prints the marshal sheets (sheet=marshal) or the chair of
// judges sheets (sheet=judges), one panel to a page.
func (p *competitionPages) printTimetable(w http.ResponseWriter, r *http.Request) {
	c, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if c.Timetable == nil {
		back(w, r, "Plan the timetable first.")
		return
	}
	judges := r.URL.Query().Get("sheet") == "judges"
	title := "Marshal sheets · " + c.Name
	if judges {
		title = "Chair of judges sheets · " + c.Name
	}
	sheets := views.TimetableSheets{
		Title: title, Back: timetablePath(r), Competition: summary(c.Competition, p.now()), Judges: judges,
		Panels: panelsView(c.Competition, *c.Timetable, entries), Exercises: map[string][2]string{}, Notes: map[string]string{},
	}
	if judges {
		for _, j := range judge(c.Competition, entries) {
			if j.err != nil {
				sheets.Notes[j.ID] = j.err.Error()
				continue
			}
			var ex [2]string
			for i, checked := range []requirements.Checked{j.card.First, j.card.Second} {
				ex[i] = exerciseSummary(j.Entry.Entry.Exercises[i], checked)
			}
			sheets.Exercises[j.ID] = ex
			var notes []string
			if n := len(j.problems); n > 0 {
				notes = append(notes, problemsWord(n))
			}
			if j.Checked() {
				notes = append(notes, "card checked")
			}
			sheets.Notes[j.ID] = strings.Join(notes, " · ")
		}
	}
	render(w, r, views.TimetablePrint(sheets))
}

func problemsWord(n int) string {
	if n == 1 {
		return "1 problem"
	}
	return fmt.Sprintf("%d problems", n)
}

// placement is where an entry competes, once the timetable is published.
func placement(c store.Competition, entryID string) *views.Placement {
	t := c.Timetable
	if t == nil || !t.Published || entryID == "" {
		return nil
	}
	at, ok := t.Find(entryID)
	if !ok {
		return nil
	}
	slot := t.Slots(day(c.Competition))[at.Panel][at.Flight]
	return &views.Placement{
		Flight: t.Panels[at.Panel][at.Flight].Name(), Panel: strconv.Itoa(at.Panel + 1), Time: slot.Start.Format("15:04"),
	}
}
