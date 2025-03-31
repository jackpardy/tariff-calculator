// main.go
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	CommonSkills  []CommonSkillEntry
	Index         int
	EnabledPhases int
	CurrentTwists []int
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
		return fmt.Sprintf("R%d_T%s_S%s_B%t_SL%t_TP%s", s.Rotation, strings.Join(convertIntSliceToStringSlice(s.TwistDistribution), "_"), s.Shape.String(), s.Backward, s.SeatLanding, s.TakeoffPosition.String())
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

	fragmentFiles := []string{
		"templates/skill-form-fragment.html",
		// "templates/twist-fields-fragment.html", // Removed
		"templates/evaluation-fragment.html",
	}
	allFiles := append(tmplFiles, fragmentFiles...)
	existingFiles := []string{}

	for _, f := range allFiles {
		if _, err := os.Stat(f); err == nil {
			existingFiles = append(existingFiles, f)
		} else if !os.IsNotExist(err) {
			log.Printf("Error checking template file %s: %v", f, err)
		} else {
			if strings.Contains(f, "-fragment.html") && f != "templates/twist-fields-fragment.html" {
				log.Printf("Warning: Template fragment '%s' not found, skipping.", f)
			} else if !strings.Contains(f, "-fragment.html") && !strings.Contains(f, "results.html") {
				log.Printf("Warning: Core template file '%s' not found.", f) // Log if core files missing
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
	http.HandleFunc("/edit-skill-form-data/", handleEditSkillFormData)
	// No /twist-fields route needed
	http.HandleFunc("/calculate-skill", handleCalculateSingleSkill)
	http.HandleFunc("/evaluate-skill-fragment", handleEvaluateSkillFragment)
	http.HandleFunc("/validate-routine-client-state", handleValidateRoutineClientState)

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

func getSortedCommonSkills() []CommonSkillEntry {
	skillList := make([]CommonSkillEntry, 0, len(skills.CommonSkills))
	for key, s := range skills.CommonSkills {
		tempSkill := s
		tempSkill.SetTariff()
		skillList = append(skillList, CommonSkillEntry{Key: key, Name: tempSkill.Name, Tariff: tempSkill.Tariff})
	}
	sort.Slice(skillList, func(i, j int) bool {
		if skillList[i].Tariff != skillList[j].Tariff {
			return skillList[i].Tariff > skillList[j].Tariff
		}
		return skillList[i].Name < skillList[j].Name
	})
	return skillList
}

// handleSkillFormFragment serves the form, rendering twist fields directly.
func handleSkillFormFragment(w http.ResponseWriter, r *http.Request) {
	skillKey := r.URL.Query().Get("commonSkillKey")
	editIndexStr := r.URL.Query().Get("editIndex")
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
		}
	} else if editIndex == -1 {
		skillData = skills.TrampolineSkill{Rotation: 4, TakeoffPosition: skills.Feet, Shape: skills.Straight, TwistDistribution: []int{0}}
	} else {
		skillData = skills.TrampolineSkill{Rotation: 4, TakeoffPosition: skills.Feet, Shape: skills.Straight, TwistDistribution: []int{0}}
	} // Load empty but keep index

	absRotation := int(math.Abs(float64(skillData.Rotation)))
	enabledPhases := 1
	if absRotation <= 6 {
		enabledPhases = 1
	} else if absRotation <= 10 {
		enabledPhases = 2
	} else if absRotation <= 14 {
		enabledPhases = 3
	} else if absRotation > 0 {
		enabledPhases = 4
	}
	currentTwists := make([]int, 4)
	copy(currentTwists, skillData.TwistDistribution)

	commonSkillsList := getSortedCommonSkills()
	formData := SkillFormData{Skill: skillData, CommonSkills: commonSkillsList, Index: editIndex, EnabledPhases: enabledPhases, CurrentTwists: currentTwists}

	if tmpl.Lookup("skill-form-fragment.html") == nil {
		log.Println("Error: skill-form-fragment.html template not loaded")
		http.Error(w, "ISE", 500)
		return
	}
	err = tmpl.ExecuteTemplate(w, "skill-form-fragment.html", formData)
	if err != nil {
		log.Printf("Error executing skill-form-fragment template: %v", err)
	}
}

// handleEditSkillFormData loads data for editing and renders the form fragment.
func handleEditSkillFormData(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/edit-skill-form-data/"), "/")
	if len(parts) < 1 {
		http.Error(w, "Not Found", 404)
		return
	}
	indexStr := parts[0]
	index, err := strconv.Atoi(indexStr)
	if err != nil || index < 0 {
		http.Error(w, "Bad Request", 400)
		return
	}
	routine, err := parseRoutineFromRequest(r)
	if err != nil {
		log.Printf("Error parsing routine for edit: %v", err)
		http.Error(w, "Bad Request", 400)
		return
	}
	if index >= len(routine) {
		http.Error(w, "Bad Request: Index OOB", 400)
		return
	}

	skillToEdit := routine[index]
	absRotation := int(math.Abs(float64(skillToEdit.Rotation)))
	enabledPhases := 1
	if absRotation <= 6 {
		enabledPhases = 1
	} else if absRotation <= 10 {
		enabledPhases = 2
	} else if absRotation <= 14 {
		enabledPhases = 3
	} else if absRotation > 0 {
		enabledPhases = 4
	}
	currentTwists := make([]int, 4)
	copy(currentTwists, skillToEdit.TwistDistribution)

	commonSkillsList := getSortedCommonSkills()
	formData := SkillFormData{Skill: skillToEdit, CommonSkills: commonSkillsList, Index: index, EnabledPhases: enabledPhases, CurrentTwists: currentTwists}

	if tmpl.Lookup("skill-form-fragment.html") == nil {
		log.Println("Error: skill-form-fragment.html not loaded")
		http.Error(w, "ISE", 500)
		return
	}
	err = tmpl.ExecuteTemplate(w, "skill-form-fragment.html", formData)
	if err != nil {
		log.Printf("Error executing edit form fragment template: %v", err)
	}
}
func parseSkillFromForm(r *http.Request) (skills.TrampolineSkill, error) {
	// Note: Assumes r.ParseForm() has already been called successfully
	skill := skills.TrampolineSkill{}
	skill.Name = r.FormValue("name") // Read name if submitted

	rotationVal := r.FormValue("rotation")
	rotation, _ := strconv.Atoi(rotationVal) // Default 0 if invalid

	skill.TakeoffPosition = skills.BodyPositionFromString(r.FormValue("takeoff_position")) // Use correct form name
	skill.Shape = ShapeFromString(r.FormValue("shape"))                                    // Use helper

	skill.Backward = r.FormValue("backward") == "on" // Standard form value for checked
	skill.SeatLanding = r.FormValue("seat_landing") == "on"

	absRotation := int(math.Abs(float64(rotation)))
	numPhases := 1
	if absRotation <= 6 {
		numPhases = 1
	} else if absRotation <= 10 {
		numPhases = 2
	} else if absRotation <= 14 {
		numPhases = 3
	} else if absRotation > 0 {
		numPhases = 4
	}

	// Get twists using the correct form key 'twist_distribution[]'
	twistValues := r.Form["twist_distribution[]"]
	skill.TwistDistribution = make([]int, 0, numPhases)
	for i := 0; i < numPhases; i++ {
		twist := 0
		if i < len(twistValues) {
			parsedTwist, err := strconv.Atoi(twistValues[i])
			if err != nil {
				log.Printf("Warning: Invalid twist value '%s', using 0.", twistValues[i])
			} else {
				twist = parsedTwist
			}
		}
		skill.TwistDistribution = append(skill.TwistDistribution, twist)
	}

	skill.Rotation = absRotation // Store absolute

	// Find common name only if name wasn't provided or was placeholder
	if skill.Name == "" || skill.Name == "Custom Skill" {
		foundName := findCommonSkillName(skill)
		if foundName != "" {
			skill.Name = foundName
		} else {
			skill.Name = "Custom Skill"
		}
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
		log.Printf("Error decoding JSON payload: %v", err)
		http.Error(w, "Bad Request", 400)
		return
	}

	skill := skills.TrampolineSkill{Name: requestPayload.Name, Rotation: requestPayload.Rotation, TwistDistribution: requestPayload.TwistDistribution, TakeoffPosition: skills.BodyPositionFromString(requestPayload.TakeoffPosition), Shape: ShapeFromString(requestPayload.Shape), Backward: requestPayload.Backward, SeatLanding: requestPayload.SeatLanding}
	foundName := findCommonSkillName(skill) // Check if params match any common skill
	if foundName != "" {
		skill.Name = foundName // Use the found common name
	} else {
		skill.Name = "Custom Skill" // Default to Custom if no match
	}

	skill.SetTariff()
	landingPos := skill.LandingPosition()
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
		Name: skill.Name, Rotation: skill.Rotation, TwistDistribution: skill.TwistDistribution, TakeoffPosition: skill.TakeoffPosition.String(), Shape: skill.Shape.String(), Backward: skill.Backward, SeatLanding: skill.SeatLanding, Tariff: skill.Tariff, LandingPosition: landingPos.String(),
	}

	// w.Header().Set("HX-Trigger", "reloadForm") // Removed for now to prevent EOF errors with fetch
	w.Header().Set("Content-Type", "application/json")
	encodeErr := json.NewEncoder(w).Encode(response)
	if encodeErr != nil {
		log.Printf("Error encoding JSON response: %v", encodeErr)
		return
	}
}

