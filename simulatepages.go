package main

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Simulation (ADR 0005 Decision 11): the organiser tries entry numbers
// against the venue setup before entries arrive, or alongside them, and
// compares scenarios. The real timetable isn't touched.

func simulatePath(r *http.Request) string { return timetablePath(r) + "/simulate" }

func backToSimulation(w http.ResponseWriter, r *http.Request, notice string) {
	http.Redirect(w, r, simulatePath(r)+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

// actualScenario is the competition as it stands: its entries per event, how
// many gymnasts make them, its clubs, and who has offered to judge and help.
func actualScenario(c competitions.Competition, entries []store.Entry, people []competitions.RotaPerson) competitions.Scenario {
	sc := competitions.Scenario{Entries: map[string]int{}, Judges: map[string]int{}, Chairs: map[string]int{}}
	clubs := map[string]bool{}
	in := map[string]map[string]bool{} // person → disciplines
	keys := personKeys(entries)
	for _, e := range entries {
		sc.Entries[e.Entry.Event()]++
		if !e.Individual {
			clubs[e.ClubName] = true
		}
		for _, k := range keys[e.ID] {
			if in[k] == nil {
				in[k] = map[string]bool{}
			}
			in[k][e.Entry.Discipline] = true
		}
	}
	sc.Gymnasts = len(in)
	sc.Clubs = len(clubs)
	if sc.Clubs == 0 {
		sc.Clubs = 6 // a guess, until clubs enter
	}
	disciplineOf := map[string]string{}
	for _, d := range c.Disciplines() {
		for _, l := range c.LevelNames(d) {
			disciplineOf[competitions.EventName(d, l)] = d
		}
	}
	for _, o := range people {
		judges, chairs := map[string]bool{}, map[string]bool{}
		for ev, ok := range o.Judge {
			judges[disciplineOf[ev]] = judges[disciplineOf[ev]] || ok
		}
		for ev, ok := range o.Chair {
			chairs[disciplineOf[ev]] = chairs[disciplineOf[ev]] || ok
		}
		for d := range judges {
			sc.Judges[d]++
			if chairs[d] {
				sc.Chairs[d]++
			}
		}
		switch {
		case len(judges) > 0:
			sc.JudgePeople++
			if in[o.Key] != nil {
				sc.Competing++
			}
		case o.Recorder || o.Marshal:
			sc.Helpers++
		}
	}
	return sc
}

// scenarioForm is a scenario's numbers as the form shows them.
func scenarioForm(c competitions.Competition, sc competitions.Scenario) views.ScenarioForm {
	f := views.ScenarioForm{Name: sc.Name, AlsoJudge: sc.AlsoJudge, Gymnasts: sc.Gymnasts, Clubs: sc.Clubs, JudgePeople: sc.JudgePeople, Competing: sc.Competing, Helpers: sc.Helpers}
	for i, ev := range c.EventNames() {
		f.Events = append(f.Events, views.SimEvent{Field: "entries-" + strconv.Itoa(i), Name: ev, Entries: sc.Entries[ev]})
	}
	for _, d := range c.Disciplines() {
		q := sc.Quota[d]
		f.Disciplines = append(f.Disciplines, views.SimDiscipline{Key: offerKey(d), Name: competitions.DisciplineName(d), Judges: sc.Judges[d], Chairs: sc.Chairs[d],
			QuotaPer: q.Per, QuotaJudges: q.Judges, QuotaChairs: q.Chairs})
	}
	return f
}

// postedScenario reads the scenario form: name, entries-<i> by event,
// judges-<discipline> and chairs-<discipline>, quota-per-<discipline>,
// quota-judges-<discipline> and quota-chairs-<discipline> (what each club
// must bring), alsoJudge (percent of the clubs' judges who can also judge
// other disciplines), judgePeople, gymnasts, clubs, competing and helpers. A blank number is none.
func postedScenario(r *http.Request, c competitions.Competition) (competitions.Scenario, error) {
	number := func(field string) (int, error) {
		v := strings.TrimSpace(r.FormValue(field))
		if v == "" {
			return 0, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("numbers should be whole numbers, not %q", v)
		}
		return n, nil
	}
	var errs []string
	read := func(field string) int {
		n, err := number(field)
		if err != nil {
			errs = append(errs, err.Error())
		}
		return n
	}
	sc := competitions.Scenario{Name: strings.TrimSpace(r.FormValue("name")), Entries: map[string]int{}, Judges: map[string]int{}, Chairs: map[string]int{}}
	for i, ev := range c.EventNames() {
		if n := read("entries-" + strconv.Itoa(i)); n != 0 {
			sc.Entries[ev] = n
		}
	}
	for _, d := range c.Disciplines() {
		key := offerKey(d)
		if n := read("judges-" + key); n != 0 {
			sc.Judges[d] = n
		}
		if n := read("chairs-" + key); n != 0 {
			sc.Chairs[d] = n
		}
		q := competitions.ClubQuota{Per: read("quota-per-" + key), Judges: read("quota-judges-" + key), Chairs: read("quota-chairs-" + key)}
		if q != (competitions.ClubQuota{}) {
			if sc.Quota == nil {
				sc.Quota = map[string]competitions.ClubQuota{}
			}
			sc.Quota[d] = q
		}
	}
	sc.Gymnasts, sc.Clubs, sc.Competing, sc.Helpers = read("gymnasts"), read("clubs"), read("competing"), read("helpers")
	sc.JudgePeople, sc.AlsoJudge = read("judgePeople"), read("alsoJudge")
	if len(errs) > 0 {
		return sc, fmt.Errorf("%s", errs[0])
	}
	return sc, nil
}

// scenarioView is a scenario's result as the comparison shows it.
func scenarioView(c competitions.Competition, setup competitions.Setup, i int, sc competitions.Scenario) views.ScenarioView {
	v := views.ScenarioView{Index: i, Name: sc.Name, GymnastsIn: sc.Gymnasts, Clubs: sc.Clubs, JudgePeople: sc.JudgePeople, Competing: sc.Competing, Helpers: sc.Helpers}
	for _, ev := range c.EventNames() {
		if n := sc.Entries[ev]; n > 0 {
			v.Entries = append(v.Entries, fmt.Sprintf("%s: %d", ev, n))
		}
	}
	for _, d := range competitions.AllDisciplines {
		if n, ok := sc.Judges[d]; ok && n > 0 {
			v.Judges = append(v.Judges, fmt.Sprintf("%s %d (%d can chair)", competitions.DisciplineName(d), n, sc.Chairs[d]))
		}
	}
	for _, d := range competitions.AllDisciplines {
		if q, ok := sc.Quota[d]; ok {
			v.Quota = append(v.Quota, fmt.Sprintf("%s %d per %d competitors (%d can chair)", competitions.DisciplineName(d), q.Judges, q.Per, q.Chairs))
		}
	}
	if len(sc.Quota) > 0 {
		v.AlsoJudge = sc.AlsoJudge
	}
	r := sc.Result
	if r == nil {
		return v
	}
	v.ClubJudges = r.ClubJudges
	v.Run = true
	v.Stale = r.Basis != c.SimBasis(setup)
	v.Gymnasts, v.EntryCount = r.Gymnasts, r.Entries
	for i, d := range r.Days {
		day := views.SimDay{DayFinish: views.DayFinish{Name: d.Name, Finish: d.Finish, End: d.End, Spare: d.SpareMinutes}}
		if i < len(r.FlightsEnd) {
			day.FlightsEnd = r.FlightsEnd[i]
		}
		v.Days = append(v.Days, day)
	}
	v.Unplaced = append(unplacedByEvent(r.Unplaced), r.UnplacedBlocks...)
	v.Fix, v.NoFix = r.Fix, r.NoFix
	v.Seats, v.Empty, v.ShortRest = r.Seats, r.Empty, r.ShortRest
	return v
}

// unplacedByEvent counts flights that don't fit by event (and men or women),
// e.g. "BUCS L3 Men: 4 flights", in the order they first appear.
func unplacedByEvent(flights []string) []string {
	var order []string
	count := map[string]int{}
	for _, f := range flights {
		event, _, _ := strings.Cut(f, " · flight ")
		if count[event] == 0 {
			order = append(order, event)
		}
		count[event]++
	}
	slices.Sort(order)
	out := make([]string, len(order))
	for i, ev := range order {
		out[i] = ev + ": 1 flight"
		if n := count[ev]; n > 1 {
			out[i] = fmt.Sprintf("%s: %d flights", ev, n)
		}
	}
	return out
}

// simulation is the simulation page: the scenario form (filled from the
// competition as it stands, or from=<i> a scenario to change) and the
// scenarios so far, side by side.
func (p *competitionPages) simulation(w http.ResponseWriter, r *http.Request) {
	c, s, entries, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	scenarios, err := p.st.Scenarios(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	people, err := p.rotaOf(r, c, entries)
	if err != nil {
		failed(w, r, err)
		return
	}
	start := actualScenario(c.Competition, entries, people)
	if i, err := strconv.Atoi(r.URL.Query().Get("from")); err == nil && i >= 0 && i < len(scenarios) {
		start = scenarios[i]
	}
	start.Name = competitions.SimName(scenarios)
	page := views.SimulationPage{
		Base: adminPath(r.PathValue("token")), Competition: summary(c.Competition, p.now()), Notice: r.URL.Query().Get("notice"),
		Form: scenarioForm(c.Competition, start), Entries: len(entries), Max: competitions.MaxScenarios,
	}
	if err := s.Setup.Check(c.EventNames()); err != nil {
		page.SetupProblems = sentences(err)
	}
	for i, sc := range scenarios {
		page.Scenarios = append(page.Scenarios, scenarioView(c.Competition, s.Setup, i, sc))
	}
	render(w, r, views.Simulation(page))
}

// simulate runs a scenario and keeps it (the oldest goes past the most kept),
// or runs one again with the current setup (again=<i>).
func (p *competitionPages) simulate(w http.ResponseWriter, r *http.Request) {
	c, s, _, ok := p.timetableOf(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	scenarios, err := p.st.Scenarios(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	again := -1
	var sc competitions.Scenario
	if v := r.FormValue("again"); v != "" {
		again, err = strconv.Atoi(v)
		if err != nil || again < 0 || again >= len(scenarios) {
			badRequest(w, fmt.Errorf("no scenario %q", v))
			return
		}
		sc = scenarios[again]
	} else if sc, err = postedScenario(r, c.Competition); err != nil {
		backToSimulation(w, r, "Nothing was simulated: "+err.Error()+".")
		return
	}
	if again < 0 && slices.ContainsFunc(scenarios, func(o competitions.Scenario) bool { return strings.EqualFold(o.Name, sc.Name) }) {
		backToSimulation(w, r, "Nothing was simulated: there's already a scenario called "+sc.Name+".")
		return
	}
	result, err := c.Simulate(sc, s.Setup, uint64(p.now().UnixNano()))
	if err != nil {
		backToSimulation(w, r, "Nothing was simulated: "+strings.Join(sentences(err), " "))
		return
	}
	sc.Result = &result
	if again >= 0 {
		scenarios[again] = sc
	} else {
		scenarios = append(scenarios, sc)
		if len(scenarios) > competitions.MaxScenarios {
			scenarios = scenarios[len(scenarios)-competitions.MaxScenarios:]
		}
	}
	if err := p.st.SetScenarios(r.Context(), c.ID, scenarios); err != nil {
		failed(w, r, err)
		return
	}
	notice := sc.Name + ": everything fits."
	if n := len(result.Unplaced) + len(result.UnplacedBlocks); n > 0 {
		notice = sc.Name + ": some of it doesn't fit."
	}
	backToSimulation(w, r, notice)
}

// deleteScenario removes a scenario (remove=<i>).
func (p *competitionPages) deleteScenario(w http.ResponseWriter, r *http.Request) {
	c, ok := p.admin(w, r)
	if !ok {
		return
	}
	scenarios, err := p.st.Scenarios(r.Context(), c.ID)
	if err != nil {
		failed(w, r, err)
		return
	}
	i, err := strconv.Atoi(r.FormValue("remove"))
	if err != nil || i < 0 || i >= len(scenarios) {
		badRequest(w, fmt.Errorf("no scenario %q", r.FormValue("remove")))
		return
	}
	name := scenarios[i].Name
	if err := p.st.SetScenarios(r.Context(), c.ID, slices.Delete(scenarios, i, i+1)); err != nil {
		failed(w, r, err)
		return
	}
	backToSimulation(w, r, name+" removed.")
}
