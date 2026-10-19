package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The timetable (ADR 0005 step 4): the organiser sets up the days, areas,
// timings, blocked time and rules; plans; sees what doesn't fit and what would
// fix it; adjusts manually; prints the sheets; and publishes, so clubs and
// gymnasts see their flight, area and time.

func (p *competitionPages) registerTimetable(handle func(string, http.HandlerFunc)) {
	handle("GET /competitions/admin/{token}/timetable", p.timetable)
	handle("GET /competitions/admin/{token}/timetable/setup", p.timetableSetup)
	handle("POST /competitions/admin/{token}/timetable/setup/days", p.setupDays)
	handle("POST /competitions/admin/{token}/timetable/setup/areas", p.setupAreas)
	handle("POST /competitions/admin/{token}/timetable/setup/timings", p.setupTimings)
	handle("POST /competitions/admin/{token}/timetable/setup/blocks", p.setupBlocks)
	handle("POST /competitions/admin/{token}/timetable/setup/rules", p.setupRules)
	handle("POST /competitions/admin/{token}/timetable/plan", p.planTimetable)
	handle("POST /competitions/admin/{token}/timetable/flight", p.editFlight)
	handle("POST /competitions/admin/{token}/timetable/entry", p.moveEntry)
	handle("POST /competitions/admin/{token}/timetable/officials", p.editRota)
	handle("POST /competitions/admin/{token}/timetable/publish", p.publishTimetable)
	handle("GET /competitions/admin/{token}/timetable/print", p.printTimetable)
	handle("GET /competitions/admin/{token}/timetable/timeline.csv", p.timelineCSV)
	handle("GET /competitions/admin/{token}/timetable/timeline", p.panelTimelinePage)
	handle("GET /competitions/admin/{token}/timetable/simulate", p.simulation)
	handle("GET /competitions/admin/{token}/timetable/delay", p.delay)
	handle("POST /competitions/admin/{token}/timetable/delay", p.delay)
	handle("GET /competitions/admin/{token}/timetable/leave", p.leave)
	handle("POST /competitions/admin/{token}/timetable/leave", p.leave)
	handle("POST /competitions/admin/{token}/timetable/simulate", p.simulate)
	handle("POST /competitions/admin/{token}/timetable/simulate/delete", p.deleteScenario)
}

// timetablePath is the timetable page's path for an admin link.
func timetablePath(r *http.Request) string {
	return adminPath(r.PathValue("token")) + "/timetable"
}

// back redirects to the timetable page with a notice: its Setup page after a
// change to the setup.
func back(w http.ResponseWriter, r *http.Request, notice string) { backTo(w, r, notice, "") }

