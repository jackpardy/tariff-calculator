package requirements

import (
	"embed"
	"fmt"
	"path"
	"sort"
	"strings"
)

// Built-in sets ship in sets/, one JSON file each (ADR 0003): an official set
// is only added from a cited source and says which season it reflects.
//
//go:embed sets/*.json
var builtinFiles embed.FS

// Builtin is a requirement set that ships with the app.
type Builtin struct {
	ID  string // file name without .json; referenced as "builtin:<ID>"
	Set Set
}

// BuiltinPrefix marks a reference to a built-in set, e.g. "builtin:example-club-novice".
const BuiltinPrefix = "builtin:"

var builtins = mustLoadBuiltins()

func mustLoadBuiltins() []Builtin {
	entries, err := builtinFiles.ReadDir("sets")
	if err != nil {
		panic(err)
	}
	var out []Builtin
	for _, e := range entries {
		data, err := builtinFiles.ReadFile(path.Join("sets", e.Name()))
		if err != nil {
			panic(err)
		}
		set, err := Parse(data)
		if err != nil {
			panic(fmt.Sprintf("built-in requirement set %s: %v", e.Name(), err))
		}
		out = append(out, Builtin{ID: strings.TrimSuffix(e.Name(), ".json"), Set: set})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Set.Name < out[j].Set.Name })
	return out
}

// Builtins lists the built-in sets by name.
func Builtins() []Builtin { return builtins }

// LookupBuiltin finds a built-in set by its reference ("builtin:<ID>") or ID.
func LookupBuiltin(ref string) (Set, bool) {
	id := strings.TrimPrefix(ref, BuiltinPrefix)
	for _, b := range builtins {
		if b.ID == id {
			return b.Set, true
		}
	}
	return Set{}, false
}
