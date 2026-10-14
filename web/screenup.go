package web

import (
	"context"
	"log"

	"tariffCalculator/competitions"
	"tariffCalculator/requirements"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Who's up now (roadmap item 7): on the day the marshal taps through each
// flight's gymnasts as they compete (uppages.go), and the venue screen shows
// who's on, which routine it is, and who's next. Each gymnast competes twice:
// everyone's 1st routine in running order, then everyone's 2nd in the same
// order, so a flight of n has positions 0 to 2n-1 (store.Up).

// upOrder is a flight's running order for who's up now.
type upOrder struct {
	entries []store.Entry // in running order; a zero Entry for one no longer entered
	open    []bool        // by position: whether anyone competes there (entered, not scratched)
}

// upOrderOf is a flight's running order, with the entries still in the
// competition by id and those scratched.
func upOrderOf(f competitions.ScheduledFlight, byID map[string]store.Entry, scratched map[string]bool) upOrder {
	n := len(f.Entries)
	out := upOrder{entries: make([]store.Entry, n), open: make([]bool, 2*n)}
	for i, id := range f.Entries {
		e, ok := byID[id]
		if !ok {
			continue
		}
		out.entries[i] = e
		out.open[i], out.open[n+i] = !scratched[id], !scratched[id]
	}
	return out
}

// at is who competes at a position, and which of their routines it is (0 for
// the 1st, 1 for the 2nd); false for a position the flight doesn't have.
func (o upOrder) at(pos int) (store.Entry, int, bool) {
	n := len(o.entries)
	if pos < 0 || pos >= 2*n {
		return store.Entry{}, 0, false
	}
	return o.entries[pos%n], pos / n, true
}

// after are up to k open positions after pos, in order.
func (o upOrder) after(pos, k int) []int {
	var out []int
	for i := pos + 1; i < len(o.open) && len(out) < k; i++ {
		if o.open[i] {
			out = append(out, i)
		}
	}
	return out
}

// routineNames are what an entry's two routines are called: the level's own
// names where its exercises have one ("Set" for set routines, "Voluntary" for
// the rest, when the two differ), else "1st routine" and "2nd routine"
// ("pass" for tumbling and DMT).
func routineNames(c competitions.Competition, e competitions.Entry) [2]string {
	switch e.Discipline {
	case competitions.Tumbling, competitions.DMT:
		return [2]string{"1st pass", "2nd pass"}
	}
	numbered := [2]string{"1st routine", "2nd routine"}
	if e.Discipline == competitions.Synchro {
		return numbered // a pair's routines aren't the level's two exercises
	}
	l, level, ok := c.EntryLevel(e)
	if !ok {
		return numbered
	}
	var out [2]string
	for i := range out {
		for _, ref := range level.Exercise(i + 1).Options {
			set, err := l.Set(ref)
			if err != nil {
				return numbered
			}
			name := "Voluntary"
			if _, ok := requirements.SetRoutine(set); ok {
				name = "Set"
			}
			if out[i] != "" && out[i] != name {
				return numbered // some options are set routines, some not
			}
			out[i] = name
		}
	}
	if out[0] == "" || out[0] == out[1] {
		return numbered
	}
	return out
}

// upGymnast is the gymnast at a position, as the pages name them.
func upGymnast(c competitions.Competition, o upOrder, pos int) views.ScreenUpGymnast {
	e, round, _ := o.at(pos)
	// An individual has no club to name.
	return views.ScreenUpGymnast{Name: e.Entry.Gymnasts(), Club: e.ClubName, Routine: routineNames(c, e.Entry)[round]}
}

// screenUp is who's on in a flight at a position the marshal has tapped to,
// and the next two to go; nil if nobody competes there (scratched since, say).
func screenUp(c competitions.Competition, f competitions.ScheduledFlight, pos int, byID map[string]store.Entry, scratched map[string]bool) *views.ScreenUp {
	o := upOrderOf(f, byID, scratched)
	if _, _, ok := o.at(pos); !ok || !o.open[pos] {
		return nil
	}
	out := &views.ScreenUp{On: upGymnast(c, o, pos)}
	for _, i := range o.after(pos, 2) {
		out.Next = append(out.Next, upGymnast(c, o, i))
	}
	return out
}

// screenUpOf is who's up now on an area of the venue screen: in the flight
// marked started and on now, once the marshal has tapped; nil otherwise, as
// the screen doesn't guess.
func (p *competitionPages) screenUpOf(ctx context.Context, c store.Competition, n areaNow, all []store.Entry, scratched map[string]bool) *views.ScreenUp {
	if n.Flight == nil || n.Planned {
		return nil
	}
	ups, err := p.st.UpNow(ctx, c.ID)
	if err != nil {
		log.Printf("Who's up now: %s: %v", c.ID, err)
		return nil
	}
	up, ok := ups[competitions.FlightKey(*n.Flight)]
	if !ok {
		return nil
	}
	byID := map[string]store.Entry{}
	for _, e := range live(all) {
		byID[e.ID] = e
	}
	return screenUp(c.Competition, *n.Flight, up.Position, byID, scratched)
}
