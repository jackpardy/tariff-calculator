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

// BuiltinGroup is a group of built-in sets, e.g. one organisation's rules.
type BuiltinGroup struct {
	Name string
	Sets []Builtin
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

var builtinGroups = mustLoadBuiltins()

// groupFile is sets/groups.json: each group's folder, name and sets in order.
type groupFile []struct {
	Dir  string   `json:"dir"`
	Name string   `json:"name"`
	Sets []string `json:"sets"`
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
	return out
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
