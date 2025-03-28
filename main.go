// main.go
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
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
	log.Println("Successfully parsed templates:", tmpl.DefinedTemplates())
	//tmpl := template.Must(template.New("").Funcs(funcMap).ParseGlob("templates/*.html"))

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

	http.HandleFunc("/api/calculate", func(w http.ResponseWriter, r *http.Request) {
		var skill skills.TrampolineSkill
		if err := json.NewDecoder(r.Body).Decode(&skill); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		skill.SetTariff()

		response := struct {
			Rotation          int     `json:"rotation"`
			TwistDistribution []int   `json:"twist_distribution"`
			Shape             string  `json:"shape"`
			Backward          bool    `json:"backward"`
			SeatLanding       bool    `json:"seat_landing"`
			Tariff            float64 `json:"tariff"`
			TakeoffPosition   string  `json:"takeoff_position"`
			LandingPosition   string  `json:"landing_position"`
		}{
			Rotation:          skill.Rotation,
			TwistDistribution: skill.TwistDistribution,
			Shape:             skill.Shape.String(),
			Backward:          skill.Backward,
			SeatLanding:       skill.SeatLanding,
			Tariff:            skill.Tariff,
			TakeoffPosition:   skill.TakeoffPosition.String(),
			LandingPosition:   skill.LandingPosition().String(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})
	http.HandleFunc("/validate-routine", func(w http.ResponseWriter, r *http.Request) {
		var trampolineSkills []skills.TrampolineSkill
		if err := json.NewDecoder(r.Body).Decode(&trampolineSkills); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		response := ValidationResponse{Valid: true, TotalTariff: 0}

		for i, skill := range trampolineSkills {
			response.TotalTariff += math.Abs(skill.Tariff)

			if i > 0 {
				prevLanding := trampolineSkills[i-1].LandingPosition()
				if prevLanding != skill.TakeoffPosition {
					response.Valid = false
					response.InvalidIndex = i
					response.Message = fmt.Sprintf(
						"Cannot %s after %s landing",
						skill.TakeoffPosition.String(),
						prevLanding.String(),
					)
					break
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	log.Println("Server starting on :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
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
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(trampolineSkills) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{"valid": false, "message": "Empty routine"})
		return
	}

	validation := struct {
		Valid         bool    `json:"valid"`
		Message       string  `json:"message,omitempty"`
		CurrentTariff float64 `json:"current_tariff"`
		TotalTariff   float64 `json:"total_tariff"`
	}{
		CurrentTariff: trampolineSkills[len(trampolineSkills)-1].Tariff,
	}

	// Validate transitions
	for i := 1; i < len(trampolineSkills); i++ {
		prev := trampolineSkills[i-1].LandingPosition()
		current := trampolineSkills[i].TakeoffPosition

		if prev != current {
			validation.Valid = false
			validation.Message = fmt.Sprintf(
				"Invalid transition from %s to %s at skill %d",
				prev.String(),
				current.String(),
				i+1,
			)
			break
		}
	}

	// Calculate total tariff
	for _, skill := range trampolineSkills {
		validation.TotalTariff += skill.Tariff
	}

	if validation.Message == "" {
		validation.Valid = true
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(validation)
}

type ValidationResponse struct {
	Valid        bool    `json:"valid"`
	Message      string  `json:"message,omitempty"`
	InvalidIndex int     `json:"invalidIndex,omitempty"`
	TotalTariff  float64 `json:"totalTariff"`
}

func validateSkill(skill skills.TrampolineSkill) error {
	requiredTwistRotations := (skill.Rotation + 3) / 4 // Ceiling division
	if len(skill.TwistDistribution) != requiredTwistRotations {
		return fmt.Errorf("expected %d twist rotations for %d/4 rotation",
			requiredTwistRotations, skill.Rotation)
	}

	totalTwist := 0
	for _, t := range skill.TwistDistribution {
		totalTwist += t
	}

	return nil
}