// handleEvaluateSkillFragment parses JSON, calculates, renders HTML fragment.
func handleEvaluateSkillFragment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", 405)
		return
	}

	// --- Parse Form Data ---
	err := r.ParseForm()
	if err != nil {
		log.Printf("Error parsing form for eval: %v", err)
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}

	skill, err := parseSkillFromForm(r) // Use the form parsing helper
	if err != nil {
		// parseSkillFromForm doesn't return error currently, but good practice
		log.Printf("Error processing form data for eval: %v", err)
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}
	// --- End Parse Form ---

	skill.SetTariff()
	landingPos := skill.LandingPosition()
	skillJson, _ := json.Marshal(skill)

	data := map[string]interface{}{
		"Skill":          skill,
		"LandingPosStr":  landingPos.String(),
		"LandingIsValid": landingPos != skills.Invalid,
		"SkillDataJSON":  string(skillJson),
	}

	w.Header().Set("X-Skill-Data", string(skillJson)) // Keep for Alpine if needed

	if tmpl.Lookup("evaluation-fragment.html") == nil {
		log.Println("Error: evaluation-fragment.html template not loaded")
		http.Error(w, "ISE", 500)
		return
	}
	err = tmpl.ExecuteTemplate(w, "evaluation-fragment.html", data)
	if err != nil {
		log.Printf("Error executing eval fragment: %v", err)
	} else {
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
		http.Error(w, "Bad Request", 400)
		return
	}
	validationData := performRoutineValidation(routine)
	w.Header().Set("Content-Type", "application/json")
	encodeErr := json.NewEncoder(w).Encode(validationData)
	if encodeErr != nil {
		log.Printf("Error encoding validation JSON: %v", encodeErr)
		return
	}
}

