package competitions

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
	"time"
)

// The scheduler (ADR 0005 Decisions 6–10 and 13): a competition's flights,
// placed at times on the venue's named areas across its days, around blocked
// time, keeping the organiser's rules, never putting a person in two places
// at once, and finishing each day by its end. What can't be placed is
// reported, not forced.

// Area is one of the venue's named areas: a trampoline panel (which also
// runs synchro), a tumbling track or a DMT.
type Area struct {
	Name       string `json:"name"`
	Discipline string `json:"discipline"` // Trampoline, Tumbling or DMT
}

// runs says whether an area runs a discipline: synchro on trampolines.
func (a Area) runs(discipline string) bool {
	if discipline == Synchro {
		return a.Discipline == Trampoline
	}
	return a.Discipline == discipline
}

// Day is one of the competition's days, with its strict end.
type Day struct {
	Name  string   `json:"name"`            // e.g. "Friday"
	Start string   `json:"start"`           // "09:00"
	End   string   `json:"end"`             // "18:00": everything finishes by then
	Areas []string `json:"areas,omitempty"` // the areas in use; none for all
}

// uses says whether a day uses an area.
func (d Day) uses(area string) bool { return len(d.Areas) == 0 || slices.Contains(d.Areas, area) }

// Hours are when the day starts and ends, in minutes since midnight (0 for a
// time that can't be read; Setup.Check reports it).
func (d Day) Hours() (start, end int) {
	start, _ = clock(d.Start)
	end, _ = clock(d.End)
	return start, end
}

// Block is blocked time: lunch, awards, warm-ups, or an ad hoc event taking
// entries on the day. It's at a fixed time, or anywhere within a window.
type Block struct {
	Name    string   `json:"name"`
	Minutes int      `json:"minutes"`
	Day     int      `json:"day"`             // which day (0-based)
	Areas   []string `json:"areas,omitempty"` // the areas it takes; none for all
	At      string   `json:"at,omitempty"`    // a fixed start, "12:30"
	From    string   `json:"from,omitempty"`  // or a window: between From
	To      string   `json:"to,omitempty"`    // and To it starts and ends
	// Officials it needs, as a flight's panel does (an ad hoc event's judges,
	// say); none for lunch.
	Officials Panel `json:"officials,omitzero"`
}

// Rule kinds: an event on an area or day, an event before another, or two
// events never at the same time; and for officials, a person in a role at an
// event, a person not officiating (at an event, or at all), or a person
// officiating on a day only between two times.
const (
	RuleArea   = "area"
	RuleDay    = "day"
	RuleBefore = "before"
	RuleApart  = "apart"
	RuleRole   = "role"
	RuleOff    = "off"
	RuleHours  = "hours"
)

// personRule says whether a rule is about an official.
func personRule(kind string) bool { return kind == RuleRole || kind == RuleOff || kind == RuleHours }

// Rule is one of the organiser's rules, a must (never broken) or a prefer
// (a cost if broken).
type Rule struct {
	Kind   string `json:"kind"`
	Must   bool   `json:"must,omitempty"`
	Event  string `json:"event"`
	Event2 string `json:"event2,omitempty"` // RuleBefore, RuleApart
	Area   string `json:"area,omitempty"`   // RuleArea
	Day    int    `json:"day,omitempty"`    // RuleDay, RuleHours
	Person string `json:"person,omitempty"` // RuleRole, RuleOff, RuleHours: their key
	Name   string `json:"name,omitempty"`   // and their name
	Role   string `json:"role,omitempty"`   // RuleRole
	From   string `json:"from,omitempty"`   // RuleHours: "" for the day's start
	To     string `json:"to,omitempty"`     // RuleHours: "" for the day's end
}

