package main

import (
	"encoding/csv"
	"fmt"
	"maps"
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
// time running down, each flight and blocked time with who officiates it.
// Printed, or downloaded as CSV.

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

// officialLines group a panel's seats by role, e.g. "Execution: A, B, C", with
// "—" for a seat no one could take.
func officialLines(duties []competitions.Duty, name func(key string) string) []string {
	byRole := map[string][]string{}
	for _, d := range duties {
		who := "—"
		if d.Person != "" {
			who = name(d.Person)
		}
		byRole[d.Role] = append(byRole[d.Role], who)
	}
	var out []string
	for _, r := range competitions.Roles {
		if names := byRole[r]; len(names) > 0 {
			out = append(out, roleShort[r]+": "+strings.Join(names, ", "))
		}
	}
	return out
}

// roleShort are the roles as the timeline labels them.
var roleShort = map[string]string{
	competitions.RoleChair: "Chair", competitions.RoleDifficulty: "Difficulty", competitions.RoleHD: "HD",
	competitions.RoleExecution: "Execution", competitions.RoleRecorder: "Recorder", competitions.RoleMarshal: "Marshal",
}

// timeline lays the schedule out as a table: a column per day's area, and a
// row from each time anything starts or ends (and each hour) to the next, so
// areas line up by time. An item spans the rows it runs over; items that
// overlap on one area (moved by hand) share a cell.
func timeline(s competitions.Schedule, name func(key string) string) ([]views.TimelineDay, []views.TimelineRow) {
	items := timelineItems(s)
	var days []views.TimelineDay
	times := map[int]bool{}
	type column struct {
		day        int
		area       string
		start, end int // the day's hours
	}
	var columns []column
	for d, day := range s.Setup.Days {
		start, end := day.Hours()
		times[start], times[end] = true, true
		areas := dayAreas(s.Setup, day)
		days = append(days, views.TimelineDay{Name: day.Name, Areas: areas})
		for _, a := range areas {
			columns = append(columns, column{d, a, start, end})
		}
	}
	for _, it := range items {
		times[it.start], times[it.end] = true, true
	}
	if len(times) == 0 {
		return days, nil
	}
	breaks := slices.Sorted(maps.Keys(times))
	for h := (breaks[0]/60 + 1) * 60; h < breaks[len(breaks)-1]; h += 60 {
		times[h] = true
	}
	breaks = slices.Sorted(maps.Keys(times))
	row := func(t int) int { i, _ := slices.BinarySearch(breaks, t); return i }

	rows := make([]views.TimelineRow, len(breaks)-1)
	for i := range rows {
		rows[i] = views.TimelineRow{Time: competitions.Clock(breaks[i]), Hour: breaks[i]%60 == 0}
	}
	for _, col := range columns {
		// The column's items, merged where they overlap.
		type cluster struct {
			start, end int
			items      []timelineItem
		}
		var clusters []cluster
		var mine []timelineItem
		for _, it := range items {
			if it.day == col.day && it.area == col.area {
				mine = append(mine, it)
			}
		}
		slices.SortStableFunc(mine, func(a, b timelineItem) int { return a.start - b.start })
		for _, it := range mine {
			if n := len(clusters); n > 0 && it.start < clusters[n-1].end {
				clusters[n-1].items = append(clusters[n-1].items, it)
				clusters[n-1].end = max(clusters[n-1].end, it.end)
				continue
			}
			clusters = append(clusters, cluster{it.start, it.end, []timelineItem{it}})
		}
		starts := map[int]cluster{}
		for _, c := range clusters {
			starts[row(c.start)] = c
		}
		off := func(i int) bool { return breaks[i] < col.start || breaks[i] >= col.end }
		for i := 0; i < len(rows); {
			if c, ok := starts[i]; ok {
				cell := views.TimelineCell{Span: max(1, row(c.end)-i), Kind: "block"}
				for _, it := range c.items {
					if it.flight {
						cell.Kind = "flight"
					}
					cell.Items = append(cell.Items, views.TimelineItem{
						Name: it.name, Time: competitions.Clock(it.start) + "–" + competitions.Clock(it.end),
						Gymnasts: it.gymnasts, Flight: it.flight, Officials: officialLines(it.officials, name),
					})
				}
				rows[i].Cells = append(rows[i].Cells, cell)
				i += cell.Span
				continue
			}
			// A gap: free time, or outside the day's hours, until something starts.
			j := i + 1
			for j < len(rows) && off(j) == off(i) {
				if _, ok := starts[j]; ok {
					break
				}
				j++
			}
			kind := ""
			if off(i) {
				kind = "off"
			}
			rows[i].Cells = append(rows[i].Cells, views.TimelineCell{Span: j - i, Kind: kind})
			i = j
		}
	}
	return days, rows
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

// printTimeline is the panel timeline, to print.
func printTimeline(w http.ResponseWriter, r *http.Request, c store.Competition, s competitions.Schedule, entries []store.Entry, people []competitions.RotaPerson, now time.Time) {
	_, names := peopleOf(entries)
	days, rows := timeline(s, officialNames(people, names, false))
	render(w, r, views.TimelinePrint(views.Timeline{
		Title: "Panel timeline · " + c.Name, Back: timetablePath(r), CSV: timetablePath(r) + "/timeline.csv",
		Competition: summary(c.Competition, now), Days: days, Rows: rows,
	}))
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
