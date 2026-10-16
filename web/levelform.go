// levelform.go: the level editor on the requirements page.
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"tariffCalculator/requirements"
	"tariffCalculator/views"
)

// The level editor names its fields: name, description, source, structure
// (one of the views.Structure constants), first.n and first.<i> (the first
// exercise's options, by reference), and second.n and second.<i>. custom
// carries the requirements saved in the browser ([{id, name, set_routine}])
// for the choices.

// handleLevelEditor renders the level editor, either for a level posted as JSON
// in "level" (opening, duplicating or importing one) or for the editor's own
// form after applying any button action. The level is validated each time and
// its problems listed.
func handleLevelEditor(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	var custom []views.CustomRequirements
	if raw := r.FormValue("custom"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &custom); err != nil {
			badRequest(w, fmt.Errorf("reading your requirements: %w", err))
			return
		}
	}
	// isSet reports whether requirements are a set routine (built-in or the user's).
	isSet := func(ref string) bool {
		if set, ok := requirements.LookupBuiltin(ref); ok {
			_, isSetRoutine := requirements.SetRoutine(set)
			return isSetRoutine
		}
		i := slices.IndexFunc(custom, func(c views.CustomRequirements) bool { return c.ID == ref })
		return i >= 0 && custom[i].SetRoutine
	}
	var level requirements.Level
	var structure string
	if raw := r.FormValue("level"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &level); err != nil {
			badRequest(w, fmt.Errorf("that isn't a level: %w", err))
			return
		}
		if level.Format == 0 {
			level.Format = requirements.Format
		}
		structure = levelStructure(level, isSet)
	} else {
		level, structure = parseLevelForm(r, isSet)
		applyLevelAction(&level, r.FormValue("action"))
	}
	heldJSON, _ := json.Marshal(heldOf(r, level))
	if level.First.Options == nil {
		level.First.Options = []string{}
	}

	var problems []string
	if err := level.Validate(); err != nil {
		problems = strings.Split(err.Error(), "\n")
	}
	saved := func(ref string) bool {
		return slices.ContainsFunc(custom, func(c views.CustomRequirements) bool { return c.ID == ref })
	}
	for n := 1; n <= 2; n++ {
		for _, ref := range level.Exercise(n).Options {
			if ref != "" && !strings.HasPrefix(ref, requirements.BuiltinPrefix) && !saved(ref) {
				problems = append(problems, fmt.Sprintf("the %s exercise uses requirements that are no longer saved here", []string{"first", "second"}[n-1]))
			}
		}
		if level.Second == nil {
			break
		}
	}

	data, err := json.Marshal(level)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	customJSON, err := json.Marshal(custom)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	render(w, r, views.LevelEditor(views.LevelEditorData{
		Level: level, Structure: structure, JSON: string(data), Problems: problems, Tried: r.FormValue("tried") == "1", Held: string(heldJSON),
		Groups: requirements.BuiltinGroups(), Custom: custom, CustomJSON: string(customJSON),
	}))
}

// levelStructure is the shape of a level being opened: whether its first
// exercise is set routines, and whether it has a second. A new level (nothing
// chosen yet) is a set routine then a voluntary.
func levelStructure(l requirements.Level, isSet func(string) bool) string {
	firstIsSet := true
	for _, ref := range l.First.Options {
		if ref != "" {
			firstIsSet = isSet(ref)
			break
		}
	}
	switch {
	case firstIsSet && l.Second != nil:
		return views.StructureSetVoluntary
	case firstIsSet:
		return views.StructureSet
	case l.Second != nil:
		return views.StructureTwoVoluntaries
	default:
		return views.StructureVoluntary
	}
}