// --- Helper Functions ---

// ShapeFromString converts string to skills.Shape
func ShapeFromString(s string) skills.Shape {
	for shapeEnum, name := range skills.ShapeName {
		if strings.EqualFold(s, name) {
			return shapeEnum
		}
	}
	log.Printf("Warning: Invalid shape string '%s' received, using default Straight.", s)
	return skills.Straight
}

// findCommonSkillName attempts to match parsed skill to a common skill name.
func findCommonSkillName(parsedSkill skills.TrampolineSkill) string {
	for _, commonSkill := range skills.CommonSkills {
		tempCommon := commonSkill
		match := parsedSkill.Equal(&tempCommon)
		if match {
			return tempCommon.Name
		}
	}
	return ""
}

// parseRoutineFromRequest parses JSON routine data from form/query.
func parseRoutineFromRequest(r *http.Request) ([]skills.TrampolineSkill, error) {
	var routine []skills.TrampolineSkill
	if err := r.ParseForm(); err != nil && r.ContentLength > 0 {
		log.Printf("Warning: Error parsing form in parseRoutine: %v", err)
	}
	routineDataStr := r.FormValue("routineData")
	if routineDataStr == "" {
		routineDataStr = r.URL.Query().Get("routineData")
	}
	if routineDataStr == "" {
		return []skills.TrampolineSkill{}, nil
	}
	err := json.Unmarshal([]byte(routineDataStr), &routine)
	if err != nil {
		decodedStr, decErr := url.QueryUnescape(routineDataStr)
		if decErr == nil {
			err = json.Unmarshal([]byte(decodedStr), &routine)
		}
		if err != nil {
			log.Printf("ERROR: Failed to decode routine JSON: %v", err)
			return nil, fmt.Errorf("failed to decode routine JSON: %w", err)
		}
	}
	for i := range routine {
		routine[i].SetTariff()
		routine[i].LandingPosStr = routine[i].LandingPosition().String()
	} // Recalc server-side
	return routine, nil
}

