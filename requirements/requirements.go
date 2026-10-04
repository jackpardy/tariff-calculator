// Package requirements checks a routine against a set of competition rules:
// required and forbidden elements, element counts, difficulty limits and set
// routines (ADR 0003). A Set is plain JSON so coaches can write, share and
// adjust their own; built-in sets use the same format.
package requirements

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"tariffCalculator/skills"
)

// Format is the current version of the requirement set JSON.
const Format = 1

// Set is a named group of rules, e.g. one competition level.
type Set struct {
	Format      int    `json:"format"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"` // where the rules come from, and which season
	Rules       []Rule `json:"rules"`
}

// Rule types.
const (
	Count      = "count"      // at least Min / at most Max elements match Match
	Every      = "every"      // every element matches Match
	Elements   = "elements"   // the number of elements is within Min/Max
	Difficulty = "difficulty" // the counted difficulty is within Min/Max
	Position   = "position"   // the element at Position (1-based) matches Match
	Sequence   = "sequence"   // a set routine: element i matches Sequence[i], and no more
)

// RuleTypes lists the rule types in the order the editor offers them.
var RuleTypes = []string{Count, Every, Elements, Difficulty, Position, Sequence}

// Rule is one requirement. Which fields apply depends on Type.
type Rule struct {
	Type     string    `json:"type"`
	Label    string    `json:"label,omitempty"` // the author's wording; a description is generated otherwise
	Match    *Matcher  `json:"match,omitempty"`
	Min      *float64  `json:"min,omitempty"`
	Max      *float64  `json:"max,omitempty"`
	Position int       `json:"position,omitempty"`
	Sequence []Matcher `json:"sequence,omitempty"`
}

// Range bounds a whole number; either end may be open.
type Range struct {
	Min *int `json:"min,omitempty"`
	Max *int `json:"max,omitempty"`
}

// TariffRange bounds a tariff; either end may be open.
type TariffRange struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

// Matcher describes elements; every condition given must hold. Shapes only
// distinguish skills whose shape matters: other skills (twisting jumps, drops,
// singles with a full twist or more) count as straight.
type Matcher struct {
	Rotation  *Range       `json:"rotation,omitempty"`  // quarter somersaults
	Direction string       `json:"direction,omitempty"` // "forward" or "backward"
	Twist     *Range       `json:"twist,omitempty"`     // total half twists
	Shapes    []string     `json:"shapes,omitempty"`    // tuck, pike, straight, straddle
	Takeoff   []string     `json:"takeoff,omitempty"`   // feet, front, back, seat
	Landing   []string     `json:"landing,omitempty"`   // feet, front, back, seat
	Tariff    *TariffRange `json:"tariff,omitempty"`
	FIG       string       `json:"fig,omitempty"` // exact FIG notation, e.g. "4 - o"
}

var (
	shapeNames    = []string{"tuck", "pike", "straight", "straddle"}
	positionNames = []string{"feet", "front", "back", "seat"}
)

// Parse reads a requirement set from JSON and validates it. Unknown fields are
// errors, so typos in hand-written sets are caught.
func Parse(data []byte) (Set, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var s Set
	if err := dec.Decode(&s); err != nil {
		return Set{}, fmt.Errorf("reading requirement set: %w", err)
	}
	return s, s.Validate()
}

// Validate reports everything wrong with a set, one problem per line.
func (s Set) Validate() error {
	var errs []error
	if s.Format != Format {
		errs = append(errs, fmt.Errorf("format must be %d, got %d", Format, s.Format))
	}
	if strings.TrimSpace(s.Name) == "" {
		errs = append(errs, errors.New("the set needs a name"))
	}
	for i, r := range s.Rules {
		if err := r.validate(); err != nil {
			errs = append(errs, fmt.Errorf("rule %d: %w", i+1, err))
		}
	}
	return errors.Join(errs...)
}

