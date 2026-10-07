package competitions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Simulation (ADR 0005 Decision 11): before entries exist, or alongside them,
// the organiser says how many will enter each event, how many gymnasts enter
// two disciplines, and how many can judge and help. Stand-in people are made
// up from those numbers and planned with the real venue setup and panels, as
// the real timetable would be, without touching it. Each scenario keeps its
// result, so several can be compared.

// Scenario is one simulation's numbers and, once run, its result.
type Scenario struct {
	Name    string         `json:"name"`
	Entries map[string]int `json:"entries"` // by event; a synchro entry is a pair
	// Gymnasts is how many gymnasts make the entries: fewer than the
	// entries' gymnasts together means some enter several disciplines. None
	// means no one does.
	Gymnasts int            `json:"gymnasts,omitempty"`
	Clubs    int            `json:"clubs"`            // the clubs gymnasts and judges come from
	Judges   map[string]int `json:"judges,omitempty"` // by discipline: people who can judge it, any level
	Chairs   map[string]int `json:"chairs,omitempty"` // of them, who can also chair
	// JudgePeople is how many people those judges are: someone judging
	// trampoline and synchro counts once. None means no one judges two.
	JudgePeople int `json:"judge_people,omitempty"`
	// Competing is how many of the judges are also competing: they can't
	// judge while they compete.
	Competing int        `json:"competing,omitempty"`
	Helpers   int        `json:"helpers,omitempty"` // people who can record or marshal
	Result    *SimResult `json:"result,omitempty"`
}

// SimResult is what a scenario's plan came to.
type SimResult struct {
	Basis          string      `json:"basis"` // the setup, panels and events it was planned with (SimBasis)
	Gymnasts       int         `json:"gymnasts"`
	Entries        int         `json:"entries"`
	Days           []DayReport `json:"days"`
	FlightsEnd     []string    `json:"flights_end"` // by day, when its last flight ends ("" for none): blocks such as awards can end later
	Unplaced       []string    `json:"unplaced,omitempty"`
	UnplacedBlocks []string    `json:"unplaced_blocks,omitempty"`
	ShortRest      int         `json:"short_rest,omitempty"` // people with less rest than wanted between turns
	Seats          int         `json:"seats"`                // every official's seat
	Empty          int         `json:"empty,omitempty"`      // seats no one could take
	Fix            string      `json:"fix,omitempty"`        // a change that would fit everything, if anything doesn't fit
	NoFix          bool        `json:"no_fix,omitempty"`     // nothing tried fits everything
}

// Most a scenario can have, so a simulation stays quick.
const (
	maxSimEntries = 1000
	maxSimPeople  = 500
)

// MaxScenarios is how many scenarios a competition keeps; the oldest goes.
const MaxScenarios = 8

// eventDisciplines are the competition's events, in order, and each one's
// discipline.
func (c Competition) eventDisciplines() ([]string, map[string]string) {
	var events []string
	of := map[string]string{}
	for _, d := range c.Disciplines() {
		for _, l := range c.levelNames(d) {
			ev := EventName(d, l)
			events = append(events, ev)
			of[ev] = d
		}
	}
	return events, of
}

// CheckScenario reports what's wrong with a scenario's numbers.
func (c Competition) CheckScenario(sc Scenario) error {
	var errs []error
	if err := CheckName("scenario", sc.Name); err != nil {
		errs = append(errs, err)
	}
	events, _ := c.eventDisciplines()
	total := 0
	for ev, n := range sc.Entries {
		switch {
		case !slices.Contains(events, ev):
			errs = append(errs, fmt.Errorf("the competition has no event %q", ev))
		case n < 0:
			errs = append(errs, fmt.Errorf("%s: entries can't be fewer than none", ev))
		}
		total += n
	}
	switch {
	case total == 0:
		errs = append(errs, errors.New("a scenario needs some entries"))
	case total > maxSimEntries:
		errs = append(errs, fmt.Errorf("a scenario can have up to %d entries", maxSimEntries))
	}
	if most, all := c.simSlots(sc); sc.Gymnasts != 0 && (sc.Gymnasts < most || sc.Gymnasts > all) {
		errs = append(errs, fmt.Errorf("the gymnasts should be from %d (the most in one discipline) to %d (no one in two)", most, all))
	}
	if sc.Clubs < 1 || sc.Clubs > 100 {
		errs = append(errs, errors.New("clubs should be from 1 to 100"))
	}
	slots, most := 0, 0
	for d, n := range sc.Judges {
		if !slices.Contains(c.Disciplines(), d) {
			errs = append(errs, fmt.Errorf("the competition doesn't offer %s", DisciplineName(d)))
		}
		if n < 0 || sc.Chairs[d] < 0 || sc.Chairs[d] > n {
			errs = append(errs, fmt.Errorf("%s: judges can't be fewer than none, nor chairs more than judges", DisciplineName(d)))
		}
		slots += n
		most = max(most, n)
	}
	if sc.JudgePeople != 0 && (sc.JudgePeople < most || sc.JudgePeople > slots) {
		errs = append(errs, fmt.Errorf("the people who judge should be from %d (the most for one discipline) to %d (no one judging two)", most, slots))
	}
	people := sc.Helpers + sc.judgePeople()
	for d := range sc.Chairs {
		if _, ok := sc.Judges[d]; !ok {
			errs = append(errs, fmt.Errorf("%s: chairs are counted among its judges", DisciplineName(d)))
		}
	}
	if sc.Helpers < 0 || sc.Competing < 0 || sc.Competing > sc.judgePeople() {
		errs = append(errs, errors.New("helpers can't be fewer than none, nor competing judges more than the judges"))
	}
	if people > maxSimPeople {
		errs = append(errs, fmt.Errorf("a scenario can have up to %d judges and helpers", maxSimPeople))
	}
	return errors.Join(errs...)
}

