// main.go
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"tariffCalculator/catalog"
	"tariffCalculator/requirements"
	"tariffCalculator/skills"
	"tariffCalculator/static"
	"tariffCalculator/views"
)

// maxRequestBytes bounds request bodies and headers; a full routine is a few kilobytes.
const maxRequestBytes = 64 << 10

// --- Main Function ---
func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    maxRequestBytes,
	}
	log.Printf("Starting server on :%s\n", port)
	log.Fatal(srv.ListenAndServe())
}

// routes builds the application's handler, with every request body size-limited.
func routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET "+static.Prefix, static.Handler())

	mux.HandleFunc("GET /{$}", handleIndex)
	mux.HandleFunc("POST /skill-form", handleSkillForm)
	mux.HandleFunc("POST /skill-inputs", handleSkillInputs)
	mux.HandleFunc("GET /skill-search", handleSkillSearch)
	mux.HandleFunc("POST /calculate-skill", handleCalculateSkill)
	mux.HandleFunc("POST /routine", handleRoutineView)
	mux.HandleFunc("POST /set-routine", handleSetRoutine)
	mux.HandleFunc("GET /compare", handleComparePage)
	mux.HandleFunc("POST /compare", handleCompare)
	mux.HandleFunc("GET /requirements", handleRequirementsPage)
	mux.HandleFunc("POST /requirements/editor", handleSetEditor)
	mux.HandleFunc("GET /tariff-sheet", handleTariffSheetPage)
	mux.HandleFunc("POST /tariff-sheet", handleTariffSheet)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
		w.Header().Set(static.VersionHeader, static.Version)
		mux.ServeHTTP(w, r)
	})
}

// --- Route Handlers ---

// render writes a templ component, logging (rather than reporting) render errors,
// since the response may already be partly written.
func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	if err := c.Render(r.Context(), w); err != nil {
		log.Printf("Error rendering %s: %v", r.URL.Path, err)
	}
}

// badRequest reports a client error with a message the page shows to the user.
func badRequest(w http.ResponseWriter, err error) {
	http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
}

// defaultSkill is the skill the form starts with: a front somersault in the straight position.
func defaultSkill() skills.TrampolineSkill {
	return skills.TrampolineSkill{Rotation: 4, TakeoffPosition: skills.Feet, Shape: skills.Straight, TwistDistribution: []int{0}}
}

// handleIndex serves the calculator page; the form and routine load into it.
func handleIndex(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.Page(requirements.BuiltinGroups()))
}

// prepared readies a skill for the editor: one twist per phase, straddle only for
// basic jumps, and its official name and tariff set.
func prepared(s skills.TrampolineSkill) skills.TrampolineSkill {
	s.NormalizePhases()
	if s.Shape == skills.Straddle && !s.IsBasicJump() {
		s.Shape = skills.Straight // straddle only applies to basic jumps
	}
	s.Name = skills.FindCommonSkillName(s)
	s.SetTariff()
	return s
}

// editIndexFrom is the routine index the form is editing, or -1 when adding.
func editIndexFrom(r *http.Request) int {
	index, err := strconv.Atoi(r.FormValue("editIndex"))
	if err != nil || index < 0 {
		return -1
	}
	return index
}

// handleSkillForm renders the "Add a skill" panel: for the default skill, or
// (when the page posts the routine skill being edited as JSON with its index)
// loaded with that skill and its builder open.
func handleSkillForm(w http.ResponseWriter, r *http.Request) {
	editor := views.SkillEditor{Skill: defaultSkill(), EditIndex: -1}

	if raw := r.FormValue("skill"); raw != "" {
		var s skills.TrampolineSkill
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			badRequest(w, fmt.Errorf("decoding skill: %w", err))
			return
		}
		s.NormalizePhases()
		if err := s.Validate(); err != nil {
			badRequest(w, err)
			return
		}
		if editor.EditIndex = editIndexFrom(r); editor.EditIndex < 0 {
			badRequest(w, fmt.Errorf("invalid edit index %q", r.FormValue("editIndex")))
			return
		}
		editor.Skill, editor.BuilderOpen = s, true
	}

	editor.Skill = prepared(editor.Skill)
	render(w, r, views.SkillFormView(views.SkillForm{Editor: editor, Categories: catalog.Categories()}))
}

