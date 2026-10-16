package web

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
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
	slices.SortFunc(names, competitions.CompareNames)
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

// The values of the "What they do" filter on all people.
const (
	doesGymnast  = "gymnast"
	doesOfficial = "official"
	doesCoach    = "coach"
)

// peopleDoFilter is a filter by what someone does at the competition.
func peopleDoFilter() views.ListFilter {
	return views.ListFilter{Key: "does", Label: "What they do", Any: "Anything", Options: []views.ListOption{
		{Value: doesGymnast, Label: "Gymnast"}, {Value: doesOfficial, Label: "Official"}, {Value: doesCoach, Label: "Coach"},
	}}
}

// peopleDo is what someone does, as peopleDoFilter's values, from their
// roles (gymnast, judge, chair, recorder, marshal, coach): judging,
// chairing, recording and marshalling all make an official.
func peopleDo(roles []string) []string {
	var out []string
	if slices.Contains(roles, "gymnast") {
		out = append(out, doesGymnast)
	}
	if slices.ContainsFunc(roles, func(r string) bool { return r != "gymnast" && r != "coach" }) {
		out = append(out, doesOfficial)
	}
	if slices.Contains(roles, "coach") {
		out = append(out, doesCoach)
	}
	return out
}

// noCoach is the coach filter's value for a member with no coach.
const noCoach = "-"

// memberCoachFilter is a filter by a club member's coach: each of the club's
// coaches, then "No coach".
func memberCoachFilter(coaches []views.CoachOption) views.ListFilter {
	f := views.ListFilter{Key: "coach", Label: "Coach", Any: "Any coach"}
	for _, c := range coaches {
		f.Options = append(f.Options, views.ListOption{Value: c.ID, Label: c.Name})
	}
	f.Options = append(f.Options, views.ListOption{Value: noCoach, Label: "No coach"})
	return f
}

// memberCoachValue is a member's value for the coach filter.
func memberCoachValue(coachID string) string {
	if coachID == "" {
		return noCoach
	}
	return coachID
}

// hasFilter is a filter by whether a row has something (entries,
// qualifications): has is the label of the rows that do, none of those that
// don't.
func hasFilter(key, label, any, has, none string) views.ListFilter {
	return views.ListFilter{Key: key, Label: label, Any: any, Options: []views.ListOption{
		{Value: "yes", Label: has}, {Value: "no", Label: none},
	}}
}

// hasValue is a row's value for a hasFilter.
func hasValue(has bool) string {
	if has {
		return "yes"
	}
	return "no"
}

// coachEntryFilters are the filters on a coach's entries: the competition
// (when there is more than one), each level once, by name,
// whether the entry has problems, and where its sign-off stands.
func coachEntryFilters(competitionOptions []views.ListOption, levels []string) []views.ListFilter {
	var out []views.ListFilter
	if len(competitionOptions) > 1 {
		out = append(out, views.ListFilter{Key: "competition", Label: "Competition", Any: "Every competition", Options: competitionOptions})
	}
	level := views.ListFilter{Key: "level", Label: "Level", Any: "Every level"}
	for _, l := range levels {
		if !slices.ContainsFunc(level.Options, func(o views.ListOption) bool { return o.Value == l }) {
			level.Options = append(level.Options, views.ListOption{Value: l, Label: l})
		}
	}
	slices.SortFunc(level.Options, func(a, b views.ListOption) int { return cmp.Compare(a.Label, b.Label) })
	return append(out, level, hasFilter("problems", "Problems", "Problems or not", "Has problems", "No problems"),
		views.ListFilter{Key: "signoff", Label: "Sign-off", Any: "Any sign-off", Options: []views.ListOption{
			{Value: "signed", Label: "Signed off"}, {Value: "not-yet", Label: "Needs changes"}, {Value: "waiting", Label: "Waiting"},
		}})
}

// coachSignoffValue is an entry's value for the sign-off filter, matching
// what the coach's table says: signed off, not yet (the coach asked for
// changes) or waiting.
func coachSignoffValue(s views.SignoffView) string {
	switch {
	case s.Signed:
		return "signed"
	case s.By != "":
		return "not-yet"
	}
	return "waiting"
}

// coachStatusFilter is a filter by where a coach stands with the organiser's
// approval.
func coachStatusFilter() views.ListFilter {
	return views.ListFilter{Key: "status", Label: "Status", Any: "Any status", Options: []views.ListOption{
		{Value: "approved", Label: "Approved"}, {Value: "waiting", Label: "Waiting"},
		{Value: "refused", Label: "Not approved"}, {Value: "withdrawn", Label: "Approval withdrawn"},
	}}
}

// coachStatusValue is a coach's value for the status filter, from their
// store status.
func coachStatusValue(status string) string {
	switch status {
	case store.CoachApproved:
		return "approved"
	case store.CoachRefused:
		return "refused"
	case store.CoachWithdrawn:
		return "withdrawn"
	}
	return "waiting"
}
