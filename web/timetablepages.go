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
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/requirements"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The timetable (ADR 0005 step 4): the organiser sets up the days, areas,
// timings, blocked time and rules; plans; sees what doesn't fit and what would
// fix it; adjusts manually; prints the sheets; and publishes, so clubs and
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
	handle("GET /competitions/admin/{token}/timetable/delay", p.delay)
	handle("POST /competitions/admin/{token}/timetable/delay", p.delay)
	handle("GET /competitions/admin/{token}/timetable/leave", p.leave)
	handle("POST /competitions/admin/{token}/timetable/leave", p.leave)
	handle("POST /competitions/admin/{token}/timetable/simulate", p.simulate)
	handle("POST /competitions/admin/{token}/timetable/simulate/delete", p.deleteScenario)
}

// live are a competition's entries that count: not withdrawn.
func live(entries []store.Entry) []store.Entry {
	var out []store.Entry
	for _, e := range entries {
		if !e.Withdrawn && e.Waiting == 0 {
			out = append(out, e)
		}
	}
	return out
}

// personKeys are each entry's people, by entry id, matched across entries:
// members by member, and individuals by person (ADR 0005 Decision 4, ADR
// 0010), their entries all keyed alike in a competition.
func personKeys(entries []store.Entry) map[string][]string {
	out := map[string][]string{}
	for _, e := range entries {
		out[e.ID] = e.People()
	}
	return out
}

// samePeople says which person keys are one person (ADR 0010: a member who
// also coaches, or also enters on their own), for working out clashes and
// rest: each key of someone with more than one in use maps to the one that
// stands for them all. Saved keys stay as they are; only the working out
// uses it.
type samePeople map[string]string

// of is the key standing for a key's person.
func (sp samePeople) of(key string) string {
	if k, ok := sp[key]; ok {
		return k
	}
	return key
}

// all are keys as their people's.
func (sp samePeople) all(keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = sp.of(k)
	}
	return out
}

// newSamePeople works out who's the same person among the keys in use, from
// groups (key to person, store.PersonGroups): the entries' gymnasts and
// coaches, and the officials. Someone who officiates is stood for by their
// official's key, which the rota's seats are saved under, so their seats and
// their entries are seen as one person's; anyone else by their first key, a
// member's before an individual's before a coach's.
func newSamePeople(groups map[string]string, entries []store.Entry, officials []competitions.RotaPerson) samePeople {
	official := map[string]bool{}
	var inUse []string
	for _, o := range officials {
		official[o.Key] = true
		inUse = append(inUse, o.Key)
	}
	for _, e := range entries {
		inUse = append(inUse, e.People()...)
		if k := e.CoachKey(); k != "" {
			inUse = append(inUse, k)
		}
	}
	rank := func(k string) int {
		if official[k] {
			return 0
		}
		kind, _ := store.KeyKind(k)
		return 1 + strings.Index(store.KindMember+store.KindIndividual+store.KindCoach, kind)
	}
	stand := map[string]string{} // person → the key standing for them
	for _, k := range inUse {
		person, ok := groups[k]
		if !ok {
			continue
		}
		if s, seen := stand[person]; !seen || rank(k) < rank(s) || rank(k) == rank(s) && k < s {
			stand[person] = k
		}
	}
	out := samePeople{}
	for _, k := range inUse {
		if person, ok := groups[k]; ok && stand[person] != k {
			out[k] = stand[person]
		}
	}
	return out
}

// clashPeople are each entry's people and their names, as peopleOf has them,
// keyed as their people (same) for working out clashes and rest, with names
// for those keys too.
func clashPeople(entries []store.Entry, same samePeople) (map[string][]string, map[string]string) {
	people, names := peopleOf(entries)
	for id, keys := range people {
		people[id] = same.all(keys)
	}
	for k, n := range names {
		if s := same.of(k); names[s] == "" {
			names[s] = n
		}
	}
	return people, names
}

// coachesOf are each entry's coaches, by entry id, keyed as their people
// (same).
func coachesOf(entries []store.Entry, same samePeople) map[string][]string {
	out := map[string][]string{}
	for _, e := range entries {
		if k := e.CoachKey(); k != "" {
			out[e.ID] = []string{same.of(k)}
		}
	}
	return out
}