func (r Rule) validate() error {
	var errs []error
	needMatch := func() {
		if r.Match == nil {
			errs = append(errs, errors.New("needs a description of the elements"))
		} else if err := r.Match.validate(); err != nil {
			errs = append(errs, err)
		}
	}
	needBounds := func(whole bool) {
		if r.Min == nil && r.Max == nil {
			errs = append(errs, errors.New("needs a minimum or a maximum"))
		}
		for _, v := range []*float64{r.Min, r.Max} {
			if v != nil && (*v < 0 || (whole && *v != math.Trunc(*v))) {
				errs = append(errs, fmt.Errorf("%v must be a whole number of at least 0", *v))
			}
		}
		if r.Min != nil && r.Max != nil && *r.Min > *r.Max {
			errs = append(errs, errors.New("minimum is more than maximum"))
		}
	}

	switch r.Type {
	case Count:
		needMatch()
		needBounds(true)
	case Every:
		needMatch()
	case Elements:
		needBounds(true)
	case Difficulty:
		needBounds(false)
	case Position:
		needMatch()
		if r.Position < 1 {
			errs = append(errs, errors.New("position must be 1 or more"))
		}
	case Sequence:
		if len(r.Sequence) == 0 {
			errs = append(errs, errors.New("a set routine needs at least one element"))
		}
		for i, m := range r.Sequence {
			if err := m.validate(); err != nil {
				errs = append(errs, fmt.Errorf("element %d: %w", i+1, err))
			}
		}
	default:
		errs = append(errs, fmt.Errorf("unknown rule type %q", r.Type))
	}
	return errors.Join(errs...)
}

func (m Matcher) validate() error {
	var errs []error
	checkRange := func(name string, r *Range) {
		if r == nil {
			return
		}
		if (r.Min != nil && *r.Min < 0) || (r.Max != nil && *r.Max < 0) {
			errs = append(errs, fmt.Errorf("%s can't be negative", name))
		}
		if r.Min != nil && r.Max != nil && *r.Min > *r.Max {
			errs = append(errs, fmt.Errorf("%s minimum is more than maximum", name))
		}
	}
	checkRange("rotation", m.Rotation)
	checkRange("twist", m.Twist)
	if m.Tariff != nil && m.Tariff.Min != nil && m.Tariff.Max != nil && *m.Tariff.Min > *m.Tariff.Max {
		errs = append(errs, errors.New("tariff minimum is more than maximum"))
	}
	if m.Direction != "" && m.Direction != "forward" && m.Direction != "backward" {
		errs = append(errs, fmt.Errorf("direction must be forward or backward, got %q", m.Direction))
	}
	for _, v := range m.Shapes {
		if !slices.Contains(shapeNames, v) {
			errs = append(errs, fmt.Errorf("unknown shape %q", v))
		}
	}
	for _, list := range [][]string{m.Takeoff, m.Landing} {
		for _, v := range list {
			if !slices.Contains(positionNames, v) {
				errs = append(errs, fmt.Errorf("unknown position %q", v))
			}
		}
	}
	return errors.Join(errs...)
}

// Matches reports whether a skill meets every condition of the matcher.
func (m Matcher) Matches(s skills.TrampolineSkill) bool {
	if !inRange(m.Rotation, s.Rotation) || !inRange(m.Twist, s.TotalTwist()) {
		return false
	}
	if m.Direction == "forward" && s.Backward || m.Direction == "backward" && !s.Backward {
		return false
	}
	if len(m.Shapes) > 0 && !slices.Contains(m.Shapes, shapeOf(s)) {
		return false
	}
	if len(m.Takeoff) > 0 && !slices.Contains(m.Takeoff, strings.ToLower(s.TakeoffPosition.String())) {
		return false
	}
	if len(m.Landing) > 0 && !slices.Contains(m.Landing, strings.ToLower(s.LandingPosition().String())) {
		return false
	}
	if m.Tariff != nil && (m.Tariff.Min != nil && s.Tariff < *m.Tariff.Min-1e-9 || m.Tariff.Max != nil && s.Tariff > *m.Tariff.Max+1e-9) {
		return false
	}
	if m.FIG != "" && compactFIG(m.FIG) != compactFIG(s.FIGNotation()) {
		return false
	}
	return true
}

