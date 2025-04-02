// main.go
package main

import (
	"bytes" // Required for body reading/replacement
	"encoding/json"
	"fmt"
	"html/template"
	"io" // Required for body reading
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"tariffCalculator/skills" // Ensure this path is correct
)

// --- Global Variables & Types ---
var tmpl *template.Template

// --- Structs for Validation & Template Data ---

type ValidatedSkill struct {
	skills.TrampolineSkill
	InvalidTransition bool   `json:"-"`
	InvalidLanding    bool   `json:"-"`
	IsDuplicate       bool   `json:"-"`
	LandingPosStr     string `json:"landing_position"`
	SkillDataJSON     string `json:"-"`
	FIGNotation       string `json:"FIGNotation"`
}

type RoutineValidationData struct {
	Skills                []ValidatedSkill `json:"skills"`
	TotalTariff           float64          `json:"totalTariff"`
	RawTariff             float64          `json:"rawTariff"`
	HasDuplicates         bool             `json:"hasDuplicates"`
	HasInvalidTransitions bool             `json:"hasInvalidTransitions"`
	HasInvalidLandings    bool             `json:"hasInvalidLandings"`
	TenthSkillWarning     bool             `json:"tenthSkillWarning"`
	RoutineTooLong        bool             `json:"routineTooLong"`
	Messages              []string         `json:"messages"`
}

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
	"safeHTMLAttr": func(s string) template.HTMLAttr { return template.HTMLAttr(s) },
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
	tmplFiles, err := filepath.Glob("templates/*.html")
	if err != nil {
		log.Fatalf("Error finding templates: %v", err)
	}
	if len(tmplFiles) == 0 {
		log.Fatal("No template files found in templates/ directory")
	}

	// Add the new options template
	fragmentFiles := []string{
		"templates/skill-form-fragment.html",
		"templates/skill-inputs-fragment.html",
		"templates/evaluation-fragment.html",
		"templates/common-skills-options.html", // <-- Add new template
	}
	allFiles := append(tmplFiles, fragmentFiles...)
	existingFiles := []string{}

	for _, f := range allFiles {
		if _, err := os.Stat(f); err == nil {
			existingFiles = append(existingFiles, f)
		} else if !os.IsNotExist(err) {
			log.Printf("Error checking template file %s: %v", f, err)
		} else {
			// Adjust logging based on which files are expected fragments
			isFragment := strings.Contains(f, "-fragment.html") || strings.Contains(f, "common-skills-options.html")
			if isFragment {
				log.Printf("Warning: Template fragment '%s' not found, skipping.", f)
			} else if !strings.Contains(f, "results.html") { // Don't warn about results.html if missing
				log.Printf("Warning: Core template file '%s' not found.", f)
			}
		}
	}
	if len(existingFiles) == 0 {
		log.Fatal("No existing template files could be loaded.")
	}

	tmpl = template.Must(template.New("base.html").Funcs(funcMap).ParseFiles(existingFiles...))
	log.Printf("Loaded templates: ; defined templates are: %v", tmpl.DefinedTemplates())
}