// SimBasis fingerprints what a plan depends on besides the entries and
// officials: the venue setup, the panels and judging rule, and the events. A
// scenario planned on another basis is out of date.
func (c Competition) SimBasis(setup Setup) string {
	events, _ := c.eventDisciplines()
	data, _ := json.Marshal(struct {
		Setup     Setup
		Officials OfficialSettings
		Events    []string
	}{setup, c.Officials, events})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// simSlots are the most gymnasts a scenario's entries need in one discipline,
// and in all, counting both of each synchro pair.
func (c Competition) simSlots(sc Scenario) (most, all int) {
	_, of := c.eventDisciplines()
	in := map[string]int{}
	for ev, n := range sc.Entries {
		if of[ev] == Synchro {
			n *= 2
		}
		in[of[ev]] += max(n, 0)
		all += max(n, 0)
	}
	for _, n := range in {
		most = max(most, n)
	}
	return most, all
}

// standIns are a scenario's made-up entries and officials.
func (c Competition) standIns(sc Scenario, setup Setup) ([]SchedEntry, []RotaPerson) {
	events, of := c.eventDisciplines()
	club := func(i int) string { return fmt.Sprintf("Club %d", i%sc.Clubs+1) }
	var entries []SchedEntry
	type slot struct{ entry, person int }
	slots := map[string][]slot{} // each entry's gymnasts, by discipline
	for _, ev := range events {
		for i := range sc.Entries[ev] {
			d := of[ev]
			e := SchedEntry{PlanEntry: PlanEntry{ID: fmt.Sprintf("sim:e%d", len(entries)+1), Level: ev}, Discipline: d, People: []string{""}}
			if setup.Separate.Splits(ev) {
				e.Category = Categories[i%len(Categories)]
			}
			if d == Synchro {
				e.People = append(e.People, "")
			}
			for k := range e.People {
				slots[d] = append(slots[d], slot{len(entries), k})
			}
			entries = append(entries, e)
		}
	}

	// The gymnasts: each takes a place in the biggest discipline first, then
	// the next; once there's one for every gymnast, the rest go to gymnasts
	// already entered, in turn, but never twice in one discipline.
	ds := slices.Clone(AllDisciplines)
	slices.SortStableFunc(ds, func(a, b string) int { return len(slots[b]) - len(slots[a]) })
	most, all := c.simSlots(sc)
	gymnasts := all
	if sc.Gymnasts > 0 {
		gymnasts = min(max(sc.Gymnasts, most), all)
	}
	clubOf := map[string]string{}
	k, next := 0, 0
	for _, d := range ds {
		in := map[int]bool{}
		for _, sl := range slots[d] {
			g := k
			if k >= gymnasts {
				for in[next%gymnasts] {
					next++
				}
				g = next % gymnasts
				next++
			}
			k++
			in[g] = true
			key := fmt.Sprintf("sim:g%d", g+1)
			clubOf[key] = club(g)
			entries[sl.entry].People[sl.person] = key
		}
	}
	for i := range entries {
		entries[i].Club = clubOf[entries[i].People[0]]
	}

	// Officials: a pool of judges, and helpers.
	var competitors []string
	seen := map[string]bool{}
	for _, e := range entries {
		for _, p := range e.People {
			if !seen[p] {
				seen[p] = true
				competitors = append(competitors, p)
			}
		}
	}
	// Judge i judges a run of disciplines: each discipline takes the next
	// Judges[d] people round the pool, so they overlap only as much as the
	// pool is smaller than the disciplines' judges together.
	judges := sc.judgePeople()
	officials := make([]RotaPerson, judges)
	for j := range officials {
		officials[j] = RotaPerson{Key: fmt.Sprintf("sim:j%d", j+1), Name: fmt.Sprintf("Judge %d", j+1), Club: club(j), Judge: map[string]bool{}, Chair: map[string]bool{}}
	}
	chairs := map[int]bool{}
	at := 0
	for _, d := range AllDisciplines {
		for i := range sc.Judges[d] {
			j := (at + i) % judges
			for _, ev := range events {
				if of[ev] == d {
					officials[j].Judge[ev] = true
					officials[j].Chair[ev] = officials[j].Chair[ev] || i < sc.Chairs[d]
				}
			}
			chairs[j] = chairs[j] || i < sc.Chairs[d]
		}
		at += sc.Judges[d]
	}
	// The competing judges are gymnasts, spread across the entries, and
	// spread across the judges, those who don't chair first.
	var others, chairing []int
	for j := range judges {
		if chairs[j] {
			chairing = append(chairing, j)
		} else {
			others = append(others, j)
		}
	}
	slices.Reverse(chairing)
	pool := append(others, chairing...) // every judge who doesn't chair, then chairs from the last
	competing := min(sc.Competing, len(competitors), judges)
	for k := range competing {
		j := pool[k]
		if competing <= len(others) {
			j = others[k*len(others)/competing]
		}
		g := competitors[k*len(competitors)/competing]
		officials[j].Key, officials[j].Club = g, clubOf[g]
	}
	for i := range sc.Helpers {
		officials = append(officials, RotaPerson{Key: fmt.Sprintf("sim:h%d", i+1), Name: fmt.Sprintf("Helper %d", i+1), Club: club(judges + i), Recorder: true, Marshal: true})
	}
	return entries, officials
}

// Simulate plans a scenario's stand-ins with the setup, as the real timetable
// is planned (PlanStaffed, then the rota), and reports what it came to.
func (c Competition) Simulate(sc Scenario, setup Setup, seed uint64) (SimResult, error) {
	if err := c.CheckScenario(sc); err != nil {
		return SimResult{}, err
	}
	events, _ := c.eventDisciplines()
	if err := setup.Check(events); err != nil {
		return SimResult{}, err
	}
	entries, officials := c.standIns(sc, setup)
	staff := &Staffing{Judges: map[string][]string{}, Need: map[string]int{}}
	for _, d := range c.Disciplines() {
		staff.Need[d] = c.Officials.Panel(d).Judges()
	}
	for _, o := range officials {
		for ev, ok := range o.Judge {
			if ok {
				staff.Judges[ev] = append(staff.Judges[ev], o.Key)
			}
		}
	}
	s := PlanStaffed(entries, events, setup, staff, seed)
	s.Rota(entries, officials, c.Officials, seed)

	people := map[string][]string{}
	gymnasts := map[string]bool{}
	for _, e := range entries {
		people[e.ID] = e.People
		for _, p := range e.People {
			gymnasts[p] = true
		}
	}
	report := s.Report(people)
	r := SimResult{
		Basis: c.SimBasis(setup), Gymnasts: len(gymnasts), Entries: len(entries), Days: report.Days,
		Unplaced: report.Unplaced, UnplacedBlocks: report.UnplacedBlocks,
	}
	r.FlightsEnd = make([]string, len(setup.Days))
	for _, f := range s.Flights {
		if end := Clock(f.End); end > r.FlightsEnd[f.Day] {
			r.FlightsEnd[f.Day] = end
		}
	}
	short := map[string]bool{}
	for _, sr := range report.ShortRest {
		short[sr.Person] = true
	}
	r.ShortRest = len(short)
	for _, f := range s.Staffed() {
		for _, d := range f.Officials {
			r.Seats++
			if d.Person == "" {
				r.Empty++
			}
		}
	}
	if len(r.Unplaced)+len(r.UnplacedBlocks) > 0 {
		r.NoFix = true
		for _, f := range Fixes(entries, events, setup, seed) {
			if f.Fits {
				r.Fix, r.NoFix = f.Change, false
				break
			}
		}
	}
	return r, nil
}

// judgePeople is how many people the scenario's judges are.
func (sc Scenario) judgePeople() int {
	if sc.JudgePeople > 0 {
		return sc.JudgePeople
	}
	n := 0
	for _, j := range sc.Judges {
		n += j
	}
	return n
}

// SimName is a name for a new scenario, "Scenario 3", not one already used.
func SimName(scenarios []Scenario) string {
	for n := len(scenarios) + 1; ; n++ {
		name := fmt.Sprintf("Scenario %d", n)
		if !slices.ContainsFunc(scenarios, func(s Scenario) bool { return strings.EqualFold(s.Name, name) }) {
			return name
		}
	}
}
