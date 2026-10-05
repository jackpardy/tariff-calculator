// Package competitions is competition card collection's domain (ADR 0004): a
// competition and the levels it offers, a gymnast's entry, and the checking of
// an entry exactly as the routine builder checks it. It knows nothing about
// HTTP, SQL or templates.
package competitions

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"tariffCalculator/requirements"
	"tariffCalculator/skills"
)

const (
	// MaxName is the longest name, of a competition, club or gymnast, in characters.
	MaxName = 80
	// MaxSkills is the most skills an exercise can hold. A routine has ten
	// elements; the builder lets a coach go over, and the check flags it.
	MaxSkills = 20
	// RetentionDays is how long a competition and its entries are kept after
	// the competition date, and a club after it was last used (ADR 0004
	// Decision 6).
	RetentionDays = 120
	// DateLayout is how a competition's date is written.
	DateLayout = "2006-01-02"
)

// Competition is what an organiser sets up: when it is, when entries close,
// whether individuals can enter, and the levels gymnasts enter.
type Competition struct {
	Name        string
	Date        string    // the competition's (first) day, as DateLayout
	Deadline    time.Time // entries can be sent and changed until then
	Individuals bool      // individuals can enter directly, not only through a club
	Levels      []Level
	Video       Video // whether gymnasts send video proof
}

// Level is a level a competition offers. A built-in is kept by reference; a
// coach's own level is copied in, with the requirements its options name,
// because references to a browser's saved items can't be resolved on the
// server (ADR 0004 Decision 5).
type Level struct {
	Ref    string                      `json:"ref,omitempty"`    // "builtin-level:<id>"
	Custom *requirements.Level         `json:"custom,omitempty"` // a coach's own level
	Sets   map[string]requirements.Set `json:"sets,omitempty"`   // the custom requirements its options name, by id
}

// Resolve is the level itself.
func (l Level) Resolve() (requirements.Level, error) {
	switch {
	case l.Ref != "" && l.Custom != nil:
		return requirements.Level{}, errors.New("a level is either built in or a copy, not both")
	case l.Custom != nil:
		return *l.Custom, l.Custom.Validate()
	case strings.HasPrefix(l.Ref, requirements.BuiltinLevelPrefix):
		return requirements.ResolveLevel(l.Ref)
	}
	return requirements.Level{}, errors.New("a level needs a built-in reference or a copy")
}

// Set is the requirements one of the level's options names: a built-in, or a
// copy kept with the level.
func (l Level) Set(ref string) (requirements.Set, error) {
	if strings.HasPrefix(ref, requirements.BuiltinPrefix) {
		return requirements.ResolveSet(ref)
	}
	set, ok := l.Sets[ref]
	if !ok {
		return requirements.Set{}, fmt.Errorf("the requirements %q aren't part of the competition", ref)
	}
	return set, set.Validate()
}

// validate reports what's wrong with a level: it must resolve, and every
// option of both exercises must name requirements that can be used.
func (l Level) validate() (requirements.Level, error) {
	level, err := l.Resolve()
	if err != nil {
		return level, err
	}
	var errs []error
	for n := 1; n <= 2; n++ {
		for _, ref := range level.Exercise(n).Options {
			if _, err := l.Set(ref); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", level.Name, err))
			}
		}
	}
	return level, errors.Join(errs...)
}

// Day is the competition's date, at midnight UTC.
func (c Competition) Day() (time.Time, error) {
	return time.Parse(DateLayout, c.Date)
}

// DeleteAfter is when the competition and its entries are deleted.
func (c Competition) DeleteAfter() time.Time {
	day, err := c.Day()
	if err != nil {
		return time.Time{}
	}
	return day.AddDate(0, 0, RetentionDays)
}

// Open says whether entries can still be sent or changed at now.
func (c Competition) Open(now time.Time) bool {
	return now.Before(c.Deadline)
}

// Validate reports everything wrong with a competition, one problem per line.
func (c Competition) Validate() error {
	var errs []error
	if err := CheckName("competition", c.Name); err != nil {
		errs = append(errs, err)
	}
	day, err := c.Day()
	if err != nil {
		errs = append(errs, fmt.Errorf("the date should be written as %s", DateLayout))
	}
	switch {
	case c.Deadline.IsZero():
		errs = append(errs, errors.New("entries need a deadline"))
	case err == nil && c.Deadline.After(day.AddDate(0, 0, 1)):
		errs = append(errs, errors.New("entries must close by the end of the competition date"))
	}
	if len(c.Levels) == 0 {
		errs = append(errs, errors.New("the competition needs at least one level"))
	}
	if err := c.Video.validate(); err != nil {
		errs = append(errs, err)
	}
	names := map[string]bool{}
	for _, l := range c.Levels {
		level, err := l.validate()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if names[level.Name] {
			errs = append(errs, fmt.Errorf("two levels are called %q", level.Name))
		}
		names[level.Name] = true
	}
	return errors.Join(errs...)
}

// Level is the offered level with this name.
func (c Competition) Level(name string) (Level, requirements.Level, bool) {
	for _, l := range c.Levels {
		if level, err := l.Resolve(); err == nil && level.Name == name {
			return l, level, true
		}
	}
	return Level{}, requirements.Level{}, false
}

// Entry is one gymnast's entry: a level and a routine for each of its two
// exercises. A member keeps one with their club for each competition, and the
// competition keeps a copy of what the club sent.
type Entry struct {
	Gymnast   string      `json:"gymnast"`
	Level     string      `json:"level"` // the name of one of the competition's levels
	Exercises [2]Exercise `json:"exercises"`
}

