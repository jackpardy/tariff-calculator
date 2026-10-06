package competitions

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"tariffCalculator/requirements"
)

// Events (ADR 0005 Decision 1): a discipline at a level. Individual
// trampoline is the discipline "", so entries from before there were others
// still read as trampoline.

const (
	Trampoline = ""
	Synchro    = "synchro"
	Tumbling   = "tumbling"
	DMT        = "dmt"
)

// AllDisciplines are every discipline, in the order the pages list them.
var AllDisciplines = []string{Trampoline, Synchro, Tumbling, DMT}

// DisciplineName is a discipline's name, e.g. "Synchro".
func DisciplineName(d string) string {
	switch d {
	case Synchro:
		return "Synchro"
	case Tumbling:
		return "Tumbling"
	case DMT:
		return "DMT"
	}
	return "Trampoline"
}

// Checked says whether a discipline's routines are checked: trampoline and
// synchro are; tumbling and DMT aren't, until their tariff is built in.
func Checked(d string) bool { return d == Trampoline || d == Synchro }

// EventName is an event's name: the level alone for individual trampoline
// (as it always was), else the discipline and level, e.g. "Synchro BUCS L3".
func EventName(discipline, level string) string {
	if discipline == Trampoline {
		return level
	}
	return DisciplineName(discipline) + " " + level
}

// Event is the entry's event's name.
func (e Entry) Event() string { return EventName(e.Discipline, e.Level) }

// Gymnasts are the entry's gymnasts' names: one, or a synchro pair.
func (e Entry) Gymnasts() string {
	if e.Partner != nil && e.Partner.Name != "" {
		return e.Gymnast + " & " + e.Partner.Name
	}
	return e.Gymnast
}

// Partner is the other gymnast of a synchro pair (ADR 0005 Decision 3). Any
// gymnast can be a partner; until they confirm, they're only a name.
type Partner struct {
	Name string `json:"name"`
	Club string `json:"club,omitempty"` // as the entering gymnast gave it; "" for none
}

func (p *Partner) check() error {
	if p == nil || strings.TrimSpace(p.Name) == "" {
		return errors.New("synchro needs your partner's name")
	}
	p.Name, p.Club = strings.TrimSpace(p.Name), strings.TrimSpace(p.Club)
	if err := CheckName("partner", p.Name); err != nil {
		return err
	}
	if p.Club != "" {
		return CheckName("partner's club", p.Club)
	}
	return nil
}

// levels are a checked discipline's levels.
func (c Competition) levels(discipline string) []Level {
	if discipline == Synchro {
		return c.Synchro
	}
	return c.Levels
}

// levelNames are a discipline's levels' names, in order.
func (c Competition) levelNames(discipline string) []string {
	switch discipline {
	case Tumbling:
		return c.Tumbling
	case DMT:
		return c.DMT
	}
	var out []string
	for _, l := range c.levels(discipline) {
		if level, err := l.Resolve(); err == nil {
			out = append(out, level.Name)
		}
	}
	return out
}

// LevelNames are a discipline's levels' names, in order.
func (c Competition) LevelNames(discipline string) []string { return c.levelNames(discipline) }

// LevelFor is a checked discipline's level with this name.
func (c Competition) LevelFor(discipline, name string) (Level, requirements.Level, bool) {
	for _, l := range c.levels(discipline) {
		if level, err := l.Resolve(); err == nil && level.Name == name {
			return l, level, true
		}
	}
	return Level{}, requirements.Level{}, false
}

// Disciplines are the disciplines the competition offers, in order.
func (c Competition) Disciplines() []string {
	var out []string
	for _, d := range AllDisciplines {
		if len(c.levelNames(d)) > 0 {
			out = append(out, d)
		}
	}
	return out
}

// EventNames are every event's name, discipline by discipline.
func (c Competition) EventNames() []string {
	var out []string
	for _, d := range AllDisciplines {
		for _, l := range c.levelNames(d) {
			out = append(out, EventName(d, l))
		}
	}
	return out
}

// checkCategory checks an entry's men or women against its event's split.
func (c Competition) checkCategory(e *Entry, errs *[]error) {
	switch {
	case !c.Split.Splits(e.Event()):
		e.Category = ""
	case !slices.Contains(Categories, e.Category):
		*errs = append(*errs, fmt.Errorf("%s is split into men and women: choose one", e.Event()))
	}
}

// validateEvents reports what's wrong with the other disciplines' events.
func (c Competition) validateEvents() []error {
	var errs []error
	seen := map[string]bool{}
	for _, l := range c.Synchro {
		level, err := l.validate()
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("synchro: %w", err))
		case seen[level.Name]:
			errs = append(errs, fmt.Errorf("two synchro levels are called %q", level.Name))
		}
		seen[level.Name] = true
	}
	for _, d := range []string{Tumbling, DMT} {
		seen := map[string]bool{}
		for _, name := range c.levelNames(d) {
			switch err := CheckName(DisciplineName(d)+" level", name); {
			case err != nil:
				errs = append(errs, err)
			case seen[name]:
				errs = append(errs, fmt.Errorf("two %s levels are called %q", DisciplineName(d), name))
			}
			seen[name] = true
		}
	}
	return errs
}

// LevelsOf are a checked discipline's levels: trampoline's or synchro's.
func (c Competition) LevelsOf(discipline string) []Level { return c.levels(discipline) }
