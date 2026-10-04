package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
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
	for i := 2; i < 5; i++ {
		if !strings.Contains(cards[i], "not-counted") || !strings.Contains(cards[i], "After Interruption (No Tariff)") {
			t.Errorf("card %d should be shown as not counted", i+1)
		}
	}
	if !strings.Contains(cards[0], `aria-label="Issue: Must Start From Feet"`) {
		t.Errorf("a card with a problem should show the ⚠️ marker in its header")
	}
	if strings.Contains(cards[0], "not-counted") {
		t.Errorf("card 1 comes before the interruption and should count")
	}

	// Reordering is SortableJS on #routine-skills; the old per-card drag markup is gone.
	for _, old := range []string{"insertion-point", "draggable", "dragstart"} {
		if strings.Contains(html, old) {
			t.Errorf("routine view still contains %q", old)
		}
	}
	for _, flag := range []string{"invalid-transition", "duplicate-skill", "invalid-landing"} {
		if strings.Contains(cards[4], flag) {
			t.Errorf("card 5 (crash dive) should not be flagged %q", flag)
		}
	}

	for _, s := range []string{
		"Total Tariff: 0.10", // the straight jump (skill 2) interrupts; only skill 1 counts
		"(Raw Total: 1.40)",
		"5 of 10 skills",
		"Duplicate skills only count once",
		"Invalid transitions detected",
		"Routine interrupted at skill 2",
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

// postForm posts form values through the full handler stack.
func postForm(t *testing.T, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	routes().ServeHTTP(rec, req)
	return rec
}

// skillFormValues is the calculator form for a skill, as the browser submits it
// (only enabled twist boxes are sent).
func skillFormValues(rotation, takeoff, shape string, twists ...string) url.Values {
	return url.Values{
		"rotation":             {rotation},
		"takeoff_position":     {takeoff},
		"shape":                {shape},
		"twist_distribution[]": twists,
	}
}

// tagWithID returns the opening tag of the element with the given id.
func tagWithID(t *testing.T, html, id string) string {
	t.Helper()
	m := regexp.MustCompile(`<[a-z]+[^>]*\sid="` + regexp.QuoteMeta(id) + `"[^>]*>`).FindString(html)
	if m == "" {
		t.Fatalf("no element with id %q in:\n%s", id, html)
	}
	return m
}

func TestCalculateSkillValidatesInput(t *testing.T) {
	cases := []struct {
		name string
		form url.Values
		want int
	}{
		{"valid back tuck", withValue(skillFormValues("4", "feet", "tuck", "0"), "backward", "on"), http.StatusOK},
		{"negative rotation", skillFormValues("-4", "feet", "tuck", "0"), http.StatusBadRequest},
		{"rotation beyond a quad", skillFormValues("40", "feet", "tuck", "0"), http.StatusBadRequest},
		{"missing rotation", skillFormValues("", "feet", "tuck", "0"), http.StatusBadRequest},
		{"negative twist", skillFormValues("4", "feet", "straight", "-3"), http.StatusBadRequest},
		{"non-numeric twist", skillFormValues("4", "feet", "straight", "x"), http.StatusBadRequest},
		{"unknown shape", skillFormValues("4", "feet", "banana", "0"), http.StatusBadRequest},
		{"unknown take-off", skillFormValues("4", "head", "tuck", "0"), http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if rec := postForm(t, "/calculate-skill", c.form); rec.Code != c.want {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, c.want, rec.Body)
			}
		})
	}
}

func withValue(v url.Values, key, value string) url.Values {
	v.Set(key, value)
	return v
}

func TestCalculateSkillReturnsTheStoredSkill(t *testing.T) {
	form := withValue(skillFormValues("8", "feet", "tuck", "0", "1"), "custom_name", "  Opener  ")
	rec := postForm(t, "/calculate-skill", form)
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("status %d, decoding %q: %v", rec.Code, rec.Body, err)
	}
	want := map[string]any{
		"name": "Half-Out Tuck", "custom_name": "Opener", "rotation": 8.0,
		"takeoff_position": "Feet", "shape": "Tuck", "tariff": 1.1,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if twists, _ := got["twist_distribution"].([]any); len(twists) != 2 || twists[1] != 1.0 {
		t.Errorf("twist_distribution = %v, want [0 1]", got["twist_distribution"])
	}
}

