package competitions

import (
	"fmt"
	"slices"

	"tariffCalculator/requirements"
)

// A competition lists each discipline's levels easiest first: judging "up to"
// a level, and "only levels below their own", go by that order. Built-in
// levels have a known rank (requirements.BuiltinLevelRank); the organiser puts
// their own levels, and tumbling and DMT's typed ones, where they belong, and
// can move any level.

// OrderLevels lists levels easiest first, keeping the order of those already
// in order and placing the rest: a built-in level among its group's by rank,
// any other level last, in the order given.
func OrderLevels(order, levels []Level) []Level {
	var out []Level
	for _, l := range order {
		if slices.ContainsFunc(levels, l.same) && !slices.ContainsFunc(out, l.same) {
			out = append(out, l)
		}
	}
	for _, l := range levels {
		if !slices.ContainsFunc(out, l.same) {
			out = slices.Insert(out, l.place(out), l)
		}
	}
	return out
}

// same says whether two levels are the same level: the same built-in, or own
// levels of the same name.
func (l Level) same(other Level) bool {
	if l.Custom != nil || other.Custom != nil {
		return l.Custom != nil && other.Custom != nil && l.Custom.Name == other.Custom.Name
	}
	return l.Ref == other.Ref
}

// place is where a level goes among levels in order: a built-in before the
// first harder level of its group, else after the last of its group, else
// last.
func (l Level) place(levels []Level) int {
	group, rank, ok := requirements.BuiltinLevelRank(l.Ref)
	if l.Custom != nil || !ok {
		return len(levels)
	}
	after := -1
	for i, other := range levels {
		g, r, ok := requirements.BuiltinLevelRank(other.Ref)
		if other.Custom != nil || !ok || g != group {
			continue
		}
		if r > rank {
			return i
		}
		after = i
	}
	if after >= 0 {
		return after + 1
	}
	return len(levels)
}

// MoveLevel moves a discipline's level, by its place in the order, one place
// easier (by -1) or harder (by 1).
func (c *Competition) MoveLevel(discipline string, at, by int) error {
	var ok bool
	switch discipline {
	case Trampoline:
		ok = swap(c.Levels, at, by)
	case Synchro:
		ok = swap(c.Synchro, at, by)
	case Tumbling:
		ok = swap(c.Tumbling, at, by)
	case DMT:
		ok = swap(c.DMT, at, by)
	}
	if !ok {
		return fmt.Errorf("%s has no level to move there", DisciplineName(discipline))
	}
	return nil
}

func swap[T any](s []T, at, by int) bool {
	to := at + by
	if by != -1 && by != 1 || at < 0 || at >= len(s) || to < 0 || to >= len(s) {
		return false
	}
	s[at], s[to] = s[to], s[at]
	return true
}

// LevelOrder names every level of a discipline in order, one for each place
// MoveLevel moves: a level that can't be read is named by its reference.
func (c Competition) LevelOrder(discipline string) []string {
	switch discipline {
	case Tumbling, DMT:
		return c.levelNames(discipline)
	}
	var out []string
	for _, l := range c.levels(discipline) {
		level, err := l.Resolve()
		if err != nil {
			level.Name = l.Ref
		}
		out = append(out, level.Name)
	}
	return out
}
