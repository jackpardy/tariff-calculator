// main.go
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"tariffCalculator/skills"
	"tariffCalculator/views"
)

// tmpl holds the page shell (base.html, calculator.html); everything rendered
// into it is a templ component from the views package.
var tmpl *template.Template

func loadTemplates() {
	tmpl = template.Must(template.ParseGlob("templates/*.html"))
}

// maxRequestBytes bounds request bodies and headers; a full routine is a few kilobytes.
const maxRequestBytes = 64 << 10

// --- Main Function ---
func main() {
	loadTemplates()

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
	mux.Handle("/static/", http.StripPrefix("/static/", staticFileServer("static")))

	mux.HandleFunc("GET /{$}", handleIndex)
	mux.HandleFunc("POST /skill-form", handleSkillForm)
	mux.HandleFunc("POST /skill-inputs", handleSkillInputs)
	mux.HandleFunc("GET /common-skills-options", handleCommonSkillsOptions)
	mux.HandleFunc("POST /skill-evaluation", handleSkillEvaluation)
	mux.HandleFunc("POST /calculate-skill", handleCalculateSkill)
	mux.HandleFunc("POST /routine", handleRoutineView)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
		mux.ServeHTTP(w, r)
	})
}

// --- Static File Server ---
func staticFileServer(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Asset URLs are not versioned, so browsers must revalidate (cheap: the file
		// server answers 304 from Last-Modified) or they keep stale CSS/JS after a deploy.
		w.Header().Set("Cache-Control", "no-cache")
		if strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Content-Type", "application/javascript")
		}
		if strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Content-Type", "text/css")
		}
		fs.ServeHTTP(w, r)
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

// defaultSortBy is the common-skill order used until the user picks one.
const defaultSortBy = "tariff-asc"

// sortByFrom returns the requested common-skill sort order, or the default if
// it is missing or unknown.
func sortByFrom(r *http.Request) string {
	sortBy := r.FormValue("sortBy")
	for _, o := range views.SortOptions {
		if o.Value == sortBy {
			return sortBy
		}
	}
	return defaultSortBy
}

// sortedCommonSkills lists the common skills, priced, in the given order; ties
// fall back to name (or tariff for name orders) so the order is stable.
func sortedCommonSkills(sortBy string) []views.CommonSkillOption {
	list := make([]views.CommonSkillOption, 0, len(skills.CommonSkills))
	for key, s := range skills.CommonSkills {
		list = append(list, views.CommonSkillOption{Key: key, Name: s.Name, Tariff: s.SetTariff()})
	}
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		switch sortBy {
		case "tariff-desc":
			if a.Tariff != b.Tariff {
				return a.Tariff > b.Tariff
			}
			return a.Name < b.Name
		case "alpha-asc":
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return a.Tariff > b.Tariff
		case "alpha-desc":
			if a.Name != b.Name {
				return a.Name > b.Name
			}
			return a.Tariff > b.Tariff
		default: // tariff-asc
			if a.Tariff != b.Tariff {
				return a.Tariff < b.Tariff
			}
			return a.Name < b.Name
		}
	})
	return list
}

// defaultSkill is the skill the form starts with: a front somersault in the straight position.
func defaultSkill() skills.TrampolineSkill {
	return skills.TrampolineSkill{Rotation: 4, TakeoffPosition: skills.Feet, Shape: skills.Straight, TwistDistribution: []int{0}}
}