func TestSkillEvaluationRejectsInvalidForm(t *testing.T) {
	if rec := postForm(t, "/skill-evaluation", skillFormValues("-4", "feet", "tuck", "0")); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a negative rotation", rec.Code)
	}
}

func TestSkillForm(t *testing.T) {
	t.Run("a new form adds the default skill", func(t *testing.T) {
		rec := postForm(t, "/skill-form", url.Values{})
		html := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(html, "Add to Routine") || strings.Contains(html, "Cancel Edit") {
			t.Fatalf("status %d, body:\n%s", rec.Code, html)
		}
		if tag := tagWithID(t, html, "rotation"); !strings.Contains(tag, `value="4"`) {
			t.Errorf("rotation = %s, want the default front (4)", tag)
		}
		if !strings.Contains(html, `<option value="tariff-asc" selected>`) {
			t.Errorf("the default sort order should be selected")
		}
	})

	t.Run("an edit form is loaded with the skill being edited", func(t *testing.T) {
		rec := postForm(t, "/skill-form", url.Values{
			"skill":     {`{"name":"Barani Tuck","custom_name":"Opener","rotation":4,"twist_distribution":[1],"takeoff_position":"Feet","shape":"Tuck"}`},
			"editIndex": {"2"},
			"sortBy":    {"alpha-desc"},
		})
		html := rec.Body.String()
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, html)
		}
		for _, want := range []string{"Update Skill", "Cancel Edit", `name="editIndex" value="2"`, `<option value="alpha-desc" selected>`} {
			if !strings.Contains(html, want) {
				t.Errorf("edit form is missing %q", want)
			}
		}
		if tag := tagWithID(t, html, "custom-name"); !strings.Contains(tag, `value="Opener"`) {
			t.Errorf("custom name = %s", tag)
		}
		if tag := tagWithID(t, html, "twist-1"); !strings.Contains(tag, `value="1"`) {
			t.Errorf("twist-1 = %s, want 1", tag)
		}
	})

	t.Run("bad edit requests are rejected", func(t *testing.T) {
		skill := `{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}`
		for name, form := range map[string]url.Values{
			"missing index": {"skill": {skill}},
			"invalid skill": {"skill": {`{"rotation":-1}`}, "editIndex": {"0"}},
			"not JSON":      {"skill": {"nope"}, "editIndex": {"0"}},
		} {
			if rec := postForm(t, "/skill-form", form); rec.Code != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400", name, rec.Code)
			}
		}
	})
}

// TestSkillInputsFollowTheSkill covers the form behaviour the server now owns:
// twist boxes per phase, when shape is shown, and when straddle is offered.
func TestSkillInputsFollowTheSkill(t *testing.T) {
	inputs := func(t *testing.T, form url.Values) string {
		t.Helper()
		rec := postForm(t, "/skill-inputs", form)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		return rec.Body.String()
	}
	disabled := func(tag string) bool { return strings.Contains(tag, " disabled") }

	t.Run("a double enables two twist boxes", func(t *testing.T) {
		html := inputs(t, skillFormValues("8", "feet", "tuck", "0"))
		if disabled(tagWithID(t, html, "twist-2")) || !disabled(tagWithID(t, html, "twist-3")) {
			t.Errorf("want twist-1..2 enabled and twist-3..4 disabled")
		}
	})

	t.Run("shape is hidden for a full back", func(t *testing.T) {
		html := inputs(t, withValue(skillFormValues("4", "feet", "straight", "2"), "backward", "on"))
		if !strings.Contains(html, `class="field is-hidden"`) {
			t.Errorf("shape field should be hidden when shape does not matter")
		}
		html = inputs(t, skillFormValues("4", "feet", "tuck", "0"))
		if strings.Contains(html, "is-hidden") {
			t.Errorf("shape field should show for a front")
		}
	})

	t.Run("straddle is only offered for basic jumps", func(t *testing.T) {
		if html := inputs(t, skillFormValues("0", "feet", "straddle", "0")); !strings.Contains(html, `<option value="straddle" selected>`) {
			t.Errorf("a straddle jump should keep straddle selected")
		}
		html := inputs(t, skillFormValues("4", "feet", "straddle", "0"))
		if strings.Contains(html, `value="straddle"`) {
			t.Errorf("straddle should not be offered for a somersault")
		}
		if !strings.Contains(html, `<option value="straight" selected>`) {
			t.Errorf("a straddle somersault should fall back to straight")
		}
	})

	t.Run("choosing a common skill loads it", func(t *testing.T) {
		form := withValue(skillFormValues("0", "feet", "straight", "0"), "load", "common")
		form.Set("commonSkillKey", "halfOut")
		html := inputs(t, form)
		if !strings.Contains(tagWithID(t, html, "rotation"), `value="8"`) || !strings.Contains(tagWithID(t, html, "twist-2"), `value="1"`) {
			t.Errorf("want the half-out (8, [0 1]) loaded")
		}
	})

	t.Run("nothing to render leaves the form alone", func(t *testing.T) {
		for name, form := range map[string]url.Values{
			"placeholder chosen":   withValue(skillFormValues("4", "feet", "tuck", "0"), "load", "common"),
			"rotation being typed": skillFormValues("", "feet", "tuck", "0"),
			"rotation too high":    skillFormValues("17", "feet", "tuck", "0"),
		} {
			if rec := postForm(t, "/skill-inputs", form); rec.Code != http.StatusNoContent {
				t.Errorf("%s: status = %d, want 204", name, rec.Code)
			}
		}
	})
}

