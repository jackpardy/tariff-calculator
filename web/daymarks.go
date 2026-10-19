package web

import (
	"context"
	"net/http"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Nothing marked on the day is lost without a word (audit 2026-10-16). A
// flight's times, who's up and its panel's check-ins are kept by its ID,
// which moving, redrawing and renumbering keep; a fresh plan gives its
// flights the IDs of the ones they were (KeepIDs); and a gymnast's check-in
// is kept by their entry, wherever they fly. What still can't carry across,
// such as a check-in for a seat no longer in the plan, Publish changes says
// before it publishes; and Plan again asks first once the day has begun.

// dayMarks is what's been marked on the day: each flight's times, who's up
// and its panel's check-ins (by flight key), and the gymnasts' check-ins (by
// entry).
type dayMarks struct {
	times    map[string]store.FlightTime
	up       map[string]store.Up
	official map[string]map[string]store.Checkin
	checkins map[string]store.Checkin
}

func (p *competitionPages) dayMarksOf(ctx context.Context, id string) (m dayMarks, err error) {
	if m.times, err = p.st.FlightTimes(ctx, id); err != nil {
		return m, err
	}
	if m.up, err = p.st.UpNow(ctx, id); err != nil {
		return m, err
	}
	if m.official, err = p.st.OfficialCheckins(ctx, id); err != nil {
		return m, err
	}
	m.checkins, err = p.st.Checkins(ctx, id)
	return m, err
}

// underWay says whether a flight has started or finished, or the marshal
// has tapped through it.
func (m dayMarks) underWay(flight string) bool {
	t := m.times[flight]
	_, up := m.up[flight]
	return up || !t.Started.IsZero() || !t.Finished.IsZero()
}

// panelMarked are the panel check-ins on a flight, by person: here or
// missing.
func (m dayMarks) panelMarked(flight string) []string {
	var out []string
	for person, c := range m.official[flight] {
		if c.Status != "" {
			out = append(out, person)
		}
	}
	return out
}

// begun says what's been marked on the day for the published timetable, in
// plain words with counts; none if nothing has.
func (m dayMarks) begun(published competitions.Schedule) []string {
	flights, panel, gymnasts := 0, 0, 0
	for _, f := range published.Flights {
		k := competitions.FlightKey(f)
		if m.underWay(k) {
			flights++
		}
		panel += len(m.panelMarked(k))
		for _, id := range f.Entries {
			if m.checkins[id].Status != "" {
				gymnasts++
			}
		}
	}
	var out []string
	if flights > 0 {
		out = append(out, views.Count(flights, "flight has", "flights have")+" started or finished.")
	}
	if panel > 0 {
		out = append(out, views.Count(panel, "official is", "officials are")+" checked in or marked missing on a panel.")
	}
	if gymnasts > 0 {
		out = append(out, views.Count(gymnasts, "gymnast is", "gymnasts are")+" checked in or scratched.")
	}
	return out
}

// lost says what's marked on the day for the published timetable that
// publishing next instead would no longer show, in plain words with counts:
// flights under way or run that aren't in it, panel check-ins for seats it
// doesn't have, and gymnasts checked in or scratched it doesn't have. name
// names a gymnast by their entry.
func (m dayMarks) lost(published, next competitions.Schedule, name func(entryID string) string) []string {
	nextFlights := map[string]competitions.ScheduledFlight{}
	for _, f := range next.Flights {
		nextFlights[competitions.FlightKey(f)] = f
	}
	var flights, gymnasts []string
	panel := 0
	for _, f := range published.Flights {
		k := competitions.FlightKey(f)
		g, kept := nextFlights[k]
		if !kept && m.underWay(k) {
			flights = append(flights, f.Name())
		}
		for _, person := range m.panelMarked(k) {
			if !kept || !holdsSeat(g, person) {
				panel++
			}
		}
		for _, id := range f.Entries {
			if _, in := next.Find(id); !in && m.checkins[id].Status != "" {
				gymnasts = append(gymnasts, name(id))
			}
		}
	}
	var out []string
	if len(flights) > 0 {
		its := map[bool]string{true: "its", false: "their"}[len(flights) == 1]
		out = append(out, views.Count(len(flights), "flight that has started or finished isn't", "flights that have started or finished aren't")+
			" in the plan any more: "+its+" times and who's up are dropped ("+someOf(flights)+").")
	}
	if panel > 0 {
		out = append(out, views.Count(panel, "panel check-in is", "panel check-ins are")+" for seats no longer in the plan.")
	}
	if len(gymnasts) > 0 {
		out = append(out, views.Count(len(gymnasts), "gymnast checked in or scratched isn't", "gymnasts checked in or scratched aren't")+
			" in the plan any more: "+someOf(gymnasts)+".")
	}
	var moved []string
	for _, id := range movedFlight(published, next) {
		if m.checkins[id].Status != "" {
			moved = append(moved, name(id))
		}
	}
	if len(moved) > 0 {
		out = append(out, views.Count(len(moved), "gymnast checked in or scratched moves", "gymnasts checked in or scratched move")+
			" to another flight, so "+map[bool]string{true: "their check-in clears", false: "their check-ins clear"}[len(moved) == 1]+
			" to be done again there: "+someOf(moved)+".")
	}
	return out
}

// movedFlight are the entries in another flight in next than in published:
// gymnasts moved by hand from one flight to another (moving or redrawing a
// whole flight keeps its gymnasts in it). Their check-in is for the flight
// they were in, so it clears when the move is published (the owner,
// 2026-10-19): they're checked in again at the new one.
func movedFlight(published, next competitions.Schedule) []string {
	var out []string
	for _, f := range published.Flights {
		k := competitions.FlightKey(f)
		for _, id := range f.Entries {
			if i, in := next.Find(id); in && competitions.FlightKey(next.Flights[i]) != k {
				out = append(out, id)
			}
		}
	}
	return out
}

// someOf lists the first few names, and how many more.
func someOf(names []string) string {
	const few = 3
	if len(names) <= few {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:few], ", ") + " and " + views.Count(len(names)-few, "more", "more")
}

// askDayMarks shows what a change to the timetable would do to what's been
// marked on the day (marks), and the change anyway (button), and notes that
// it only asked: nothing changed, nothing to record.
func (p *competitionPages) askDayMarks(w http.ResponseWriter, r *http.Request, c store.Competition, what, title string, marks []string, button, help string) {
	noteAsked(w)
	render(w, r, views.ChangeCheck(views.ChangeCheckPage{
		Base: adminPath(r.PathValue("token")), Competition: summary(c, p.now()), Menu: p.menuFor(r, c, menuTimetable),
		What: what, DayTitle: title, DayMarks: marks, Anyway: button, Help: help,
		Action: r.URL.Path, Fields: postedFields(r), Back: timetablePath(r),
	}))
}