// handleIndex serves the page shell; the form and routine load into it.
func handleIndex(w http.ResponseWriter, r *http.Request) {
	if err := tmpl.ExecuteTemplate(w, "base.html", nil); err != nil {
		log.Printf("Error executing base template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handleSkillForm renders the skill form: a fresh one, or (when the page posts
// the routine skill being edited as JSON with its index) one loaded with it.
func handleSkillForm(w http.ResponseWriter, r *http.Request) {
	form := views.SkillForm{Skill: defaultSkill(), EditIndex: -1, SortBy: sortByFrom(r)}

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
		index, err := strconv.Atoi(r.FormValue("editIndex"))
		if err != nil || index < 0 {
			badRequest(w, fmt.Errorf("invalid edit index %q", r.FormValue("editIndex")))
			return
		}
		form.Skill, form.EditIndex = s, index
	}

	form.CommonSkills = sortedCommonSkills(form.SortBy)
	render(w, r, views.SkillFormView(form))
}

// handleSkillInputs re-renders the skill inputs, either for a common skill the
// user just chose or for the values currently in the form. While the form holds
// something unscorable (e.g. a rotation being typed), it answers 204 so htmx
// leaves the user's input alone; Add and Evaluate report the problem.
func handleSkillInputs(w http.ResponseWriter, r *http.Request) {
	var s skills.TrampolineSkill
	if r.FormValue("load") == "common" {
		common, ok := skills.CommonSkills[r.FormValue("commonSkillKey")]
		if !ok {
			w.WriteHeader(http.StatusNoContent) // the "Select..." placeholder
			return
		}
		s = common
	} else {
		parsed, err := parseSkillFromForm(r)
		if err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s = parsed
	}
	if s.Shape == skills.Straddle && !s.IsBasicJump() {
		s.Shape = skills.Straight // straddle only applies to basic jumps
	}
	render(w, r, views.SkillInputs(s))
}

// handleCommonSkillsOptions re-renders the common skills dropdown in a new order,
// keeping the current selection.
func handleCommonSkillsOptions(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.CommonSkillOptions(sortedCommonSkills(sortByFrom(r)), r.FormValue("commonSkillKey")))
}

// parseSkillFromForm reads and validates the skill in the calculator form.
// Twist boxes for phases the rotation does not have are disabled in the form and
// so not submitted; missing phases count as no twist.
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

// handleSkillEvaluation renders the evaluation preview of the form's skill.
func handleSkillEvaluation(w http.ResponseWriter, r *http.Request) {
	skill, err := calculatedSkill(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	skillJSON, err := json.Marshal(skill)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	render(w, r, views.EvaluationView(views.Evaluation{
		Skill:       skill,
		Landing:     skill.LandingPosition(),
		FIGNotation: skill.FIGNotation(),
		SkillJSON:   string(skillJSON),
	}))
}

// handleRoutineView renders the routine builder (cards, validation, totals) for
// the routine the browser posts. Official names are always re-derived, so names
// stored by older versions are corrected.
func handleRoutineView(w http.ResponseWriter, r *http.Request) {
	routine, err := parseRoutineFromRequest(r)
	if err != nil {
		log.Printf("Error parsing routine for view: %v", err)
		badRequest(w, err)
		return
	}
	for i := range routine {
		routine[i].Name = skills.FindCommonSkillName(routine[i])
	}
	render(w, r, views.Routine(skills.ValidateRoutine(routine)))
}

// --- Helper Functions ---

// parseRoutineFromRequest parses JSON routine data from the routineData form or
// query value, falling back to a raw request body. Each skill is normalised to
// its phase count, validated and priced.
func parseRoutineFromRequest(r *http.Request) ([]skills.TrampolineSkill, error) {
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("parsing form: %w", err)
	}
	rawData := []byte(r.FormValue("routineData")) // includes the query string

	// Fallback to request body
	if len(rawData) == 0 && r.Body != nil && r.ContentLength > 0 && (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, fmt.Errorf("reading request body: %w", err)
		}
		rawData = bodyBytes
	}

	if len(rawData) == 0 {
		return []skills.TrampolineSkill{}, nil // No data found
	}

	var routine []skills.TrampolineSkill
	err := json.Unmarshal(rawData, &routine)
	if err != nil {
		decodedStr, decErr := url.QueryUnescape(string(rawData))
		if decErr == nil {
			err = json.Unmarshal([]byte(decodedStr), &routine)
		}
		if err != nil {
			log.Printf("ERROR: Failed to decode routine JSON: %v", err)
			return nil, fmt.Errorf("failed to decode routine JSON: %w", err)
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
