// main.go
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"tariffCalculator/catalog"
	"tariffCalculator/demo"
	"tariffCalculator/requirements"
	"tariffCalculator/skills"
	"tariffCalculator/static"
	"tariffCalculator/store"
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
	// Competition storage is on only where DATA_DIR is set, a directory that
	// survives deploys (ADR 0004 Decision 8). Without it, or if it can't be
	// opened, the calculator works as ever and only the competition pages
	// say storage isn't available.
	var st *store.Store
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		var err error
		if st, err = store.Open(context.Background(), dir); err != nil {
			log.Printf("Competition storage is off: %v", err)
		} else {
			defer st.Close()
			go deleteExpired(st)
		}
	}
	// "demo" fills the storage with a made-up competition to show off, then stops.
	if len(os.Args) > 1 && os.Args[1] == "demo" {
		if st == nil {
			log.Fatal("The demo needs competition storage: set DATA_DIR.")
		}
		if err := demo.Seed(routesWith(st), os.Stdout, uint64(time.Now().UnixNano())); err != nil {
			log.Fatalf("Demo: %v", err)
		}
		return
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           routesWith(st),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    maxRequestBytes,
	}
	log.Printf("Starting server on :%s\n", port)
	log.Fatal(srv.ListenAndServe())
}

// routes builds the application's handler without competition storage.
func routes() http.Handler {
	return routesWith(nil)
}

// routesWith builds the application's handler, with every request body
// size-limited. st is the competition storage, nil when it's off.
func routesWith(st *store.Store) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET "+static.Prefix, static.Handler())

	mux.HandleFunc("GET /{$}", handleIndex)
	mux.HandleFunc("POST /skill-form", handleSkillForm)
	mux.HandleFunc("POST /skill-inputs", handleSkillInputs)
	mux.HandleFunc("GET /skill-search", handleSkillSearch)
	mux.HandleFunc("POST /calculate-skill", handleCalculateSkill)
	mux.HandleFunc("POST /routine", handleRoutineView)
	mux.HandleFunc("POST /set-routine", handleSetRoutine)
	mux.HandleFunc("POST /qr", handleQR)
	mux.HandleFunc("GET /compare", handleComparePage)
	mux.HandleFunc("POST /compare", handleCompare)
	mux.HandleFunc("GET /requirements", handleRequirementsPage)
	mux.HandleFunc("POST /requirements/editor", handleSetEditor)
	mux.HandleFunc("POST /requirements/level-editor", handleLevelEditor)
	mux.HandleFunc("POST /requirements/set-routine", handleSetRoutineFrom)
	mux.HandleFunc("GET /view", handleViewPage)
	mux.HandleFunc("POST /view", handleView)
	mux.HandleFunc("GET /tariff-sheet", handleTariffSheetPage)
	mux.HandleFunc("POST /tariff-sheet", handleTariffSheet)
	newCompetitionPages(st).register(mux)

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

// validateRoutineJSON parses a routine posted as JSON and validates it.
func validateRoutineJSON(raw string) (skills.RoutineValidation, error) {
	routine, err := namedRoutine(raw)
	if err != nil {
		return skills.RoutineValidation{}, err
	}
	return skills.ValidateRoutine(routine), nil
}

// namedRoutine parses a routine posted as JSON. Official names are always
// re-derived, so names stored by older versions are corrected.
func namedRoutine(raw string) ([]skills.TrampolineSkill, error) {
	routine, err := parseRoutineJSON(raw)
	if err != nil {
		return nil, err
	}
	for i := range routine {
		routine[i].Name = skills.FindCommonSkillName(routine[i])
	}
	return routine, nil
}

// checkedRoutine is a posted routine, validated with the checks that apply to
// it and checked against its requirement set, and against its level if it has one.
type checkedRoutine struct {
	rv       skills.RoutineValidation
	check    *views.RequirementCheck // nil without a set
	required map[int]bool            // elements meeting a requirement, for the sheet
	checks   views.Checks
	level    *views.LevelCheck // nil without a level
	pair     *checkedRoutine   // the routine doing the level's other exercise, if posted and valid
}

// postedRoutine is a routine as the page posts it: its skills, its requirement
// set (a built-in reference, a custom set as JSON, or nothing) and its own
// choice of checks, all as posted.
type postedRoutine struct {
	data, set, checks string
}