// schedEntries are entries as the scheduler needs them, with their people
// and coaches keyed as their people (same).
func schedEntries(entries []store.Entry, same samePeople) []competitions.SchedEntry {
	keys := personKeys(entries)
	out := make([]competitions.SchedEntry, len(entries))
	for i, e := range entries {
		club := e.ClubName
		if e.Individual {
			club = ""
		}
		out[i] = competitions.SchedEntry{
			PlanEntry:  competitions.PlanEntry{ID: e.ID, Level: e.Entry.Event(), Category: e.Entry.Category, Club: club},
			Discipline: e.Entry.Discipline, People: same.all(keys[e.ID]),
		}
		if k := e.CoachKey(); k != "" {
			out[i].Coaches = []string{same.of(k)}
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
		if k := e.CoachKey(); k != "" {
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
	var no refused
	switch {
	case errors.As(err, &no):
		back(w, r, string(no))
	case err != nil:
		failed(w, r, err)
	default:
		back(w, r, notice)
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
				it := views.ItemView{Flight: true, Index: i, Name: f.Name(), Start: competitions.Clock(f.Start), End: competitions.Clock(f.End), Seats: seatsOf(f, people, names),
					Pairs: f.Discipline == competitions.Synchro, InRun: f.Of > 1, Earlier: f.Number > 1}
				for n, id := range f.Entries {
					e, ok := byID[id]
					if !ok {
						continue // withdrawn since
					}
					g := views.TimetableGymnast{ID: id, FlightGymnast: views.FlightGymnast{No: n + 1, Name: e.Entry.Gymnasts(), Club: clubOf(e)}, Category: e.Entry.Category}
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
	f := views.SetupForm{Rest: s.Rest, RestMust: s.RestMust, AcrossBreaks: s.AcrossBreaks, Events: c.EventNames()}
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
	people, same, err := p.rotaAndSame(r.Context(), c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	page := views.TimetablePage{
		Base: adminPath(r.PathValue("token")), Competition: summary(c.Competition, p.now()), Menu: p.menuFor(r, c, menuTimetable), Notice: r.URL.Query().Get("notice"),
		Setup: setupForm(c.Competition, s.Setup, people), Planned: s.Planned, Stale: s.Stale, Published: c.Published != nil, Changes: c.Published != nil && !sameTimetable(s, *c.Published), Entries: len(entries), Version: s.Version(),
	}
	if s.Planned {
		officials := people
		page.Rota = rotaView(s, entries, officials, same)
		people, names := clashPeople(entries, same)
		report := s.Report(people)
		page.Report = views.ReportView{Unplaced: report.Unplaced, Blocks: report.UnplacedBlocks, Broken: report.Broken, Problems: s.Problems(people, coachesOf(entries, same), names)}
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
					ID: e.ID, FlightGymnast: views.FlightGymnast{Name: e.Entry.Gymnasts(), Club: clubOf(e)}, Category: strings.TrimSpace(e.Entry.Event() + " " + e.Entry.Category),
				})
			}
		}
	}
	render(w, r, views.CompetitionTimetable(page))
}

// setupChange is what a change to the setup says once saved, and whether
// it's only to the rules about officials (rota): the flights then stand, and
// only the officials need assigning again.
type setupChange struct {
	notice string
	rota   bool
}

// saveSetup changes the setup with edit, checks it and saves it, marking the
// plan stale unless only the rules about officials changed. edit may refuse
// the change, or say more in the notice.
func (p *competitionPages) saveSetup(w http.ResponseWriter, r *http.Request, c store.Competition, notice string, edit func(*competitions.Schedule, *setupChange) error) {
	ch := setupChange{notice: notice}
	err := p.changeDraft(r, c, func(s *competitions.Schedule) error {
		if err := edit(s, &ch); err != nil {
			return err
		}
		if err := s.Setup.Check(c.EventNames()); err != nil {
			return refused("Nothing was changed: " + strings.Join(sentences(err), " "))
		}
		switch {
		case !ch.rota:
			s.Stale = s.Planned
		case s.Planned:
			ch.notice += " Assign officials again to use it."
		}
		return nil
	})
	changed(w, r, err, ch.notice)
}

// setupDays saves the days (day-<i>-name, -start, -end, -area), adds one
// (add=1) or removes one (remove=<i>).
func (p *competitionPages) setupDays(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	p.saveSetup(w, r, c, "Days saved.", func(s *competitions.Schedule, _ *setupChange) error {
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
		return nil
	})
}

// setupAreas saves the areas (area-<i>-name, -discipline), adds one (add=1)
// or removes one (remove=<i>). Days, blocks and rules follow a renamed area.
func (p *competitionPages) setupAreas(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	p.saveSetup(w, r, c, "Areas saved.", func(s *competitions.Schedule, _ *setupChange) error {
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
		return nil
	})
}

// setupTimings saves each discipline's timings (per-, between-, max-<key>),
// rest (rest, restMust) and separate men's and women's flights.
func (p *competitionPages) setupTimings(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	timings := map[string]competitions.Timings{}
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
			timings[d] = t
		}
	}
	rest, err := strconv.Atoi(r.FormValue("rest"))
	if err != nil {
		back(w, r, "Nothing was changed: rest should be a number of minutes.")
		return
	}
	p.saveSetup(w, r, c, "Timings saved.", func(s *competitions.Schedule, _ *setupChange) error {
		s.Setup.Timings = timings
		s.Setup.Rest, s.Setup.RestMust = rest, r.FormValue("restMust") == "1"
		if r.Form.Has("separate") {
			s.Setup.Separate = postedSplit(r, "separate", "separateLevel")
		}
		return nil
	})
}