// shapeOf is the skill's shape for matching: straight when its shape doesn't matter.
func shapeOf(s skills.TrampolineSkill) string {
	if !s.ShapeIsRelevant() {
		return "straight"
	}
	return strings.ToLower(s.Shape.String())
}

// compactFIG drops brackets and spaces so "(4 - o)" and "4-o" compare equal.
func compactFIG(f string) string {
	return strings.NewReplacer("(", "", ")", "", " ", "").Replace(f)
}

func inRange(r *Range, v int) bool {
	return r == nil || (r.Min == nil || v >= *r.Min) && (r.Max == nil || v <= *r.Max)
}

func within(min, max *float64, v float64) bool {
	return (min == nil || v >= *min-1e-9) && (max == nil || v <= *max+1e-9)
}

// Result is the outcome of one rule for a routine.
type Result struct {
	Rule        int    // index in Set.Rules
	Description string // the rule's label, or a generated description
	Passed      bool
	Detail      string // what was found, e.g. "found 0"
	Elements    []int  // 1-based: the matching elements (count), or the offending ones
}

// Evaluate checks every rule of the set against a validated routine and reports
// all results; it never stops at the first failure. Rules look at the routine
// as written (every element, counted or not), except difficulty, which is the
// counted total.
func Evaluate(set Set, rv skills.RoutineValidation) []Result {
	routine := make([]skills.TrampolineSkill, len(rv.Skills))
	for i, sv := range rv.Skills {
		routine[i] = sv.Skill
	}
	results := make([]Result, len(set.Rules))
	for i, rule := range set.Rules {
		res := Result{Rule: i, Description: Describe(rule)}
		switch rule.Type {
		case Count:
			for j, s := range routine {
				if rule.Match.Matches(s) {
					res.Elements = append(res.Elements, j+1)
				}
			}
			res.Passed = within(rule.Min, rule.Max, float64(len(res.Elements)))
			res.Detail = fmt.Sprintf("found %d", len(res.Elements))
		case Every:
			for j, s := range routine {
				if !rule.Match.Matches(s) {
					res.Elements = append(res.Elements, j+1)
				}
			}
			res.Passed = len(res.Elements) == 0
			if !res.Passed {
				res.Detail = "not " + plural(len(res.Elements), "element", "elements") + " " + elementList(res.Elements)
			}
		case Elements:
			res.Passed = within(rule.Min, rule.Max, float64(len(routine)))
			res.Detail = fmt.Sprintf("has %d", len(routine))
		case Difficulty:
			res.Passed = within(rule.Min, rule.Max, rv.TotalTariff)
			res.Detail = fmt.Sprintf("is %.1f", rv.TotalTariff)
		case Position:
			p := rule.Position
			res.Passed = p <= len(routine) && rule.Match.Matches(routine[p-1])
			if !res.Passed {
				res.Elements = []int{p}
				if p > len(routine) {
					res.Detail = fmt.Sprintf("there is no element %d", p)
				}
			}
		case Sequence:
			for j := range max(len(routine), len(rule.Sequence)) {
				if j >= len(routine) || j >= len(rule.Sequence) || !rule.Sequence[j].Matches(routine[j]) {
					res.Elements = append(res.Elements, j+1)
				}
			}
			res.Passed = len(res.Elements) == 0
			if !res.Passed {
				res.Detail = "differs at " + elementList(res.Elements)
			}
		}
		results[i] = res
	}
	return results
}

// RequiredElements is the set of elements (1-based) that satisfy a rule requiring
// at least one matching element, e.g. to mark them on the tariff sheet.
func RequiredElements(set Set, results []Result) map[int]bool {
	required := map[int]bool{}
	for _, res := range results {
		rule := set.Rules[res.Rule]
		if rule.Type == Count && rule.Min != nil && *rule.Min >= 1 {
			for _, e := range res.Elements {
				required[e] = true
			}
		}
	}
	return required
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func elementList(elements []int) string {
	parts := make([]string, len(elements))
	for i, e := range elements {
		parts[i] = fmt.Sprint(e)
	}
	return strings.Join(parts, ", ")
}