// Describe says what a rule says, e.g. "Synchro BUCS L3 on Panel 2 (must)".
func (r Rule) Describe(days []Day) string {
	var out string
	day := fmt.Sprintf("day %d", r.Day+1)
	if r.Day >= 0 && r.Day < len(days) && days[r.Day].Name != "" {
		day = days[r.Day].Name
	}
	switch r.Kind {
	case RuleArea:
		out = r.Event + " on " + r.Area
	case RuleDay:
		out = r.Event + " on " + day
	case RuleRole:
		out = r.Name + " " + roleVerbs[r.Role] + " " + r.Event
	case RuleOff:
		out = r.Name + " doesn't officiate"
		if r.Event != "" {
			out += " at " + r.Event
		}
	case RuleHours:
		out = r.Name + " officiates on " + day + " only"
		switch {
		case r.From != "" && r.To != "":
			out += " between " + r.From + " and " + r.To
		case r.From != "":
			out += " from " + r.From
		default:
			out += " until " + r.To
		}
	case RuleBefore:
		out = r.Event + " before " + r.Event2
	case RuleApart:
		out = r.Event + " and " + r.Event2 + " not at the same time"
	}
	if r.Must {
		return out + " (must)"
	}
	return out + " (prefer)"
}

// Timings are a discipline's settings (Decision 13).
type Timings struct {
	PerCompetitor float64 `json:"per_competitor"` // minutes, all their turns (a pair, for synchro)
	Between       int     `json:"between"`        // minutes before each flight: warm-up and changeover
	MaxFlight     int     `json:"max_flight"`     // the most in a flight
}

// DefaultTimings are a discipline's defaults: 5 minutes per gymnast for
// trampoline's two rounds (British Gymnastics allows 4½–6½), 2½ per synchro
// pair, who do one routine, and 2 for tumbling's and DMT's two passes, until
// better figures are known.
func DefaultTimings(discipline string) Timings {
	switch discipline {
	case Tumbling, DMT:
		return Timings{PerCompetitor: 2, Between: 10, MaxFlight: 15}
	case Synchro:
		return Timings{PerCompetitor: 2.5, Between: 10, MaxFlight: 12}
	}
	return Timings{PerCompetitor: 5, Between: 10, MaxFlight: 12}
}

// Setup is everything the organiser plans with.
type Setup struct {
	Areas   []Area             `json:"areas"`
	Days    []Day              `json:"days"`
	Blocks  []Block            `json:"blocks,omitempty"`
	Rules   []Rule             `json:"rules,omitempty"`
	Timings map[string]Timings `json:"timings,omitempty"` // by discipline; missing: the defaults
	// Rest is the minutes wanted between a person's turns, so one running long
	// doesn't make them late for the next; RestMust makes it a must.
	Rest     int   `json:"rest,omitempty"`
	RestMust bool  `json:"rest_must,omitempty"`
	Separate Split `json:"separate"` // levels with separate men's and women's flights
}

// TimingsFor are a discipline's timings.
func (s Setup) TimingsFor(discipline string) Timings {
	if t, ok := s.Timings[discipline]; ok {
		return t
	}
	return DefaultTimings(discipline)
}

// DefaultSetup is a starting point: one day, a trampoline panel per discipline
// the competition offers, no breaks.
func DefaultSetup(disciplines []string) Setup {
	s := Setup{Days: []Day{{Name: "Day 1", Start: "09:00", End: "18:00"}}, Rest: 20, Separate: Split{Mode: SplitAll}}
	names := map[string]string{Trampoline: "Panel", Tumbling: "Track", DMT: "DMT"}
	seen := map[string]bool{}
	for _, d := range disciplines {
		area := d
		if d == Synchro {
			area = Trampoline
		}
		if seen[area] {
			continue
		}
		seen[area] = true
		s.Areas = append(s.Areas, Area{Name: names[area] + " 1", Discipline: area})
	}
	return s
}