// handleSkillInputs re-renders the editor (skill card and builder), either for a
// common skill the user just chose or for the values currently in the form; the
// label, edit index and builder state carry over from the form. While the form
// holds something unscorable (e.g. a rotation being typed), it answers 204 so
// htmx leaves the user's input alone; Add reports the problem.
func handleSkillInputs(w http.ResponseWriter, r *http.Request) {
	editor := views.SkillEditor{EditIndex: editIndexFrom(r), BuilderOpen: r.FormValue("builder_open") == "1"}

	var s skills.TrampolineSkill
	switch r.FormValue("load") {
	case "common": // a skill chosen in the picker
		common, ok := skills.CommonSkills[r.FormValue("commonSkillKey")]
		if !ok {
			w.WriteHeader(http.StatusNoContent) // not a common skill
			return
		}
		s = common
		// The picker can ask for a shape (a jump's, or a somersault's shape button).
		if shape := skills.ShapeFromString(r.FormValue("shape")); shape != skills.InvalidShape {
			s.Shape = shape
		}
		s.CustomName = strings.TrimSpace(r.FormValue("custom_name"))
	case "skill": // a search result
		if err := json.Unmarshal([]byte(r.FormValue("skill")), &s); err != nil {
			badRequest(w, fmt.Errorf("decoding skill: %w", err))
			return
		}
		s.NormalizePhases()
		if err := s.Validate(); err != nil {
			badRequest(w, err)
			return
		}
		s.CustomName = strings.TrimSpace(r.FormValue("custom_name"))
	default:
		parsed, err := parseSkillFromForm(r)
		if err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s = parsed
	}

	editor.Skill = prepared(s)
	render(w, r, views.SkillEditorView(editor))
}

// handleSkillSearch lists the skills matching the picker's search box, by name or
// FIG notation (an empty query lists nothing).
func handleSkillSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.FormValue("q"))
	if q == "" {
		return
	}
	render(w, r, views.SearchResults(q, catalog.Search(q)))
}

// parseSkillFromForm reads and validates the skill in the "Add a skill" form. The
// form shows one twist box per phase; if the rotation has just changed, extra
// values are ignored and missing phases count as no twist.
func parseSkillFromForm(r *http.Request) (skills.TrampolineSkill, error) {
	skill := skills.TrampolineSkill{
		CustomName:      strings.TrimSpace(r.FormValue("custom_name")),
		TakeoffPosition: skills.BodyPositionFromString(r.FormValue("takeoff_position")),
		Shape:           skills.ShapeFromString(r.FormValue("shape")),
		Backward:        r.FormValue("backward") == "on",
		SeatLanding:     r.FormValue("seat_landing") == "on",
	}
	rotation, err := strconv.Atoi(r.FormValue("rotation"))
	if err != nil {
		return skill, fmt.Errorf("invalid rotation %q", r.FormValue("rotation"))
	}
	skill.Rotation = rotation

	twists := r.Form["twist_distribution[]"]
	skill.TwistDistribution = make([]int, skills.CalculatePhases(skill.Rotation))
	for i := range skill.TwistDistribution {
		if i >= len(twists) {
			break
		}
		twist, err := strconv.Atoi(twists[i])
		if err != nil {
			return skill, fmt.Errorf("invalid twist %q in phase %d", twists[i], i+1)
		}
		skill.TwistDistribution[i] = twist
	}
	return skill, skill.Validate()
}

// calculatedSkill is the form's skill with its official name and tariff set.
func calculatedSkill(r *http.Request) (skills.TrampolineSkill, error) {
	skill, err := parseSkillFromForm(r)
	if err != nil {
		return skill, err
	}
	skill.Name = skills.FindCommonSkillName(skill)
	skill.SetTariff()
	return skill, nil
}

