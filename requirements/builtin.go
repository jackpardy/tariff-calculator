package requirements

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// Built-in sets ship in sets/<group>/, one JSON file each (ADR 0003): an
// official set is only added from a cited source and says which season it
// reflects. sets/groups.json names the groups and orders them and their sets.
//
//go:embed sets/groups.json sets/*/*.json
var builtinFiles embed.FS

// Builtin is a requirement set that ships with the app.
type Builtin struct {
	ID    string // file name without .json; referenced as "builtin:<ID>"
	Group string // the group's name, e.g. "FIG age groups (2025–2028)"
	Set   Set
}

// BuiltinGroup is a group of built-in sets, e.g. one organisation's rules,
// and the levels they make up.
type BuiltinGroup struct {
	Name   string
	Sets   []Builtin
	Levels []BuiltinLevel
}

// BuiltinLevel is a level that ships with the app, pairing built-in sets.
type BuiltinLevel struct {
	ID    string // referenced as "builtin-level:<ID>"
	Group string
	Level Level
}

// Requirements are the group's sets that aren't set routines.
func (g BuiltinGroup) Requirements() []Builtin {
	var out []Builtin
	for _, b := range g.Sets {
		if !b.IsSetRoutine() {
			out = append(out, b)
		}
	}
	return out
}

// SetRoutines are the group's set (prescribed) routines.
func (g BuiltinGroup) SetRoutines() []Builtin {
	var out []Builtin
	for _, b := range g.Sets {
		if b.IsSetRoutine() {
			out = append(out, b)
		}
	}
	return out
}

// IsSetRoutine reports whether the set is a set (prescribed) routine, which
// the app shows apart from other requirements.
func (b Builtin) IsSetRoutine() bool {
	_, ok := SetRoutine(b.Set)
	return ok
}

// BuiltinPrefix marks a reference to a built-in set, e.g. "builtin:fig-ag1-first".
const BuiltinPrefix = "builtin:"

// BuiltinLevelPrefix marks a reference to a built-in level, e.g. "builtin-level:bucs-l3".
const BuiltinLevelPrefix = "builtin-level:"

var builtinGroups = mustLoadBuiltins()

// groupFile is sets/groups.json: each group's folder, name and sets in order,
// and its levels. A level names its sets by ID; its source is its first set's.
type groupFile []struct {
	Dir    string   `json:"dir"`
	Name   string   `json:"name"`
	Sets   []string `json:"sets"`
	Levels []struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		First       []string `json:"first"`
		Second      []string `json:"second"` // none: the same as the first
	} `json:"levels"`
}

func mustLoadBuiltins() []BuiltinGroup {
	data, err := builtinFiles.ReadFile("sets/groups.json")
	if err != nil {
		panic(err)
	}
	var groups groupFile
	if err := json.Unmarshal(data, &groups); err != nil {
		panic(fmt.Sprintf("sets/groups.json: %v", err))
	}
	ids := map[string]bool{}
	var out []BuiltinGroup
	for _, g := range groups {
		entries, err := builtinFiles.ReadDir(path.Join("sets", g.Dir))
		if err != nil {
			panic(err)
		}
		if len(entries) != len(g.Sets) {
			panic(fmt.Sprintf("sets/groups.json lists %d sets for %s, but it has %d files", len(g.Sets), g.Dir, len(entries)))
		}
		group := BuiltinGroup{Name: g.Name}
		for _, id := range g.Sets {
			if ids[id] {
				panic(fmt.Sprintf("built-in requirement set %s is listed twice", id))
			}
			ids[id] = true
			data, err := builtinFiles.ReadFile(path.Join("sets", g.Dir, id+".json"))
			if err != nil {
				panic(err)
			}
			set, err := Parse(data)
			if err != nil {
				panic(fmt.Sprintf("built-in requirement set %s: %v", id, err))
			}
			group.Sets = append(group.Sets, Builtin{ID: id, Group: g.Name, Set: set})
		}
		out = append(out, group)
	}

	// Levels pair sets from any group, so they're read once every set is.
	exists := func(ref string) bool { return ids[strings.TrimPrefix(ref, BuiltinPrefix)] }
	refs := func(sets []string) []string {
		out := make([]string, len(sets))
		for i, id := range sets {
			out[i] = BuiltinPrefix + id
		}
		return out
	}
	levelIDs := map[string]bool{}
	for gi, g := range groups {
		for _, l := range g.Levels {
			if levelIDs[l.ID] || l.ID == "" {
				panic(fmt.Sprintf("built-in level %q is listed twice or has no id", l.ID))
			}
			levelIDs[l.ID] = true
			level := Level{Format: Format, Name: l.Name, Description: l.Description, First: Exercise{Options: refs(l.First)}}
			if len(l.Second) > 0 {
				level.Second = &Exercise{Options: refs(l.Second)}
			}
			if err := level.validate(exists); err != nil {
				panic(fmt.Sprintf("built-in level %s: %v", l.ID, err))
			}
			if len(l.First) > 0 {
				level.Source = findBuiltin(out, l.First[0]).Source
			}
			out[gi].Levels = append(out[gi].Levels, BuiltinLevel{ID: l.ID, Group: g.Name, Level: level})
		}
	}
	return out
}

func findBuiltin(groups []BuiltinGroup, id string) Set {
	for _, g := range groups {
		for _, b := range g.Sets {
			if b.ID == id {
				return b.Set
			}
		}
	}
	return Set{}
}

// BuiltinGroups lists the built-in sets by group, in order.
func BuiltinGroups() []BuiltinGroup { return builtinGroups }

// Builtins lists every built-in set, group by group.
func Builtins() []Builtin {
	var all []Builtin
	for _, g := range builtinGroups {
		all = append(all, g.Sets...)
	}
	return all
}

// LookupBuiltin finds a built-in set by its reference ("builtin:<ID>") or ID.
func LookupBuiltin(ref string) (Set, bool) {
	id := strings.TrimPrefix(ref, BuiltinPrefix)
	for _, b := range Builtins() {
		if b.ID == id {
			return b.Set, true
		}
	}
	return Set{}, false
}

// BuiltinLevels lists every built-in level, group by group.
func BuiltinLevels() []BuiltinLevel {
	var all []BuiltinLevel
	for _, g := range builtinGroups {
		all = append(all, g.Levels...)
	}
	return all
}

// LookupBuiltinLevel finds a built-in level by its reference
// ("builtin-level:<ID>") or ID.
func LookupBuiltinLevel(ref string) (Level, bool) {
	id := strings.TrimPrefix(ref, BuiltinLevelPrefix)
	for _, l := range BuiltinLevels() {
		if l.ID == id {
			return l.Level, true
		}
	}
	return Level{}, false
}