// clock reads "09:30" as minutes since midnight.
func clock(s string) (int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("times should be written as 09:30, not %q", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}

// Clock writes minutes since midnight as "09:30".
func Clock(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }

// Check reports what's wrong with the setup, for a competition's events.
func (s Setup) Check(events []string) error {
	var errs []error
	if len(s.Days) == 0 {
		errs = append(errs, errors.New("the competition needs at least one day"))
	}
	areas := map[string]bool{}
	for _, a := range s.Areas {
		switch {
		case strings.TrimSpace(a.Name) == "":
			errs = append(errs, errors.New("every area needs a name"))
		case areas[a.Name]:
			errs = append(errs, fmt.Errorf("two areas are called %q", a.Name))
		case !slices.Contains([]string{Trampoline, Tumbling, DMT}, a.Discipline):
			errs = append(errs, fmt.Errorf("%s: unknown discipline %q", a.Name, a.Discipline))
		}
		areas[a.Name] = true
	}
	for i, d := range s.Days {
		start, err1 := clock(d.Start)
		end, err2 := clock(d.End)
		switch {
		case err1 != nil || err2 != nil:
			errs = append(errs, fmt.Errorf("day %d: times should be written as 09:30", i+1))
		case end <= start:
			errs = append(errs, fmt.Errorf("day %d ends before it starts", i+1))
		}
		for _, a := range d.Areas {
			if !areas[a] {
				errs = append(errs, fmt.Errorf("day %d uses an area %q that isn't set up", i+1, a))
			}
		}
	}
	for _, b := range s.Blocks {
		if strings.TrimSpace(b.Name) == "" || b.Minutes <= 0 {
			errs = append(errs, errors.New("blocked time needs a name and a length"))
		}
		if b.Day < 0 || b.Day >= len(s.Days) {
			errs = append(errs, fmt.Errorf("%s is on a day the competition doesn't have", b.Name))
		}
		if _, err := clock(b.At); b.At != "" && err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", b.Name, err))
		}
		if b.At == "" {
			from, err1 := clock(b.From)
			to, err2 := clock(b.To)
			if err1 != nil || err2 != nil || to-from < b.Minutes {
				errs = append(errs, fmt.Errorf("%s needs a fixed time, or a window at least as long as it", b.Name))
			}
		}
		for _, a := range b.Areas {
			if !areas[a] {
				errs = append(errs, fmt.Errorf("%s takes an area %q that isn't set up", b.Name, a))
			}
		}
		if err := b.Officials.check(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", b.Name, err))
		}
	}
	for _, r := range s.Rules {
		if (r.Event != "" || !personRule(r.Kind)) && !slices.Contains(events, r.Event) {
			errs = append(errs, fmt.Errorf("a rule is about an event %q the competition doesn't offer", r.Event))
		}
		if personRule(r.Kind) && r.Person == "" {
			errs = append(errs, errors.New("a rule about an official needs a person"))
		}
		switch r.Kind {
		case RuleRole:
			if !slices.Contains(Roles, r.Role) {
				errs = append(errs, fmt.Errorf("a rule gives %s an unknown role %q", r.Name, r.Role))
			}
			if r.Event == "" {
				errs = append(errs, fmt.Errorf("a rule about %s's role needs an event", r.Name))
			}
		case RuleOff:
		case RuleHours:
			from, err1 := clock(r.From)
			to, err2 := clock(r.To)
			switch {
			case r.Day < 0 || r.Day >= len(s.Days):
				errs = append(errs, fmt.Errorf("a rule about %s is on a day the competition doesn't have", r.Name))
			case r.From == "" && r.To == "":
				errs = append(errs, fmt.Errorf("a rule about %s's hours needs a time", r.Name))
			case (r.From != "" && err1 != nil) || (r.To != "" && err2 != nil):
				errs = append(errs, fmt.Errorf("a rule about %s: times should be written as 09:30", r.Name))
			case r.From != "" && r.To != "" && to <= from:
				errs = append(errs, fmt.Errorf("a rule about %s's hours ends before it starts", r.Name))
			}
		case RuleArea:
			if !areas[r.Area] {
				errs = append(errs, fmt.Errorf("a rule puts %s on an area %q that isn't set up", r.Event, r.Area))
			}
		case RuleDay:
			if r.Day < 0 || r.Day >= len(s.Days) {
				errs = append(errs, fmt.Errorf("a rule puts %s on a day the competition doesn't have", r.Event))
			}
		case RuleBefore, RuleApart:
			if !slices.Contains(events, r.Event2) || r.Event2 == r.Event {
				errs = append(errs, fmt.Errorf("a rule about %s needs another event", r.Event))
			}
		default:
			errs = append(errs, fmt.Errorf("unknown rule %q", r.Kind))
		}
	}
	for d, t := range s.Timings {
		if t.PerCompetitor <= 0 || t.PerCompetitor > 30 || t.Between < 0 || t.Between > 120 || t.MaxFlight < 1 || t.MaxFlight > 60 {
			errs = append(errs, fmt.Errorf("%s's timings are out of range", DisciplineName(d)))
		}
	}
	if s.Rest < 0 || s.Rest > 240 {
		errs = append(errs, errors.New("rest should be from 0 to 240 minutes"))
	}
	return errors.Join(errs...)
}