// backTo is back to a place on the page (anchor: an area's id, as
// flightAnchor gives it), where the change shows, with the notice there.
func backTo(w http.ResponseWriter, r *http.Request, notice, anchor string) {
	to := timetablePath(r)
	if strings.Contains(r.Pattern, "/timetable/setup/") {
		to += "/setup"
	}
	if notice != "" {
		to += "?notice=" + url.QueryEscape(notice)
	}
	if anchor != "" {
		to += "#" + anchor
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// flightAnchor is the timetable page's place for a flight: its day's area.
func flightAnchor(f competitions.ScheduledFlight) string {
	return fmt.Sprintf("day-%d-", f.Day) + areaID(f.Area)
}

// refused is a change to the draft timetable turned down, with why: nothing
// is changed, and the organiser is told.
type refused string

func (e refused) Error() string { return string(e) }

// staleTimetable refuses a form made from the draft timetable as it was
// before someone else changed it: its flights and seats, by position, may now
// be others.
const staleTimetable = refused("The timetable changed since you opened this page; nothing was changed.")

// changeDraft changes the draft timetable with change, in one transaction
// (store.UpdateTimetable). A form carrying the version of the draft it was
// made from (version) is refused if the draft has changed since.
func (p *competitionPages) changeDraft(r *http.Request, c store.Competition, change func(*competitions.Schedule) error) error {
	r.ParseForm()
	version, checked := r.PostForm["version"]
	return p.st.UpdateTimetable(r.Context(), c.ID, func(s *competitions.Schedule) error {
		if checked && (len(version) != 1 || version[0] != s.Version()) {
			return staleTimetable
		}
		return change(s)
	})
}

// changed goes back to the timetable page after changeDraft: with notice if
// the change was made, or why it wasn't.
func changed(w http.ResponseWriter, r *http.Request, err error, notice string) {
	changedAt(w, r, err, notice, "")
}

// changedAt is changed, back at the place the change shows (anchor).
func changedAt(w http.ResponseWriter, r *http.Request, err error, notice, anchor string) {
	var no refused
	switch {
	case errors.As(err, &no):
		backTo(w, r, string(no), anchor)
	case err != nil:
		failed(w, r, err)
	default:
		backTo(w, r, notice, anchor)
	}
}

// schedule is the competition's timetable: the one saved, or a new one with
// the default setup.
func schedule(c store.Competition) competitions.Schedule {
	if c.Timetable != nil {
		return *c.Timetable
	}
	return competitions.Schedule{Setup: competitions.DefaultSetup(c.Disciplines())}
}

// timetableOf is the competition, its timetable and its live entries.
func (p *competitionPages) timetableOf(w http.ResponseWriter, r *http.Request) (store.Competition, competitions.Schedule, []store.Entry, bool) {
	c, ok := p.admin(w, r)
	if !ok {
		return c, competitions.Schedule{}, nil, false
	}
	entries, err := p.entriesOf(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return c, competitions.Schedule{}, nil, false
	}
	return c, schedule(c), live(entries), true
}

// itemsOf are each day's areas' flights and blocks, as the pages show them,
// with the gymnasts still entered (flagged if they've changed event since).
func itemsOf(s competitions.Schedule, entries []store.Entry, people []competitions.RotaPerson) []views.DayView {
	_, names := peopleOf(entries)
	byID := map[string]store.Entry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	var out []views.DayView
	for d, day := range s.Setup.Days {
		dv := views.DayView{ID: fmt.Sprintf("day-%d", d), Name: day.Name}
		for _, a := range s.Setup.Areas {
			if len(day.Areas) > 0 && !slices.Contains(day.Areas, a.Name) {
				continue
			}
			av := views.AreaView{ID: fmt.Sprintf("day-%d-", d) + areaID(a.Name), Name: a.Name}
			type timed struct {
				start int
				item  views.ItemView
			}
			var items []timed
			for i, f := range s.Flights {
				if f.Day != d || f.Area != a.Name {
					continue
				}
				it := views.ItemView{Flight: true, Index: i, Discipline: f.Discipline, Name: f.Name(), Start: competitions.Clock(f.Start), End: competitions.Clock(f.End), Seats: seatsOf(f, people, names),
					Pairs: f.Discipline == competitions.Synchro, InRun: f.Of > 1, Earlier: f.Number > 1}
				for n, id := range f.Entries {
					e, ok := byID[id]
					if !ok {
						continue // withdrawn since
					}
					g := views.TimetableGymnast{ID: id, FlightGymnast: views.FlightGymnast{No: n + 1, Name: e.Entry.Gymnasts(), Club: clubOf(e)}, Category: e.Entry.Category, Person: e.Key()}
					if e.Entry.Event() != f.Event || (f.Category != "" && e.Entry.Category != f.Category) {
						g.Moved = strings.TrimSpace(e.Entry.Event() + " " + e.Entry.Category)
					}
					it.Gymnasts = append(it.Gymnasts, g)
				}
				items = append(items, timed{f.Start, it})
			}
			for i, b := range s.Blocks {
				if b.Day == d && slices.Contains(b.Areas, a.Name) {
					it := views.ItemView{Index: i, Name: b.Name, Start: competitions.Clock(b.Start), End: competitions.Clock(b.End)}
					if len(b.Officials) > 0 {
						it.Seats = seatsOf(competitions.ScheduledFlight{Officials: b.Officials}, people, names)
					}
					items = append(items, timed{b.Start, it})
				}
			}
			slices.SortStableFunc(items, func(x, y timed) int { return x.start - y.start })
			// An area with nothing on that day but breaks (warm-up, lunch)
			// takes no column; one with an event taking entries does.
			busy := false
			for _, it := range items {
				av.Items = append(av.Items, it.item)
				busy = busy || it.item.Flight || len(it.item.Seats) > 0
			}
			if busy || len(s.Flights) == 0 {
				dv.Areas = append(dv.Areas, av)
			}
		}
		out = append(out, dv)
	}
	return out
}

func (p *competitionPages) timetable(w http.ResponseWriter, r *http.Request) {
	c, s, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	people, same, err := p.rotaAndSame(r.Context(), c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	page := views.TimetablePage{
		Base: adminPath(r.PathValue("token")), Competition: summary(c, p.now()), Menu: p.menuFor(r, c, menuTimetable), Notice: r.URL.Query().Get("notice"),
		Setup: setupForm(c.Competition, s.Setup, people), Planned: s.Planned, Stale: s.Stale, Published: c.Published != nil, Changes: c.Published != nil && !sameTimetable(s, *c.Published), Entries: len(entries), Version: s.Version(),
	}
	if c.Published != nil && s.Stale {
		marks, err := p.dayMarksOf(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		page.Begun = len(marks.begun(*c.Published)) > 0 // no "plan again" once it has
	}
	if s.Planned {
		officials := people
		page.Rota = rotaView(s, entries, officials, same)
		people, names := clashPeople(entries, same)
		report := s.Report(people)
		page.Report = views.ReportView{Unplaced: report.Unplaced, Blocks: report.UnplacedBlocks, Broken: report.Broken, Problems: s.Problems(people, names)}
		for _, d := range report.Days {
			page.Report.Days = append(page.Report.Days, views.DayFinish{Name: d.Name, Finish: d.Finish, End: d.End, Spare: d.SpareMinutes, FlightsEnd: d.FlightsEnd, Free: d.FreeMinutes})
		}
		gymnasts := map[string]string{}
		for k, n := range names {
			if kind, _ := store.KeyKind(k); kind != store.KindCoach {
				gymnasts[same.of(k)] = n // one person's keys, once
			}
		}
		for _, pair := range competitions.LookAlike(gymnasts) {
			page.Report.LookAlike = append(page.Report.LookAlike, fmt.Sprintf("%s and %s", pair[0], pair[1]))
		}
		for _, sr := range report.ShortRest {
			page.Report.ShortRest = append(page.Report.ShortRest, fmt.Sprintf("%s: %d minutes between %s and %s", names[sr.Person], sr.Minutes, sr.First, sr.Second))
		}
		if len(report.Unplaced)+len(report.UnplacedBlocks) > 0 {
			for _, f := range competitions.Fixes(schedEntries(entries, same), c.EventNames(), s.Setup, 1) {
				page.Report.Fixes = append(page.Report.Fixes, views.FixView{Change: f.Change, Fits: f.Fits})
			}
		}
		page.Days = itemsOf(s, entries, officials)
		for i, f := range s.Flights {
			page.Moves = append(page.Moves, views.FlightOption{Value: strconv.Itoa(i), Label: fmt.Sprintf("%s · %s %s · %s", f.Name(), s.Setup.Days[f.Day].Name, competitions.Clock(f.Start), f.Area), Group: f.Discipline})
		}
		for d, day := range s.Setup.Days {
			for _, a := range s.Setup.Areas {
				if day.Areas == nil || slices.Contains(day.Areas, a.Name) {
					page.Places = append(page.Places, views.FlightOption{Value: fmt.Sprintf("%d:%s", d, a.Name), Label: day.Name + " · " + a.Name})
				}
			}
		}
		for _, e := range entries {
			if _, placed := s.Find(e.ID); !placed {
				page.Unplaced = append(page.Unplaced, views.TimetableGymnast{
					ID: e.ID, FlightGymnast: views.FlightGymnast{Name: e.Entry.Gymnasts(), Club: clubOf(e)}, Category: strings.TrimSpace(e.Entry.Event() + " " + e.Entry.Category),
					Discipline: e.Entry.Discipline,
				})
			}
		}
	}
	render(w, r, views.CompetitionTimetable(page))
}

// timetableSetup is the timetable's Setup page: the days and areas, timings,
// blocked time and rules, and planning from them.
func (p *competitionPages) timetableSetup(w http.ResponseWriter, r *http.Request) {
	c, s, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	people, _, err := p.rotaAndSame(r.Context(), c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	begun := false
	if c.Published != nil {
		marks, err := p.dayMarksOf(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		begun = len(marks.begun(*c.Published)) > 0
	}
	render(w, r, views.TimetableSetup(views.TimetablePage{
		Base: adminPath(r.PathValue("token")), Competition: summary(c, p.now()), Menu: p.menuFor(r, c, menuSetup), Notice: r.URL.Query().Get("notice"),
		Setup: setupForm(c.Competition, s.Setup, people), Planned: s.Planned, Stale: s.Stale, Begun: begun, Entries: len(entries), Version: s.Version(),
	}))
}

// planTimetable plans afresh with the setup, keeping whether it's published.
func (p *competitionPages) planTimetable(w http.ResponseWriter, r *http.Request) {
	c, _, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	people, same, err := p.rotaAndSame(r.Context(), c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	marks, err := p.dayMarksOf(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	// Once the day has begun the timetable isn't planned again: a fresh plan
	// starts from the first morning, and what's run has run. Changes on the
	// day are made by hand or with the "what ifs".
	if c.Published != nil && len(marks.begun(*c.Published)) > 0 {
		changed(w, r, refused(dayBegunNote), "")
		return
	}
	notice := fmt.Sprintf("Planned %s.", gymnastsWord(len(entries)))
	err = p.changeDraft(r, c, func(s *competitions.Schedule) error {
		if err := s.Setup.Check(c.EventNames()); err != nil {
			return refused("Fix the setup first: " + strings.Join(sentences(err), " "))
		}
		// The flights it replaces, the published ones first, for what's
		// marked on the day to stay with them.
		var old []competitions.ScheduledFlight
		if c.Published != nil {
			old = slices.Clone(c.Published.Flights)
		}
		old = slices.Concat(old, s.Flights, s.Unplaced)
		*s = competitions.PlanStaffed(schedEntries(entries, same), c.EventNames(), s.Setup, staffing(c.Competition, people), uint64(p.now().UnixNano()))
		s.KeepIDs(old, func(id string) bool { return !marks.underWay(id) })
		p.staff(s, c, entries, people, same)
		if n := len(s.Unplaced) + len(s.UnplacedBlocks); n > 0 {
			notice += " Some of it doesn't fit: see below."
		}
		return nil
	})
	changed(w, r, err, notice)
}

// dayBegunNote says why the timetable isn't planned again once the
// competition has begun, and what to do instead.
const dayBegunNote = "The competition has begun, so the timetable isn't planned again; nothing was changed. " +
	"Change flights and seats by hand, or use What if there's a delay and What if an official leaves."

func gymnastsWord(n int) string {
	if n == 1 {
		return "1 gymnast"
	}
	return fmt.Sprintf("%d gymnasts", n)
}

// cantGo refuses a change to a flight or gymnast that no longer fits the
// draft.
const cantGo = refused("That flight can't go there, or has moved since; nothing was changed.")

// editFlight redraws a flight's running order (action=redraw), moves its
// event to the end of a day's area (action=move, to: "<day>:<area>"), or swaps
// it with the flight before it in its event (action=earlier).
func (p *competitionPages) editFlight(w http.ResponseWriter, r *http.Request) {
	c, draft, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	flight, _ := strconv.Atoi(r.FormValue("flight"))
	_, same, err := p.rotaAndSame(r.Context(), c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	notice, anchor := "", ""
	apply := func(s *competitions.Schedule) error {
		done := false
		if flight >= 0 && flight < len(s.Flights) {
			anchor = flightAnchor(s.Flights[flight])
		}
		switch r.FormValue("action") {
		case "redraw":
			clubs := map[string]string{}
			for _, e := range schedEntries(entries, same) {
				clubs[e.ID] = e.Club
			}
			people, _ := clashPeople(entries, same)
			done = s.Redraw(flight, clubs, people, rand.New(rand.NewPCG(uint64(p.now().UnixNano()), 1)))
			if done {
				notice = "Redrew " + s.Flights[flight].Name() + "'s running order."
			}
		case "move":
			day, area, _ := strings.Cut(r.FormValue("to"), ":")
			d, err := strconv.Atoi(day)
			name := ""
			if flight >= 0 && flight < len(s.Flights) {
				name = s.Flights[flight].Group()
			}
			done = err == nil && s.MoveFlight(flight, d, area)
			if done && d >= 0 && d < len(s.Setup.Days) {
				notice = fmt.Sprintf("Moved %s to %s · %s.", name, s.Setup.Days[d].Name, area)
				anchor = fmt.Sprintf("day-%d-", d) + areaID(area)
			}
		case "earlier":
			done = s.Earlier(flight)
			if done {
				notice = "Moved it earlier in its event."
			}
		}
		if !done {
			return cantGo
		}
		return nil
	}
	// A move that breaks something asks first, offering days and areas
	// where the event fits as things are.
	if r.FormValue("action") == "move" && flight >= 0 && flight < len(draft.Flights) {
		f, err := p.liveRota(r.Context(), c)
		if err != nil {
			failed(w, r, err)
			return
		}
		what := "Move " + draft.Flights[flight].Group() + " to " + placeName(draft, r.FormValue("to"))
		if p.askFirst(w, r, c, draft, f, what, apply, func(clean func(func(*competitions.Schedule) bool) bool) []views.ChangeFix {
			var fixes []views.ChangeFix
			now := draft.Flights[flight]
			for d := range draft.Setup.Days {
				for _, a := range draft.Setup.Areas {
					to := fmt.Sprintf("%d:%s", d, a.Name)
					// Not the place asked for, nor where it is now: that's no move.
					if len(fixes) == maxFixes || to == r.FormValue("to") || d == now.Day && a.Name == now.Area {
						continue
					}
					if clean(func(t *competitions.Schedule) bool { return t.MoveFlight(flight, d, a.Name) }) {
						fixes = append(fixes, views.ChangeFix{Label: "Move it to " + placeName(draft, to), Fields: withField(postedFields(r), "to", to)})
					}
				}
			}
			return fixes
		}) {
			return
		}
	}
	err = p.changeDraft(r, c, apply)
	changedAt(w, r, err, notice, anchor)
}

// placeName names a "<day>:<area>" of the schedule, e.g. "Saturday · Panel 2".
func placeName(s competitions.Schedule, to string) string {
	day, area, _ := strings.Cut(to, ":")
	if d, err := strconv.Atoi(day); err == nil && d >= 0 && d < len(s.Setup.Days) {
		return s.Setup.Days[d].Name + " · " + area
	}
	return area
}

// moveEntry moves a gymnast to a flight (to: its index), or out of the
// timetable (to: "").
func (p *competitionPages) moveEntry(w http.ResponseWriter, r *http.Request) {
	c, draft, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	id := r.FormValue("entry")
	if id == "" {
		// A flight's Move a gymnast left on "Choose a gymnast".
		back(w, r, "Choose a gymnast to move; nothing was changed.")
		return
	}
	known := slices.ContainsFunc(entries, func(e store.Entry) bool { return e.ID == id })
	if !r.Form.Has("to") {
		// The dropdown was left on "Choose a flight", which sends nothing:
		// not the same as choosing to take them out.
		back(w, r, "Choose a flight to move them to; nothing was changed.")
		return
	}
	to := r.FormValue("to")
	if to != "" && !known {
		back(w, r, "That gymnast isn't entered any more.")
		return
	}
	name := ""
	if i := slices.IndexFunc(entries, func(e store.Entry) bool { return e.ID == id }); i >= 0 {
		name = entries[i].Entry.Gymnasts()
	}
	notice, anchor := "", ""
	apply := func(s *competitions.Schedule) error {
		if from, ok := s.Find(id); ok {
			anchor = flightAnchor(s.Flights[from])
		}
		if to == "" {
			s.RemoveEntry(id)
			s.Retime()
			notice = "Took " + name + " out of the timetable."
			return nil
		}
		flight, err := strconv.Atoi(to)
		if err != nil || flight < 0 || flight >= len(s.Flights) {
			return refused("That flight has moved since; nothing was changed.")
		}
		// A gymnast moves only to a flight of their own discipline.
		if i := slices.IndexFunc(entries, func(e store.Entry) bool { return e.ID == id }); i >= 0 && entries[i].Entry.Discipline != s.Flights[flight].Discipline {
			return refused("They can only move to a flight of their own discipline; nothing was changed.")
		}
		dest := s.Flights[flight]
		if !s.MoveEntry(id, flight) {
			return refused("That flight has moved since; nothing was changed.")
		}
		notice, anchor = "Moved "+name+" to "+dest.Name()+".", flightAnchor(dest)
		return nil
	}
	// A move that breaks something asks first, offering the event's other
	// flights where they'd fit as things are.
	if flight, err := strconv.Atoi(to); err == nil && flight >= 0 && flight < len(draft.Flights) {
		f, err := p.liveRota(r.Context(), c)
		if err != nil {
			failed(w, r, err)
			return
		}
		dest := draft.Flights[flight]
		if p.askFirst(w, r, c, draft, f, "Move "+name+" to "+dest.Name(), apply, func(clean func(func(*competitions.Schedule) bool) bool) []views.ChangeFix {
			var fixes []views.ChangeFix
			for i, g := range draft.Flights {
				if len(fixes) == maxFixes || i == flight || g.Event != dest.Event || g.Category != dest.Category || g.Discipline != dest.Discipline || slices.Contains(g.Entries, id) {
					continue
				}
				if clean(func(t *competitions.Schedule) bool { return t.MoveEntry(id, i) }) {
					fixes = append(fixes, views.ChangeFix{Label: "Put them in " + g.Name() + " instead", Fields: withField(postedFields(r), "to", strconv.Itoa(i))})
				}
			}
			return fixes
		}) {
			return
		}
	}
	err := p.changeDraft(r, c, apply)
	changedAt(w, r, err, notice, anchor)
}

func (p *competitionPages) publishTimetable(w http.ResponseWriter, r *http.Request) {
	c, s, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if !s.Planned {
		back(w, r, "Plan the timetable first.")
		return
	}
	notice, action := "", r.FormValue("action")
	if action == "" { // on=1 or 0, as before
		action = map[string]string{"1": "publish", "0": "unpublish"}[r.FormValue("on")]
	}
	switch action {
	case "publish":
		// Not a draft someone has changed since the page was opened: that
		// would publish what nobody has looked at.
		if v, ok := r.PostForm["version"]; ok && (len(v) != 1 || v[0] != s.Version()) {
			back(w, r, string(staleTimetable))
			return
		}
		// What's been marked on the day that this wouldn't show any more
		// is said first.
		if c.Published != nil && r.FormValue("anyway") != "1" {
			marks, err := p.dayMarksOf(r.Context(), c.ID)
			if err != nil {
				failed(w, r, err)
				return
			}
			names := map[string]string{}
			for _, e := range entries {
				names[e.ID] = e.Entry.Gymnasts()
			}
			name := func(id string) string {
				if n, ok := names[id]; ok {
					return n
				}
				return "someone no longer entered"
			}
			if lost := marks.lost(*c.Published, s, name); len(lost) > 0 {
				p.askDayMarks(w, r, c, "Publish the changes", "Publishing drops what was marked on the day:", lost, "Publish anyway",
					"Everything else marked on the day stays as it is.")
				return
			}
		}
		p.notify.changing(r.Context(), c)
		if err := p.st.PublishTimetable(r.Context(), c.ID, &s); err != nil {
			failed(w, r, err)
			return
		}
		notice = "Published: clubs, members and gymnasts entering on their own see their flight, area and time, and officials their duties."
	case "unpublish":
		p.notify.changing(r.Context(), c)
		if err := p.st.PublishTimetable(r.Context(), c.ID, nil); err != nil {
			failed(w, r, err)
			return
		}
		notice = "Unpublished: only you see the timetable."
	case "discard":
		if c.Published == nil {
			back(w, r, "There's nothing published to go back to.")
			return
		}
		draft := *c.Published
		draft.Published = false
		if err := p.st.SetTimetable(r.Context(), c.ID, &draft); err != nil {
			failed(w, r, err)
			return
		}
		notice = "Changes discarded: the timetable is as published."
	default:
		back(w, r, "")
		return
	}
	back(w, r, notice)
}

// placement is where an entry competes, on the published timetable.
func placement(c store.Competition, entryID string) *views.Placement {
	t := c.Published
	if t == nil || entryID == "" {
		return nil
	}
	i, ok := t.Find(entryID)
	if !ok {
		return nil
	}
	f := t.Flights[i]
	day := ""
	if f.Day < len(t.Setup.Days) {
		day = t.Setup.Days[f.Day].Name + " "
	}
	return &views.Placement{Flight: f.Name(), Panel: f.Area, Time: day + competitions.Clock(f.Start)}
}

// sameTimetable says whether a draft is what's published.
func sameTimetable(draft, published competitions.Schedule) bool {
	draft.Published, published.Published = false, false
	a, err1 := json.Marshal(draft)
	b, err2 := json.Marshal(published)
	return err1 == nil && err2 == nil && string(a) == string(b)
}

// keepWhatIf makes a "what if"'s timetable the draft, for the organiser to
// publish: whatIf works it out again from the draft as it is when saved,
// and a form from a page made from another draft is refused (changeDraft).
func (p *competitionPages) keepWhatIf(w http.ResponseWriter, r *http.Request, c store.Competition, whatIf func(competitions.Schedule) (competitions.Schedule, error), done string) {
	err := p.changeDraft(r, c, func(s *competitions.Schedule) error {
		out, err := whatIf(*s)
		if err != nil {
			return refused("Nothing was changed: " + strings.Join(sentences(err), " "))
		}
		*s = out
		return nil
	})
	notice := done
	if c.Published != nil {
		notice += " Publish the changes to show them to everyone."
	}
	changed(w, r, err, notice)
}