// checkRoutine reads the posted routine (routineData, requirementSet, checks)
// and validates and checks it. A routine doing one of a level's exercises also
// posts the level (level: a built-in reference or a custom level as JSON),
// which exercise it is (exercise: 1 or 2), and the routine doing the other
// exercise, if there is one (pairData, pairSet, pairChecks, pairName). The two
// are checked together (requirements.CheckPair): when the first exercise
// scores only some elements, their difficulty carries over, and they can't be
// repeated in the second.
func checkRoutine(r *http.Request) (checkedRoutine, error) {
	if err := r.ParseForm(); err != nil {
		return checkedRoutine{}, fmt.Errorf("parsing form: %w", err)
	}
	own := postedRoutine{r.FormValue("routineData"), r.FormValue("requirementSet"), r.FormValue("checks")}
	// A set routine shown as prescribed (prescribed=1) is the routine its requirements describe.
	if r.FormValue("prescribed") == "1" {
		own.data = prescribedRoutine(own.set)
	}
	raw := strings.TrimSpace(r.FormValue("level"))
	if raw == "" {
		return checkPosted(own)
	}

	exercise := 1
	if r.FormValue("exercise") == "2" {
		exercise = 2
	}
	lc := &views.LevelCheck{Exercise: exercise, OtherName: strings.TrimSpace(r.FormValue("pairName"))}
	level, err := requirements.ResolveLevel(raw)
	lc.Name = level.Name
	if err != nil {
		lc.Err = err.Error()
		out, err := checkPosted(own)
		out.level = lc
		return out, err
	}

	ownRoutine, err := routineFrom(own)
	if err != nil {
		return checkedRoutine{}, err
	}
	alone := func() (checkedRoutine, error) {
		out := checkedFrom(requirements.Check(ownRoutine, nil))
		out.level = lc
		return out, nil
	}
	if !r.Form.Has("pairData") {
		return alone()
	}
	pair := postedRoutine{r.FormValue("pairData"), r.FormValue("pairSet"), r.FormValue("pairChecks")}
	// A set routine is posted without skills: it's the routine its requirements describe.
	if strings.TrimSpace(pair.data) == "" {
		pair.data = prescribedRoutine(pair.set)
	}
	lc.HasOther = true
	pairRoutine, err := routineFrom(pair)
	if err != nil {
		lc.OtherErr = err.Error()
		return alone()
	}

	first, second := ownRoutine, pairRoutine
	if exercise == 2 {
		first, second = pairRoutine, ownRoutine
	}
	checked := requirements.CheckPair(first, second)
	out, other := checkedFrom(checked.First), checkedFrom(checked.Second)
	if exercise == 2 {
		out, other = other, out
	}
	if other.check != nil && other.check.Err == "" {
		lc.OtherMet, lc.OtherRules = other.check.Met(), len(other.check.Results)
	}
	lc.Carried, lc.Repeated = checked.Carried, checked.Repeated
	out.pair, out.level = &other, lc
	return out, nil
}

// prescribedRoutine is the routine a posted requirement set's set routine
// describes, as JSON, or "" (an empty routine) if it isn't a set routine.
func prescribedRoutine(rawSet string) string {
	set, err := requirements.ResolveSet(rawSet)
	if err != nil {
		return ""
	}
	routine, ok := requirements.SetRoutine(set)
	if !ok {
		return ""
	}
	data, err := json.Marshal(routine)
	if err != nil {
		return ""
	}
	return string(data)
}

// routineFrom is a posted routine as the engine checks it. A set that can't be
// used is reported in the check rather than failing the request.
func routineFrom(p postedRoutine) (requirements.Routine, error) {
	routine, err := parseRoutineJSON(p.data)
	if err != nil {
		return requirements.Routine{}, err
	}
	out := requirements.Routine{Skills: routine, Checks: p.checks}
	if raw := strings.TrimSpace(p.set); raw != "" {
		set, err := requirements.ResolveSet(raw)
		if err != nil {
			out.SetName, out.SetErr = set.Name, err
		} else {
			out.Set = &set
		}
	}
	return out, nil
}