// setupBlocks adds blocked time (add=1: name, minutes, day, at or from and
// to, area), removes some (remove=<i>), or says whether events can run
// across breaks (breaks=1, across=1 if they can).
func (p *competitionPages) setupBlocks(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	if r.Form.Has("breaks") {
		across := r.FormValue("across") == "1"
		notice := "Saved: each event is all before a break or all after it."
		if across {
			notice = "Saved: events can run across breaks."
		}
		p.saveSetup(w, r, c, notice, func(s *competitions.Schedule, _ *setupChange) error {
			s.Setup.AcrossBreaks = across
			return nil
		})
		return
	}
	if i, err := strconv.Atoi(r.FormValue("remove")); err == nil {
		p.saveSetup(w, r, c, "Blocked time removed.", func(s *competitions.Schedule, _ *setupChange) error {
			if i < 0 || i >= len(s.Setup.Blocks) {
				return staleTimetable
			}
			s.Setup.Blocks = slices.Delete(s.Setup.Blocks, i, i+1)
			return nil
		})
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
	p.saveSetup(w, r, c, "", func(s *competitions.Schedule, ch *setupChange) error {
		s.Setup.Blocks = append(s.Setup.Blocks, b)
		ch.notice = "Blocked time added: " + describeBlock(b, s.Setup.Days) + "."
		return nil
	})
}

// setupRules adds a rule (add=1: kind, must, event, and the area, day or
// other event it needs) or removes one (remove=<i>).
func (p *competitionPages) setupRules(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	if i, err := strconv.Atoi(r.FormValue("remove")); err == nil {
		p.saveSetup(w, r, c, "Rule removed.", func(s *competitions.Schedule, ch *setupChange) error {
			if i < 0 || i >= len(s.Setup.Rules) {
				return staleTimetable
			}
			ch.rota = s.Setup.Rules[i].Person != ""
			s.Setup.Rules = slices.Delete(s.Setup.Rules, i, i+1)
			return nil
		})
		return
	}
	if kind := r.FormValue("kind"); kind == competitions.RuleRole || kind == competitions.RuleOff || kind == competitions.RuleHours {
		entries, err := p.st.Entries(r.Context(), c.ID)
		if err != nil {
			failed(w, r, err)
			return
		}
		people, err := p.rotaOf(r, c, live(entries))
		if err != nil {
			failed(w, r, err)
			return
		}
		rule, ok := personRule(r, people)
		if !ok {
			back(w, r, "Choose someone who's offered to officiate.")
			return
		}
		p.saveSetup(w, r, c, "", func(s *competitions.Schedule, ch *setupChange) error {
			ch.rota = true
			s.Setup.Rules = append(s.Setup.Rules, rule)
			ch.notice = "Rule added: " + rule.Describe(s.Setup.Days) + "."
			return nil
		})
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
	p.saveSetup(w, r, c, "", func(s *competitions.Schedule, ch *setupChange) error {
		s.Setup.Rules = append(s.Setup.Rules, rule)
		ch.notice = "Rule added: " + rule.Describe(s.Setup.Days) + "."
		return nil
	})
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
	notice := fmt.Sprintf("Planned %s.", gymnastsWord(len(entries)))
	err = p.changeDraft(r, c, func(s *competitions.Schedule) error {
		if err := s.Setup.Check(c.EventNames()); err != nil {
			return refused("Fix the setup first: " + strings.Join(sentences(err), " "))
		}
		*s = competitions.PlanStaffed(schedEntries(entries, same), c.EventNames(), s.Setup, staffing(c.Competition, people), uint64(p.now().UnixNano()))
		p.staff(s, c, entries, people, same)
		if n := len(s.Unplaced) + len(s.UnplacedBlocks); n > 0 {
			notice += " Some of it doesn't fit: see below."
		}
		return nil
	})
	changed(w, r, err, notice)
}

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
	c, _, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	flight, _ := strconv.Atoi(r.FormValue("flight"))
	_, same, err := p.rotaAndSame(r.Context(), c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	err = p.changeDraft(r, c, func(s *competitions.Schedule) error {
		done := false
		switch r.FormValue("action") {
		case "redraw":
			clubs := map[string]string{}
			for _, e := range schedEntries(entries, same) {
				clubs[e.ID] = e.Club
			}
			people, _ := clashPeople(entries, same)
			done = s.Redraw(flight, clubs, people, rand.New(rand.NewPCG(uint64(p.now().UnixNano()), 1)))
		case "move":
			day, area, _ := strings.Cut(r.FormValue("to"), ":")
			d, err := strconv.Atoi(day)
			done = err == nil && s.MoveFlight(flight, d, area)
		case "earlier":
			done = s.Earlier(flight)
		}
		if !done {
			return cantGo
		}
		return nil
	})
	changed(w, r, err, "")
}

// moveEntry moves a gymnast to a flight (to: its index), or out of the
// timetable (to: "").
func (p *competitionPages) moveEntry(w http.ResponseWriter, r *http.Request) {
	c, _, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	id := r.FormValue("entry")
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
	err := p.changeDraft(r, c, func(s *competitions.Schedule) error {
		if to == "" {
			s.RemoveEntry(id)
			s.Retime()
			return nil
		}
		flight, err := strconv.Atoi(to)
		if err != nil || !s.MoveEntry(id, flight) {
			return refused("That flight has moved since; nothing was changed.")
		}
		return nil
	})
	changed(w, r, err, "")
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

// printTimetable prints the marshal sheets (sheet=marshal) or the chair of
// judges sheets (sheet=judges), one area's day to a page, each flight with its
// panel; the recorders' score sheets (sheet=scores), a flight to a page; the officials rota (sheet=rota), each person's duties; or the panel
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
	scratched, err := p.scratchedOf(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	if r.URL.Query().Get("sheet") == "scores" {
		render(w, r, views.ScorePrint(scoreSheets(c, s, entries, people, scratched, timetablePath(r), p.now(), "")))
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
	var arrived map[string]map[string]store.Checkin // panel check-ins, ticked on the chair of judges sheets
	if judges {
		if arrived, err = p.st.OfficialCheckins(r.Context(), c.ID); err != nil {
			failed(w, r, err)
			return
		}
	}
	for _, day := range itemsOf(s, entries, people) {
		for _, area := range day.Areas {
			sheet := views.SheetView{Day: day.Name, Area: area.Name}
			for _, it := range area.Items {
				if it.Flight {
					for i := range it.Seats {
						it.Seats[i].Here = arrived[competitions.FlightKey(s.Flights[it.Index])][it.Seats[i].Person].Status == store.OfficialHere
					}
					for i := range it.Gymnasts {
						it.Gymnasts[i].Scratched = scratched[it.Gymnasts[i].ID]
					}
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
				if !j.card.Does(i) {
					continue
				}
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

// scoreSheets are the recorders' score sheets: each flight in time order,
// its gymnasts in running order, and a column for each mark its panel gives
// each routine, all left to fill in.
// With a person's key, only the flights they have a seat on.
// Scratched gymnasts (entry ids) are struck through.
func scoreSheets(c store.Competition, s competitions.Schedule, entries []store.Entry, people []competitions.RotaPerson, scratched map[string]bool, back string, now time.Time, key string) views.ScoreSheets {
	out := views.ScoreSheets{Title: "Score sheets · " + c.Name, Back: back, BackLabel: "← Timetable", Competition: summary(c.Competition, now)}
	for _, day := range itemsOf(s, entries, people) {
		for _, area := range day.Areas {
			for _, it := range area.Items {
				if !it.Flight || key != "" && !slices.ContainsFunc(s.Flights[it.Index].Officials, func(d competitions.Duty) bool { return d.Person == key }) {
					continue
				}
				f := views.ScoreFlight{Name: it.Name, Day: day.Name, Area: area.Name, Start: it.Start, Seats: it.Seats,
					Marks: scoreMarks(c.Officials.Panel(s.Flights[it.Index].Discipline))}
				for _, g := range it.Gymnasts {
					f.Rows = append(f.Rows, views.FlightGymnast{Name: g.Name, No: g.No, Club: g.Club, Scratched: scratched[g.ID]})
				}
				out.Flights = append(out.Flights, f)
			}
		}
	}
	return out
}

// scoreMarks are the columns a panel gives each routine: each execution
// judge's, difficulty, each HD and synchronisation judge's, any penalty, and
// the routine's total.
func scoreMarks(p competitions.Panel) []string {
	var out []string
	numbered := func(label string, n int) {
		for i := range n {
			if n == 1 {
				out = append(out, label)
			} else {
				out = append(out, label+strconv.Itoa(i+1))
			}
		}
	}
	numbered("E", p.Execution)
	if p.Difficulty > 0 {
		out = append(out, "D")
	}
	numbered("HD", p.HD)
	numbered("S", p.Sync)
	return append(out, "Pen", "Total")
}
