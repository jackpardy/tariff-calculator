package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func postRoutine(t *testing.T, routineJSON string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"routineData": {routineJSON}}
	req := httptest.NewRequest(http.MethodPost, "/routine", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	routes().ServeHTTP(rec, req)
	return rec
}

// routineCards splits a rendered routine view into one chunk of HTML per card.
func routineCards(html string) []string {
	parts := strings.Split(html, `class="routine-skill-container"`)
	return parts[1:]
}

// TestRoutineViewFlagsSkills checks that validation results reach the rendered
// cards (the coloured borders and messages) and the routine-level warnings.
func TestRoutineViewFlagsSkills(t *testing.T) {
	rec := postRoutine(t, `[
		{"rotation":1,"twist_distribution":[0],"takeoff_position":"Back","shape":"Straight"},
		{"rotation":0,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Straight"},
		{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},
		{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},
		{"rotation":3,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Straight"}
	]`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	html := rec.Body.String()
	cards := routineCards(html)
	if len(cards) != 5 {
		t.Fatalf("rendered %d cards, want 5", len(cards))
	}

	want := []struct{ class, message string }{
		{"invalid-transition", "Must Start From Feet"},
		{"invalid-transition", "Straight Jump Interrupts Routine"},
		{"duplicate-skill", "Duplicate (Counts Once)"},
		{"duplicate-skill", "Duplicate"},
		{"", ""},
	}
	for i, w := range want {
		if w.class != "" && !strings.Contains(cards[i], w.class) {
			t.Errorf("card %d is missing class %q", i+1, w.class)
		}
		if w.message != "" && !strings.Contains(cards[i], w.message) {
			t.Errorf("card %d is missing message %q", i+1, w.message)
		}
	}
	for _, flag := range []string{"invalid-transition", "duplicate-skill", "invalid-landing"} {
		if strings.Contains(cards[4], flag) {
			t.Errorf("card 5 (crash dive) should not be flagged %q", flag)
		}
	}

	for _, s := range []string{
		"Total Tariff: 0.90",
		"5 of 10 skills",
		"Duplicate skills only count once",
		"Invalid transitions detected",
		"Straight jumps interrupt the routine",
		"2. Straight Jump",
		"3. Back Tuck",
		"(4 - o)",
		"Feet → Back", // crash dive landing
	} {
		if !strings.Contains(html, s) {
			t.Errorf("view is missing %q", s)
		}
	}
}

func TestRoutineViewEmptyAndInvalid(t *testing.T) {
	rec := postRoutine(t, `[]`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Add skills using the form above.") {
		t.Errorf("empty routine: status %d, body %s", rec.Code, rec.Body)
	}

	rec = postRoutine(t, `[{"rotation":-4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}]`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a negative rotation", rec.Code)
	}

	rec = httptest.NewRecorder()
	routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/routine", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /routine status = %d, want 405", rec.Code)
	}
}

func TestCalculateSkillValidatesInput(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"valid back tuck", `{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true}`, http.StatusOK},
		{"negative rotation", `{"rotation":-4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}`, http.StatusBadRequest},
		{"rotation beyond a quad", `{"rotation":40,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}`, http.StatusBadRequest},
		{"negative twist", `{"rotation":4,"twist_distribution":[-3],"takeoff_position":"Feet","shape":"Straight"}`, http.StatusBadRequest},
		{"unknown shape", `{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Banana"}`, http.StatusBadRequest},
		{"unknown take-off", `{"rotation":4,"twist_distribution":[0],"takeoff_position":"Head","shape":"Tuck"}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/calculate-skill", strings.NewReader(c.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handleCalculateSingleSkill(rec, req)
			if rec.Code != c.want {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, c.want, rec.Body)
			}
		})
	}
}

func TestEvaluateSkillRejectsInvalidForm(t *testing.T) {
	form := url.Values{
		"rotation":             {"-4"},
		"takeoff_position":     {"feet"},
		"shape":                {"tuck"},
		"twist_distribution[]": {"0"},
	}
	req := httptest.NewRequest(http.MethodPost, "/evaluate-skill-fragment", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handleEvaluateSkillFragment(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a negative rotation", rec.Code)
	}
}

func TestRoutesRejectUnknownPathsAndOversizeBodies(t *testing.T) {
	h := routes()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/no-such-page", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /no-such-page status = %d, want 404", rec.Code)
	}

	form := url.Values{"routineData": {strings.Repeat(" ", maxRequestBytes)}}
	req := httptest.NewRequest(http.MethodPost, "/routine", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("oversize body status = %d, want 400", rec.Code)
	}
}

func TestCustomNames(t *testing.T) {
	t.Run("calculate keeps the custom name and derives the official one", func(t *testing.T) {
		body := `{"name":"Typed Over","custom_name":"  Opener  ","rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true}`
		req := httptest.NewRequest(http.MethodPost, "/calculate-skill", strings.NewReader(body))
		rec := httptest.NewRecorder()
		handleCalculateSingleSkill(rec, req)
		var got struct {
			Name       string `json:"name"`
			CustomName string `json:"custom_name"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("status %d, decoding %q: %v", rec.Code, rec.Body, err)
		}
		if got.Name != "Back Tuck" || got.CustomName != "Opener" {
			t.Errorf("got name %q, custom name %q; want Back Tuck, Opener", got.Name, got.CustomName)
		}
	})

	t.Run("calculate rejects an over-long custom name", func(t *testing.T) {
		body := `{"custom_name":"` + strings.Repeat("x", 61) + `","rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}`
		rec := httptest.NewRecorder()
		handleCalculateSingleSkill(rec, httptest.NewRequest(http.MethodPost, "/calculate-skill", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("routine view shows custom and refreshed official names, escaped", func(t *testing.T) {
		rec := postRoutine(t, `[
			{"name":"Back","custom_name":"Opener","rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},
			{"custom_name":"<img src=x onerror=alert(1)>","rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Pike","backward":true}
		]`)
		html := rec.Body.String()
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, html)
		}
		if !strings.Contains(html, "1. Opener") || !strings.Contains(html, "· Back Tuck") {
			t.Errorf("want the custom name with the refreshed official name, got:\n%s", html)
		}
		if strings.Contains(html, "<img") {
			t.Errorf("custom name was rendered unescaped")
		}
	})

	t.Run("evaluation preview escapes the custom name", func(t *testing.T) {
		loadTemplates()
		form := url.Values{
			"custom_name":          {`"><img src=x onerror=alert(1)>`},
			"rotation":             {"4"},
			"takeoff_position":     {"feet"},
			"shape":                {"tuck"},
			"twist_distribution[]": {"0"},
		}
		req := httptest.NewRequest(http.MethodPost, "/evaluate-skill-fragment", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		handleEvaluateSkillFragment(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		html := rec.Body.String()
		if strings.Contains(html, "<img") {
			t.Errorf("custom name was rendered unescaped:\n%s", html)
		}
		if !strings.Contains(html, "Front Tuck") {
			t.Errorf("preview should show the official name")
		}
	})
}
