package requirements

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Level is a competition level: the requirements for its two exercises, which a
// gymnast performs as a pair. Each exercise lists the requirements the gymnast
// chooses between, e.g. a level's two set routines for the first exercise, or
// one voluntary's requirements. A level with no second exercise uses the first
// exercise's requirements for both, as when both exercises are voluntaries
// under the same rules, or the same set routine.
//
// Options are references to requirements, not copies: "builtin:<id>" for a
// built-in, or the id of requirements saved in the browser. Editing those
// requirements changes the level.
//
// When the first exercise scores only some elements (its requirements'
// scored_elements, e.g. 2 in AG3), their difficulty carries over, and they
// can't be repeated in the second exercise. That follows from the first
// exercise's requirements, so a level doesn't say it.
type Level struct {
	Format      int       `json:"format"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Source      string    `json:"source,omitempty"`
	First       Exercise  `json:"first"`
	Second      *Exercise `json:"second,omitempty"` // nil: the same as the first
}

// Exercise is the requirements for one of a level's exercises.
type Exercise struct {
	Options []string `json:"options"` // the gymnast performs to one of these
}

// Exercise is the level's first (1) or second (2) exercise.
func (l Level) Exercise(n int) Exercise {
	if n == 2 && l.Second != nil {
		return *l.Second
	}
	return l.First
}

// ParseLevel reads a level from JSON and validates it. Unknown fields are
// errors, so typos in hand-written levels are caught.
func ParseLevel(data []byte) (Level, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var l Level
	if err := dec.Decode(&l); err != nil {
		return Level{}, fmt.Errorf("reading the level: %w", err)
	}
	return l, l.Validate()
}

// Validate reports everything wrong with a level, one problem per line.
// References to built-in requirements must exist; references to requirements
// saved in a browser can only be checked there.
func (l Level) Validate() error {
	return l.validate(func(ref string) bool {
		_, ok := LookupBuiltin(ref)
		return ok
	})
}

// validate is Validate with a way to tell whether built-in requirements exist,
// so built-in levels can be checked while the built-ins load.
func (l Level) validate(builtinExists func(ref string) bool) error {
	var errs []error
	if l.Format != Format {
		errs = append(errs, fmt.Errorf("format must be %d, got %d", Format, l.Format))
	}
	if strings.TrimSpace(l.Name) == "" {
		errs = append(errs, errors.New("the level needs a name"))
	}
	check := func(name string, e Exercise) {
		if len(e.Options) == 0 {
			errs = append(errs, fmt.Errorf("the %s exercise needs requirements", name))
		}
		seen := map[string]bool{}
		for _, ref := range e.Options {
			switch {
			case strings.TrimSpace(ref) == "":
				errs = append(errs, fmt.Errorf("the %s exercise has a choice still to make", name))
			case seen[ref]:
				errs = append(errs, fmt.Errorf("the %s exercise lists the same requirements twice", name))
			case strings.HasPrefix(ref, BuiltinPrefix):
				if !builtinExists(ref) {
					errs = append(errs, fmt.Errorf("the %s exercise's built-in requirements %q don't exist", name, strings.TrimPrefix(ref, BuiltinPrefix)))
				}
			}
			seen[ref] = true
		}
	}
	check("first", l.First)
	if l.Second != nil {
		check("second", *l.Second)
	}
	return errors.Join(errs...)
}