// checkPosted validates one posted routine and checks it against its
// requirement set.
func checkPosted(p postedRoutine) (checkedRoutine, error) {
	routine, err := routineFrom(p)
	if err != nil {
		return checkedRoutine{}, err
	}
	return checkedFrom(requirements.Check(routine, nil)), nil
}

// checkedFrom is a routine the engine checked, as the views show it.
func checkedFrom(c requirements.Checked) checkedRoutine {
	out := checkedRoutine{rv: c.Validation, required: c.Required, checks: c.Checks}
	if c.HasSet {
		out.check = &views.RequirementCheck{SetName: c.SetName, Results: c.Results}
		if c.SetErr != nil {
			out.check.Err = c.SetErr.Error()
		}
	}
	return out
}

// handleRoutineView renders the routine builder (cards, validation, totals).
func handleRoutineView(w http.ResponseWriter, r *http.Request) {
	checked, err := checkRoutine(r)
	if err != nil {
		log.Printf("Error parsing routine for view: %v", err)
		badRequest(w, err)
		return
	}
	rv, check := checked.rv, checked.check
	side := views.RoutineSide{Side: "a", Checks: checked.checks, Level: checked.level, ReadOnly: r.FormValue("prescribed") == "1"}
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

// handleSetRoutine returns the routine a set's set routine describes, as the
// skills the page stores, so it can be loaded into the builder. It also says
// whether the posted routine (routineData) already is that routine.
func handleSetRoutine(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	set, err := requirements.ResolveSet(r.FormValue("requirementSet"))
	if err != nil {
		badRequest(w, err)
		return
	}
	routine, ok := requirements.SetRoutine(set)
	if !ok {
		badRequest(w, errors.New("these requirements have no set routine to load"))
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

// handleSetRoutineFrom makes a set routine of a posted routine (routineData),
// named name: requirements with exactly its skills, for the page to save. This
// is how coaches write set routines: by building them.
func handleSetRoutineFrom(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	routine, err := namedRoutine(r.FormValue("routineData"))
	if err != nil {
		badRequest(w, err)
		return
	}
	if len(routine) == 0 {
		badRequest(w, errors.New("the routine has no skills"))
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = "Set routine"
	}
	set := requirements.SetRoutineFrom(name, routine)
	set.Source = strings.TrimSpace(r.FormValue("source"))
	if err := set.Validate(); err != nil {
		badRequest(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(set); err != nil {
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
// validated each time and its problems listed; a valid set is also checked for
// rules that can't be met (which doesn't stop it being saved).
func handleSetEditor(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		badRequest(w, err)
		return
	}
	var set requirements.Set
	var problems []string
	if raw := r.FormValue("set"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &set); err != nil {
			badRequest(w, fmt.Errorf("those aren't requirements: %w", err))
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
	var conflicts []string
	if err := set.Validate(); err != nil {
		problems = append(problems, strings.Split(err.Error(), "\n")...)
	} else {
		conflicts = requirements.Conflicts(set)
	}
	data, err := json.Marshal(set)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	render(w, r, views.SetEditor(views.SetEditorData{Set: set, JSON: string(data), Problems: problems, Conflicts: conflicts}))
}

// handleViewPage serves the view screen, which loads a routine saved in the
// browser (and the routine doing its level's other exercise) to show full screen.
func handleViewPage(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.ViewPage())
}

// handleView renders the view screen for the posted routine (named
// routineName). For a level's exercise, it adds the routine doing the other
// exercise, and the level's set routine options that no routine is doing (so
// both options show beside the voluntary), all in exercise and option order.
// The page posts which options the two routines use (optionRef, pairOptionRef),
// the JSON of any custom requirements the level uses (optionSets, by id), and
// what it calls each option (tabNames, by reference).
func handleView(w http.ResponseWriter, r *http.Request) {
	checked, err := checkRoutine(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	name := strings.TrimSpace(r.FormValue("routineName"))
	if name == "" {
		name = "Routine"
	}
	lc := checked.level
	if lc == nil || lc.Err != "" {
		columns := []views.DisplayColumn{displayColumn(checked, name, nil, 0)}
		// A routine shown beside it (besideData, besideSet, besideChecks,
		// besideName), as in the builder, each checked against its own requirements.
		if r.Form.Has("besideData") {
			beside, err := checkPosted(postedRoutine{r.FormValue("besideData"), r.FormValue("besideSet"), r.FormValue("besideChecks")})
			if err != nil {
				badRequest(w, fmt.Errorf("the routine beside it: %w", err))
				return
			}
			col := displayColumn(beside, strings.TrimSpace(r.FormValue("besideName")), nil, 0)
			col.Key = "beside"
			columns = append(columns, col)
		}
		render(w, r, views.RoutineDisplay(columns))
		return
	}
	level, err := requirements.ResolveLevel(r.FormValue("level"))
	if err != nil {
		badRequest(w, err)
		return
	}

	// Each column's place: its exercise, then its option's position.
	type placed struct {
		column      views.DisplayColumn
		exercise, n int
	}
	optionIndex := func(exercise int, ref string) int {
		return max(slices.Index(level.Exercise(exercise).Options, ref), 0)
	}
	ownRef, pairRef := r.FormValue("optionRef"), r.FormValue("pairOptionRef")
	own := displayColumn(checked, name, lc, lc.Exercise)
	own.Key = columnKey(lc.Exercise, ownRef)
	columns := []placed{{own, lc.Exercise, optionIndex(lc.Exercise, ownRef)}}
	performed := map[string]bool{ownRef: true}
	if checked.pair != nil {
		other := displayColumn(*checked.pair, lc.OtherName, lc, 3-lc.Exercise)
		other.Key = columnKey(3-lc.Exercise, pairRef)
		columns = append(columns, placed{other, 3 - lc.Exercise, optionIndex(3-lc.Exercise, pairRef)})
		performed[pairRef] = true
	}

	var custom map[string]json.RawMessage
	_ = json.Unmarshal([]byte(r.FormValue("optionSets")), &custom) // none: built-in options only
	var tabNames map[string]string
	_ = json.Unmarshal([]byte(r.FormValue("tabNames")), &tabNames) // none: options go by their requirements' names
	for exercise := 1; exercise <= 2; exercise++ {
		for i, ref := range level.Exercise(exercise).Options {
			if performed[ref] {
				continue
			}
			performed[ref] = true // an option both exercises share shows once
			set, ok := requirements.LookupBuiltin(ref)
			if !ok {
				if raw, saved := custom[ref]; !saved {
					continue
				} else if set, err = requirements.Parse(raw); err != nil {
					continue
				}
			}
			routine, ok := requirements.SetRoutine(set)
			if !ok {
				continue // requirements for a voluntary: nothing to show
			}
			c := checkedFrom(requirements.Check(requirements.Routine{Skills: routine, Set: &set}, nil))
			name := set.Name
			if tabNames[ref] != "" {
				name = tabNames[ref]
			}
			col := displayColumn(c, name, lc, exercise)
			col.Key, col.Option = columnKey(exercise, ref), true
			columns = append(columns, placed{col, exercise, i})
		}
	}
	slices.SortStableFunc(columns, func(a, b placed) int {
		return cmp.Or(cmp.Compare(a.exercise, b.exercise), cmp.Compare(a.n, b.n))
	})
	out := make([]views.DisplayColumn, len(columns))
	for i, c := range columns {
		out[i] = c.column
	}
	render(w, r, views.RoutineDisplay(out))
}

// columnKey names a level's column on the view screen, so the page can
// remember which ones the coach hides: its exercise and option, e.g.
// "1:builtin:bucs-l7-option-2".
func columnKey(exercise int, ref string) string {
	return fmt.Sprintf("%d:%s", exercise, ref)
}

// displayColumn is a checked routine as a view screen column; with a level,
// as its exercise.
func displayColumn(c checkedRoutine, name string, level *views.LevelCheck, exercise int) views.DisplayColumn {
	col := views.DisplayColumn{Name: name, Key: "routine", Validation: c.rv, Check: c.check, Required: c.required, Checks: c.checks}
	if level != nil {
		col.Level, col.Exercise = level.Name, exercise
	}
	return col
}

// handleTariffSheetPage serves the tariff sheet page, which loads the sheet for
// the routine saved in the browser.
func handleTariffSheetPage(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.TariffSheetPage())
}

// handleTariffSheet renders the sheet itself for the posted routine.
func handleTariffSheet(w http.ResponseWriter, r *http.Request) {
	checked, err := checkRoutine(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	render(w, r, views.TariffSheet(checked.rv, checked.required, checked.checks, nil))
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
