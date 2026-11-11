package web

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The timetable's setup: its days, areas, timings, blocked time and rules,
// and the forms that change them.

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
	minutes := func(m float64) string { return strconv.FormatFloat(m, 'f', -1, 64) }
	n := 0 // the events with times of their own so far, numbering their fields
	for _, d := range c.Disciplines() {
		t := s.TimingsFor(d)
		tf := views.TimingForm{
			Key: offerKey(d), Name: competitions.DisciplineName(d), WarmUp: minutes(t.WarmUp), LeastWarmUp: strconv.Itoa(t.LeastWarmUp),
			PerCompetitor: minutes(t.PerCompetitor), Between: strconv.Itoa(t.Between), MaxFlight: strconv.Itoa(t.MaxFlight),
		}
		for _, l := range c.LevelNames(d) {
			ev := competitions.EventName(d, l)
			e, own := s.EventTimes[ev]
			if !own {
				tf.Choices = append(tf.Choices, ev)
				continue
			}
			tf.Events = append(tf.Events, views.EventTimingForm{Index: n, Name: ev, WarmUp: minutes(e.WarmUp), LeastWarmUp: strconv.Itoa(e.LeastWarmUp), PerCompetitor: minutes(e.PerCompetitor)})
			n++
		}
		f.Timings = append(f.Timings, tf)
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
		// Earlier (up: its index): the order the timetable, On the day and
		// the venue screen show the areas in.
		if i, err := strconv.Atoi(r.FormValue("up")); err == nil && i > 0 && i < len(s.Setup.Areas) {
			s.Setup.Areas[i-1], s.Setup.Areas[i] = s.Setup.Areas[i], s.Setup.Areas[i-1]
		}
		return nil
	})
}

