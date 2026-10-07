package main

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// The panel timeline (roadmap, competitions 3): every area's day side by side,
// time running down at an even scale, each flight and blocked time; or each
// area with a column for each seat on its panel and who has it. Printed, or
// downloaded as CSV.

// timelineItem is a flight or blocked time on one area, as the timeline needs
// it.
type timelineItem struct {
	day        int
	area       string
	start, end int
	flight     bool
	name       string
	gymnasts   int
	officials  []competitions.Duty
}

// timelineItems are every placed flight and block, area by area: a block on
// several areas is on each.
func timelineItems(s competitions.Schedule) []timelineItem {
	var out []timelineItem
	for _, f := range s.Flights {
		out = append(out, timelineItem{day: f.Day, area: f.Area, start: f.Start, end: f.End, flight: true, name: f.Name(), gymnasts: len(f.Entries), officials: f.Officials})
	}
	for _, b := range s.Blocks {
		for _, a := range b.Areas {
			out = append(out, timelineItem{day: b.Day, area: a, start: b.Start, end: b.End, name: b.Name, officials: b.Officials})
		}
	}
	return out
}

// dayAreas are the areas a day uses, in the setup's order.
func dayAreas(s competitions.Setup, day competitions.Day) []string {
	var out []string
	for _, a := range s.Areas {
		if len(day.Areas) == 0 || slices.Contains(day.Areas, a.Name) {
			out = append(out, a.Name)
		}
	}
	return out
}

// seatLabels are the timeline's names for seats: "Chair", "D1", "HD2", "E6".
var seatLabels = map[string]string{
	competitions.RoleChair: "Chair", competitions.RoleDifficulty: "D", competitions.RoleHD: "HD", competitions.RoleSync: "S",
	competitions.RoleExecution: "E", competitions.RoleRecorder: "Rec", competitions.RoleMarshal: "Mar",
}

// seatLabel names a role's nth seat (0-based) of how many the area has.
func seatLabel(role string, n, of int) string {
	if of == 1 && role != competitions.RoleDifficulty && role != competitions.RoleHD && role != competitions.RoleSync && role != competitions.RoleExecution {
		return seatLabels[role]
	}
	return seatLabels[role] + strconv.Itoa(n+1)
}

// timelineHeaderRows are the rows above the minutes: the day, the area, and
// with officials each seat.
func timelineHeaderRows(officials bool) int {
	if officials {
		return 3
	}
	return 2
}

// timeline lays the schedule out on a time scale: a row for every minute from
// the earliest day's start to the latest day's end, so every area and day
// lines up by time. Without officials, each day is a sheet with a column per
// area; with them, each day's area is a sheet with a column for what's on and
// one for each seat on its panel, a name to a cell.
func timeline(s competitions.Schedule, name func(key string) string, officials bool) views.Timeline {
	items := timelineItems(s)
	// The minutes shown: from the earliest start to the latest end, of every
	// day side by side; with officials, a sheet to a page, of its own day.
	span := func(day int) (first, last int) {
		first, last = -1, -1
		widen := func(start, end int) {
			if first < 0 || start < first {
				first = start
			}
			last = max(last, end)
		}
		for d, dd := range s.Setup.Days {
			if day < 0 || d == day {
				widen(dd.Hours())
			}
		}
		for _, it := range items {
			if day < 0 || it.day == day {
				widen(it.start, it.end)
			}
		}
		return first, last
	}
	t := views.Timeline{Officials: officials, Header: timelineHeaderRows(officials)}
	header := t.Header
	first, last := span(-1)
	if first < 0 || last <= first {
		return t
	}
	row := func(minute int) int { return header + minute - first + 1 }

	// The sheet's time column: each hour labelled, and a line across it.
	frame := func(sh *views.TimelineSheet, dayStart, dayEnd int) {
		sh.Minutes = last - first
		for h := (first + 59) / 60 * 60; h < last; h += 60 {
			sh.Hours = append(sh.Hours, views.TimelineHour{Row: row(h), Rows: min(60, last-h), Label: competitions.Clock(h)})
		}
		if first < dayStart {
			sh.Off = append(sh.Off, views.TimelineSpan{Row: row(first), Rows: dayStart - first})
		}
		if dayEnd < last {
			sh.Off = append(sh.Off, views.TimelineSpan{Row: row(dayEnd), Rows: last - dayEnd})
		}
	}

	for d, day := range s.Setup.Days {
		dayStart, dayEnd := day.Hours()
		if officials {
			first, last = span(d)
		}
		var sheet *views.TimelineSheet
		newSheet := func(title string) {
			t.Sheets = append(t.Sheets, views.TimelineSheet{Day: day.Name, Title: title, Columns: "var(--tl-time)"})
			sheet = &t.Sheets[len(t.Sheets)-1]
			frame(sheet, dayStart, dayEnd)
		}
		if !officials {
			newSheet(day.Name)
		}
		for _, area := range dayAreas(s.Setup, day) {
			if officials {
				newSheet(day.Name + " · " + area)
			}
			var mine []timelineItem
			for _, it := range items {
				if it.day == d && it.area == area {
					mine = append(mine, it)
				}
			}
			slices.SortStableFunc(mine, func(a, b timelineItem) int { return a.start - b.start })

			// The area's seats: as many of each role as any of its items has.
			seats := map[string]int{}
			if officials {
				for _, it := range mine {
					n := map[string]int{}
					for _, duty := range it.officials {
						n[duty.Role]++
						seats[duty.Role] = max(seats[duty.Role], n[duty.Role])
					}
				}
			}
			col := 2 // after the time column
			for _, a := range sheet.Areas {
				col += a.Columns
			}
			av := views.TimelineArea{Name: area, Column: col, Columns: 1}
			seatCol := map[string]int{} // "role n" → column
			sheet.Columns += " var(--tl-event)"
			for _, role := range competitions.Roles {
				for n := range seats[role] {
					seatCol[fmt.Sprintf("%s %d", role, n)] = col + av.Columns
					av.Seats = append(av.Seats, views.TimelineSeat{Label: seatLabel(role, n, seats[role]), Title: competitions.RoleName(role)})
					av.Columns++
					sheet.Columns += " var(--tl-seat)"
				}
			}
			for i, it := range mine {
				overlaps := i > 0 && it.start < mine[i-1].end || i+1 < len(mine) && mine[i+1].start < it.end
				kind := "block"
				if it.flight {
					kind = "flight"
				}
				cell := views.TimelineCell{
					Row: row(it.start), Rows: max(1, it.end-it.start), Column: col, Columns: 1, Kind: kind, Overlaps: overlaps,
					Name: it.name, Time: competitions.Clock(it.start) + "–" + competitions.Clock(it.end), Flight: it.flight, Gymnasts: it.gymnasts,
				}
				if len(it.officials) == 0 {
					cell.Columns = av.Columns // blocked time with no officials takes the whole area
				}
				av.Cells = append(av.Cells, cell)
				n := map[string]int{}
				for _, duty := range it.officials {
					c, ok := seatCol[fmt.Sprintf("%s %d", duty.Role, n[duty.Role])]
					n[duty.Role]++
					if !ok {
						continue
					}
					who := name(duty.Person)
					if duty.Person == "" {
						who = "—"
					}
					av.Cells = append(av.Cells, views.TimelineCell{Row: cell.Row, Rows: cell.Rows, Column: c, Columns: 1, Kind: "seat", Name: who,
						Empty: duty.Person == "", Overlaps: overlaps, Time: competitions.RoleName(duty.Role) + " · " + it.name + " · " + cell.Time})
				}
			}
			sheet.Areas = append(sheet.Areas, av)
		}
	}
	return t
}

