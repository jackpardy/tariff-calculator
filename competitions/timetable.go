package competitions

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"time"
)

// The timetable (roadmap: competitions 3) puts a competition's entries into
// flights on panels, with running orders and estimated times. A panel is two
// trampolines with their officials. A level's flights stay together on one
// panel, so the same judges see the whole category; small levels share a
// panel. How the timetable splits a level needn't match how it's ranked: a
// level ranking men and women separately (Competition.Split) can still run
// them in mixed flights (PlanSettings.Separate).

// PlanSettings are what the organiser plans with.
type PlanSettings struct {
	Panels            int     `json:"panels"`
	Start             string  `json:"start"`               // when the first flights' warm-up starts, as "09:00"
	MinutesPerGymnast float64 `json:"per_gymnast"`         // both rounds
	MinutesBetween    int     `json:"between"`             // each flight's warm-up and changeover
	MaxFlight         int     `json:"max_flight"`          // the most gymnasts in a flight
	FinishBy          string  `json:"finish_by,omitempty"` // as "17:30": how many panels that needs
	// Separate is which levels get separate men's and women's flights. Only
	// levels whose gymnasts say which (Competition.Split) can be separated;
	// the rest of a split level's men and women fly together.
	Separate Split `json:"separate"`
}

// DefaultSettings are a starting point: British Gymnastics allows 4½–6½
// minutes per competitor for two rounds and warm-up.
var DefaultSettings = PlanSettings{Panels: 2, Start: "09:00", MinutesPerGymnast: 5, MinutesBetween: 10, MaxFlight: 12, Separate: Split{Mode: SplitAll}}

// Check reports what's wrong with the settings.
func (s PlanSettings) Check() error {
	switch {
	case s.Panels < 1 || s.Panels > 20:
		return fmt.Errorf("panels should be from 1 to 20")
	case s.MinutesPerGymnast <= 0 || s.MinutesPerGymnast > 30:
		return fmt.Errorf("minutes per gymnast should be more than 0 and at most 30")
	case s.MinutesBetween < 0 || s.MinutesBetween > 120:
		return fmt.Errorf("minutes between flights should be from 0 to 120")
	case s.MaxFlight < 2 || s.MaxFlight > 60:
		return fmt.Errorf("the largest flight should be from 2 to 60 gymnasts")
	}
	if err := s.Separate.validate(nil); err != nil && s.Separate.Mode != SplitSome {
		return err
	}
	if _, err := time.Parse("15:04", s.Start); err != nil {
		return fmt.Errorf("the start time should be written as 09:00")
	}
	if s.FinishBy != "" {
		if _, err := time.Parse("15:04", s.FinishBy); err != nil {
			return fmt.Errorf("the finish time should be written as 17:30")
		}
	}
	return nil
}

// Timetable is the competition's flights, panel by panel, in order.
type Timetable struct {
	Settings  PlanSettings `json:"settings"`
	Panels    [][]Flight   `json:"panels"`
	Published bool         `json:"published,omitempty"` // clubs and gymnasts see their flight and time
}

// Flight is gymnasts who warm up and compete together, in running order.
type Flight struct {
	Level    string   `json:"level"`
	Category string   `json:"category,omitempty"` // Men or Women, where the level is split
	Number   int      `json:"number"`             // which of the category's flights (1-based)
	Of       int      `json:"of"`                 // how many flights the category has
	Entries  []string `json:"entries"`            // entry ids, in running order
}

// Name is the flight's name, e.g. "BUCS L3 Women · flight 2 of 3".
func (f Flight) Name() string {
	name := f.Level
	if f.Category != "" {
		name += " " + f.Category
	}
	if f.Of > 1 {
		name += fmt.Sprintf(" · flight %d of %d", f.Number, f.Of)
	}
	return name
}

// PlanEntry is an entry as the planner needs it.
type PlanEntry struct {
	ID, Level, Category, Club string
}

// group is a category's entries: a level, or one half of a split level.
type group struct {
	level, category string
	entries         []PlanEntry
}

// groups are the entries by category, in the competition's level order, men
// then women; entries for levels it no longer offers come last. A level whose
// men and women aren't separated is one category.
func groups(entries []PlanEntry, levelOrder []string, separate Split) []group {
	rank := func(level string) int {
		if i := slices.Index(levelOrder, level); i >= 0 {
			return i
		}
		return len(levelOrder)
	}
	var out []group
	for _, e := range entries {
		if !separate.Splits(e.Level) {
			e.Category = ""
		}
		i := slices.IndexFunc(out, func(g group) bool { return g.level == e.Level && g.category == e.Category })
		if i < 0 {
			out = append(out, group{level: e.Level, category: e.Category})
			i = len(out) - 1
		}
		out[i].entries = append(out[i].entries, e)
	}
	slices.SortStableFunc(out, func(a, b group) int {
		if d := rank(a.level) - rank(b.level); d != 0 {
			return d
		}
		return slices.Index(Categories, a.category) - slices.Index(Categories, b.category)
	})
	return out
}

