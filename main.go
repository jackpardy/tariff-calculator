// main.go
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"tariffCalculator/skills"
)

func main() {
	funcMap := template.FuncMap{
		"multiply": multiply,
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("Template parsing error: %v", err)
	}

	// Serve static files
	http.Handle("/static/", http.StripPrefix("/static/", staticFileServer("static")))

	// Routes
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl.ExecuteTemplate(w, "base.html", nil)
	})

	http.HandleFunc("/calculate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		rotation, _ := strconv.Atoi(r.FormValue("rotation"))
		if rotation < 0 || rotation > 16 {
			http.Error(w, "Rotation must be between 0 and 16 quarter-rotations", http.StatusBadRequest)
			return
		}
		takeoffPositionStr := r.FormValue("takeoff")
		takeoffPosition := skills.BodyPositionFromString(takeoffPositionStr)
		shape, _ := strconv.Atoi(r.FormValue("shape"))
		backward := r.FormValue("backward") == "on"
		seatLanding := r.FormValue("seat_landing") == "on"
		twistDistribution := make([]int, 0)
		for _, t := range r.Form["twist_distribution[]"] {
			twist, _ := strconv.Atoi(t)
			twistDistribution = append(twistDistribution, twist)
		}
		if rotation < 0 {
			rotation = -rotation
			backward = !backward
		}

		skill := skills.TrampolineSkill{
			Rotation:          rotation,
			TwistDistribution: twistDistribution,
			TakeoffPosition:   takeoffPosition,
			Shape:             skills.Shape(shape),
			Backward:          backward,
			SeatLanding:       seatLanding,
		}

		skill.SetTariff()
		tmpl.ExecuteTemplate(w, "results.html", skill)
	})

	http.HandleFunc("/api/calculate", calculateHandler)
	http.HandleFunc("/validate-routine", validateRoutineHandler)
	http.HandleFunc("/api/common-skills", func(w http.ResponseWriter, r *http.Request) {
		// Create a copy of the map with calculated tariffs
		skillsWithTariff := make(map[string]skills.TrampolineSkill)
		for key, skill := range skills.CommonSkills {
			s := skill    // Copy to avoid modifying original
			s.SetTariff() // Calculate tariff
			skillsWithTariff[key] = s
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(skillsWithTariff)
	})

	http.HandleFunc("/api/common-skill/", func(w http.ResponseWriter, r *http.Request) {
		skillName := strings.TrimPrefix(r.URL.Path, "/api/common-skill/")
		skill, exists := skills.GetCommonSkill(skillName)
		if !exists {
			http.Error(w, "Skill not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(skill)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
func staticFileServer(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		// Set MIME types for specific files
		if strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Content-Type", "application/javascript")
		}
		if strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Content-Type", "text/css")
		}
		fs.ServeHTTP(w, r)
	})
}

func multiply(a, b int) int {
	return a * b
}

func validateRoutineHandler(w http.ResponseWriter, r *http.Request) {
	var trampolineSkills []skills.TrampolineSkill
	if err := json.NewDecoder(r.Body).Decode(&trampolineSkills); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := ValidationResponse{
		Valid:              true,
		InvalidTransitions: []int{},
		Duplicates:         []int{},
		InvalidLandings:    []int{}, // New field
		TenthSkillWarning:  false,
		TotalTariff:        0,
		RawTariff:          0,
		Messages:           make([]string, len(trampolineSkills)),
	}
	response.TotalTariff = 0
	response.RawTariff = 0
	seenIndices := make(map[int]bool) // Track first occurrences
	for i := range trampolineSkills {
		response.RawTariff += trampolineSkills[i].Tariff
		var messages []string
		// Check transitions
		if i > 0 {
			prevLanding := trampolineSkills[i-1].LandingPosition()
			currentTakeoff := trampolineSkills[i].TakeoffPosition
			if prevLanding != currentTakeoff {
				response.InvalidTransitions = append(response.InvalidTransitions, i)
				messages = append(messages, fmt.Sprintf(
					"Invalid transition from %s landing to %s takeoff",
					prevLanding.String(),
					currentTakeoff.String(),
				))
			}
		}
		// Check duplicates

		// Check transitions (existing code)
		// Check duplicates by comparing with all previous skills
		isDuplicate := false
		for j := 0; j < i; j++ {
			if trampolineSkills[i].Equal(&trampolineSkills[j]) {
				isDuplicate = true
				// Mark first duplicate if not already marked
				if !seenIndices[j] {
					response.Duplicates = append(response.Duplicates, j)
					seenIndices[j] = true
				}
				break
			}
		}
		if isDuplicate {
			response.Duplicates = append(response.Duplicates, i)
			messages = append(messages, "Duplicate skill detected")
		}
		if !isDuplicate {
			response.TotalTariff += trampolineSkills[i].Tariff
		}
		seenIndices[i] = isDuplicate

		landing := trampolineSkills[i].LandingPosition()
		if landing == skills.Invalid {
			response.InvalidLandings = append(response.InvalidLandings, i)
			messages = append(messages, "Invalid landing position")
		}
		response.Messages[i] = strings.Join(messages, " + ")

		// Track validation states
		if len(messages) > 0 {
			response.Valid = false
		}

	}
	if len(trampolineSkills) == 10 {
		lastSkill := trampolineSkills[9]
		if lastSkill.LandingPosition() != skills.Feet {
			response.TenthSkillWarning = true
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type ValidationResponse struct {
	Valid              bool     `json:"valid"`
	Messages           []string `json:"messages"`
	InvalidTransitions []int    `json:"invalidTransitions"`
	InvalidLandings    []int    `json:"invalidLandings"`   // Added
	TenthSkillWarning  bool     `json:"tenthSkillWarning"` // Added
	Duplicates         []int    `json:"duplicates"`
	TotalTariff        float64  `json:"totalTariff"` // Unique skills total
	RawTariff          float64  `json:"rawTariff"`
}

func calculateHandler(w http.ResponseWriter, r *http.Request) {
	var skill skills.TrampolineSkill
	if err := json.NewDecoder(r.Body).Decode(&skill); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	skill.SetTariff()

	response := struct {
		Name              string  `json:"name"`
		Rotation          int     `json:"rotation"`
		TwistDistribution []int   `json:"twist_distribution"`
		Shape             string  `json:"shape"`
		Backward          bool    `json:"backward"`
		SeatLanding       bool    `json:"seat_landing"`
		Tariff            float64 `json:"tariff"`
		TakeoffPosition   string  `json:"takeoff_position"`
		LandingPosition   string  `json:"landing_position"`
	}{
		Name:              skill.Name,
		Rotation:          skill.Rotation,
		TwistDistribution: skill.TwistDistribution,
		Shape:             skill.Shape.String(),
		Backward:          skill.Backward,
		SeatLanding:       skill.SeatLanding,
		Tariff:            skill.Tariff,
		TakeoffPosition:   skill.TakeoffPosition.String(),
		LandingPosition:   skill.LandingPosition().String(),
	}
	for _, commonSkill := range skills.CommonSkills {
		if skill.Equal(&commonSkill) {
			response.Name = commonSkill.Name // Use common name if match found
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