// performRoutineValidation performs validation and returns structured data.
func performRoutineValidation(routine []skills.TrampolineSkill) RoutineValidationData {
	data := RoutineValidationData{Skills: make([]ValidatedSkill, len(routine)), Messages: make([]string, len(routine)), RoutineTooLong: len(routine) > 10}
	seenIndices := make(map[int]bool)
	validSkillCount := 0
	for i := range routine {
		data.Skills[i].TrampolineSkill = routine[i]
		data.Skills[i].SetTariff()
		landing := data.Skills[i].LandingPosition()
		data.Skills[i].LandingPosStr = landing.String()
		data.RawTariff += data.Skills[i].Tariff
		var messages []string
		isDuplicate := false
		for j := 0; j < i; j++ {
			if data.Skills[i].Equal(&data.Skills[j].TrampolineSkill) {
				isDuplicate = true
				if !seenIndices[j] {
					data.Skills[j].IsDuplicate = true
					seenIndices[j] = true
				}
				break
			}
		}
		if isDuplicate {
			data.Skills[i].IsDuplicate = true
			data.HasDuplicates = true
			messages = append(messages, "Duplicate")
		}
		if !isDuplicate && validSkillCount < 10 {
			data.TotalTariff += data.Skills[i].Tariff
			validSkillCount++
		}
		if isDuplicate {
			seenIndices[i] = true
		}
		if i > 0 {
			prevLanding := data.Skills[i-1].LandingPosition()
			currentTakeoff := data.Skills[i].TakeoffPosition
			if prevLanding != skills.Invalid && prevLanding != currentTakeoff {
				data.Skills[i].InvalidTransition = true
				data.HasInvalidTransitions = true
				if i < 10 || len(routine) <= 10 {
					messages = append(messages, fmt.Sprintf("Bad Transition: %s -> %s", prevLanding.String(), currentTakeoff.String()))
				}
			}
		}
		if landing == skills.Invalid {
			data.Skills[i].InvalidLanding = true
			data.HasInvalidLandings = true
			if i < 10 || len(routine) <= 10 {
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
			messages = append(messages, "Skill >10")
		}
		data.Messages[i] = strings.Join(messages, " / ")
	}
	return data
}