// flightsFor splits a category into the fewest flights of at most max, as
// even as possible.
func flightsFor(g group, max int, rng *rand.Rand) []Flight {
	order := Draw(g.entries, rng)
	k := (len(order) + max - 1) / max
	out := make([]Flight, k)
	for i := range out {
		lo, hi := i*len(order)/k, (i+1)*len(order)/k
		out[i] = Flight{Level: g.level, Category: g.category, Number: i + 1, Of: k}
		for _, e := range order[lo:hi] {
			out[i].Entries = append(out[i].Entries, e.ID)
		}
	}
	return out
}

// Draw is a random running order that, as a soft preference, avoids the
// same club twice in a row where it easily can. It's only a tie-break
// between random orders: it gives way whenever it can't, and never changes
// who is in which flight. Individuals ("" club) never clash.
func Draw(entries []PlanEntry, rng *rand.Rand) []PlanEntry {
	pool := slices.Clone(entries)
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	out := make([]PlanEntry, 0, len(pool))
	for len(pool) > 0 {
		// Next is whoever's club has most left, other than the club just drawn,
		// so a big club is spread through the order rather than bunched at the
		// end; ties keep the shuffled order.
		last := ""
		if n := len(out); n > 0 {
			last = out[n-1].Club
		}
		pick := -1
		for i, e := range pool {
			if e.Club != "" && e.Club == last {
				continue
			}
			if pick < 0 || clubCount(pool, e.Club) > clubCount(pool, pool[pick].Club) {
				pick = i
			}
		}
		if pick < 0 {
			pick = 0 // only the last club's gymnasts are left
		}
		out = append(out, pool[pick])
		pool = slices.Delete(pool, pick, pick+1)
	}
	return out
}

func clubCount(entries []PlanEntry, club string) int {
	if club == "" {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.Club == club {
			n++
		}
	}
	return n
}

// minutes is how long a flight takes on its panel: warm-up and changeover,
// then each gymnast's two rounds.
func (s PlanSettings) minutes(gymnasts int) float64 {
	return float64(s.MinutesBetween) + float64(gymnasts)*s.MinutesPerGymnast
}

// Plan makes a timetable: each category drawn into flights, each category's
// flights kept together on one panel, and categories shared out so the
// panels finish as early as possible.
func Plan(entries []PlanEntry, levelOrder []string, s PlanSettings, rng *rand.Rand) Timetable {
	t := Timetable{Settings: s, Panels: make([][]Flight, s.Panels)}
	type planned struct {
		order   int
		flights []Flight
		minutes float64
	}
	var all []planned
	for i, g := range groups(entries, levelOrder, s.Separate) {
		p := planned{order: i, flights: flightsFor(g, s.MaxFlight, rng)}
		for _, f := range p.flights {
			p.minutes += s.minutes(len(f.Entries))
		}
		all = append(all, p)
	}
	// Longest first, each to the panel that's free soonest; then each panel
	// runs its categories in level order.
	byLength := slices.Clone(all)
	slices.SortStableFunc(byLength, func(a, b planned) int {
		switch {
		case a.minutes > b.minutes:
			return -1
		case a.minutes < b.minutes:
			return 1
		}
		return 0
	})
	load := make([]float64, s.Panels)
	onPanel := make([][]planned, s.Panels)
	for _, p := range byLength {
		best := 0
		for i := range load {
			if load[i] < load[best] {
				best = i
			}
		}
		load[best] += p.minutes
		onPanel[best] = append(onPanel[best], p)
	}
	for i, ps := range onPanel {
		slices.SortFunc(ps, func(a, b planned) int { return a.order - b.order })
		for _, p := range ps {
			t.Panels[i] = append(t.Panels[i], p.flights...)
		}
	}
	return t
}

// PanelsNeeded is the fewest panels (up to 20) that finish by the settings'
// FinishBy, or 0 if even 20 can't, or none is set.
func PanelsNeeded(entries []PlanEntry, levelOrder []string, s PlanSettings) int {
	if s.FinishBy == "" {
		return 0
	}
	for n := 1; n <= 20; n++ {
		try := s
		try.Panels = n
		plan := Plan(entries, levelOrder, try, rand.New(rand.NewPCG(1, 2)))
		if !plan.FinishesAfter(s.FinishBy) {
			return n
		}
	}
	return 0
}

