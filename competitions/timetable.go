package competitions

import (
	"fmt"
	"math/rand/v2"
	"slices"
)

// Flights: each event's entries (or each of its men and women, where the
// timetable separates them) drawn into even flights with a running order.
// The scheduler (schedule.go) places them. How the timetable splits an event
// needn't match how it's ranked: an event ranking men and women separately
// (Competition.Split) can still fly them together (Setup.Separate).

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