// ScheduledFlight is a flight placed on an area at a time.
type ScheduledFlight struct {
	Flight
	Discipline string `json:"discipline,omitempty"`
	Day        int    `json:"day"`
	Area       string `json:"area"`
	Start      int    `json:"start"` // minutes since midnight, when its warm-up starts
	End        int    `json:"end"`
	Officials  []Duty `json:"officials,omitempty"` // its panel's seats, from the rota
}

// ScheduledBlock is blocked time placed on its areas.
type ScheduledBlock struct {
	Name      string   `json:"name"`
	Day       int      `json:"day"`
	Areas     []string `json:"areas"`
	Start     int      `json:"start"`
	End       int      `json:"end"`
	Officials []Duty   `json:"officials,omitempty"` // its seats, from the rota, if it needs officials
}

// Schedule is the timetable: the setup it was planned with, every flight and
// block placed, and what couldn't be.
type Schedule struct {
	Setup          Setup             `json:"setup"`
	Flights        []ScheduledFlight `json:"flights"`
	Blocks         []ScheduledBlock  `json:"blocks,omitempty"`
	Unplaced       []ScheduledFlight `json:"unplaced,omitempty"` // flights that fit nowhere
	UnplacedBlocks []string          `json:"unplaced_blocks,omitempty"`
	Published      bool              `json:"published,omitempty"`
	Planned        bool              `json:"planned,omitempty"` // planned at least once
	Stale          bool              `json:"stale,omitempty"`   // the setup changed since it was planned
}

// SchedEntry is an entry as the scheduler needs it: its event, discipline,
// category and club, and the people in it (one, or a synchro pair), by a key
// that's the same wherever the same person appears.
type SchedEntry struct {
	PlanEntry
	Discipline string
	People     []string
	Coaches    []string // who coaches its gymnasts, by a key: kept from being needed in two places where that can be done
}

// interval is a span of minutes on a day.
type interval struct{ day, start, end int }

func overlaps(a, b interval) bool { return a.day == b.day && a.start < b.end && b.start < a.end }

// planner is one attempt at a schedule.
type planner struct {
	setup   Setup
	areas   []Area
	days    [][2]int              // each day's start and end
	busy    map[string][]interval // by area
	people  map[string][]interval // by person
	coaches map[string][]interval // by coach
	staff   *Staffing
	placed  []ScheduledFlight
	blocks  []ScheduledBlock
	byEvent map[string][]ScheduledFlight
	cost    float64
}

// flightsOf builds each event's flights, in event order, with their people.
func flightsOf(entries []SchedEntry, eventOrder []string, setup Setup, rng *rand.Rand) ([]ScheduledFlight, map[string][]string, map[string][]string) {
	people, coaches := map[string][]string{}, map[string][]string{}
	discipline := map[string]string{}
	var plan []PlanEntry
	for _, e := range entries {
		people[e.ID], coaches[e.ID] = e.People, e.Coaches
		discipline[e.Level] = e.Discipline
		plan = append(plan, e.PlanEntry)
	}
	var out []ScheduledFlight
	for _, g := range groups(plan, eventOrder, setup.Separate) {
		d := discipline[g.level]
		for _, f := range flightsFor(g, setup.TimingsFor(d).MaxFlight, rng) {
			out = append(out, ScheduledFlight{Flight: f, Discipline: d})
		}
	}
	return out, people, coaches
}

// duration is how long a flight takes, in whole minutes.
func (s Setup) duration(f ScheduledFlight) int {
	t := s.TimingsFor(f.Discipline)
	return t.Between + int(math.Ceil(float64(len(f.Entries))*t.PerCompetitor))
}

// Staffing is who may judge each event, so that flights go where enough of
// them are free (not competing), and how many judges each discipline's
// panel needs.
type Staffing struct {
	Judges map[string][]string // by event: the keys of the people who may judge it
	Need   map[string]int      // by discipline
}

// PlanSchedule schedules the entries: each event's flights, placed around blocked
// time, keeping the rules and people's clashes, as early as they can go. It
// tries several orders (deterministically, from seed) and keeps the best.
func PlanSchedule(entries []SchedEntry, eventOrder []string, setup Setup, seed uint64) Schedule {
	return PlanStaffed(entries, eventOrder, setup, nil, seed)
}

