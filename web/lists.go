package web

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/views"
)

// Search and filters on lists of people (roadmap: search and filter every
// list of people): the choices each list's filters offer, built the same way
// on every list. views/lists.templ draws them.

// noClub is the club filter's value for someone with no club.
const noClub = "-"

// clubFilter is a filter by club, offering each of the clubs, by name, and
// "No club" if anyone has none.
func clubFilter(clubs []string) views.ListFilter {
	f := views.ListFilter{Key: "club", Label: "Club", Any: "Every club"}
	none := false
	seen := map[string]bool{}
	var names []string
	for _, c := range clubs {
		switch {
		case c == "":
			none = true
		case !seen[c]:
			seen[c] = true
			names = append(names, c)
		}
	}
	slices.SortFunc(names, func(a, b string) int { return cmp.Compare(strings.ToLower(a), strings.ToLower(b)) })
	for _, n := range names {
		f.Options = append(f.Options, views.ListOption{Value: n, Label: n})
	}
	if none {
		f.Options = append(f.Options, views.ListOption{Value: noClub, Label: "No club"})
	}
	return f
}

// clubValue is a row's value for the club filter.
func clubValue(club string) string {
	if club == "" {
		return noClub
	}
	return club
}

// The values of the "Can do" filter on officials.
const (
	canJudge    = "judge"
	canChair    = "chair"
	canRecorder = "recorder"
	canMarshal  = "marshal"
)

// canDoFilter is a filter by what someone offers to do.
func canDoFilter() views.ListFilter {
	return views.ListFilter{Key: "can", Label: "Can do", Any: "Can do anything", Options: []views.ListOption{
		{Value: canJudge, Label: "Judge"}, {Value: canChair, Label: "Chair"},
		{Value: canRecorder, Label: "Recorder"}, {Value: canMarshal, Label: "Marshal"},
	}}
}

// canDo is what an offer offers to do, as canDoFilter's values.
func canDo(o competitions.Offer) []string {
	var out []string
	if len(o.Judge) > 0 {
		out = append(out, canJudge)
	}
	for _, j := range o.Judge {
		if j.Chair {
			out = append(out, canChair)
			break
		}
	}
	if o.Recorder {
		out = append(out, canRecorder)
	}
	if o.Marshal {
		out = append(out, canMarshal)
	}
	return out
}

// judgesFilter is a filter by the discipline and level someone can judge: a
// group for each discipline, offering any level or each level, easiest first.
// A level matches everyone who judges up to it or a harder one.
func judgesFilter(c competitions.Competition) views.ListFilter {
	f := views.ListFilter{Key: "judges", Label: "Can judge", Any: "Judges any event", AtLeast: true}
	for _, d := range c.Disciplines() {
		group := competitions.DisciplineName(d)
		f.Options = append(f.Options, views.ListOption{Value: offerKey(d) + ":0", Label: "Any " + strings.ToLower(group) + " level", Group: group})
		for i, l := range c.LevelNames(d) {
			f.Options = append(f.Options, views.ListOption{Value: offerKey(d) + ":" + strconv.Itoa(i+1), Label: l, Group: group})
		}
	}
	return f
}

// judges is what an offer judges, as judgesFilter's values: each discipline
// with the place of the hardest level they judge, easiest first. Up to no
// level, or one the competition no longer has, is every level.
func judges(c competitions.Competition, o competitions.Offer) []string {
	var out []string
	for _, d := range c.Disciplines() {
		j, ok := o.Judge[d]
		if !ok {
			continue
		}
		levels := c.LevelNames(d)
		top := len(levels)
		if i := slices.Index(levels, j.UpTo); i >= 0 {
			top = i + 1
		}
		out = append(out, offerKey(d)+":"+strconv.Itoa(top))
	}
	return out
}

// qualifiedFilter is a filter by whether the organiser has marked someone
// qualified.
func qualifiedFilter() views.ListFilter {
	return views.ListFilter{Key: "qualified", Label: "Qualified", Any: "Qualified or not", Options: []views.ListOption{
		{Value: "yes", Label: "Qualified"}, {Value: "no", Label: "Not qualified"},
	}}
}

// qualifiedValue is a row's value for the qualified filter.
func qualifiedValue(on bool) string {
	if on {
		return "yes"
	}
	return "no"
}