// --- Main Function ---
func main() {
	loadTemplates()
	http.Handle("/static/", http.StripPrefix("/static/", staticFileServer("static")))

	// --- Routes ---
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/skill-form-fragment", handleSkillFormFragment)
	http.HandleFunc("/skill-inputs-fragment", handleSkillInputsFragment)
	http.HandleFunc("/edit-skill-form-data/", handleEditSkillFormData)
	http.HandleFunc("/calculate-skill", handleCalculateSingleSkill)
	http.HandleFunc("/evaluate-skill-fragment", handleEvaluateSkillFragment)
	http.HandleFunc("/validate-routine-client-state", handleValidateRoutineClientState)
	http.HandleFunc("/common-skills-options", handleCommonSkillsOptions) // <-- Add new route

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Starting server on :%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// --- Static File Server ---
func staticFileServer(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=604800")
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

	return SkillFormData{
		Skill:         skillData,
		CommonSkills:  nil, // Will be populated later if needed
		Index:         index,
		EnabledPhases: enabledPhases,
		CurrentTwists: currentTwists,
		SortBy:        sortBy, // Store current sort order
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
			skillData = skills.TrampolineSkill{Rotation: 4, TakeoffPosition: skills.Feet, Shape: skills.Straight, TwistDistribution: []int{0}}
			skillData.SetTariff()
		}
	} else if editIndex == -1 {
		skillData = skills.TrampolineSkill{Rotation: 4, TakeoffPosition: skills.Feet, Shape: skills.Straight, TwistDistribution: []int{0}}
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
			skillData = skills.TrampolineSkill{Rotation: 4, TakeoffPosition: skills.Feet, Shape: skills.Straight, TwistDistribution: []int{0}}
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
			skillData = skills.TrampolineSkill{Rotation: 4, TakeoffPosition: skills.Feet, Shape: skills.Straight, TwistDistribution: []int{0}}
		}
	} else {
		// If no common skill, load default or existing skill for edit
		if editIndex != -1 {
			routine, parseErr := parseRoutineFromRequest(r)
			if parseErr == nil && editIndex < len(routine) {
				skillData = routine[editIndex]
			} else {
				log.Printf("Error parsing routine or index out of bounds for edit in handleSkillInputsFragment: %v", parseErr)
				skillData = skills.TrampolineSkill{Rotation: 4, TakeoffPosition: skills.Feet, Shape: skills.Straight, TwistDistribution: []int{0}}
			}
		} else {
			skillData = skills.TrampolineSkill{Rotation: 4, TakeoffPosition: skills.Feet, Shape: skills.Straight, TwistDistribution: []int{0}}
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
	skill.Name = r.FormValue("name")
	rotationVal := r.FormValue("rotation")
	rotation, _ := strconv.Atoi(rotationVal)
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

	return skill, nil
}

// handleCalculateSingleSkill parses JSON, calculates, finds name, returns JSON.
func handleCalculateSingleSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	var requestPayload struct {
		Name              string `json:"name"`
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
		Name:              requestPayload.Name, // Start with name from request
		Rotation:          requestPayload.Rotation,
		TwistDistribution: requestPayload.TwistDistribution,
		TakeoffPosition:   skills.BodyPositionFromString(requestPayload.TakeoffPosition),
		Shape:             skills.ShapeFromString(requestPayload.Shape), // Use function from skills package
		Backward:          requestPayload.Backward,
		SeatLanding:       requestPayload.SeatLanding,
	}

	// Adjust twist distribution slice length based on rotation
	expectedPhases := skills.CalculatePhases(skill.Rotation)
	if len(skill.TwistDistribution) > expectedPhases {
		skill.TwistDistribution = skill.TwistDistribution[:expectedPhases]
	} else {
		for len(skill.TwistDistribution) < expectedPhases {
			skill.TwistDistribution = append(skill.TwistDistribution, 0)
		}
	}

	skill.Name = findCommonSkillName(skill)

	skill.SetTariff()
	landingPos := skill.LandingPosition()

	// Prepare response
	response := struct {
		Name              string  `json:"name"`
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

	skill.Name = findCommonSkillName(skill)

	skill.SetTariff()
	landingPos := skill.LandingPosition()
	figNotation := skill.FIGNotation()

	// Ensure skill data for fragment has correct twist length before marshalling
	expectedPhases := skills.CalculatePhases(skill.Rotation)
	if len(skill.TwistDistribution) > expectedPhases {
		skill.TwistDistribution = skill.TwistDistribution[:expectedPhases]
	}

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

// handleValidateRoutineClientState receives routine JSON and returns validation JSON.
func handleValidateRoutineClientState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	routine, err := parseRoutineFromRequest(r)
	if err != nil {
		log.Printf("Error parsing routine for validation: %v", err)
		http.Error(w, "Bad Request: "+err.Error(), 400)
		return
	}

	// Ensure twist lengths and names are correct in the routine before validation
	for i := range routine {
		expectedPhases := skills.CalculatePhases(routine[i].Rotation)
		if len(routine[i].TwistDistribution) > expectedPhases {
			routine[i].TwistDistribution = routine[i].TwistDistribution[:expectedPhases]
		} else {
			for len(routine[i].TwistDistribution) < expectedPhases {
				routine[i].TwistDistribution = append(routine[i].TwistDistribution, 0)
			}
		}
		// Also ensure Name is correct based on parameters (in case loaded from storage)
		foundName := findCommonSkillName(routine[i])
		if foundName != "" {
			routine[i].Name = foundName
		} else {
			// If loaded from storage/request and doesn't match, ensure it's Custom Skill
			routine[i].Name = "Custom Skill"
		}

	}

	validationData := performRoutineValidation(routine)
	w.Header().Set("Content-Type", "application/json")
	encodeErr := json.NewEncoder(w).Encode(validationData)
	if encodeErr != nil {
		log.Printf("Error encoding validation JSON: %v", encodeErr)
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

func findCommonSkillName(parsedSkill skills.TrampolineSkill) string {
	compareSkill := parsedSkill // Use the input skill directly for checks

	// Ensure twist distribution slice length is correct based on rotation for comparison
	expectedPhases := skills.CalculatePhases(compareSkill.Rotation)
	if len(compareSkill.TwistDistribution) > expectedPhases {
		compareSkill.TwistDistribution = compareSkill.TwistDistribution[:expectedPhases]
	} else {
		for len(compareSkill.TwistDistribution) < expectedPhases {
			compareSkill.TwistDistribution = append(compareSkill.TwistDistribution, 0)
		}
	}

	// Iterate through the refactored CommonSkills map
	for _, commonSkill := range skills.CommonSkills {
		tempCommon := commonSkill // Work with a copy

		// Ensure common skill twist distribution is also correct length for comparison
		commonExpectedPhases := skills.CalculatePhases(tempCommon.Rotation)
		if len(tempCommon.TwistDistribution) > commonExpectedPhases {
			tempCommon.TwistDistribution = tempCommon.TwistDistribution[:commonExpectedPhases]
		} else {
			for len(tempCommon.TwistDistribution) < commonExpectedPhases {
				tempCommon.TwistDistribution = append(tempCommon.TwistDistribution, 0)
			}
		}

		// --- Core Parameter Check (Ignoring Shape initially) ---
		// Check Rotation, Takeoff, Backward, SeatLanding, and Twist Distribution
		// Note: Using slices.Equal for twist distribution comparison.
		if compareSkill.Rotation == tempCommon.Rotation &&
			compareSkill.TakeoffPosition == tempCommon.TakeoffPosition &&
			compareSkill.Backward == tempCommon.Backward &&
			compareSkill.SeatLanding == tempCommon.SeatLanding &&
			slices.Equal(compareSkill.TwistDistribution, tempCommon.TwistDistribution) {

			// Found a match based on core parameters! Now check shape.
			baseName := tempCommon.Name
			inputShape := compareSkill.Shape
			defaultShape := tempCommon.Shape // Shape stored in the CommonSkills map entry

			// Determine if shape matters for uniqueness based on FIG rules
			shapeMatters := false
			rotation := compareSkill.Rotation
			totalTwist := compareSkill.TotalTwist() // Use the method from skills.go

			if rotation == 0 && totalTwist == 0 && compareSkill.LandingPosition() != skills.Seat && compareSkill.TakeoffPosition != skills.Seat { // Basic Jumps
				// Shape always matters for non-straight basic jumps
				if baseName == "Shape Jump" && (inputShape == skills.Tuck || inputShape == skills.Pike || inputShape == skills.Straddle) {
					return fmt.Sprintf("%s Jump", inputShape.String())
				} else {
					return "Straight Jump"
				}
				// For straight jump, shape doesn't result in appending name
			} else if rotation >= 6 { // Doubles+
				shapeMatters = true
			} else if rotation >= 3 && totalTwist < 2 { // Singles/Crash/Lazy with < Full twist
				shapeMatters = true
			}
			// Note: For Rotation < 3 (Front/Back drops) or Rotation 3-5 with >= Full twist, shapeMatters remains false.

			// Append shape name ONLY if it matters AND it's different from the default
			if shapeMatters && (defaultShape != skills.Straight || defaultShape != inputShape) {
				// Append the actual shape name
				return fmt.Sprintf("%s %s", baseName, inputShape.String())
			} else {
				// Return the base name (shape didn't matter, or it matched the default)
				return baseName
			}
		}
	}

	// No common skill match found
	return "Custom Skill"
}

// parseRoutineFromRequest parses JSON routine data from form/query/body.
func parseRoutineFromRequest(r *http.Request) ([]skills.TrampolineSkill, error) {
	var routine []skills.TrampolineSkill
	var rawData []byte
	var err error

	// Try form value first
	if errForm := r.ParseForm(); errForm == nil {
		routineDataStr := r.FormValue("routineData")
		if routineDataStr != "" {
			rawData = []byte(routineDataStr)
		}
	} else if r.ContentLength > 0 {
		log.Printf("Warning: Error parsing form in parseRoutineFromRequest: %v", errForm)
	}

	// Fallback to query parameter
	if len(rawData) == 0 {
		routineDataStr := r.URL.Query().Get("routineData")
		if routineDataStr != "" {
			rawData = []byte(routineDataStr)
		}
	}

	// Fallback to request body
	if len(rawData) == 0 && r.Body != nil && r.ContentLength > 0 && (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch) {
		bodyBytes, readErr := io.ReadAll(r.Body)
		if readErr == nil {
			rawData = bodyBytes
		} else if readErr != io.EOF {
			log.Printf("Error reading request body: %v", readErr)
		}
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Replace body
		if r.Form != nil {                                // Reset ContentLength if ParseForm was called
			r.ContentLength = int64(len(bodyBytes))
		}
	}

	if len(rawData) == 0 {
		return []skills.TrampolineSkill{}, nil // No data found
	}

	// Attempt to unmarshal
	err = json.Unmarshal(rawData, &routine)
	if err != nil {
		decodedStr, decErr := url.QueryUnescape(string(rawData))
		if decErr == nil {
			err = json.Unmarshal([]byte(decodedStr), &routine)
		}
		if err != nil {
			log.Printf("ERROR: Failed to decode routine JSON: %v. Raw data: %s", err, string(rawData))
			return nil, fmt.Errorf("failed to decode routine JSON: %w", err)
		}
	}

	// Post-processing: Set tariff, landing string, and correct twist length
	for i := range routine {
		routine[i].SetTariff()
		routine[i].LandingPosStr = routine[i].LandingPosition().String()
		expectedPhases := skills.CalculatePhases(routine[i].Rotation)
		if len(routine[i].TwistDistribution) > expectedPhases {
			routine[i].TwistDistribution = routine[i].TwistDistribution[:expectedPhases]
		} else {
			for len(routine[i].TwistDistribution) < expectedPhases {
				routine[i].TwistDistribution = append(routine[i].TwistDistribution, 0)
			}
		}
		// Don't update name here, let validation handle it if needed
	}
	return routine, nil
}

// performRoutineValidation performs validation and returns structured data.
func performRoutineValidation(routine []skills.TrampolineSkill) RoutineValidationData {
	data := RoutineValidationData{
		Skills:                make([]ValidatedSkill, len(routine)),
		Messages:              make([]string, len(routine)),
		HasDuplicates:         false,
		HasInvalidTransitions: false,
		HasInvalidLandings:    false,
		TenthSkillWarning:     false,
		RoutineTooLong:        len(routine) > 10,
		TotalTariff:           0.0,
		RawTariff:             0.0,
	}

	duplicateMap := make(map[int]bool)
	validSkillCount := 0

	for i := range routine {
		data.Skills[i].TrampolineSkill = routine[i] // Already has correct twist length and name from caller
		landing := data.Skills[i].LandingPosition()
		data.Skills[i].LandingPosStr = landing.String()

		data.RawTariff += data.Skills[i].Tariff
		data.Skills[i].FIGNotation = data.Skills[i].TrampolineSkill.FIGNotation() // Calculate and store

		var messages []string
		isCurrentSkillDuplicate := false
		for j := 0; j < i; j++ {
			// Use the Equal method which compares based on rules
			if data.Skills[i].Equal(&data.Skills[j].TrampolineSkill) {
				isCurrentSkillDuplicate = true
				data.HasDuplicates = true

				if _, marked := duplicateMap[j]; !marked {
					data.Skills[j].IsDuplicate = true
					duplicateMap[j] = true
					if data.Messages[j] == "" {
						data.Messages[j] = "Duplicate (Counts Once)"
					} else {
						data.Messages[j] += " / Duplicate (Counts Once)"
					}
				}
				data.Skills[i].IsDuplicate = true
				messages = append(messages, "Duplicate")
				break
			}
		}

		if !isCurrentSkillDuplicate && validSkillCount < 10 {
			data.TotalTariff += data.Skills[i].Tariff
			validSkillCount++
		}

		if i > 0 {
			prevLanding := data.Skills[i-1].LandingPosition()
			currentTakeoff := data.Skills[i].TakeoffPosition
			if prevLanding != skills.Invalid && prevLanding != currentTakeoff {
				data.Skills[i].InvalidTransition = true
				data.HasInvalidTransitions = true
				if i < 10 || !data.RoutineTooLong {
					messages = append(messages, fmt.Sprintf("Bad Transition: %s -> %s", prevLanding.String(), currentTakeoff.String()))
				}
			}
		}

		if landing == skills.Invalid {
			data.Skills[i].InvalidLanding = true
			data.HasInvalidLandings = true
			if i < 10 || !data.RoutineTooLong {
				messages = append(messages, "Invalid Landing")
			}
		}

		if i == 9 {
			if landing != skills.Feet {
				data.TenthSkillWarning = true
				messages = append(messages, "10th Must Land Feet")
			}
		}

		if i >= 10 {
			messages = append(messages, "Skill >10 (No Tariff)")
		}

		data.Messages[i] = strings.Join(messages, " / ")
	}

	return data
}
