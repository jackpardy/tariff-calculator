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
	req := httptest.NewRequest(http.MethodPost, "/validate-routine-client-state", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handleValidateRoutineClientState(rec, req)
	return rec
}

// TestValidateRoutineJSONContract pins the per-skill and routine-level fields the
// client reads; the per-skill flags drive the coloured card borders.
func TestValidateRoutineJSONContract(t *testing.T) {
	rec := postRoutine(t, `[
		{"rotation":1,"twist_distribution":[0],"takeoff_position":"Back","shape":"Straight"},
		{"rotation":0,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Straight"},
		{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},
		{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true}
	]`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got struct {
		Skills []struct {
			InvalidTransition bool
			InvalidLanding    bool
			IsDuplicate       bool
			IntermediateJump  bool
		} `json:"skills"`
		HasIntermediateJumps bool `json:"hasIntermediateJumps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	raw := rec.Body.String()
	for _, field := range []string{`"InvalidTransition"`, `"InvalidLanding"`, `"IsDuplicate"`, `"IntermediateJump"`} {
		if !strings.Contains(raw, field) {
			t.Errorf("response is missing per-skill field %s", field)
		}
	}
	if !got.Skills[0].InvalidTransition {
		t.Errorf("skill 1 (starts from back) should be an invalid transition")
	}
	if !got.Skills[1].IntermediateJump || !got.HasIntermediateJumps {
		t.Errorf("skill 2 (straight jump) should be an intermediate jump")
	}
	if !got.Skills[2].IsDuplicate || !got.Skills[3].IsDuplicate {
		t.Errorf("skills 3 and 4 should be flagged duplicate")
	}
}

func TestValidateRoutineRejectsInvalidSkill(t *testing.T) {
	rec := postRoutine(t, `[{"rotation":-4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}]`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a negative rotation", rec.Code)
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
	req := httptest.NewRequest(http.MethodPost, "/validate-routine-client-state", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("oversize body status = %d, want 400", rec.Code)
	}
}