func TestCommonSkillsOptions(t *testing.T) {
	rec := httptest.NewRecorder()
	routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/common-skills-options?sortBy=alpha-asc&commonSkillKey=rudi", nil))
	html := rec.Body.String()
	first := regexp.MustCompile(`<option value="([a-zA-Z]+)"`).FindStringSubmatch(html)
	if first == nil || first[1] != "backSomersault" {
		t.Errorf("first option = %v, want backSomersault (\"Back\") for A-Z", first)
	}
	if !strings.Contains(html, `<option value="rudi" selected>`) {
		t.Errorf("the current selection should be kept")
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
	t.Run("calculate rejects an over-long custom name", func(t *testing.T) {
		form := withValue(skillFormValues("4", "feet", "tuck", "0"), "custom_name", strings.Repeat("x", 61))
		if rec := postForm(t, "/calculate-skill", form); rec.Code != http.StatusBadRequest {
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

	t.Run("evaluation preview shows and escapes the custom name", func(t *testing.T) {
		form := withValue(skillFormValues("4", "feet", "tuck", "0"), "custom_name", `"><img src=x onerror=alert(1)>`)
		rec := postForm(t, "/skill-evaluation", form)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		html := rec.Body.String()
		if strings.Contains(html, "<img") {
			t.Errorf("custom name was rendered unescaped:\n%s", html)
		}
		for _, want := range []string{"Front Tuck", "(4 - o)", "0.50", "addFromForm('evaluation-insert-position')"} {
			if !strings.Contains(html, want) {
				t.Errorf("preview is missing %q", want)
			}
		}
	})
}

func TestIndexRendersThePage(t *testing.T) {
	rec := httptest.NewRecorder()
	routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	html := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, html)
	}
	for _, want := range []string{`<!doctype html>`, `id="skill-form-wrapper"`, `id="routine-view"`, `x-data="tariffCalculatorStore()"`} {
		if !strings.Contains(strings.ToLower(html), strings.ToLower(want)) {
			t.Errorf("page is missing %q", want)
		}
	}
	// Every asset is linked by a versioned URL that the server actually serves.
	links := regexp.MustCompile(`(?:href|src)="(/static/[^"]+)"`).FindAllStringSubmatch(html, -1)
	if len(links) != 6 {
		t.Errorf("found %d asset links, want 6 (2 CSS, 4 JS)", len(links))
	}
	for _, l := range links {
		u := strings.ReplaceAll(l[1], "&amp;", "&")
		if !strings.Contains(u, "?v=") {
			t.Errorf("asset %s is not versioned", u)
		}
		rec := httptest.NewRecorder()
		routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, u, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
			t.Errorf("GET %s: status %d, Cache-Control %q", u, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
}