// PlanStaffed is PlanSchedule, also preferring times when enough of each
// event's judges are free to fill its panel.
func PlanStaffed(entries []SchedEntry, eventOrder []string, setup Setup, staff *Staffing, seed uint64) Schedule {
	rng := rand.New(rand.NewPCG(seed, 7))
	flights, people, coaches := flightsOf(entries, eventOrder, setup, rng)
	var best *planner
	var bestUnplaced []ScheduledFlight
	for attempt := range 24 {
		order := slices.Clone(flights)
		if attempt > 0 {
			// Shuffle within the rules' order; longest-first and event order are tried first.
			rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		} else {
			slices.SortStableFunc(order, func(a, b ScheduledFlight) int { return setup.duration(b) - setup.duration(a) })
			slices.SortStableFunc(order, func(a, b ScheduledFlight) int {
				return slices.Index(eventOrder, a.Level) - slices.Index(eventOrder, b.Level)
			})
		}
		order = beforeOrder(order, setup.Rules)
		p := newPlanner(setup)
		p.staff = staff
		var unplaced []ScheduledFlight
		for _, f := range order {
			if !p.place(f, people, coaches) {
				unplaced = append(unplaced, f)
			}
		}
		p.cost += float64(len(unplaced)) * 1e6
		if best == nil || p.cost < best.cost {
			best, bestUnplaced = p, unplaced
		}
	}
	out := Schedule{Setup: setup, Flights: best.placed, Blocks: best.blocks, Unplaced: bestUnplaced, Planned: true}
	for _, b := range setup.Blocks {
		found := false
		for _, sb := range best.blocks {
			found = found || sb.Name == b.Name && sb.Day == b.Day
		}
		if !found {
			out.UnplacedBlocks = append(out.UnplacedBlocks, b.Name)
		}
	}
	sortFlights(out.Flights)
	out.OrderForRest(people)
	return out
}

// beforeOrder moves flights so every "before" rule's first event comes ahead
// of its second, keeping the order otherwise.
func beforeOrder(order []ScheduledFlight, rules []Rule) []ScheduledFlight {
	rank := map[string]int{}
	for i := 0; i < len(rules)+1; i++ {
		for _, r := range rules {
			if r.Kind == RuleBefore && rank[r.Event2] <= rank[r.Event] {
				rank[r.Event2] = rank[r.Event] + 1
			}
		}
	}
	slices.SortStableFunc(order, func(a, b ScheduledFlight) int { return rank[a.Level] - rank[b.Level] })
	return order
}

func newPlanner(setup Setup) *planner {
	p := &planner{setup: setup, areas: setup.Areas, busy: map[string][]interval{}, people: map[string][]interval{}, coaches: map[string][]interval{}, byEvent: map[string][]ScheduledFlight{}}
	for _, d := range setup.Days {
		start, end := d.Hours()
		p.days = append(p.days, [2]int{start, end})
	}
	p.placeBlocks()
	return p
}

// placeBlocks puts blocked time first: fixed blocks at their time, windowed
// ones at the earliest time in their window that all their areas are free.
func (p *planner) placeBlocks() {
	for _, b := range p.setup.Blocks {
		if b.Day < 0 || b.Day >= len(p.days) {
			continue
		}
		areas := b.Areas
		if len(areas) == 0 {
			for _, a := range p.areas {
				if p.setup.Days[b.Day].uses(a.Name) {
					areas = append(areas, a.Name)
				}
			}
		}
		var starts []int
		if b.At != "" {
			at, _ := clock(b.At)
			starts = []int{at}
		} else {
			from, _ := clock(b.From)
			to, _ := clock(b.To)
			for t := from; t+b.Minutes <= to; t += 5 {
				starts = append(starts, t)
			}
		}
		for _, t := range starts {
			iv := interval{b.Day, t, t + b.Minutes}
			if t < p.days[b.Day][0] || iv.end > p.days[b.Day][1] {
				continue // blocked time sits within its day
			}
			free := true
			for _, a := range areas {
				for _, used := range p.busy[a] {
					free = free && !overlaps(iv, used)
				}
			}
			if free {
				for _, a := range areas {
					p.busy[a] = append(p.busy[a], iv)
				}
				p.blocks = append(p.blocks, ScheduledBlock{Name: b.Name, Day: b.Day, Areas: areas, Start: iv.start, End: iv.end})
				break
			}
		}
	}
}

