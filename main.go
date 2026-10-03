// main.go
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io" // Required for body reading
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"tariffCalculator/skills"
	"tariffCalculator/views"
)

// --- Global Variables & Types ---
var tmpl *template.Template

// --- Structs for Validation & Template Data ---

type CommonSkillEntry struct {
	Key    string
	Name   string
	Tariff float64
}

type SkillFormData struct {
	Skill         skills.TrampolineSkill
	CommonSkills  []CommonSkillEntry // Keep this for the main form fragment
	Index         int
	EnabledPhases int
	CurrentTwists []int  // Note: This is for FORM display, might still be 4 elements
	SortBy        string // Add SortBy for initial form load state
	ShapeRelevant bool   // Whether the shape control should be shown for this skill
	IsBasicJump   bool   // Whether the straddle shape option applies (basic jumps only)

	MaxCustomNameLength int
}

// Added struct for the options template
type CommonSkillsOptionsData struct {
	CommonSkills  []CommonSkillEntry
	SelectedValue string // The key of the currently selected skill (if any)
}

// --- Template Setup ---

func convertIntSliceToStringSlice(intSlice []int) []string {
	stringSlice := make([]string, len(intSlice))
	for i, v := range intSlice {
		stringSlice[i] = strconv.Itoa(v)
	}
	return stringSlice
}

func seq(start, end int) []int {
	if start > end {
		return []int{}
	}
	s := make([]int, end-start+1)
	for i := range s {
		s[i] = start + i
	}
	return s
}

var funcMap = template.FuncMap{
	"add":      func(a, b int) int { return a + b },
	"sub":      func(a, b int) int { return a - b },
	"multiply": func(a, b int) int { return a * b },
	"json": func(v interface{}) (template.JS, error) {
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return template.JS(b), nil
	},
	"ternary": func(condition bool, trueVal, falseVal interface{}) interface{} {
		if condition {
			return trueVal
		}
		return falseVal
	},
	"abs": func(x int) int {
		if x < 0 {
			return -x
		}
		return x
	},
	"default": func(def, val interface{}) interface{} {
		sVal := fmt.Sprintf("%v", val)
		if sVal == "" || sVal == "0" || sVal == "<nil>" || sVal == "[]" {
			return def
		}
		return val
	},
	"skillKey": func(s skills.TrampolineSkill) string {
		// Ensure TwistDistribution is not nil before joining
		twists := []int{0} // Default if nil
		if s.TwistDistribution != nil {
			twists = s.TwistDistribution
		}
		return fmt.Sprintf("R%d_T%s_S%s_B%t_SL%t_TP%s", s.Rotation, strings.Join(convertIntSliceToStringSlice(twists), "_"), s.Shape.String(), s.Backward, s.SeatLanding, s.TakeoffPosition.String())
	},
	"join": func(sep string, a []int) string { return strings.Join(convertIntSliceToStringSlice(a), sep) },
	"seq":  seq,
}

