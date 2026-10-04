// main.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"tariffCalculator/catalog"
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
	mux.HandleFunc("GET /tariff-sheet", handleTariffSheetPage)
	mux.HandleFunc("POST /tariff-sheet", handleTariffSheet)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
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
	render(w, r, views.Page())
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

// validatedRoutine parses and validates the routine the browser posts. Official
// names are always re-derived, so names stored by older versions are corrected.
func validatedRoutine(r *http.Request) (skills.RoutineValidation, error) {
	routine, err := parseRoutineFromRequest(r)
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
	render(w, r, views.Routine(rv))
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
	render(w, r, views.TariffSheet(rv))
}

// parseRoutineFromRequest parses the routine the page posts as JSON in the
// routineData form value. Each skill is normalised to its phase count,
// validated and priced.
func parseRoutineFromRequest(r *http.Request) ([]skills.TrampolineSkill, error) {
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("parsing form: %w", err)
	}
	routine := []skills.TrampolineSkill{}
	if raw := r.FormValue("routineData"); raw != "" {
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