// place puts a flight at its best feasible place, if it has one.
func (p *planner) place(f ScheduledFlight, people, coaches map[string][]string) bool {
	who, coached := map[string]bool{}, map[string]bool{}
	for _, id := range f.Entries {
		for _, person := range people[id] {
			who[person] = true
		}
		for _, c := range coaches[id] {
			coached[c] = true
		}
	}
	length := p.setup.duration(f)
	bestCost := math.Inf(1)
	var best ScheduledFlight
	for day := range p.days {
		for _, a := range p.areas {
			if !a.runs(f.Discipline) || !p.setup.Days[day].uses(a.Name) || p.broken(f.Level, a.Name, day, true) {
				continue
			}
			start, ok := p.earliest(f.Level, who, a.Name, day, length)
			if !ok {
				continue
			}
			cand := ScheduledFlight{Flight: f.Flight, Discipline: f.Discipline, Day: day, Area: a.Name, Start: start, End: start + length}
			if c := p.score(cand, who, coached); c < bestCost {
				bestCost, best = c, cand
			}
		}
	}
	if math.IsInf(bestCost, 1) {
		return false
	}
	p.placed = append(p.placed, best)
	p.byEvent[best.Level] = append(p.byEvent[best.Level], best)
	iv := interval{best.Day, best.Start, best.End}
	p.busy[best.Area] = append(p.busy[best.Area], iv)
	for person := range who {
		p.people[person] = append(p.people[person], iv)
	}
	for c := range coached {
		p.coaches[c] = append(p.coaches[c], iv)
	}
	p.cost += bestCost
	return true
}

// broken says whether putting an event on an area and day breaks a rule
// about where it goes: musts only, or prefers too.
func (p *planner) broken(event, area string, day int, mustOnly bool) bool {
	for _, r := range p.setup.Rules {
		if r.Event != event || (mustOnly && !r.Must) || (!mustOnly && r.Must) {
			continue
		}
		if (r.Kind == RuleArea && r.Area != area) || (r.Kind == RuleDay && r.Day != day) {
			return true
		}
	}
	return false
}

// earliest is the earliest start on an area and day that fits before the
// day's end, clear of the area's other flights and blocks, the people's other
// turns (and their rest, if it's a must), and the order rules.
func (p *planner) earliest(event string, who map[string]bool, area string, day, length int) (int, bool) {
	dayStart, dayEnd := p.days[day][0], p.days[day][1]
	rest := 0
	if p.setup.RestMust {
		rest = p.setup.Rest
	}
	notBefore, notAfter := dayStart, dayEnd // must-rules on order
	var apart []interval
	for _, r := range p.setup.Rules {
		if !r.Must {
			continue
		}
		switch {
		case r.Kind == RuleBefore && r.Event2 == event:
			for _, f := range p.byEvent[r.Event] {
				switch {
				case f.Day > day:
					return 0, false
				case f.Day == day && f.End > notBefore:
					notBefore = f.End
				}
			}
		case r.Kind == RuleBefore && r.Event == event:
			for _, f := range p.byEvent[r.Event2] {
				switch {
				case f.Day < day:
					return 0, false
				case f.Day == day && f.Start < notAfter:
					notAfter = f.Start
				}
			}
		case r.Kind == RuleApart && (r.Event == event || r.Event2 == event):
			other := r.Event2
			if other == event {
				other = r.Event
			}
			for _, f := range p.byEvent[other] {
				apart = append(apart, interval{f.Day, f.Start, f.End})
			}
		}
	}
	candidates := []int{max(dayStart, notBefore)}
	for _, iv := range p.busy[area] {
		if iv.day == day {
			candidates = append(candidates, iv.end)
		}
	}
	for person := range who {
		for _, iv := range p.people[person] {
			if iv.day == day {
				candidates = append(candidates, iv.end+rest)
			}
		}
	}
	for _, iv := range apart {
		if iv.day == day {
			candidates = append(candidates, iv.end)
		}
	}
	sort.Ints(candidates)
	for _, t := range candidates {
		if t < dayStart || t < notBefore || t+length > min(dayEnd, notAfter) {
			continue
		}
		iv := interval{day, t, t + length}
		if p.clear(iv, area, who, rest, apart) {
			return t, true
		}
	}
	return 0, false
}

