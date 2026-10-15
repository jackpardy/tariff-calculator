package web

import (
	"context"
	"maps"
	"slices"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

// Which flights are a person's or a club's, as their timetable and their
// calendar pick them out: one rule for both.

// inSlot are the entries (by id) of a flight or block that a person (key)
// competes in, as a gymnast or synchro partner (people: each entry's), and
// the seats they hold on its panel.
func inSlot(key string, people map[string][]string, entries []string, seats []competitions.Duty) ([]string, []competitions.Duty) {
	var in []string
	var held []competitions.Duty
	for _, id := range entries {
		if slices.Contains(people[id], key) {
			in = append(in, id)
		}
	}
	for _, d := range seats {
		if d.Person == key {
			held = append(held, d)
		}
	}
	return in, held
}

// ourClub is a club at a competition, as its timetable and calendar pick out
// its flights.
type ourClub struct {
	id, name string
	people   map[string]bool        // its people, by key
	entries  map[string]store.Entry // the live entries, by id (from at)
}

// clubPeople is a club and its members, by key.
func (p *competitionPages) clubPeople(ctx context.Context, clubID string) (ourClub, error) {
	name, err := p.st.ClubName(ctx, clubID)
	if err != nil {
		return ourClub{}, err
	}
	members, err := p.st.Members(ctx, clubID)
	if err != nil {
		return ourClub{}, err
	}
	c := ourClub{id: clubID, name: name, people: map[string]bool{}}
	for _, m := range members {
		c.people[store.MemberKey(m.ID)] = true
	}
	return c, nil
}

// at is the club at the competition f is of: its people with the
// organiser's own officials from it (by its name), and the live entries.
func (c ourClub) at(f rotaFacts) ourClub {
	c.people = maps.Clone(c.people)
	for _, o := range f.officials {
		if o.Club == c.name {
			c.people[o.Key] = true
		}
	}
	c.entries = entriesByID(f.entries)
	return c
}

// inSlot are the club's entries in a flight or block (entries, by id), and
// the seats its people hold on its panel.
func (c ourClub) inSlot(entries []string, seats []competitions.Duty) ([]store.Entry, []competitions.Duty) {
	var in []store.Entry
	var held []competitions.Duty
	for _, id := range entries {
		if e, ok := c.entries[id]; ok && e.ClubID == c.id {
			in = append(in, e)
		}
	}
	for _, d := range seats {
		if c.people[d.Person] {
			held = append(held, d)
		}
	}
	return in, held
}

// entriesByID are entries by id.
func entriesByID(entries []store.Entry) map[string]store.Entry {
	out := make(map[string]store.Entry, len(entries))
	for _, e := range entries {
		out[e.ID] = e
	}
	return out
}
