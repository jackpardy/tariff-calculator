package main

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/requirements"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The timetable (ADR 0005 step 4): the organiser sets up the days, areas,
// timings, blocked time and rules; plans; sees what doesn't fit and what would
// fix it; adjusts by hand; prints the sheets; and publishes, so clubs and
// gymnasts see their flight, area and time.

func (p *competitionPages) registerTimetable(handle func(string, http.HandlerFunc)) {
	handle("GET /competitions/admin/{token}/timetable", p.timetable)
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
	handle("GET /competitions/admin/{token}/timetable/simulate", p.simulation)
	handle("POST /competitions/admin/{token}/timetable/simulate", p.simulate)
	handle("POST /competitions/admin/{token}/timetable/simulate/delete", p.deleteScenario)
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

// personKeys are each entry's people, by entry id, matched across entries:
// members by member, and individuals by name, since an individual enters each
// discipline separately (ADR 0005 Decision 4). Two different individuals with
// the same name are treated as one: kept apart, never put in two places.
func personKeys(entries []store.Entry) map[string][]string {
	individual := map[string]string{} // entry id → key
	for _, e := range entries {
		if e.Individual {
			individual[e.ID] = individualKey(e.Entry.Gymnast)
		}
	}
	out := map[string][]string{}
	for _, e := range entries {
		var keys []string
		for _, k := range e.People() {
			if id, ok := strings.CutPrefix(k, "e:"); ok {
				if key, known := individual[id]; known {
					k = key
				}
			}
			keys = append(keys, k)
		}
		out[e.ID] = keys
	}
	return out
}

// individualKey is the person key of a gymnast entering on their own, by name.
func individualKey(name string) string {
	return "i:" + strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// coachKey is the key of an entry's coach, if it has one: by club and name.
func coachKey(e store.Entry) string {
	name := strings.ToLower(strings.Join(strings.Fields(e.CoachName()), " "))
	if name == "" {
		return ""
	}
	return "c:" + e.ClubID + ":" + name
}

// schedEntries are entries as the scheduler needs them, with their people.
func schedEntries(entries []store.Entry) []competitions.SchedEntry {
	keys := personKeys(entries)
	out := make([]competitions.SchedEntry, len(entries))
	for i, e := range entries {
		club := e.ClubName
		if e.Individual {
			club = ""
		}
		out[i] = competitions.SchedEntry{
			PlanEntry:  competitions.PlanEntry{ID: e.ID, Level: e.Entry.Event(), Category: e.Entry.Category, Club: club},
			Discipline: e.Entry.Discipline, People: keys[e.ID],
		}
		if k := coachKey(e); k != "" {
			out[i].Coaches = []string{k}
		}
	}
	return out
}

// peopleOf are each entry's people, and each person's name.
func peopleOf(entries []store.Entry) (map[string][]string, map[string]string) {
	people, names := personKeys(entries), map[string]string{}
	for _, e := range entries {
		ps := people[e.ID]
		names[ps[0]] = e.Entry.Gymnast
		if len(ps) > 1 && e.Entry.Partner != nil {
			names[ps[1]] = e.Entry.Partner.Name
		}
		if k := coachKey(e); k != "" {
			names[k] = e.CoachName()
		}
	}
	return people, names
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
	entries, err := p.st.Entries(r.Context(), c.ID)
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
		dv := views.DayView{Name: day.Name}
		for _, a := range s.Setup.Areas {
			if len(day.Areas) > 0 && !slices.Contains(day.Areas, a.Name) {
				continue
			}
			av := views.AreaView{Name: a.Name}
			type timed struct {
				start int
				item  views.ItemView
			}
			var items []timed
			for i, f := range s.Flights {
				if f.Day != d || f.Area != a.Name {
					continue
				}
				it := views.ItemView{Flight: true, Index: i, Name: f.Name(), Start: competitions.Clock(f.Start), End: competitions.Clock(f.End), Seats: seatsOf(f, people, names)}
				for _, id := range f.Entries {
					e, ok := byID[id]
					if !ok {
						continue // withdrawn since
					}
					g := views.TimetableGymnast{ID: id, Name: e.Entry.Gymnasts(), Club: clubOf(e), Category: e.Entry.Category}
					if e.Entry.Event() != f.Level || (f.Category != "" && e.Entry.Category != f.Category) {
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
			for _, it := range items {
				av.Items = append(av.Items, it.item)
			}
			dv.Areas = append(dv.Areas, av)
		}
		out = append(out, dv)
	}
	return out
}

// setupForm is the setup as the forms show it.
func setupForm(c competitions.Competition, s competitions.Setup, people []competitions.RotaPerson) views.SetupForm {
	f := views.SetupForm{Rest: s.Rest, RestMust: s.RestMust, Events: c.EventNames()}
	for _, o := range people {
		f.People = append(f.People, views.FlightOption{Value: o.Key, Label: o.Name})
	}
	for _, role := range competitions.Roles {
		f.Roles = append(f.Roles, views.FlightOption{Value: role, Label: competitions.RoleName(role)})
	}
	for _, a := range s.Areas {
		f.Areas = append(f.Areas, views.AreaForm{Name: a.Name, Discipline: a.Discipline})
		f.AreaNames = append(f.AreaNames, a.Name)
	}
	for _, d := range s.Days {
		areas := map[string]bool{}
		for _, a := range d.Areas {
			areas[a] = true
		}
		f.Days = append(f.Days, views.DayForm{Name: d.Name, Start: d.Start, End: d.End, Areas: areas})
		f.DayNames = append(f.DayNames, d.Name)
	}
	for _, d := range c.Disciplines() {
		t := s.TimingsFor(d)
		f.Timings = append(f.Timings, views.TimingForm{
			Key: offerKey(d), Name: competitions.DisciplineName(d), PerCompetitor: strconv.FormatFloat(t.PerCompetitor, 'f', -1, 64),
			Between: strconv.Itoa(t.Between), MaxFlight: strconv.Itoa(t.MaxFlight),
		})
	}
	for _, b := range s.Blocks {
		f.Blocks = append(f.Blocks, describeBlock(b, s.Days))
	}
	for _, r := range s.Rules {
		f.Rules = append(f.Rules, r.Describe(s.Days))
	}
	var split []string
	for _, e := range c.EventNames() {
		if c.Split.Splits(e) {
			split = append(split, e)
		}
	}
	f.Separate, f.CanSeparate = splitForm(s.Separate, split), len(split) > 0
	return f
}

// describeBlock says what blocked time is, e.g. "Lunch, 45 minutes, Saturday
// between 12:00 and 14:00, every area".
func describeBlock(b competitions.Block, days []competitions.Day) string {
	day := fmt.Sprintf("day %d", b.Day+1)
	if b.Day >= 0 && b.Day < len(days) {
		day = days[b.Day].Name
	}
	when := day + " at " + b.At
	if b.At == "" {
		when = day + " between " + b.From + " and " + b.To
	}
	where := "every area"
	if len(b.Areas) > 0 {
		where = strings.Join(b.Areas, ", ")
	}
	out := fmt.Sprintf("%s, %d minutes, %s, %s", b.Name, b.Minutes, when, where)
	if n := len(b.Officials.Seats()); n > 0 {
		out += fmt.Sprintf(", needs %d official%s", n, map[bool]string{true: "s"}[n > 1])
	}
	return out
}

func (p *competitionPages) timetable(w http.ResponseWriter, r *http.Request) {
	c, s, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	people, err := p.rotaOf(r, c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	page := views.TimetablePage{
		Base: adminPath(r.PathValue("token")), Competition: summary(c.Competition, p.now()), Notice: r.URL.Query().Get("notice"),
		Setup: setupForm(c.Competition, s.Setup, people), Planned: s.Planned, Stale: s.Stale, Published: s.Published, Entries: len(entries),
	}
	if s.Planned {
		officials := people
		page.Rota = rotaView(s, entries, officials)
		people, names := peopleOf(entries)
		report := s.Report(people)
		page.Report = views.ReportView{Unplaced: report.Unplaced, Blocks: report.UnplacedBlocks, Broken: report.Broken, Problems: s.Problems(people, names)}
		for _, d := range report.Days {
			page.Report.Days = append(page.Report.Days, views.DayFinish{Name: d.Name, Finish: d.Finish, End: d.End, Spare: d.SpareMinutes})
		}
		gymnasts := map[string]string{}
		for k, n := range names {
			if !strings.HasPrefix(k, "c:") {
				gymnasts[k] = n
			}
		}
		for _, pair := range competitions.LookAlike(gymnasts) {
			page.Report.LookAlike = append(page.Report.LookAlike, fmt.Sprintf("%s and %s", pair[0], pair[1]))
		}
		for _, sr := range report.ShortRest {
			page.Report.ShortRest = append(page.Report.ShortRest, fmt.Sprintf("%s: %d minutes between %s and %s", names[sr.Person], sr.Minutes, sr.First, sr.Second))
		}
		if len(report.Unplaced)+len(report.UnplacedBlocks) > 0 {
			for _, f := range competitions.Fixes(schedEntries(entries), c.EventNames(), s.Setup, 1) {
				page.Report.Fixes = append(page.Report.Fixes, views.FixView{Change: f.Change, Fits: f.Fits})
			}
		}
		page.Days = itemsOf(s, entries, officials)
		for i, f := range s.Flights {
			page.Moves = append(page.Moves, views.FlightOption{Value: strconv.Itoa(i), Label: fmt.Sprintf("%s · %s %s · %s", f.Name(), s.Setup.Days[f.Day].Name, competitions.Clock(f.Start), f.Area)})
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
					ID: e.ID, Name: e.Entry.Gymnasts(), Club: clubOf(e), Category: strings.TrimSpace(e.Entry.Event() + " " + e.Entry.Category),
				})
			}
		}
	}
	render(w, r, views.CompetitionTimetable(page))
}

// saveSetup checks a changed setup and saves it, marking the plan stale.
func (p *competitionPages) saveSetup(w http.ResponseWriter, r *http.Request, c store.Competition, s competitions.Schedule, notice string) {
	if err := s.Setup.Check(c.EventNames()); err != nil {
		back(w, r, "Nothing was changed: "+strings.Join(sentences(err), " "))
		return
	}
	s.Stale = s.Planned
	if err := p.st.SetTimetable(r.Context(), c.ID, &s); err != nil {
		failed(w, r, err)
		return
	}
	back(w, r, notice)
}

// saveRotaRules saves a change to the rules about officials: the flights
// stand, so only the officials need assigning again.
func (p *competitionPages) saveRotaRules(w http.ResponseWriter, r *http.Request, c store.Competition, s competitions.Schedule, notice string) {
	if err := s.Setup.Check(c.EventNames()); err != nil {
		back(w, r, "Nothing was changed: "+strings.Join(sentences(err), " "))
		return
	}
	if err := p.st.SetTimetable(r.Context(), c.ID, &s); err != nil {
		failed(w, r, err)
		return
	}
	if s.Planned {
		notice += " Assign officials again to use it."
	}
	back(w, r, notice)
}

// setupDays saves the days (day-<i>-name, -start, -end, -area), adds one
// (add=1) or removes one (remove=<i>).
func (p *competitionPages) setupDays(w http.ResponseWriter, r *http.Request) {
	c, s, _, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	for i := range s.Setup.Days {
		d := &s.Setup.Days[i]
		d.Name = strings.TrimSpace(r.FormValue(fmt.Sprintf("day-%d-name", i)))
		d.Start, d.End = r.FormValue(fmt.Sprintf("day-%d-start", i)), r.FormValue(fmt.Sprintf("day-%d-end", i))
		d.Areas = r.Form[fmt.Sprintf("day-%d-area", i)]
	}
	if i, err := strconv.Atoi(r.FormValue("remove")); err == nil && i >= 0 && i < len(s.Setup.Days) {
		s.Setup.Days = slices.Delete(s.Setup.Days, i, i+1)
		// Blocks and day rules on later days move with them; those on the day go.
		s.Setup.Blocks = slices.DeleteFunc(s.Setup.Blocks, func(b competitions.Block) bool { return b.Day == i })
		s.Setup.Rules = slices.DeleteFunc(s.Setup.Rules, func(ru competitions.Rule) bool { return ru.Kind == competitions.RuleDay && ru.Day == i })
		for j := range s.Setup.Blocks {
			if s.Setup.Blocks[j].Day > i {
				s.Setup.Blocks[j].Day--
			}
		}
		for j := range s.Setup.Rules {
			if s.Setup.Rules[j].Kind == competitions.RuleDay && s.Setup.Rules[j].Day > i {
				s.Setup.Rules[j].Day--
			}
		}
	}
	if r.FormValue("add") == "1" {
		s.Setup.Days = append(s.Setup.Days, competitions.Day{Name: fmt.Sprintf("Day %d", len(s.Setup.Days)+1), Start: "09:00", End: "18:00"})
	}
	p.saveSetup(w, r, c, s, "Days saved.")
}

// setupAreas saves the areas (area-<i>-name, -discipline), adds one (add=1)
// or removes one (remove=<i>). Days, blocks and rules follow a renamed area.
func (p *competitionPages) setupAreas(w http.ResponseWriter, r *http.Request) {
	c, s, _, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	rename := map[string]string{}
	for i := range s.Setup.Areas {
		a := &s.Setup.Areas[i]
		name := strings.TrimSpace(r.FormValue(fmt.Sprintf("area-%d-name", i)))
		if name != a.Name {
			rename[a.Name] = name
		}
		a.Name = name
		a.Discipline = r.FormValue(fmt.Sprintf("area-%d-discipline", i))
		if a.Discipline == "trampoline" {
			a.Discipline = competitions.Trampoline
		}
	}
	removed := ""
	if i, err := strconv.Atoi(r.FormValue("remove")); err == nil && i >= 0 && i < len(s.Setup.Areas) {
		removed = s.Setup.Areas[i].Name
		s.Setup.Areas = slices.Delete(s.Setup.Areas, i, i+1)
	}
	follow := func(names []string) []string {
		var out []string
		for _, n := range names {
			if to, ok := rename[n]; ok {
				n = to
			}
			if n != removed {
				out = append(out, n)
			}
		}
		return out
	}
	for i := range s.Setup.Days {
		s.Setup.Days[i].Areas = follow(s.Setup.Days[i].Areas)
	}
	for i := range s.Setup.Blocks {
		s.Setup.Blocks[i].Areas = follow(s.Setup.Blocks[i].Areas)
	}
	s.Setup.Rules = slices.DeleteFunc(s.Setup.Rules, func(ru competitions.Rule) bool { return ru.Kind == competitions.RuleArea && ru.Area == removed })
	for i := range s.Setup.Rules {
		if to, ok := rename[s.Setup.Rules[i].Area]; ok {
			s.Setup.Rules[i].Area = to
		}
	}
	if r.FormValue("add") == "1" {
		s.Setup.Areas = append(s.Setup.Areas, competitions.Area{Name: fmt.Sprintf("Area %d", len(s.Setup.Areas)+1), Discipline: competitions.Trampoline})
	}
	p.saveSetup(w, r, c, s, "Areas saved.")
}

// setupTimings saves each discipline's timings (per-, between-, max-<key>),
// rest (rest, restMust) and separate men's and women's flights.
func (p *competitionPages) setupTimings(w http.ResponseWriter, r *http.Request) {
	c, s, _, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	s.Setup.Timings = map[string]competitions.Timings{}
	for _, d := range c.Disciplines() {
		key := offerKey(d)
		per, err1 := strconv.ParseFloat(r.FormValue("per-"+key), 64)
		between, err2 := strconv.Atoi(r.FormValue("between-" + key))
		most, err3 := strconv.Atoi(r.FormValue("max-" + key))
		if err1 != nil || err2 != nil || err3 != nil {
			back(w, r, "Nothing was changed: timings should be numbers.")
			return
		}
		t := competitions.Timings{PerCompetitor: per, Between: between, MaxFlight: most}
		if t != competitions.DefaultTimings(d) {
			s.Setup.Timings[d] = t
		}
	}
	rest, err := strconv.Atoi(r.FormValue("rest"))
	if err != nil {
		back(w, r, "Nothing was changed: rest should be a number of minutes.")
		return
	}
	s.Setup.Rest, s.Setup.RestMust = rest, r.FormValue("restMust") == "1"
	if r.Form.Has("separate") {
		s.Setup.Separate = postedSplit(r, "separate", "separateLevel")
	}
	p.saveSetup(w, r, c, s, "Timings saved.")
}

// setupBlocks adds blocked time (add=1: name, minutes, day, at or from and
// to, area) or removes some (remove=<i>).
func (p *competitionPages) setupBlocks(w http.ResponseWriter, r *http.Request) {
	c, s, _, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	if i, err := strconv.Atoi(r.FormValue("remove")); err == nil && i >= 0 && i < len(s.Setup.Blocks) {
		s.Setup.Blocks = slices.Delete(s.Setup.Blocks, i, i+1)
		p.saveSetup(w, r, c, s, "Blocked time removed.")
		return
	}
	minutes, _ := strconv.Atoi(r.FormValue("minutes"))
	day, _ := strconv.Atoi(r.FormValue("day"))
	b := competitions.Block{Name: strings.TrimSpace(r.FormValue("name")), Minutes: minutes, Day: day, Areas: r.Form["area"], At: r.FormValue("at")}
	if b.At == "" {
		b.From, b.To = r.FormValue("from"), r.FormValue("to")
	}
	need := func(name string) int {
		n, _ := strconv.Atoi(r.FormValue(name))
		return n
	}
	b.Officials = competitions.Panel{Chair: need("chair"), Execution: need("judges"), Recorder: need("recorder"), Marshal: need("marshal")}
	s.Setup.Blocks = append(s.Setup.Blocks, b)
	p.saveSetup(w, r, c, s, "Blocked time added: "+describeBlock(b, s.Setup.Days)+".")
}

// setupRules adds a rule (add=1: kind, must, event, and the area, day or
// other event it needs) or removes one (remove=<i>).
func (p *competitionPages) setupRules(w http.ResponseWriter, r *http.Request) {
	c, s, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if i, err := strconv.Atoi(r.FormValue("remove")); err == nil && i >= 0 && i < len(s.Setup.Rules) {
		about := s.Setup.Rules[i].Person != ""
		s.Setup.Rules = slices.Delete(s.Setup.Rules, i, i+1)
		if about {
			p.saveRotaRules(w, r, c, s, "Rule removed.")
		} else {
			p.saveSetup(w, r, c, s, "Rule removed.")
		}
		return
	}
	if kind := r.FormValue("kind"); kind == competitions.RuleRole || kind == competitions.RuleOff || kind == competitions.RuleHours {
		people, err := p.rotaOf(r, c, entries)
		if err != nil {
			failed(w, r, err)
			return
		}
		rule, ok := personRule(r, people)
		if !ok {
			back(w, r, "Choose someone who's offered to officiate.")
			return
		}
		s.Setup.Rules = append(s.Setup.Rules, rule)
		p.saveRotaRules(w, r, c, s, "Rule added: "+rule.Describe(s.Setup.Days)+".")
		return
	}
	rule := competitions.Rule{Kind: r.FormValue("kind"), Must: r.FormValue("must") == "1", Event: r.FormValue("event")}
	switch rule.Kind {
	case competitions.RuleArea:
		rule.Area = r.FormValue("area")
	case competitions.RuleDay:
		rule.Day, _ = strconv.Atoi(r.FormValue("day"))
	default:
		rule.Event2 = r.FormValue("event2")
	}
	s.Setup.Rules = append(s.Setup.Rules, rule)
	p.saveSetup(w, r, c, s, "Rule added: "+rule.Describe(s.Setup.Days)+".")
}

// planTimetable plans afresh with the setup, keeping whether it's published.
func (p *competitionPages) planTimetable(w http.ResponseWriter, r *http.Request) {
	c, s, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if err := s.Setup.Check(c.EventNames()); err != nil {
		back(w, r, "Fix the setup first: "+strings.Join(sentences(err), " "))
		return
	}
	people, err := p.rotaOf(r, c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	planned := competitions.PlanStaffed(schedEntries(entries), c.EventNames(), s.Setup, staffing(c.Competition, people), uint64(p.now().UnixNano()))
	planned.Published = s.Published
	p.staff(&planned, c, entries, people)
	if err := p.st.SetTimetable(r.Context(), c.ID, &planned); err != nil {
		failed(w, r, err)
		return
	}
	notice := fmt.Sprintf("Planned %s.", gymnastsWord(len(entries)))
	if n := len(planned.Unplaced) + len(planned.UnplacedBlocks); n > 0 {
		notice += " Some of it doesn't fit: see below."
	}
	back(w, r, notice)
}

func gymnastsWord(n int) string {
	if n == 1 {
		return "1 gymnast"
	}
	return fmt.Sprintf("%d gymnasts", n)
}

// editFlight redraws a flight's running order (action=redraw) or moves it to
// the end of a day's area (action=move, to: "<day>:<area>").
func (p *competitionPages) editFlight(w http.ResponseWriter, r *http.Request) {
	c, s, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	flight, _ := strconv.Atoi(r.FormValue("flight"))
	done := false
	switch r.FormValue("action") {
	case "redraw":
		clubs := map[string]string{}
		for _, e := range schedEntries(entries) {
			clubs[e.ID] = e.Club
		}
		people, _ := peopleOf(entries)
		done = s.Redraw(flight, clubs, people, rand.New(rand.NewPCG(uint64(p.now().UnixNano()), 1)))
	case "move":
		day, area, _ := strings.Cut(r.FormValue("to"), ":")
		d, err := strconv.Atoi(day)
		done = err == nil && s.MoveFlight(flight, d, area)
	}
	if !done {
		back(w, r, "That flight can't go there, or has moved since; nothing was changed.")
		return
	}
	if err := p.st.SetTimetable(r.Context(), c.ID, &s); err != nil {
		failed(w, r, err)
		return
	}
	back(w, r, "")
}

// moveEntry moves a gymnast to a flight (to: its index), or out of the
// timetable (to: "").
func (p *competitionPages) moveEntry(w http.ResponseWriter, r *http.Request) {
	c, s, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	id := r.FormValue("entry")
	known := slices.ContainsFunc(entries, func(e store.Entry) bool { return e.ID == id })
	switch to := r.FormValue("to"); {
	case to == "":
		s.RemoveEntry(id)
		s.Retime()
	case !known:
		back(w, r, "That gymnast isn't entered any more.")
		return
	default:
		flight, err := strconv.Atoi(to)
		if err != nil || !s.MoveEntry(id, flight) {
			back(w, r, "That flight has moved since; nothing was changed.")
			return
		}
	}
	if err := p.st.SetTimetable(r.Context(), c.ID, &s); err != nil {
		failed(w, r, err)
		return
	}
	back(w, r, "")
}

func (p *competitionPages) publishTimetable(w http.ResponseWriter, r *http.Request) {
	c, s, _, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if !s.Planned {
		back(w, r, "Plan the timetable first.")
		return
	}
	s.Published = r.FormValue("on") == "1"
	if err := p.st.SetTimetable(r.Context(), c.ID, &s); err != nil {
		failed(w, r, err)
		return
	}
	notice := "Unpublished: only you see the timetable."
	if s.Published {
		notice = "Published: clubs, members and gymnasts entering on their own see their flight, area and time."
	}
	back(w, r, notice)
}

// printTimetable prints the marshal sheets (sheet=marshal) or the chair of
// judges sheets (sheet=judges), one area's day to a page, each flight with its
// panel; the officials rota (sheet=rota), each person's duties; or the panel
// timeline, without officials (sheet=timeline) or with them
// (sheet=timeline-officials).
func (p *competitionPages) printTimetable(w http.ResponseWriter, r *http.Request) {
	c, s, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if !s.Planned {
		back(w, r, "Plan the timetable first.")
		return
	}
	people, err := p.rotaOf(r, c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	if sheet := r.URL.Query().Get("sheet"); sheet == "timeline" || sheet == "timeline-officials" {
		printTimeline(w, r, c, s, entries, people, p.now(), sheet == "timeline-officials")
		return
	}
	if r.URL.Query().Get("sheet") == "rota" {
		render(w, r, views.RotaPrint(views.RotaSheet{
			Title: "Officials rota · " + c.Name, Back: timetablePath(r), Competition: summary(c.Competition, p.now()), People: rotaSheet(s, people),
		}))
		return
	}
	judges := r.URL.Query().Get("sheet") == "judges"
	title := "Marshal sheets · " + c.Name
	if judges {
		title = "Chair of judges sheets · " + c.Name
	}
	sheets := views.TimetableSheets{
		Title: title, Back: timetablePath(r), Competition: summary(c.Competition, p.now()), Judges: judges,
		Exercises: map[string][2]string{}, Notes: map[string]string{},
	}
	for _, day := range itemsOf(s, entries, people) {
		for _, area := range day.Areas {
			sheet := views.SheetView{Day: day.Name, Area: area.Name}
			for _, it := range area.Items {
				if it.Flight {
					sheet.Flights = append(sheet.Flights, it)
				}
			}
			if len(sheet.Flights) > 0 {
				sheets.Sheets = append(sheets.Sheets, sheet)
			}
		}
	}
	if judges {
		for _, j := range judge(c.Competition, entries) {
			if j.err != nil {
				sheets.Notes[j.ID] = j.err.Error()
				continue
			}
			if j.card.Unchecked {
				sheets.Exercises[j.ID] = [2]string{"not checked", ""}
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