// setupTimings saves each discipline's timings (warm-, least-, per-,
// between-, max-<key>; a form without warm- and least- leaves those as they
// are), events' own times (event-<i> its name, with -warm, -least, -per),
// rest (rest, restMust) and separate men's and women's flights. addEvent=<key>
// gives the discipline's event chosen (add-<key>) times of its own, starting
// from the discipline's; removeEvent=<event> puts an event back on its
// discipline's. Before planning, that's all. After, the flights are timed
// again in place (tryTimes), unless the largest flight or separate men's and
// women's flights changed: those need the flights made again, so the
// timetable is marked out of date, as any other setup change marks it.
func (p *competitionPages) setupTimings(w http.ResponseWriter, r *http.Request) {
	c, draft, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	posted := map[string]competitions.Timings{} // the figures posted, by discipline
	warmPosted := map[string]bool{}             // disciplines whose warm-up figures were posted
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
		if r.Form.Has("warm-"+key) || r.Form.Has("least-"+key) {
			var err4, err5 error
			t.WarmUp, err4 = strconv.ParseFloat(orZero(r.FormValue("warm-"+key)), 64)
			t.LeastWarmUp, err5 = strconv.Atoi(orZero(r.FormValue("least-" + key)))
			if err4 != nil || err5 != nil {
				back(w, r, "Nothing was changed: timings should be numbers.")
				return
			}
			warmPosted[d] = true
		}
		posted[d] = t
	}
	events := map[string]competitions.EventTimes{} // events' own times posted, by event
	for i := 0; r.Form.Has(fmt.Sprintf("event-%d", i)); i++ {
		field := func(part string) string { return orZero(r.FormValue(fmt.Sprintf("event-%d-%s", i, part))) }
		warm, err1 := strconv.ParseFloat(field("warm"), 64)
		least, err2 := strconv.Atoi(field("least"))
		per, err3 := strconv.ParseFloat(r.FormValue(fmt.Sprintf("event-%d-per", i)), 64)
		if err1 != nil || err2 != nil || err3 != nil {
			back(w, r, "Nothing was changed: timings should be numbers.")
			return
		}
		events[r.FormValue(fmt.Sprintf("event-%d", i))] = competitions.EventTimes{WarmUp: warm, LeastWarmUp: least, PerCompetitor: per}
	}
	rest, err := strconv.Atoi(r.FormValue("rest"))
	if err != nil {
		back(w, r, "Nothing was changed: rest should be a number of minutes.")
		return
	}
	notice := "Timings saved."
	removed, added := r.FormValue("removeEvent"), ""
	if key := r.FormValue("addEvent"); key != "" {
		added = r.FormValue("add-" + key)
	}
	switch {
	case added != "":
		notice = added + " has times of its own: change them on its row."
	case removed != "":
		notice = removed + " is back on its discipline's times."
	}
	// apply makes the posted timings the setup's; the figures not on the
	// form stay as they are.
	apply := func(s *competitions.Setup) error {
		timings := map[string]competitions.Timings{}
		for d, pt := range posted {
			t := s.TimingsFor(d)
			t.PerCompetitor, t.Between, t.MaxFlight = pt.PerCompetitor, pt.Between, pt.MaxFlight
			if warmPosted[d] {
				t.WarmUp, t.LeastWarmUp = pt.WarmUp, pt.LeastWarmUp
			}
			if t != competitions.DefaultTimings(d) {
				timings[d] = t
			}
		}
		own := maps.Clone(s.EventTimes)
		if own == nil {
			own = map[string]competitions.EventTimes{}
		}
		maps.Copy(own, events)
		delete(own, removed)
		if added != "" {
			if _, has := own[added]; has || !slices.Contains(c.EventNames(), added) {
				return staleTimetable
			}
			// Its discipline's figures, as posted, to change from.
			for _, d := range c.Disciplines() {
				if slices.ContainsFunc(c.LevelNames(d), func(l string) bool { return competitions.EventName(d, l) == added }) {
					t, set := timings[d]
					if !set {
						t = competitions.DefaultTimings(d)
					}
					own[added] = competitions.EventTimes{WarmUp: t.WarmUp, LeastWarmUp: t.LeastWarmUp, PerCompetitor: t.PerCompetitor}
				}
			}
		}
		if len(own) == 0 {
			own = nil
		}
		s.Timings, s.EventTimes = timings, own
		s.Rest, s.RestMust = rest, r.FormValue("restMust") == "1"
		if r.Form.Has("separate") {
			s.Separate = postedSplit(r, "separate", "separateLevel")
		}
		return nil
	}
	// Before planning, or when the flights need making again, as any other
	// change to the setup.
	next := draft.Setup
	if err := apply(&next); err != nil {
		changed(w, r, err, "")
		return
	}
	reform := next.Separate.Mode != draft.Setup.Separate.Mode || !slices.Equal(next.Separate.Levels, draft.Setup.Separate.Levels)
	for _, d := range c.Disciplines() {
		reform = reform || next.TimingsFor(d).MaxFlight != draft.Setup.TimingsFor(d).MaxFlight
	}
	if !draft.Planned || reform {
		if draft.Planned {
			notice += " The flights need making again for it: plan again to use it."
		}
		p.saveSetup(w, r, c, notice, func(s *competitions.Schedule, _ *setupChange) error { return apply(&s.Setup) })
		return
	}
	marks, err := p.dayMarksOf(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	p.tryTimes(w, r, c, draft, entries, marks, timesChange{
		what: "Change the timings", menu: menuSetup,
		change: func(s *competitions.Schedule, fixed func(string) bool) ([]competitions.FlightShift, error) {
			next := s.Setup
			if err := apply(&next); err != nil {
				return nil, err
			}
			if err := next.Check(c.EventNames()); err != nil {
				return nil, err
			}
			s.Setup = next
			return s.SetTimes(next.Timings, next.EventTimes, fixed), nil
		},
		done: notice, back: timetablePath(r) + "/setup",
	})
}

// orZero is a number from a form, "" being 0.
func orZero(v string) string {
	if strings.TrimSpace(v) == "" {
		return "0"
	}
	return v
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
		entries, err := p.entriesOf(r.Context(), c.ID)
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
