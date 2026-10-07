package competitions

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"tariffCalculator/requirements"
)

// Synchro events: a synchro event can pair levels, e.g. BUCS L1 with L2 as
// "BUCS L1/L2", flighted and ranked together, each pair doing one of them
// (Entry.Choice) and checked against it. Which one follows from the pair's
// individual levels: the same level if they compete at the same one, the
// easier of the two if they're a level apart; more than a level apart, they
// usually can't pair.

// Members are a synchro event's levels: the level, then any paired with it.
func (l Level) Members() []Level {
	head := l
	head.With = nil
	return append([]Level{head}, l.With...)
}

// eventName is the event's name: its level's, or its paired levels' names
// joined, e.g. "BUCS L1/L2", or "Club A / Club B" where they don't share
// their first words.
func (l Level) eventName() (string, error) {
	var names []string
	for _, m := range l.Members() {
		level, err := m.Resolve()
		if err != nil {
			return "", err
		}
		names = append(names, level.Name)
	}
	return joinLevelNames(names), nil
}

func joinLevelNames(names []string) string {
	i := strings.LastIndex(names[0], " ")
	if len(names) == 1 || i < 0 {
		return strings.Join(names, " / ")
	}
	prefix, tails := names[0][:i+1], []string{names[0][i+1:]}
	for _, n := range names[1:] {
		tail, ok := strings.CutPrefix(n, prefix)
		if !ok || strings.Contains(tail, " ") {
			return strings.Join(names, " / ")
		}
		tails = append(tails, tail)
	}
	return prefix + strings.Join(tails, "/")
}

// EntryLevel is the level an entry is checked against: its event's, or for a
// synchro event of paired levels, the one the pair does.
func (c Competition) EntryLevel(e Entry) (Level, requirements.Level, bool) {
	l, level, ok := c.LevelFor(e.Discipline, e.Level)
	if !ok || len(l.With) == 0 {
		return l, level, ok
	}
	for _, m := range l.Members() {
		if ml, err := m.Resolve(); err == nil && ml.Name == e.Choice {
			return m, ml, true
		}
	}
	return Level{}, requirements.Level{}, false
}

// checkChoice checks which of a paired synchro event's levels an entry does,
// and clears it for any other event.
func (c Competition) checkChoice(e *Entry) error {
	l, _, ok := c.LevelFor(e.Discipline, e.Level)
	if !ok || len(l.With) == 0 {
		e.Choice = ""
		return nil
	}
	names := MemberNames(l)
	if !slices.Contains(names, e.Choice) {
		return fmt.Errorf("%s pairs %s: choose which you're doing", e.Event(), joinList(names))
	}
	return nil
}

// MemberNames are a synchro event's levels' names.
func MemberNames(l Level) []string {
	var out []string
	for _, m := range l.Members() {
		if level, err := m.Resolve(); err == nil {
			out = append(out, level.Name)
		}
	}
	return out
}

// PairLevels groups synchro levels into events: each group names levels
// (built in, by name) to pair, the first heading the event. Levels in no
// group stay events of their own, in order.
func PairLevels(levels []Level, groups [][]string) ([]Level, error) {
	name := func(l Level) string {
		level, err := l.Resolve()
		if err != nil {
			return l.Ref
		}
		return level.Name
	}
	var flat []Level
	for _, l := range levels {
		flat = append(flat, l.Members()...)
	}
	at := map[string]int{}
	for i, l := range flat {
		at[name(l)] = i
	}
	head := map[int][]int{} // the level heading a group → the others
	grouped := map[int]bool{}
	var errs []error
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		var idx []int
		for _, n := range g {
			i, ok := at[n]
			switch {
			case !ok:
				errs = append(errs, fmt.Errorf("pairing synchro levels: %q isn't one of the synchro levels ticked", n))
			case grouped[i]:
				errs = append(errs, fmt.Errorf("pairing synchro levels: %q is paired twice", n))
			default:
				grouped[i] = true
				idx = append(idx, i)
			}
		}
		if len(idx) > 1 {
			head[idx[0]] = idx[1:]
		}
	}
	if len(errs) > 0 {
		return levels, errors.Join(errs...)
	}
	var out []Level
	for i, l := range flat {
		others, heads := head[i]
		if grouped[i] && !heads {
			continue
		}
		for _, j := range others {
			l.With = append(l.With, flat[j])
		}
		out = append(out, l)
	}
	return out, nil
}

// Pairings are the synchro events that pair levels, as PairLevels takes them.
func (c Competition) Pairings() [][]string {
	var out [][]string
	for _, l := range c.Synchro {
		if len(l.With) > 0 {
			out = append(out, MemberNames(l))
		}
	}
	return out
}

// SynchroLevel is the level a pair does in synchro from their individual
// trampoline levels: the same one, or the easier of two a level apart in the
// competition's order. ok is false if they're further apart.
func (c Competition) SynchroLevel(a, b string) (level string, ok bool) {
	order := c.levelNames(Trampoline)
	i, j := slices.Index(order, a), slices.Index(order, b)
	switch {
	case i < 0 || j < 0:
		return "", false
	case i == j:
		return a, true
	case i-j == 1 || j-i == 1:
		return order[min(i, j)], true
	}
	return "", false
}

// DoesLevel is the level a synchro entry does: its choice in a paired event,
// else its event's level.
func (e Entry) DoesLevel() string {
	if e.Choice != "" {
		return e.Choice
	}
	return e.Level
}

// SynchroEventOf is the synchro event a level is in, and whether it pairs
// levels: a level paired into "BUCS L1/L2" is in that event.
func (c Competition) SynchroEventOf(level string) (event string, paired, ok bool) {
	for _, l := range c.Synchro {
		if slices.Contains(MemberNames(l), level) {
			name, err := l.eventName()
			return name, len(l.With) > 0, err == nil
		}
	}
	return "", false, false
}

// EntryLevels are the levels an entry in a checked discipline can do: each of
// its events' levels, paired ones included.
func (c Competition) EntryLevels(discipline string) []Level {
	var out []Level
	for _, l := range c.levels(discipline) {
		out = append(out, l.Members()...)
	}
	return out
}
