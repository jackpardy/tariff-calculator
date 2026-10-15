package web

import (
	"context"
	"log"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Who's up now (roadmap item 7): on the day the marshal taps through each
// flight's gymnasts as they compete (uppages.go), and the venue screen shows
// who's on, which routine it is, and who's next. The order they compete in,
// and who's skipped, are competitions.UpOrder's; this is the pages' part.

// upOrderOf is a flight's routines in order, with those still entered (by
// id) and those scratched.
func upOrderOf(f competitions.ScheduledFlight, byID map[string]store.Entry, scratched map[string]bool) competitions.UpOrder {
	return competitions.UpOrderOf(f, func(id string) bool {
		_, entered := byID[id]
		return entered && !scratched[id]
	})
}

// upGymnast is the gymnast at a routine, as the pages name them.
func upGymnast(c competitions.Competition, o competitions.UpOrder, r competitions.Routine, byID map[string]store.Entry) views.ScreenUpGymnast {
	e := byID[r.Entry] // a zero Entry for one no longer entered
	// An individual has no club to name.
	return views.ScreenUpGymnast{Name: e.Entry.Gymnasts(), No: o.No(r), Club: e.ClubName, Routine: competitions.RoutineNames(c, e.Entry)[r.Round-1]}
}

// screenUp is who's on in a flight at the routine the marshal has tapped to,
// and the next two to go; nil if nobody competes there (scratched since, or
// moved out of the flight).
func screenUp(c competitions.Competition, f competitions.ScheduledFlight, at competitions.Routine, byID map[string]store.Entry, scratched map[string]bool) *views.ScreenUp {
	o := upOrderOf(f, byID, scratched)
	if !o.Open(at) {
		return nil
	}
	out := &views.ScreenUp{On: upGymnast(c, o, at, byID)}
	for _, r := range o.After(at, 2) {
		out.Next = append(out.Next, upGymnast(c, o, r, byID))
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
	return screenUp(c.Competition, *n.Flight, up.Routine, byID, scratched)
}