func loadTemplates() {
	tmpl = template.Must(template.New("base.html").Funcs(funcMap).ParseGlob("templates/*.html"))
	log.Printf("Loaded templates: %v", tmpl.DefinedTemplates())
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

	mux.HandleFunc("/{$}", handleIndex)
	mux.HandleFunc("/skill-form-fragment", handleSkillFormFragment)
	mux.HandleFunc("/skill-inputs-fragment", handleSkillInputsFragment)
	mux.HandleFunc("/edit-skill-form-data/", handleEditSkillFormData)
	mux.HandleFunc("/calculate-skill", handleCalculateSingleSkill)
	mux.HandleFunc("/evaluate-skill-fragment", handleEvaluateSkillFragment)
	mux.HandleFunc("POST /routine", handleRoutineView)
	mux.HandleFunc("/common-skills-options", handleCommonSkillsOptions)

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

func handleIndex(w http.ResponseWriter, r *http.Request) {
	err := tmpl.ExecuteTemplate(w, "base.html", nil)
	if err != nil {
		log.Printf("Error executing base template: %v", err)
		http.Error(w, "Internal Server Error", 500)
	}
}

// getSortedCommonSkills retrieves and sorts common skills based on parameters.
// sortBy: "tariff-desc" (default), "tariff-asc", "alpha-asc", "alpha-desc"
func getSortedCommonSkills(sortBy string) []CommonSkillEntry {
	skillList := make([]CommonSkillEntry, 0, len(skills.CommonSkills))
	for key, s := range skills.CommonSkills {
		tempSkill := s
		tempSkill.SetTariff()
		skillList = append(skillList, CommonSkillEntry{Key: key, Name: tempSkill.Name, Tariff: tempSkill.Tariff})
	}

	// Sorting logic
	sort.Slice(skillList, func(i, j int) bool {
		switch sortBy {
		case "tariff-asc":
			if skillList[i].Tariff != skillList[j].Tariff {
				return skillList[i].Tariff < skillList[j].Tariff
			}
			return skillList[i].Name < skillList[j].Name // Secondary sort by name
		case "alpha-asc":
			if skillList[i].Name != skillList[j].Name {
				return skillList[i].Name < skillList[j].Name
			}
			return skillList[i].Tariff > skillList[j].Tariff // Secondary sort by tariff desc
		case "alpha-desc":
			if skillList[i].Name != skillList[j].Name {
				return skillList[i].Name > skillList[j].Name
			}
			return skillList[i].Tariff > skillList[j].Tariff // Secondary sort by tariff desc
		case "tariff-desc":
			fallthrough // Default case
		default:
			if skillList[i].Tariff != skillList[j].Tariff {
				return skillList[i].Tariff > skillList[j].Tariff
			}
			return skillList[i].Name < skillList[j].Name // Secondary sort by name
		}
	})
	return skillList
}

// defaultSkill is the skill the form starts with: a front somersault in the straight position.
func defaultSkill() skills.TrampolineSkill {
	return skills.TrampolineSkill{Rotation: 4, TakeoffPosition: skills.Feet, Shape: skills.Straight, TwistDistribution: []int{0}}
}

// prepareSkillFormData calculates derived data needed for form templates.
func prepareSkillFormData(skillData skills.TrampolineSkill, index int, sortBy string) SkillFormData {
	enabledPhases := skills.CalculatePhases(skillData.Rotation)

	currentTwists := make([]int, 4)
	if skillData.TwistDistribution != nil {
		copyCount := len(skillData.TwistDistribution)
		if copyCount > 4 {
			copyCount = 4
		}
		copy(currentTwists, skillData.TwistDistribution[:copyCount])
	}

	isBasicJump := skillData.Rotation == 0 && skillData.TotalTwist() == 0 &&
		skillData.LandingPosition() != skills.Seat && skillData.TakeoffPosition != skills.Seat

	return SkillFormData{
		Skill:         skillData,
		CommonSkills:  nil, // Will be populated later if needed
		Index:         index,
		EnabledPhases: enabledPhases,
		CurrentTwists: currentTwists,
		SortBy:        sortBy, // Store current sort order
		ShapeRelevant: skillData.ShapeIsRelevant(),
		IsBasicJump:   isBasicJump,

		MaxCustomNameLength: skills.MaxCustomNameLength,
	}
}

// handleSkillFormFragment serves the *entire* form fragment.
func handleSkillFormFragment(w http.ResponseWriter, r *http.Request) {
	skillKey := r.URL.Query().Get("commonSkillKey")
	editIndexStr := r.URL.Query().Get("editIndex")
	sortBy := r.URL.Query().Get("sortBy") // Get sort preference
	if sortBy == "" {
		sortBy = "tariff-desc" // Default sort
	}

	editIndex, err := strconv.Atoi(editIndexStr)
	if err != nil || editIndex < 0 {
		editIndex = -1
	}

	var skillData skills.TrampolineSkill
	if skillKey != "" {
		if commonSkill, exists := skills.CommonSkills[skillKey]; exists {
			skillData = commonSkill
			skillData.SetTariff()
		} else {
			skillData = defaultSkill()
			skillData.SetTariff()
		}
	} else if editIndex == -1 {
		skillData = defaultSkill()
		skillData.SetTariff()
	} else {
		// When loading for edit, we need the actual skill data, not a default
		routine, parseErr := parseRoutineFromRequest(r)
		if parseErr == nil && editIndex < len(routine) {
			skillData = routine[editIndex]
			// Recalculate tariff just in case
			skillData.SetTariff()
		} else {
			log.Printf("Error parsing routine or index out of bounds for edit in handleSkillFormFragment: %v", parseErr)
			// Fallback to default if parsing fails or index is bad
			skillData = defaultSkill()
			skillData.SetTariff()
		}
	}

	formData := prepareSkillFormData(skillData, editIndex, sortBy)
	formData.CommonSkills = getSortedCommonSkills(sortBy) // Get sorted skills

	if tmpl.Lookup("skill-form-fragment.html") == nil {
		log.Println("Error: skill-form-fragment.html template not loaded")
		http.Error(w, "Internal Server Error", 500)
		return
	}
	err = tmpl.ExecuteTemplate(w, "skill-form-fragment.html", formData)
	if err != nil {
		log.Printf("Error executing skill-form-fragment template: %v", err)
	}
}

// handleSkillInputsFragment serves ONLY the inputs part of the form.
func handleSkillInputsFragment(w http.ResponseWriter, r *http.Request) {
	skillKey := r.URL.Query().Get("commonSkillKey")
	editIndexStr := r.URL.Query().Get("editIndex")
	sortBy := r.URL.Query().Get("sortBy") // Get sort preference (though not directly used here)
	if sortBy == "" {
		sortBy = "tariff-desc" // Default sort
	}

	editIndex, err := strconv.Atoi(editIndexStr)
	if err != nil || editIndex < 0 {
		editIndex = -1
	}

	var skillData skills.TrampolineSkill
	if skillKey != "" {
		if commonSkill, exists := skills.CommonSkills[skillKey]; exists {
			skillData = commonSkill
		} else {
			skillData = defaultSkill()
		}
	} else {
		// If no common skill, load default or existing skill for edit
		if editIndex != -1 {
			routine, parseErr := parseRoutineFromRequest(r)
			if parseErr == nil && editIndex < len(routine) {
				skillData = routine[editIndex]
			} else {
				log.Printf("Error parsing routine or index out of bounds for edit in handleSkillInputsFragment: %v", parseErr)
				skillData = defaultSkill()
			}
		} else {
			skillData = defaultSkill()
		}
	}

	// We still need to prepare the full form data to pass to the fragment template
	formData := prepareSkillFormData(skillData, editIndex, sortBy)

	if tmpl.Lookup("skill-inputs-fragment.html") == nil {
		log.Println("Error: skill-inputs-fragment.html template not loaded")
		http.Error(w, "Internal Server Error", 500)
		return
	}
	err = tmpl.ExecuteTemplate(w, "skill-inputs-fragment.html", formData)
	if err != nil {
		log.Printf("Error executing skill-inputs-fragment template: %v", err)
	}
}

// handleEditSkillFormData loads data for editing and renders the *entire* form fragment.
func handleEditSkillFormData(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/edit-skill-form-data/"), "/")
	if len(parts) < 1 {
		http.Error(w, "Not Found", 404)
		return
	}
	indexStr := parts[0]
	index, err := strconv.Atoi(indexStr)
	if err != nil || index < 0 {
		http.Error(w, "Bad Request: Invalid index", 400)
		return
	}

	// Get sort preference if provided (e.g., from hidden input or previous state)
	sortBy := r.URL.Query().Get("sortBy")
	if sortBy == "" {
		sortBy = "tariff-desc" // Default
	}

	routine, err := parseRoutineFromRequest(r)
	if err != nil {
		log.Printf("Error parsing routine for edit: %v", err)
		http.Error(w, "Bad Request: Could not parse routine data", 400)
		return
	}

	if index >= len(routine) {
		log.Printf("Error: Edit index %d out of bounds for routine length %d", index, len(routine))
		http.Error(w, "Bad Request: Index out of bounds", 400)
		return
	}

	skillToEdit := routine[index]
	formData := prepareSkillFormData(skillToEdit, index, sortBy)
	formData.CommonSkills = getSortedCommonSkills(sortBy) // Get sorted skills

	if tmpl.Lookup("skill-form-fragment.html") == nil {
		log.Println("Error: skill-form-fragment.html template not loaded")
		http.Error(w, "Internal Server Error", 500)
		return
	}
	err = tmpl.ExecuteTemplate(w, "skill-form-fragment.html", formData)
	if err != nil {
		log.Printf("Error executing edit form fragment template: %v", err)
	}
}

// --- Utility Functions (parseSkillFromForm, ShapeFromString, etc.) ---

// parseSkillFromForm parses skill data from a submitted form.
func parseSkillFromForm(r *http.Request) (skills.TrampolineSkill, error) {
	skill := skills.TrampolineSkill{}
	skill.CustomName = strings.TrimSpace(r.FormValue("custom_name"))
	rotationVal := r.FormValue("rotation")
	rotation, err := strconv.Atoi(rotationVal)
	if err != nil {
		return skill, fmt.Errorf("invalid rotation %q", rotationVal)
	}
	skill.Rotation = rotation
	skill.TakeoffPosition = skills.BodyPositionFromString(r.FormValue("takeoff_position"))
	skill.Shape = skills.ShapeFromString(r.FormValue("shape")) // Use function from skills package
	skill.Backward = r.FormValue("backward") == "on"
	skill.SeatLanding = r.FormValue("seat_landing") == "on"

	numPhases := skills.CalculatePhases(skill.Rotation)

	twistValues := r.Form["twist_distribution[]"]
	skill.TwistDistribution = make([]int, 0, numPhases)
	for i := 0; i < numPhases; i++ {
		twist := 0
		if i < len(twistValues) {
			parsedTwist, err := strconv.Atoi(twistValues[i])
			if err == nil {
				twist = parsedTwist
			} else {
				log.Printf("Warning: Invalid twist value '%s' at index %d, using 0.", twistValues[i], i)
			}
		} else {
			log.Printf("Warning: Missing twist value for phase %d, using 0.", i+1)
		}
		skill.TwistDistribution = append(skill.TwistDistribution, twist)
	}

	return skill, skill.Validate()
}

// handleCalculateSingleSkill parses JSON, calculates, finds name, returns JSON.
func handleCalculateSingleSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	var requestPayload struct {
		Name              string `json:"name"` // ignored: the official name is derived; accepted for older clients
		CustomName        string `json:"custom_name"`
		Rotation          int    `json:"rotation"`
		TwistDistribution []int  `json:"twist_distribution"`
		TakeoffPosition   string `json:"takeoff_position"`
		Shape             string `json:"shape"`
		Backward          bool   `json:"backward"`
		SeatLanding       bool   `json:"seat_landing"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&requestPayload)
	if err != nil {
		log.Printf("Error decoding JSON payload for calculation: %v", err)
		http.Error(w, "Bad Request: "+err.Error(), 400)
		return
	}

	skill := skills.TrampolineSkill{
		CustomName:        strings.TrimSpace(requestPayload.CustomName),
		Rotation:          requestPayload.Rotation,
		TwistDistribution: requestPayload.TwistDistribution,
		TakeoffPosition:   skills.BodyPositionFromString(requestPayload.TakeoffPosition),
		Shape:             skills.ShapeFromString(requestPayload.Shape), // Use function from skills package
		Backward:          requestPayload.Backward,
		SeatLanding:       requestPayload.SeatLanding,
	}

	skill.NormalizePhases()
	if err := skill.Validate(); err != nil {
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}

	skill.Name = skills.FindCommonSkillName(skill)

	skill.SetTariff()
	landingPos := skill.LandingPosition()

	// Prepare response
	response := struct {
		Name              string  `json:"name"`
		CustomName        string  `json:"custom_name,omitempty"`
		Rotation          int     `json:"rotation"`
		TwistDistribution []int   `json:"twist_distribution"`
		TakeoffPosition   string  `json:"takeoff_position"`
		Shape             string  `json:"shape"`
		Backward          bool    `json:"backward"`
		SeatLanding       bool    `json:"seat_landing"`
		Tariff            float64 `json:"tariff"`
		LandingPosition   string  `json:"landing_position"`
	}{
		Name:              skill.Name, // Use the final name (either found common name or "Custom Skill")
		CustomName:        skill.CustomName,
		Rotation:          skill.Rotation,
		TwistDistribution: skill.TwistDistribution, // Use the adjusted slice
		TakeoffPosition:   skill.TakeoffPosition.String(),
		Shape:             skill.Shape.String(),
		Backward:          skill.Backward,
		SeatLanding:       skill.SeatLanding,
		Tariff:            skill.Tariff,
		LandingPosition:   landingPos.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	encodeErr := json.NewEncoder(w).Encode(response)
	if encodeErr != nil {
		log.Printf("Error encoding JSON response for calculation: %v", encodeErr)
	}
}

// handleEvaluateSkillFragment parses Form Data, calculates, renders HTML fragment.
func handleEvaluateSkillFragment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", 405)
		return
	}

	err := r.ParseForm()
	if err != nil {
		log.Printf("Error parsing form for eval: %v", err)
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}

	skill, err := parseSkillFromForm(r)
	if err != nil {
		log.Printf("Error processing form data for eval: %v", err)
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}

	skill.Name = skills.FindCommonSkillName(skill)

	skill.SetTariff()
	landingPos := skill.LandingPosition()
	figNotation := skill.FIGNotation()

	skillJson, jsonErr := json.Marshal(skill)
	if jsonErr != nil {
		log.Printf("Error marshalling skill to JSON for eval fragment: %v", jsonErr)
		skillJson = []byte("{}")
	}

	data := map[string]interface{}{
		"Skill":          skill,
		"LandingPosStr":  landingPos.String(),
		"LandingIsValid": landingPos != skills.Invalid,
		"SkillDataJSON":  string(skillJson),
		"FIGNotation":    figNotation}

	if tmpl.Lookup("evaluation-fragment.html") == nil {
		log.Println("Error: evaluation-fragment.html template not loaded")
		http.Error(w, "Internal Server Error", 500)
		return
	}
	err = tmpl.ExecuteTemplate(w, "evaluation-fragment.html", data)
	if err != nil {
		log.Printf("Error executing eval fragment: %v", err)
	}
}

// handleRoutineView renders the routine builder (cards, validation, totals) for
// the routine the browser posts. Official names are always re-derived, so names
// stored by older versions are corrected.
func handleRoutineView(w http.ResponseWriter, r *http.Request) {
	routine, err := parseRoutineFromRequest(r)
	if err != nil {
		log.Printf("Error parsing routine for view: %v", err)
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}
	for i := range routine {
		routine[i].Name = skills.FindCommonSkillName(routine[i])
	}

	if err := views.Routine(skills.ValidateRoutine(routine)).Render(r.Context(), w); err != nil {
		log.Printf("Error rendering routine view: %v", err)
	}
}

// handleCommonSkillsOptions serves *only* the <option> tags for the dropdown.
func handleCommonSkillsOptions(w http.ResponseWriter, r *http.Request) {
	sortBy := r.URL.Query().Get("sortBy")
	selectedValue := r.URL.Query().Get("selectedValue") // Get the current value if needed
	if sortBy == "" {
		sortBy = "tariff-desc" // Default sort
	}

	sortedSkills := getSortedCommonSkills(sortBy)

	data := CommonSkillsOptionsData{
		CommonSkills:  sortedSkills,
		SelectedValue: selectedValue,
	}

	// Execute the specific template for options
	if tmpl.Lookup("common-skills-options.html") == nil {
		log.Println("Error: common-skills-options.html template not loaded")
		http.Error(w, "Internal Server Error", 500)
		return
	}
	err := tmpl.ExecuteTemplate(w, "common-skills-options.html", data)
	if err != nil {
		log.Printf("Error executing common-skills-options template: %v", err)
	}
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