// clear says whether a span is free on an area, for the people (with rest
// either side), and apart from the events it mustn't share time with.
func (p *planner) clear(iv interval, area string, who map[string]bool, rest int, apart []interval) bool {
	for _, used := range p.busy[area] {
		if overlaps(iv, used) {
			return false
		}
	}
	wide := interval{iv.day, iv.start - rest, iv.end + rest}
	for person := range who {
		for _, used := range p.people[person] {
			if overlaps(wide, used) {
				return false
			}
		}
	}
	for _, used := range apart {
		if overlaps(iv, used) {
			return false
		}
	}
	return true
}

// score is how good a placement is: lower is better. Earlier is better
// (later days much worse), and each broken prefer costs.
func (p *planner) score(f ScheduledFlight, who, coached map[string]bool) float64 {
	cost := float64(f.Day*24*60 + f.End)
	if p.broken(f.Level, f.Area, f.Day, false) {
		cost += 120
	}
	// A level's flights on one area.
	for _, other := range p.byEvent[f.Level] {
		if other.Area != f.Area || other.Day != f.Day {
			cost += 60
			break
		}
	}
	// Rest, as a prefer: each minute short costs.
	if !p.setup.RestMust && p.setup.Rest > 0 {
		for person := range who {
			for _, iv := range p.people[person] {
				if iv.day != f.Day {
					continue
				}
				gap := f.Start - iv.end
				if iv.start >= f.End {
					gap = iv.start - f.End
				}
				if gap < p.setup.Rest {
					cost += float64(p.setup.Rest - gap)
				}
			}
		}
	}
	// Too few of an event's judges free, this one's or one already placed at
	// the same time (whose judges this one's gymnasts would take): each judge
	// short costs.
	if p.staff != nil {
		iv := interval{f.Day, f.Start, f.End}
		cost += 30 * float64(p.short(f, iv, who))
		for _, g := range p.placed {
			giv := interval{g.Day, g.Start, g.End}
			if overlaps(iv, giv) {
				cost += 30 * float64(p.short(g, giv, who)-p.short(g, giv, nil))
			}
		}
	}
	// A coach needed on two areas at once.
	for c := range coached {
		for _, iv := range p.coaches[c] {
			if overlaps(iv, interval{f.Day, f.Start, f.End}) {
				cost += 40
				break
			}
		}
	}
	// Order and apart, as prefers.
	for _, r := range p.setup.Rules {
		if r.Must {
			continue
		}
		switch {
		case r.Kind == RuleBefore && r.Event2 == f.Level:
			for _, other := range p.byEvent[r.Event] {
				if other.Day > f.Day || (other.Day == f.Day && other.End > f.Start) {
					cost += 120
				}
			}
		case r.Kind == RuleApart && (r.Event == f.Level || r.Event2 == f.Level):
			other := r.Event2
			if other == f.Level {
				other = r.Event
			}
			for _, o := range p.byEvent[other] {
				if overlaps(interval{o.Day, o.Start, o.End}, interval{f.Day, f.Start, f.End}) {
					cost += 120
				}
			}
		}
	}
	return cost
}

// short is how many judges a flight's panel would be short of, counting as
// busy whoever competes then, and also the people in who.
func (p *planner) short(f ScheduledFlight, iv interval, who map[string]bool) int {
	free := 0
	for _, key := range p.staff.Judges[f.Level] {
		if who[key] {
			continue
		}
		busy := false
		for _, used := range p.people[key] {
			busy = busy || overlaps(iv, used)
		}
		if !busy {
			free++
		}
	}
	return max(0, p.staff.Need[f.Discipline]-free)
}

// sortFlights orders flights by day, area, then time.
func sortFlights(fs []ScheduledFlight) {
	slices.SortStableFunc(fs, func(a, b ScheduledFlight) int {
		if a.Day != b.Day {
			return a.Day - b.Day
		}
		if a.Area != b.Area {
			return strings.Compare(a.Area, b.Area)
		}
		return a.Start - b.Start
	})
}