// handleCalculateSkill returns the form's skill as the routine stores it, for
// the page to add to (or replace in) the routine.
func handleCalculateSkill(w http.ResponseWriter, r *http.Request) {
	skill, err := calculatedSkill(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(skill); err != nil {
		log.Printf("Error encoding calculated skill: %v", err)
	}
}

// validatedRoutine parses and validates the routine the browser posts in the
// routineData form value.
func validatedRoutine(r *http.Request) (skills.RoutineValidation, error) {
	if err := r.ParseForm(); err != nil {
		return skills.RoutineValidation{}, fmt.Errorf("parsing form: %w", err)
	}
	return validateRoutineJSON(r.FormValue("routineData"))
}

// validateRoutineJSON parses a routine posted as JSON and validates it. Official
// names are always re-derived, so names stored by older versions are corrected.
func validateRoutineJSON(raw string) (skills.RoutineValidation, error) {
	routine, err := parseRoutineJSON(raw)
	if err != nil {
		return skills.RoutineValidation{}, err
	}
	for i := range routine {
		routine[i].Name = skills.FindCommonSkillName(routine[i])
	}
	return skills.ValidateRoutine(routine), nil
}

// handleRoutineView renders the routine builder (cards, validation, totals).
func handleRoutineView(w http.ResponseWriter, r *http.Request) {
	rv, err := validatedRoutine(r)
	if err != nil {
		log.Printf("Error parsing routine for view: %v", err)
		badRequest(w, err)
		return
	}
	check, _ := requirementCheck(r, rv)
	side := views.RoutineSide{Side: "a"}
	if r.FormValue("side") == "b" {
		side.Side = "b"
	}
	// When comparing, the other column's routine marks where the two differ.
	if raw := r.FormValue("compareData"); raw != "" {
		if other, err := validateRoutineJSON(raw); err == nil {
			side.Other = &views.CompareSide{Name: r.FormValue("compareName"), Validation: other}
		}
	}
	render(w, r, views.Routine(rv, check, side))
}

// requirementCheck checks the routine against the requirement set the page
// posts in requirementSet: a built-in reference ("builtin:<id>"), a custom set
// as JSON, or nothing. A set that can't be used is reported in the check rather
// than failing the request. It also returns the elements that meet a
// required-element rule.
func requirementCheck(r *http.Request, rv skills.RoutineValidation) (*views.RequirementCheck, map[int]bool) {
	raw := strings.TrimSpace(r.FormValue("requirementSet"))
	if raw == "" {
		return nil, nil
	}
	set, err := postedSet(raw)
	if err != nil {
		name := set.Name
		if name == "" {
			name = "Requirements"
		}
		return &views.RequirementCheck{SetName: name, Err: err.Error()}, nil
	}
	results := requirements.Evaluate(set, rv)
	_, isSetRoutine := requirements.SetRoutine(set)
	check := &views.RequirementCheck{SetName: set.Name, Results: results, SetRoutine: isSetRoutine}
	return check, requirements.RequiredElements(set, results)
}

// postedSet is the requirement set the page posts: a built-in reference
// ("builtin:<id>") or a custom set as JSON. A set that doesn't parse comes back
// with whatever name it has.
func postedSet(raw string) (requirements.Set, error) {
	if strings.HasPrefix(raw, requirements.BuiltinPrefix) {
		builtin, ok := requirements.LookupBuiltin(raw)
		if !ok {
			return requirements.Set{}, errors.New("this built-in set no longer exists")
		}
		return builtin, nil
	}
	return requirements.Parse([]byte(raw))
}

// handleSetRoutine returns the routine a set's set routine describes, as the
// skills the page stores, so it can be loaded into the builder. It also says
// whether the posted routine (routineData) already is that routine.
func handleSetRoutine(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	set, err := postedSet(strings.TrimSpace(r.FormValue("requirementSet")))
	if err != nil {
		badRequest(w, err)
		return
	}
	routine, ok := requirements.SetRoutine(set)
	if !ok {
		badRequest(w, errors.New("this set has no set routine to load"))
		return
	}
	matches := false
	if raw := r.FormValue("routineData"); raw != "" {
		if current, err := parseRoutineJSON(raw); err == nil {
			matches = len(current) == len(routine)
			for i := range current {
				if matches && !(current[i].Equal(&routine[i]) && current[i].Shape == routine[i].Shape) {
					matches = false
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		Name    string                   `json:"name"`
		Skills  []skills.TrampolineSkill `json:"skills"`
		Matches bool                     `json:"matches"`
	}{set.Name, routine, matches}); err != nil {
		log.Printf("Error writing set routine: %v", err)
	}
}

// handleComparePage serves the compare page; it fills its routine choices from
// the routines saved in the browser and loads the comparison.
func handleComparePage(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.ComparePage())
}

// handleCompare renders two posted routines side by side.
func handleCompare(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	var sides [2]views.CompareSide
	for i, key := range []string{"a", "b"} {
		rv, err := validateRoutineJSON(r.FormValue(key + "Data"))
		if err != nil {
			badRequest(w, fmt.Errorf("routine %s: %w", strings.ToUpper(key), err))
			return
		}
		name := strings.TrimSpace(r.FormValue(key + "Name"))
		if name == "" {
			name = "Routine " + strings.ToUpper(key)
		}
		sides[i] = views.CompareSide{Name: name, Validation: rv}
	}
	render(w, r, views.Comparison(sides[0], sides[1]))
}

// handleRequirementsPage serves the requirement sets page: built-in sets, the
// sets saved in the browser, and the set editor.
func handleRequirementsPage(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.RequirementsPage(requirements.BuiltinGroups()))
}

// handleSetEditor renders the requirement set editor, either for a set posted
// as JSON in "set" (opening, duplicating or importing one) or for the editor's
// own form after applying any button action or "Add a rule" choice. The set is
// validated each time and its problems listed.
func handleSetEditor(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	var set requirements.Set
	var problems []string
	if raw := r.FormValue("set"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &set); err != nil {
			badRequest(w, fmt.Errorf("that isn't a requirement set: %w", err))
			return
		}
		if set.Format == 0 {
			set.Format = requirements.Format
		}
	} else {
		set, problems = parseSetForm(r)
		applySetAction(&set, r.FormValue("action"), r.FormValue("add"))
	}
	if set.Rules == nil {
		set.Rules = []requirements.Rule{}
	}
	if err := set.Validate(); err != nil {
		problems = append(problems, strings.Split(err.Error(), "\n")...)
	}
	data, err := json.Marshal(set)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	render(w, r, views.SetEditor(views.SetEditorData{Set: set, JSON: string(data), Problems: problems}))
}

// handleTariffSheetPage serves the tariff sheet page, which loads the sheet for
// the routine saved in the browser.
func handleTariffSheetPage(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.TariffSheetPage())
}

// handleTariffSheet renders the sheet itself for the posted routine.
func handleTariffSheet(w http.ResponseWriter, r *http.Request) {
	rv, err := validatedRoutine(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	_, required := requirementCheck(r, rv)
	render(w, r, views.TariffSheet(rv, required))
}

// parseRoutineJSON parses a routine posted as JSON ("" is an empty routine).
// Each skill is normalised to its phase count, validated and priced.
func parseRoutineJSON(raw string) ([]skills.TrampolineSkill, error) {
	routine := []skills.TrampolineSkill{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &routine); err != nil {
			return nil, fmt.Errorf("decoding routine JSON: %w", err)
		}
	}
	for i := range routine {
		routine[i].NormalizePhases()
		if err := routine[i].Validate(); err != nil {
			return nil, fmt.Errorf("skill %d: %w", i+1, err)
		}
		routine[i].SetTariff()
	}
	return routine, nil
}
