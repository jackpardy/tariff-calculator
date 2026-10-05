package requirements

import (
	"encoding/json"
	"errors"
	"strings"

	"tariffCalculator/skills"
)

// Checks are which of the Code of Points' checks apply to a routine. Set
// (compulsory) routines usually score no difficulty and may repeat elements;
// the routine's requirements say, and the coach can change it.
type Checks struct {
	ScoreDifficulty bool
	FlagRepeats     bool
	ScoredElements  int // only this many elements score difficulty (0: all)
}

// ChecksFor are the checks that apply to a routine: its requirements' (a set
// routine has no difficulty and may repeat elements; an AG3 first exercise
// scores only 2 elements), unless the routine says otherwise in own, its
// checks as the browser stores them, e.g. {"difficulty":true,"scored":0}.
// set is nil for a routine checked against nothing.
func ChecksFor(set *Set, own string) Checks {
	checks := Checks{ScoreDifficulty: true, FlagRepeats: true}
	if set != nil {
		checks.ScoreDifficulty, checks.FlagRepeats = !set.NoDifficulty, !set.RepeatsAllowed
		checks.ScoredElements = set.ScoredElements
	}
	var o struct {
		Difficulty *bool `json:"difficulty"`
		Repeats    *bool `json:"repeats"`
		Scored     *int  `json:"scored"`
	}
	if own != "" && json.Unmarshal([]byte(own), &o) == nil {
		if o.Difficulty != nil {
			checks.ScoreDifficulty = *o.Difficulty
		}
		if o.Repeats != nil {
			checks.FlagRepeats = *o.Repeats
		}
		if o.Scored != nil && *o.Scored >= 0 && *o.Scored <= skills.RoutineLength {
			checks.ScoredElements = *o.Scored
		}
	}
	return checks
}

// ResolveSet is the requirements a reference names: a built-in
// ("builtin:<id>") or a custom set as JSON. Requirements that don't parse come
// back with whatever name they have.
func ResolveSet(raw string) (Set, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, BuiltinPrefix) {
		builtin, ok := LookupBuiltin(raw)
		if !ok {
			return Set{}, errors.New("these built-in requirements no longer exist")
		}
		return builtin, nil
	}
	return Parse([]byte(raw))
}

// ResolveLevel is the level a reference names: a built-in
// ("builtin-level:<id>") or a custom level as JSON. A level that doesn't parse
// comes back with whatever name it has, or "Level".
func ResolveLevel(raw string) (Level, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, BuiltinLevelPrefix) {
		level, ok := LookupBuiltinLevel(raw)
		if !ok {
			return Level{Name: "Level"}, errors.New("this built-in level no longer exists")
		}
		return level, nil
	}
	level, err := ParseLevel([]byte(raw))
	if level.Name == "" {
		level.Name = "Level"
	}
	return level, err
}

// Routine is a routine to check: its skills, what it's checked against and its
// own choice of checks.
type Routine struct {
	Skills []skills.TrampolineSkill
	// Set is the requirements it's checked against, nil for none. SetErr says
	// why its requirements couldn't be used, if they couldn't; SetName then
	// names them.
	Set     *Set
	SetName string
	SetErr  error
	Checks  string // its own checks as the browser stores them ("" for its requirements')
}

// Checked is a routine validated with the checks that apply to it, and checked
// against its requirements if it has any.
type Checked struct {
	Validation skills.RoutineValidation
	Checks     Checks
	HasSet     bool   // it's checked against requirements, usable or not
	SetName    string // their name
	SetErr     error  // why they couldn't be used, if they couldn't
	Results    []Result
	Required   map[int]bool // elements meeting a requirement
}

// Check validates a routine and checks it against its requirements.
// scoredEarlier are elements that scored in a level's first exercise and score
// nothing if repeated (nil for none). Tariffs and official names are always
// worked out again, so routines stored by older versions are corrected.
func Check(r Routine, scoredEarlier []skills.TrampolineSkill) Checked {
	routine := make([]skills.TrampolineSkill, len(r.Skills))
	for i, s := range r.Skills {
		s.NormalizePhases()
		s.SetTariff()
		s.Name = skills.FindCommonSkillName(s)
		routine[i] = s
	}
	out := Checked{Checks: ChecksFor(r.Set, r.Checks)}
	opts := skills.ValidateOptions{AllowRepeats: !out.Checks.FlagRepeats, ScoredEarlier: scoredEarlier}
	if out.Checks.ScoreDifficulty {
		opts.ScoredElements = out.Checks.ScoredElements
	}
	out.Validation = skills.ValidateRoutineWith(routine, opts)
	switch {
	case r.Set != nil:
		out.HasSet, out.SetName = true, r.Set.Name
		out.Results = Evaluate(*r.Set, out.Validation)
		out.Required = RequiredElements(*r.Set, out.Results)
	case r.SetErr != nil:
		out.HasSet, out.SetName, out.SetErr = true, r.SetName, r.SetErr
		if out.SetName == "" {
			out.SetName = "Requirements"
		}
	}
	return out
}

// Met is how many of the requirements' rules the routine passes.
func (c Checked) Met() int {
	n := 0
	for _, r := range c.Results {
		if r.Passed {
			n++
		}
	}
	return n
}

// Carried are the elements whose difficulty carries over to a level's second
// exercise, when this is the first: those that score, when only some do.
func (c Checked) Carried() []skills.TrampolineSkill {
	if !c.Checks.ScoreDifficulty || c.Checks.ScoredElements == 0 {
		return nil
	}
	var out []skills.TrampolineSkill
	for _, sv := range c.Validation.Skills {
		if sv.Counted {
			out = append(out, sv.Skill)
		}
	}
	return out
}

// Pair is a level's two exercises checked together.
type Pair struct {
	First, Second Checked
	// When the first exercise scores only some elements, their difficulty
	// carries over and they can't be repeated in the second exercise.
	Carried  []int // the first exercise's elements that carry over (1-based)
	Repeated []int // the second exercise's elements repeating one of them
}

// CheckPair checks the routines doing a level's first and second exercises:
// the second is checked knowing which of the first's elements carry over.
// The builder and stored competition entries both check levels this way, so
// they can't disagree.
func CheckPair(first, second Routine) Pair {
	p := Pair{First: Check(first, nil)}
	carried := p.First.Carried()
	p.Second = Check(second, carried)
	if len(carried) > 0 {
		for i, sv := range p.First.Validation.Skills {
			if sv.Counted {
				p.Carried = append(p.Carried, i+1)
			}
		}
	}
	for i, sv := range p.Second.Validation.Skills {
		if sv.ScoredEarlier {
			p.Repeated = append(p.Repeated, i+1)
		}
	}
	return p
}