// Slot is when a flight is on: its warm-up from Start, finishing at End.
type Slot struct {
	Start, End time.Time
}

// Slots are each flight's times, panel by panel, on the competition's day.
func (t Timetable) Slots(day time.Time) [][]Slot {
	start, _ := time.Parse("15:04", t.Settings.Start)
	out := make([][]Slot, len(t.Panels))
	for p, flights := range t.Panels {
		at := time.Date(day.Year(), day.Month(), day.Day(), start.Hour(), start.Minute(), 0, 0, day.Location())
		for _, f := range flights {
			end := at.Add(time.Duration(t.Settings.minutes(len(f.Entries)) * float64(time.Minute)))
			out[p] = append(out[p], Slot{Start: at, End: end})
			at = end
		}
	}
	return out
}

// Finish is when the last panel finishes, on the competition's day.
func (t Timetable) Finish(day time.Time) time.Time {
	var last time.Time
	for _, slots := range t.Slots(day) {
		if n := len(slots); n > 0 && slots[n-1].End.After(last) {
			last = slots[n-1].End
		}
	}
	return last
}

// FinishesAfter says whether the timetable runs past a time of day ("17:30").
func (t Timetable) FinishesAfter(clock string) bool {
	by, err := time.Parse("15:04", clock)
	if err != nil {
		return false
	}
	day := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	limit := time.Date(2000, 1, 1, by.Hour(), by.Minute(), 0, 0, time.UTC)
	return t.Finish(day).After(limit)
}

// Place is where an entry is in the timetable.
type Place struct {
	Panel, Flight, Position int // 0-based
}

// Find is where an entry is, if it's in the timetable.
func (t Timetable) Find(entryID string) (Place, bool) {
	for p, flights := range t.Panels {
		for f, flight := range flights {
			if i := slices.Index(flight.Entries, entryID); i >= 0 {
				return Place{p, f, i}, true
			}
		}
	}
	return Place{}, false
}

// valid says whether a panel and flight exist.
func (t Timetable) valid(panel, flight int) bool {
	return panel >= 0 && panel < len(t.Panels) && flight >= 0 && flight < len(t.Panels[panel])
}

// MoveFlight moves a flight to the end of another panel.
func (t *Timetable) MoveFlight(panel, flight, to int) bool {
	if !t.valid(panel, flight) || to < 0 || to >= len(t.Panels) || to == panel {
		return false
	}
	f := t.Panels[panel][flight]
	t.Panels[panel] = slices.Delete(t.Panels[panel], flight, flight+1)
	t.Panels[to] = append(t.Panels[to], f)
	return true
}

// ShiftFlight moves a flight earlier (-1) or later (+1) on its panel.
func (t *Timetable) ShiftFlight(panel, flight, by int) bool {
	to := flight + by
	if !t.valid(panel, flight) || !t.valid(panel, to) {
		return false
	}
	fs := t.Panels[panel]
	fs[flight], fs[to] = fs[to], fs[flight]
	return true
}

// MoveEntry moves an entry to the end of a flight's running order, from
// wherever it is (or into the timetable, if it isn't yet).
func (t *Timetable) MoveEntry(entryID string, panel, flight int) bool {
	if !t.valid(panel, flight) {
		return false
	}
	t.RemoveEntry(entryID)
	f := &t.Panels[panel][flight]
	f.Entries = append(f.Entries, entryID)
	return true
}

// RemoveEntry takes an entry out of the timetable.
func (t *Timetable) RemoveEntry(entryID string) {
	if at, ok := t.Find(entryID); ok {
		f := &t.Panels[at.Panel][at.Flight]
		f.Entries = slices.Delete(f.Entries, at.Position, at.Position+1)
	}
}

// Redraw draws a flight's running order again, spreading clubs.
func (t *Timetable) Redraw(panel, flight int, clubs map[string]string, rng *rand.Rand) bool {
	if !t.valid(panel, flight) {
		return false
	}
	f := &t.Panels[panel][flight]
	var entries []PlanEntry
	for _, id := range f.Entries {
		entries = append(entries, PlanEntry{ID: id, Club: clubs[id]})
	}
	f.Entries = f.Entries[:0]
	for _, e := range Draw(entries, rng) {
		f.Entries = append(f.Entries, e.ID)
	}
	return true
}
