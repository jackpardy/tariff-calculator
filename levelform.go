// levelform.go: the level editor on the requirements page.
package main

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

// The level editor names its fields: name, description, source,
// first.n and first.<i> (the first exercise's options, by reference),
// second_same (ticked when both exercises use the first's requirements), and
// second.n and second.<i>. custom carries the requirements saved in the browser
// ([{id, name, set_routine}]) for the option choices.

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
	var level requirements.Level
	if raw := r.FormValue("level"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &level); err != nil {
			badRequest(w, fmt.Errorf("that isn't a level: %w", err))
			return
		}
		if level.Format == 0 {
			level.Format = requirements.Format
		}
	} else {
		level = parseLevelForm(r)
		applyLevelAction(&level, r.FormValue("action"))
	}
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
		Level: level, JSON: string(data), Problems: problems,
		Groups: requirements.BuiltinGroups(), Custom: custom, CustomJSON: string(customJSON),
	}))
}

// parseLevelForm reads the editor's form into a level; validation happens afterwards.
func parseLevelForm(r *http.Request) requirements.Level {
	options := func(prefix string) []string {
		n, _ := strconv.Atoi(r.FormValue(prefix + ".n"))
		out := make([]string, max(n, 0))
		for i := range out {
			out[i] = strings.TrimSpace(r.FormValue(fmt.Sprintf("%s.%d", prefix, i)))
		}
		return out
	}
	level := requirements.Level{
		Format:      requirements.Format,
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Source:      strings.TrimSpace(r.FormValue("source")),
		First:       requirements.Exercise{Options: options("first")},
	}
	if r.FormValue("second_same") == "" {
		second := options("second")
		if len(second) == 0 {
			second = []string{""} // just unticked: one to choose
		}
		level.Second = &requirements.Exercise{Options: second}
	}
	return level
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
