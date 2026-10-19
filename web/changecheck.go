package web

import (
	"maps"
	"net/http"
	"slices"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
	"tariffCalculator/views"
)

// Saying what a manual change breaks, and how to fix it (roadmap 9). A change
// to the draft timetable by hand (moving an event or a gymnast, giving someone
// a seat) is tried on a copy first. If it breaks nothing that holds now, it's
// made. If it would, the organiser sees what, with other changes that don't
// (someone else free for the seat, another day or area, another flight), and
// can make it anyway.

// maxFixes is how many fixes a change offers at most.
const maxFixes = 3

// askFirst tries a manual change (try) on a copy of the draft s. If it would
// break something that holds now, it shows what, with the fixes fixes finds
// (given what's broken now, to check each against), and says true: the
// change waits for the organiser. With anyway=1, or a change that's refused
// anyway (try fails, saying why when it's made), it says false.
func (p *competitionPages) askFirst(w http.ResponseWriter, r *http.Request, c store.Competition, s competitions.Schedule, f rotaFacts, what string,
	try func(*competitions.Schedule) error, fixes func(clean func(func(*competitions.Schedule) bool) bool) []views.ChangeFix) bool {
	if r.FormValue("anyway") == "1" {
		return false
	}
	trial := s.Copy()
	if try(&trial) != nil {
		return false
	}
	before := f.manualBreaks(s)
	newly := competitions.NewBreaks(before, f.manualBreaks(trial))
	if len(newly) == 0 {
		return false
	}
	// clean says whether a change, tried on a copy, breaks nothing new.
	clean := func(change func(*competitions.Schedule) bool) bool {
		t := s.Copy()
		return change(&t) && len(competitions.NewBreaks(before, f.manualBreaks(t))) == 0
	}
	page := views.ChangeCheckPage{
		Base: adminPath(r.PathValue("token")), Competition: summary(c, p.now()), Menu: p.menuFor(r, c, menuTimetable),
		What: what, Breaks: competitions.BreakLines(newly), Action: r.URL.Path, Fields: postedFields(r), Back: timetablePath(r),
	}
	if fixes != nil {
		page.Fixes = fixes(clean)
	}
	render(w, r, views.ChangeCheck(page))
	return true
}

// postedFields are the form as posted, but anyway: to post it again.
func postedFields(r *http.Request) []views.FormValue {
	var out []views.FormValue
	for _, k := range slices.Sorted(maps.Keys(r.PostForm)) {
		if k == "anyway" {
			continue
		}
		for _, v := range r.PostForm[k] {
			out = append(out, views.FormValue{Name: k, Value: v})
		}
	}
	return out
}

// withField is fields with one changed (or added): a fix is the change
// asked for with another value.
func withField(fields []views.FormValue, name, value string) []views.FormValue {
	var out []views.FormValue
	set := false
	for _, f := range fields {
		if f.Name == name {
			if set {
				continue
			}
			f.Value, set = value, true
		}
		out = append(out, f)
	}
	if !set {
		out = append(out, views.FormValue{Name: name, Value: value})
	}
	return out
}