// parseLevelForm reads the editor's form into a level of the structure chosen.
// Each slot keeps only what fits it (set routines in a set routine slot,
// requirements in a voluntary one), so changing the structure carries over
// what still makes sense; what doesn't fit is held (held: a JSON list of
// references) and comes back when a structure it fits is chosen again.
// Validation happens afterwards.
func parseLevelForm(r *http.Request, isSet func(string) bool) (requirements.Level, string) {
	options := func(prefix string) []string {
		n, _ := strconv.Atoi(r.FormValue(prefix + ".n"))
		out := make([]string, max(n, 0))
		for i := range out {
			out[i] = strings.TrimSpace(r.FormValue(fmt.Sprintf("%s.%d", prefix, i)))
		}
		return out
	}
	first, second := options("first"), options("second")
	var held []string
	if raw := r.FormValue("held"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &held) // a bad one holds nothing
	}
	// sets are the set routines chosen, at least one slot to choose in; with
	// none chosen, those held from before.
	sets := func() []string {
		var out []string
		for _, ref := range first {
			if ref == "" || isSet(ref) {
				out = append(out, ref)
			}
		}
		if !slices.ContainsFunc(out, func(ref string) bool { return ref != "" }) {
			if back := slices.DeleteFunc(slices.Clone(held), func(ref string) bool { return !isSet(ref) }); len(back) > 0 {
				out = back
			}
		}
		if len(out) == 0 {
			out = []string{""}
		}
		return out
	}
	// voluntary is the first voluntary's requirements found in these, or "".
	voluntary := func(lists ...[]string) []string {
		for _, list := range lists {
			for _, ref := range list {
				if ref != "" && !isSet(ref) {
					return []string{ref}
				}
			}
		}
		return []string{""}
	}

	level := requirements.Level{
		Format:      requirements.Format,
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Source:      strings.TrimSpace(r.FormValue("source")),
	}
	structure := r.FormValue("structure")
	switch structure {
	case views.StructureVoluntary:
		level.First = requirements.Exercise{Options: voluntary(first, second, held)}
	case views.StructureTwoVoluntaries:
		level.First = requirements.Exercise{Options: voluntary(first, held)}
		level.Second = &requirements.Exercise{Options: voluntary(second, slices.DeleteFunc(slices.Clone(held), func(ref string) bool { return ref == level.First.Options[0] }))}
	case views.StructureSet:
		level.First = requirements.Exercise{Options: sets()}
	default:
		structure = views.StructureSetVoluntary
		level.First = requirements.Exercise{Options: sets()}
		level.Second = &requirements.Exercise{Options: voluntary(second, first, held)}
	}
	return level, structure
}

// heldOf is what a level's slots don't use of what the form had: chosen
// before, held for a structure that fits it.
func heldOf(r *http.Request, level requirements.Level) []string {
	used := slices.Concat(level.First.Options)
	if level.Second != nil {
		used = slices.Concat(used, level.Second.Options)
	}
	var pool []string
	for _, prefix := range []string{"first", "second"} {
		n, _ := strconv.Atoi(r.FormValue(prefix + ".n"))
		for i := range max(n, 0) {
			pool = append(pool, strings.TrimSpace(r.FormValue(fmt.Sprintf("%s.%d", prefix, i))))
		}
	}
	var held []string
	if raw := r.FormValue("held"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &held)
	}
	var out []string
	for _, ref := range slices.Concat(pool, held) {
		if ref != "" && !slices.Contains(used, ref) && !slices.Contains(out, ref) {
			out = append(out, ref)
		}
	}
	return out
}

// applyLevelAction applies an editor button: "add:first" or "add:second" adds
// an option to choose, "delete:first:2" removes one.
func applyLevelAction(level *requirements.Level, action string) {
	parts := strings.Split(action, ":")
	if len(parts) < 2 {
		return
	}
	exercise := &level.First
	if parts[1] == "second" {
		if level.Second == nil {
			return
		}
		exercise = level.Second
	}
	switch parts[0] {
	case "add":
		exercise.Options = append(exercise.Options, "")
	case "delete":
		if len(parts) != 3 {
			return
		}
		if i, err := strconv.Atoi(parts[2]); err == nil && i >= 0 && i < len(exercise.Options) {
			exercise.Options = slices.Delete(exercise.Options, i, i+1)
		}
	}
}
