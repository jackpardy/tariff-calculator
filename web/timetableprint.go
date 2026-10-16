package web

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/requirements"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The timetable's printed sheets: marshal, chair of judges, rota, timeline
// and the recorders' score sheets.

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
			Title: "Officials rota · " + c.Name, Back: timetablePath(r), Competition: summary(c, p.now()), People: rotaSheet(s, people),
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
		Title: title, Back: timetablePath(r), Competition: summary(c, p.now()), Judges: judges,
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
						status := arrived[competitions.FlightKey(s.Flights[it.Index])][it.Seats[i].Person].Status
						it.Seats[i].Here, it.Seats[i].Missing = status == store.OfficialHere, status == store.OfficialMissing
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
		cards, err := p.judgedOf(r.Context(), c.ID, true)
		if err != nil {
			failed(w, r, err)
			return
		}
		for _, j := range cards {
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
				// A voluntary's difficulty, said as one, not a bare number.
				if checked.Checks.ScoreDifficulty && len(j.Entry.Entry.Exercises[i].Skills) > 0 {
					ex[i] = "Difficulty " + ex[i]
				}
			}
			sheets.Exercises[j.ID] = ex
			var notes []string
			// What the problems are, not just how many: the first, and a count of the rest.
			if n := len(j.problems); n > 0 {
				note := j.problems[0]
				if n > 1 {
					note += fmt.Sprintf(" (and %d more)", n-1)
				}
				notes = append(notes, note)
			}
			if j.Checked() {
				notes = append(notes, "card checked")
			}
			sheets.Notes[j.ID] = strings.Join(notes, " · ")
		}
	}
	render(w, r, views.TimetablePrint(sheets))
}

// scoreSheets are the recorders' score sheets: each flight in time order,
// its gymnasts in running order, and a column for each mark its panel gives
// each routine, all left to fill in.
// With a person's key, only the flights they have a seat on.
// Scratched gymnasts (entry ids) are struck through.
func scoreSheets(c store.Competition, s competitions.Schedule, entries []store.Entry, people []competitions.RotaPerson, scratched map[string]bool, back string, now time.Time, key string) views.ScoreSheets {
	out := views.ScoreSheets{Title: "Score sheets · " + c.Name, Back: back, BackLabel: "← Timetable", Competition: summary(c, now)}
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
