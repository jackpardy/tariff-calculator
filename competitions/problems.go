package competitions

import "fmt"

// Entered is an entry as its problems are worked out: its card, its people
// by person key (the gymnast's, then a synchro partner's), and the problem
// with its coach's sign-off ("" for none, or none needed).
type Entered struct {
	Entry   Entry
	People  []string
	Signoff string
}

// EntryProblems are an entry's problems, worked out the same for every page
// that shows them (the organiser's, the club's and the coach's): a synchro
// pair's level against their individual trampoline levels among all (the
// competition's entries that count), then why its card can't be checked or
// the card's problems, then its sign-off's.
func (c Competition) EntryProblems(e Entered, all []Entered) []string {
	var out []string
	if problem := c.PairLevel(e, all); problem != "" {
		out = append(out, problem)
	}
	card, err := c.Check(e.Entry)
	if err != nil {
		return append(out, err.Error())
	}
	out = append(out, card.Problems()...)
	if e.Signoff != "" {
		out = append(out, e.Signoff)
	}
	return out
}

// PairLevel checks a synchro pair's level against their individual
// trampoline levels among all, by person key: the same level, or the easier
// of two a level apart; further apart, they usually can't pair. It's "" for
// anything else, or nothing to say.
func (c Competition) PairLevel(e Entered, all []Entered) string {
	if e.Entry.Discipline != Synchro || len(e.People) < 2 || e.Entry.Partner == nil {
		return ""
	}
	var a, b string
	for _, o := range all {
		if o.Entry.Discipline != Trampoline || len(o.People) == 0 {
			continue
		}
		// The last entry of each person's counts, as before.
		switch o.People[0] {
		case e.People[0]:
			a = o.Entry.Level
		case e.People[1]:
			b = o.Entry.Level
		}
	}
	if a == "" || b == "" {
		return ""
	}
	pair := fmt.Sprintf("%s (%s) and %s (%s)", e.Entry.Gymnast, a, e.Entry.Partner.Name, b)
	switch want, ok := c.SynchroLevel(a, b); {
	case !ok:
		return pair + " compete more than a level apart individually, so usually can't pair in synchro"
	case want != e.Entry.DoesLevel():
		return fmt.Sprintf("%s compete individually, so as a pair they do %s in synchro, not %s", pair, want, e.Entry.DoesLevel())
	}
	return ""
}