// Exercise is what a gymnast performs for one of a level's exercises.
type Exercise struct {
	// Option is which of the exercise's requirements: one of the level's
	// options for it. It can be left out when there's only one.
	Option string `json:"option,omitempty"`
	// Skills are the routine, stored as the browser stores them (ADR 0004
	// Decision 5). A set routine left empty is performed as prescribed.
	Skills []skills.TrampolineSkill `json:"skills,omitempty"`
	// There are no checks of the gymnast's own: a card is checked with the
	// checks its requirements set, so it can't turn one off (ADR 0004
	// Decision 5).

	// Video is a link to video of the exercise, where the competition asks
	// for it, and VideoNote what it shows, e.g. "Full-in at 0:42" (ADR 0004
	// Decision 10).
	Video     string `json:"video,omitempty"`
	VideoNote string `json:"video_note,omitempty"`
}

// ValidateEntry reports everything wrong with an entry for this competition,
// one problem per line. A routine that breaks the rules is still a valid
// entry: that's what checking it shows. Valid entries have their options
// filled in and their skills normalised.
func (c Competition) ValidateEntry(e *Entry) error {
	var errs []error
	e.Gymnast = strings.TrimSpace(e.Gymnast)
	if err := CheckName("gymnast", e.Gymnast); err != nil {
		errs = append(errs, err)
	}
	_, level, ok := c.Level(e.Level)
	if !ok {
		return errors.Join(append(errs, fmt.Errorf("the competition doesn't offer the level %q", e.Level))...)
	}
	for i := range e.Exercises {
		ex := &e.Exercises[i]
		name := ordinal(i + 1)
		options := level.Exercise(i + 1).Options
		switch {
		case ex.Option == "" && len(options) == 1:
			ex.Option = options[0]
		case ex.Option == "":
			errs = append(errs, fmt.Errorf("the %s exercise needs one of its options chosen", name))
		case !slices.Contains(options, ex.Option):
			errs = append(errs, fmt.Errorf("the %s exercise's option %q isn't one of the level's", name, ex.Option))
		}
		ex.Video, ex.VideoNote = strings.TrimSpace(ex.Video), strings.TrimSpace(ex.VideoNote)
		if ex.Video != "" {
			if err := CheckVideoLink(ex.Video); err != nil {
				errs = append(errs, fmt.Errorf("the %s exercise's video: %w", name, err))
			}
		}
		if n := utf8.RuneCountInString(ex.VideoNote); n > MaxVideoNote {
			errs = append(errs, fmt.Errorf("the %s exercise's video note is %d characters; the most is %d", name, n, MaxVideoNote))
		}
		if len(ex.Skills) > MaxSkills {
			errs = append(errs, fmt.Errorf("the %s exercise has %d skills; the most is %d", name, len(ex.Skills), MaxSkills))
			continue
		}
		for j := range ex.Skills {
			ex.Skills[j].NormalizePhases()
			if err := ex.Skills[j].Validate(); err != nil {
				errs = append(errs, fmt.Errorf("the %s exercise's skill %d: %w", name, j+1, err))
			}
		}
	}
	return errors.Join(errs...)
}

// Card is an entry checked exactly as the routine builder checks a level:
// both exercises together, so difficulty carried over from the first
// exercise can't be repeated in the second.
type Card struct {
	Level requirements.Level
	requirements.Pair
}

// Check checks an entry. Names, tariffs and results are worked out afresh, so
// engine fixes apply to stored entries too (ADR 0004 Decision 5).
func (c Competition) Check(e Entry) (Card, error) {
	l, level, ok := c.Level(e.Level)
	if !ok {
		return Card{}, fmt.Errorf("the competition doesn't offer the level %q", e.Level)
	}
	var routines [2]requirements.Routine
	for i, ex := range e.Exercises {
		r := requirements.Routine{Skills: ex.Skills}
		set, err := l.Set(ex.Option)
		if err != nil {
			r.SetName, r.SetErr = set.Name, err
		} else {
			r.Set = &set
			// A set routine left empty is performed as prescribed.
			if len(r.Skills) == 0 {
				r.Skills, _ = requirements.SetRoutine(set)
			}
		}
		routines[i] = r
	}
	return Card{Level: level, Pair: requirements.CheckPair(routines[0], routines[1])}, nil
}

// Problems lists what a judge would query on the card, exercise by exercise:
// requirements that can't be used or aren't met, and what the validation
// flags on each element (bad transitions, interruptions, repeats).
func (c Card) Problems() []string {
	var out []string
	for i, ex := range []requirements.Checked{c.First, c.Second} {
		prefix := [...]string{"First exercise: ", "Second exercise: "}[i]
		if ex.SetErr != nil {
			out = append(out, prefix+ex.SetErr.Error())
		}
		for _, r := range ex.Results {
			if !r.Passed {
				out = append(out, prefix+r.Description)
			}
		}
		for j, msg := range ex.Validation.Messages {
			if msg != "" {
				out = append(out, fmt.Sprintf("%selement %d: %s", prefix, j+1, msg))
			}
		}
	}
	return out
}

// CheckName reports a missing or too-long name of a competition, club or
// gymnast; what says which.
func CheckName(what, name string) error {
	switch n := utf8.RuneCountInString(strings.TrimSpace(name)); {
	case n == 0:
		return fmt.Errorf("the %s needs a name", what)
	case n > MaxName:
		return fmt.Errorf("the %s's name is %d characters; the most is %d", what, n, MaxName)
	}
	return nil
}

func ordinal(n int) string {
	if n == 2 {
		return "second"
	}
	return "first"
}
