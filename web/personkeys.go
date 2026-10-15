package web

import (
	"strings"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

// Who is who: person keys, and the people behind them across roles (ADR
// 0010), as the timetable, rota and day pages need them.

// live are a competition's entries that count: not withdrawn.
func live(entries []store.Entry) []store.Entry {
	var out []store.Entry
	for _, e := range entries {
		if !e.Withdrawn && e.Waiting == 0 {
			out = append(out, e)
		}
	}
	return out
}

// personKeys are each entry's people, by entry id, matched across entries:
// members by member, and individuals by person (ADR 0005 Decision 4, ADR
// 0010), their entries all keyed alike in a competition.
func personKeys(entries []store.Entry) map[string][]string {
	out := map[string][]string{}
	for _, e := range entries {
		out[e.ID] = e.People()
	}
	return out
}

// samePeople says which person keys are one person (ADR 0010: a member who
// also coaches, or also enters on their own), for working out clashes and
// rest: each key of someone with more than one in use maps to the one that
// stands for them all. Saved keys stay as they are; only the working out
// uses it.
type samePeople map[string]string

// of is the key standing for a key's person.
func (sp samePeople) of(key string) string {
	if k, ok := sp[key]; ok {
		return k
	}
	return key
}

// all are keys as their people's.
func (sp samePeople) all(keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = sp.of(k)
	}
	return out
}

// newSamePeople works out who's the same person among the keys in use, from
// groups (key to person, store.PersonGroups): the entries' gymnasts and
// coaches, and the officials. Someone who officiates is stood for by their
// official's key, which the rota's seats are saved under, so their seats and
// their entries are seen as one person's; anyone else by their first key, a
// member's before an individual's before a coach's.
func newSamePeople(groups map[string]string, entries []store.Entry, officials []competitions.RotaPerson) samePeople {
	official := map[string]bool{}
	var inUse []string
	for _, o := range officials {
		official[o.Key] = true
		inUse = append(inUse, o.Key)
	}
	for _, e := range entries {
		inUse = append(inUse, e.People()...)
		if k := e.CoachKey(); k != "" {
			inUse = append(inUse, k)
		}
	}
	rank := func(k string) int {
		if official[k] {
			return 0
		}
		kind, _ := store.KeyKind(k)
		return 1 + strings.Index(store.KindMember+store.KindIndividual+store.KindCoach, kind)
	}
	stand := map[string]string{} // person → the key standing for them
	for _, k := range inUse {
		person, ok := groups[k]
		if !ok {
			continue
		}
		if s, seen := stand[person]; !seen || rank(k) < rank(s) || rank(k) == rank(s) && k < s {
			stand[person] = k
		}
	}
	out := samePeople{}
	for _, k := range inUse {
		if person, ok := groups[k]; ok && stand[person] != k {
			out[k] = stand[person]
		}
	}
	return out
}

// clashPeople are each entry's people and their names, as peopleOf has them,
// keyed as their people (same) for working out clashes and rest, with names
// for those keys too.
func clashPeople(entries []store.Entry, same samePeople) (map[string][]string, map[string]string) {
	people, names := peopleOf(entries)
	for id, keys := range people {
		people[id] = same.all(keys)
	}
	for k, n := range names {
		if s := same.of(k); names[s] == "" {
			names[s] = n
		}
	}
	return people, names
}

// coachesOf are each entry's coaches, by entry id, keyed as their people
// (same).
func coachesOf(entries []store.Entry, same samePeople) map[string][]string {
	out := map[string][]string{}
	for _, e := range entries {
		if k := e.CoachKey(); k != "" {
			out[e.ID] = []string{same.of(k)}
		}
	}
	return out
}

// schedEntries are entries as the scheduler needs them, with their people
// and coaches keyed as their people (same).
func schedEntries(entries []store.Entry, same samePeople) []competitions.SchedEntry {
	keys := personKeys(entries)
	out := make([]competitions.SchedEntry, len(entries))
	for i, e := range entries {
		club := e.ClubName
		if e.Individual {
			club = ""
		}
		out[i] = competitions.SchedEntry{
			PlanEntry:  competitions.PlanEntry{ID: e.ID, Level: e.Entry.Event(), Category: e.Entry.Category, Club: club},
			Discipline: e.Entry.Discipline, People: same.all(keys[e.ID]),
		}
		if k := e.CoachKey(); k != "" {
			out[i].Coaches = []string{same.of(k)}
		}
	}
	return out
}

// peopleOf are each entry's people, and each person's name.
func peopleOf(entries []store.Entry) (map[string][]string, map[string]string) {
	people, names := personKeys(entries), map[string]string{}
	for _, e := range entries {
		ps := people[e.ID]
		names[ps[0]] = e.Entry.Gymnast
		if len(ps) > 1 && e.Entry.Partner != nil {
			names[ps[1]] = e.Entry.Partner.Name
		}
		if k := e.CoachKey(); k != "" {
			names[k] = e.CoachName()
		}
	}
	return people, names
}