// officialNames name officials by their key: the rota's people first, then
// anyone competing.
func officialNames(people []competitions.RotaPerson, names map[string]string, withClub bool) func(string) string {
	byKey := map[string]competitions.RotaPerson{}
	for _, o := range people {
		byKey[o.Key] = o
	}
	return func(key string) string {
		if o, ok := byKey[key]; ok {
			if withClub && o.Club != "" {
				return o.Name + " (" + o.Club + ")"
			}
			return o.Name
		}
		if n := names[key]; n != "" {
			return n
		}
		return "?"
	}
}

// printTimeline is the panel timeline, to print: without officials
// (sheet=timeline), or with a column for each seat (sheet=timeline-officials).
func printTimeline(w http.ResponseWriter, r *http.Request, c store.Competition, s competitions.Schedule, entries []store.Entry, people []competitions.RotaPerson, now time.Time, officials bool) {
	_, names := peopleOf(entries)
	t := timeline(s, officialNames(people, names, false), officials)
	t.Title, t.Back, t.CSV = "Panel timeline · "+c.Name, timetablePath(r), timetablePath(r)+"/timeline.csv"
	t.Competition = summary(c.Competition, now)
	t.Other, t.OtherLabel = timetablePath(r)+"/print?sheet=timeline-officials", "With officials"
	if officials {
		t.Title = "Timeline with officials · " + c.Name
		t.Other, t.OtherLabel = timetablePath(r)+"/print?sheet=timeline", "Without officials"
	}
	render(w, r, views.TimelinePrint(t))
}

// timelineCSV is the panel timeline as CSV: a row per flight or blocked time
// on each area, in time order, with who officiates it, by role.
func (p *competitionPages) timelineCSV(w http.ResponseWriter, r *http.Request) {
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
	_, names := peopleOf(entries)
	name := officialNames(people, names, true)
	items := timelineItems(s)
	areaAt := map[string]int{}
	for i, a := range s.Setup.Areas {
		areaAt[a.Name] = i
	}
	slices.SortStableFunc(items, func(a, b timelineItem) int {
		if a.day != b.day {
			return a.day - b.day
		}
		if a.start != b.start {
			return a.start - b.start
		}
		return areaAt[a.area] - areaAt[b.area]
	})
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName(c.Name)+" timeline.csv"))
	out := csv.NewWriter(w)
	header := []string{"Day", "Area", "Starts", "Ends", "What", "Gymnasts"}
	for _, role := range competitions.Roles {
		header = append(header, competitions.RoleName(role))
	}
	out.Write(header)
	for _, it := range items {
		day := ""
		if it.day >= 0 && it.day < len(s.Setup.Days) {
			day = s.Setup.Days[it.day].Name
		}
		gymnasts := ""
		if it.flight {
			gymnasts = strconv.Itoa(it.gymnasts)
		}
		row := []string{csvSafe(day), csvSafe(it.area), competitions.Clock(it.start), competitions.Clock(it.end), csvSafe(it.name), gymnasts}
		byRole := map[string][]string{}
		for _, d := range it.officials {
			who := "—"
			if d.Person != "" {
				who = name(d.Person)
			}
			byRole[d.Role] = append(byRole[d.Role], who)
		}
		for _, role := range competitions.Roles {
			row = append(row, csvSafe(strings.Join(byRole[role], "; ")))
		}
		out.Write(row)
	}
	out.Flush()
}
