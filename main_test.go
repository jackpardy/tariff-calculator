package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/skills"
	"tariffCalculator/static"
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

func TestSkillForm(t *testing.T) {
	t.Run("a new panel shows the default skill, ready to add", func(t *testing.T) {
		rec := postForm(t, "/skill-form", url.Values{})
		html := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(html, "Add to routine") || strings.Contains(html, "Update skill") {
			t.Fatalf("status %d, body:\n%s", rec.Code, html)
		}
		if tag := tagWithID(t, html, "rotation"); !strings.Contains(tag, `value="4"`) {
			t.Errorf("rotation = %s, want the default front (4)", tag)
		}
		// The card summarises the skill: name, notation, landing and tariff.
		for _, want := range []string{`<p class="skill-card-name">Front Straight</p>`, "(4 - /)", "Feet → Feet", ">0.6</p>"} {
			if !strings.Contains(html, want) {
				t.Errorf("card is missing %q", want)
			}
		}
		// The picker offers the common skills by category, named in their usual shape.
		tabs := regexp.MustCompile(`role="tab"[^>]*>([^<]+)</button>`).FindAllStringSubmatch(html, -1)
		var labels []string
		for _, m := range tabs {
			labels = append(labels, m[1])
		}
		if got := strings.Join(labels, ", "); got != "Jumps, Drops &amp; seat, Somersaults, Twists, Doubles, Triples" {
			t.Errorf("picker tabs = %s", got)
		}
		barani := regexp.MustCompile(`(?s)<button[^>]*hx-vals="([^"]*barani&#34;[^"]*)"[^>]*>.*?</button>`).FindStringSubmatch(html)
		if barani == nil || !strings.Contains(barani[0], ">Barani<") || !strings.Contains(barani[0], "0.6") || !strings.Contains(barani[1], "&#34;load&#34;:&#34;common&#34;") {
			t.Errorf("the picker should have a Barani (0.6) button that loads it: %v", barani)
		}
		if tag := tagWithID(t, html, "skill-builder"); strings.Contains(tag, " open") {
			t.Errorf("the builder should start closed when adding: %s", tag)
		}
	})

	t.Run("an edit panel is loaded with the skill being edited", func(t *testing.T) {
		rec := postForm(t, "/skill-form", url.Values{
			"skill":     {`{"name":"Barani Tuck","custom_name":"Opener","rotation":4,"twist_distribution":[1],"takeoff_position":"Feet","shape":"Tuck"}`},
			"editIndex": {"2"},
		})
		html := rec.Body.String()
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, html)
		}
		for _, want := range []string{`id="update-btn"`, "Cancel", `name="editIndex" value="2"`, `name="builder_open" value="1"`} {
			if !strings.Contains(html, want) {
				t.Errorf("edit panel is missing %q", want)
			}
		}
		if strings.Contains(html, "Add to routine") {
			t.Errorf("an edit panel should not offer Add")
		}
		if tag := tagWithID(t, html, "custom-name"); !strings.Contains(tag, `value="Opener"`) {
			t.Errorf("custom name = %s", tag)
		}
		if tag := tagWithID(t, html, "twist-1"); !strings.Contains(tag, `value="1"`) {
			t.Errorf("twist-1 = %s, want 1", tag)
		}
		if tag := tagWithID(t, html, "skill-builder"); !strings.Contains(tag, " open") {
			t.Errorf("the builder should be open when editing: %s", tag)
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

// TestSkillInputsFollowTheSkill covers the editor the server re-renders as the
// skill changes: twist boxes per phase, when shape is offered, straddle, the
// live summary, and what carries over from the form.
func TestSkillInputsFollowTheSkill(t *testing.T) {
	inputs := func(t *testing.T, form url.Values) string {
		t.Helper()
		rec := postForm(t, "/skill-inputs", form)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		return rec.Body.String()
	}

	t.Run("one twist box per somersault phase", func(t *testing.T) {
		html := inputs(t, skillFormValues("8", "feet", "tuck", "0"))
		tagWithID(t, html, "twist-2")
		if strings.Contains(html, `id="twist-3"`) {
			t.Errorf("a double has two phases, so two twist boxes")
		}
		for _, want := range []string{"2 somersaults", "1st somersault: half twists", "2nd somersault: half twists"} {
			if !strings.Contains(html, want) {
				t.Errorf("builder is missing %q", want)
			}
		}
	})

	t.Run("shape is only offered when it matters", func(t *testing.T) {
		html := inputs(t, withValue(skillFormValues("4", "feet", "straight", "2"), "backward", "on"))
		if strings.Contains(html, `name="shape" value="pike"`) || !strings.Contains(html, `<input type="hidden" name="shape" value="straight">`) {
			t.Errorf("a full back should keep its shape hidden, with no shape buttons")
		}
		html = inputs(t, skillFormValues("4", "feet", "pike", "0"))
		if !strings.Contains(html, `value="pike" checked`) || strings.Contains(html, `<input type="hidden" name="shape"`) {
			t.Errorf("a front should offer shapes with pike chosen")
		}
	})

	t.Run("straddle is only offered for basic jumps", func(t *testing.T) {
		if html := inputs(t, skillFormValues("0", "feet", "straddle", "0")); !strings.Contains(html, `value="straddle" checked`) {
			t.Errorf("a straddle jump should keep straddle chosen")
		}
		html := inputs(t, skillFormValues("4", "feet", "straddle", "0"))
		if strings.Contains(html, `value="straddle"`) {
			t.Errorf("straddle should not be offered for a somersault")
		}
		if !strings.Contains(html, `value="straight" checked`) {
			t.Errorf("a straddle somersault should fall back to straight")
		}
	})

	t.Run("the card summarises the skill as it changes", func(t *testing.T) {
		html := inputs(t, withValue(skillFormValues("8", "feet", "pike", "1", "1"), "backward", "on"))
		for _, want := range []string{`<p class="skill-card-name">Half Half Pike</p>`, "(8 1 1 &lt;)", ">1.5</p>"} {
			if !strings.Contains(html, want) {
				t.Errorf("card is missing %q", want)
			}
		}
		html = inputs(t, withValue(skillFormValues("1", "feet", "straight", "0"), "seat_landing", "on"))
		if !strings.Contains(html, "can't land") || !strings.Contains(html, "This skill can't land like that.") {
			t.Errorf("an impossible landing should be called out on the card")
		}
	})

	t.Run("choosing a common skill keeps the label and edit state", func(t *testing.T) {
		form := withValue(skillFormValues("0", "feet", "straight", "0"), "load", "common")
		form.Set("commonSkillKey", "halfOut")
		form.Set("custom_name", "Opener")
		form.Set("editIndex", "3")
		form.Set("builder_open", "1")
		html := inputs(t, form)
		if !strings.Contains(tagWithID(t, html, "rotation"), `value="8"`) || !strings.Contains(tagWithID(t, html, "twist-2"), `value="1"`) {
			t.Errorf("want the half-out (8, [0 1]) loaded")
		}
		for _, want := range []string{`value="Opener"`, `name="editIndex" value="3"`, `id="update-btn"`, `name="builder_open" value="1"`} {
			if !strings.Contains(html, want) {
				t.Errorf("editor is missing %q", want)
			}
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

func TestEditorReadouts(t *testing.T) {
	// The builder reads quarter somersaults and half twists back in plain terms.
	html := postForm(t, "/skill-inputs", skillFormValues("7", "feet", "tuck", "3", "0")).Body.String()
	for _, want := range []string{"1¾ somersaults", "1½ twists", "No twist"} {
		if !strings.Contains(html, want) {
			t.Errorf("builder is missing %q", want)
		}
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

	t.Run("the editor escapes the label", func(t *testing.T) {
		form := withValue(skillFormValues("4", "feet", "tuck", "0"), "custom_name", `"><img src=x onerror=alert(1)>`)
		rec := postForm(t, "/skill-inputs", form)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), "<img") {
			t.Errorf("custom name was rendered unescaped:\n%s", rec.Body)
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
	if len(links) != 11 {
		t.Errorf("found %d asset links, want 11 (2 CSS, 9 JS)", len(links))
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

func postSheet(t *testing.T, routineJSON string) *httptest.ResponseRecorder {
	t.Helper()
	return postForm(t, "/tariff-sheet", url.Values{"routineData": {routineJSON}})
}

func TestTariffSheet(t *testing.T) {
	rec := postSheet(t, `[
		{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"},
		{"custom_name":"Opener <b>","rotation":4,"twist_distribution":[1],"takeoff_position":"Feet","shape":"Pike"},
		{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}
	]`)
	html := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, html)
	}

	rows := regexp.MustCompile(`(?s)<tbody>(.*)</tbody>`).FindStringSubmatch(html)
	if rows == nil {
		t.Fatalf("no table body in:\n%s", html)
	}
	trs := strings.Split(rows[1], "<tr")[1:]
	if len(trs) != 10 {
		t.Errorf("rendered %d rows, want a full exercise of 10", len(trs))
	}
	for i, want := range []string{"Front Tuck", "Barani Pike", "Front Tuck"} {
		if !strings.Contains(trs[i], want) {
			t.Errorf("row %d is missing %q", i+1, want)
		}
	}
	if !strings.Contains(trs[1], "Opener &lt;b&gt;") {
		t.Errorf("the custom name should be shown, escaped")
	}
	for _, want := range []string{"(4 1 &lt;)", "0.6"} {
		if !strings.Contains(trs[1], want) {
			t.Errorf("row 2 is missing %q", want)
		}
	}
	// The Req. and Judge columns can be hidden: every row (and the footer) has a cell for each.
	for _, col := range []string{`class="req"`, `class="judge"`} {
		if n := strings.Count(html, "<td "+col); n != 11 { // 10 rows + footer
			t.Errorf("%d cells with %s, want 11", n, col)
		}
	}
	// Each details field can be hidden on its own.
	for _, key := range []string{"gymnast", "club", "category", "competition", "round", "coach"} {
		if !strings.Contains(html, `<label class="field-`+key+`">`) {
			t.Errorf("details field %q is missing its class", key)
		}
	}
	// The name column can be hidden: every row tags it, and the footer has a label for each mode.
	for i, tr := range trs {
		if !strings.Contains(tr, `class="col-name"`) {
			t.Errorf("row %d has no name cell to hide", i+1)
		}
	}
	if !strings.Contains(html, `colspan="3" class="total-with-names"`) || !strings.Contains(html, `colspan="2" class="total-without-names"`) {
		t.Errorf("the footer needs a total label for both layouts")
	}
	if !strings.Contains(trs[2], "not counted (repeat)") || strings.Contains(trs[0], "not counted") {
		t.Errorf("only the repeat (row 3) should be marked not counted")
	}
	for _, want := range []string{
		`<td class="diff">1.1</td>`, // total: 0.5 + 0.6; the repeat is not counted
		"Check before printing:",
		"Element 3: Duplicate",
		"The routine has 3 elements; an exercise has 10.",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("sheet is missing %q", want)
		}
	}
}

func TestTariffSheetPage(t *testing.T) {
	rec := httptest.NewRecorder()
	routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tariff-sheet", nil))
	html := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	for _, want := range []string{`hx-post="/tariff-sheet"`, `hx-trigger="load"`, "Exercises.values(RoutineStore.current(RoutineStore.load()))", `id="level-data"`, "/static/js/routines.js?v=", "/static/js/sets.js?v=", `onclick="window.print()"`, "/static/css/sheet.css?v=", "/static/js/htmx.min.js?v=",
		// Optional parts of the sheet.
		`id="show-names" data-hides="hide-names" checked`, "/static/js/sheet.js?v=",
		`id="show-req" data-hides="hide-req" checked`, `id="show-judge" data-hides="hide-judge" checked`,
		`id="show-field-gymnast" data-hides="hide-field-gymnast" checked`, `id="show-field-coach" data-hides="hide-field-coach" checked`} {
		if !strings.Contains(html, want) {
			t.Errorf("sheet page is missing %q", want)
		}
	}

	// The calculator links to it.
	rec = httptest.NewRecorder()
	routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rec.Body.String(), `href="/tariff-sheet" target="_blank"`) {
		t.Errorf("the calculator should link to the tariff sheet")
	}
}

func TestTariffSheetEdgeCases(t *testing.T) {
	elevenFronts := "[" + strings.TrimSuffix(strings.Repeat(`{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"},`, 11), ",") + "]"
	html := postSheet(t, elevenFronts).Body.String()
	body := regexp.MustCompile(`(?s)<tbody>(.*)</tbody>`).FindStringSubmatch(html)[1]
	if n := strings.Count(body, "<tr"); n != 11 {
		t.Errorf("11 skills should give 11 rows, got %d", n)
	}
	if !strings.Contains(html, "not counted (after the 10th)") {
		t.Errorf("the 11th skill should be marked as after the 10th")
	}

	if rec := postSheet(t, `[{"rotation":-1,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}]`); rec.Code != http.StatusBadRequest {
		t.Errorf("an invalid routine should be rejected, got %d", rec.Code)
	}
}

func TestSkillSearch(t *testing.T) {
	get := func(q string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/skill-search?q="+url.QueryEscape(q), nil))
		return rec
	}

	html := get("barani pike").Body.String()
	if !strings.Contains(html, "Barani Pike") || !strings.Contains(html, "(4 1 &lt;)") || !strings.Contains(html, "&#34;load&#34;:&#34;skill&#34;") {
		t.Errorf("a name search should list loadable skills, got:\n%s", html)
	}
	if strings.Contains(html, "forward") {
		t.Errorf("name results don't need a direction label")
	}

	html = get("4 - o").Body.String()
	if !strings.Contains(html, "· forward") || !strings.Contains(html, "· backward") {
		t.Errorf("notation results should say which direction they are")
	}

	if html := get("zzz").Body.String(); !strings.Contains(html, `No skills match "zzz"`) {
		t.Errorf("no matches should say so, got %q", html)
	}
	if rec := get("  "); rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("an empty search should render nothing")
	}
}

func TestLoadingASearchResult(t *testing.T) {
	form := url.Values{
		"load":        {"skill"},
		"skill":       {`{"rotation":8,"twist_distribution":[1,1],"takeoff_position":"Feet","shape":"Pike","backward":true}`},
		"custom_name": {"Opener"},
	}
	html := postForm(t, "/skill-inputs", form).Body.String()
	if !strings.Contains(html, `<p class="skill-card-name">Half Half Pike</p>`) || !strings.Contains(html, `value="Opener"`) {
		t.Errorf("a search result should load into the card, keeping the label:\n%s", html)
	}

	form.Set("skill", `{"rotation":-4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}`)
	if rec := postForm(t, "/skill-inputs", form); rec.Code != http.StatusBadRequest {
		t.Errorf("an invalid posted skill should be rejected, got %d", rec.Code)
	}
}

func TestCompare(t *testing.T) {
	front := `{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}`
	barani := `{"rotation":4,"twist_distribution":[1],"takeoff_position":"Feet","shape":"Tuck"}`
	rudi := `{"rotation":4,"twist_distribution":[3],"takeoff_position":"Feet","shape":"Straight"}`
	rec := postForm(t, "/compare", url.Values{
		"aName": {"Spring <b>"}, "aData": {"[" + front + "," + barani + "]"},
		"bName": {"Summer"}, "bData": {"[" + front + "," + rudi + "," + barani + "]"},
	})
	html := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, html)
	}

	rows := strings.Split(regexp.MustCompile(`(?s)<tbody>(.*)</tbody>`).FindStringSubmatch(html)[1], "<tr")[1:]
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (the longer routine)", len(rows))
	}
	if strings.Contains(rows[0], "differs") || !strings.Contains(rows[1], "differs") || !strings.Contains(rows[2], "differs") {
		t.Errorf("only rows 2 and 3 differ (barani vs rudi, and a missing skill)")
	}
	if !strings.Contains(rows[2], "compare-empty") {
		t.Errorf("the shorter routine should show an empty cell")
	}
	for _, want := range []string{
		"Spring &lt;b&gt;", "Summer",
		`<td class="compare-total">1.1</td>`, `<td class="compare-total">1.9</td>`,
		"Summer scores 0.8 more difficulty than Spring &lt;b&gt;.",
		"The routine has 2 elements; an exercise has 10.",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("comparison is missing %q", want)
		}
	}

	if rec := postForm(t, "/compare", url.Values{"aData": {`[{"rotation":-1}]`}, "bData": {"[]"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("an invalid routine should be rejected, got %d", rec.Code)
	}
}

func TestComparePage(t *testing.T) {
	rec := httptest.NewRecorder()
	routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/compare", nil))
	html := rec.Body.String()
	for _, want := range []string{`id="compare-a"`, `id="compare-b"`, `id="comparison"`, "/static/js/compare.js?v=", "/static/js/routines.js?v="} {
		if !strings.Contains(html, want) {
			t.Errorf("compare page is missing %q", want)
		}
	}
	rec = httptest.NewRecorder()
	routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rec.Body.String(), `href="/compare"`) {
		t.Errorf("the calculator should link to the compare page")
	}
}

func TestRequirementsInTheRoutineView(t *testing.T) {
	routine := `[
		{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},
		{"rotation":0,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Straight","seat_landing":true},
		{"rotation":0,"twist_distribution":[0],"takeoff_position":"Seat","shape":"Straight"}
	]`
	post := func(set string) string {
		t.Helper()
		rec := postForm(t, "/routine", url.Values{"routineData": {routine}, "requirementSet": {set}})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		return rec.Body.String()
	}

	if html := post(""); strings.Contains(html, "requirements-results") {
		t.Errorf("no set chosen: no requirements panel")
	}

	// A back tuck, seat drop and seat to feet against FIG AG1's first exercise:
	// too short, and none of the special requirements met.
	html := post("builtin:fig-ag1-first")
	for _, want := range []string{"FIG AG1 (11–12) · first exercise", "3 of 5 met", "has 3",
		"missing: Landing on the front; Landing on the back; At least a full twist"} {
		if !strings.Contains(html, want) {
			t.Errorf("built-in check is missing %q", want)
		}
	}

	custom := `{"format":1,"name":"Our gala","rules":[{"type":"count","match":{"landing":["seat"]},"min":1},{"type":"count","match":{"direction":"forward","rotation":{"min":4}},"min":1}]}`
	html = post(custom)
	for _, want := range []string{"Our gala", "1 of 2 met", "At least 1 element: landing on seat", "element 2", "found 0"} {
		if !strings.Contains(html, want) {
			t.Errorf("custom check is missing %q", want)
		}
	}

	html = post(`{"format":1,"name":"Broken","rules":[{"type":"vibes"}]}`)
	if !strings.Contains(html, "Couldn't use these requirements") || !strings.Contains(html, "unknown rule type") {
		t.Errorf("broken requirements should be reported, not fail the routine:\n%s", html)
	}
	if html := post("builtin:gone"); !strings.Contains(html, "no longer exist") {
		t.Errorf("a missing built-in should be reported")
	}
}

func TestTariffSheetTicksRequiredElements(t *testing.T) {
	routine := `[
		{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"},
		{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true}
	]`
	set := `{"format":1,"name":"x","rules":[{"type":"count","match":{"direction":"backward"},"min":1}]}`
	html := postForm(t, "/tariff-sheet", url.Values{"routineData": {routine}, "requirementSet": {set}}).Body.String()
	boxes := regexp.MustCompile(`<td class="req"><input type="checkbox"( checked)?`).FindAllStringSubmatch(html, -1)
	if len(boxes) != 2 || boxes[0][1] != "" || boxes[1][1] != " checked" {
		t.Errorf("only element 2 (the back somersault) should be ticked, got %v", boxes)
	}
}

func TestPageOffersBuiltinSets(t *testing.T) {
	rec := httptest.NewRecorder()
	routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	html := rec.Body.String()
	if !strings.Contains(html, `<option value="builtin:fig-ag1-first"`) || !strings.Contains(html, `<optgroup label="British Gymnastics national pathway (2026)"`) || !strings.Contains(html, `href="/requirements"`) {
		t.Errorf("the calculator should offer built-in sets and link to the requirements page")
	}
}

// The second builder column's cards call the page with their side, and when a
// routine is compared with another, the skills that differ are marked and the
// totals compared.
func TestRoutineViewSideBySide(t *testing.T) {
	a := `[{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},
		{"rotation":4,"twist_distribution":[1],"takeoff_position":"Feet","shape":"Tuck"}]`
	b := `[{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},
		{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Pike","backward":true},
		{"rotation":0,"twist_distribution":[1],"takeoff_position":"Feet","shape":"Straight"}]`

	html := postForm(t, "/routine", url.Values{"routineData": {a}}).Body.String()
	if !strings.Contains(html, `id="routine-skills"`) || !strings.Contains(html, `editSkill(0, &#39;a&#39;)`) || strings.Contains(html, "differs") {
		t.Errorf("a single routine: the first column's cards, nothing compared:\n%s", html)
	}

	html = postForm(t, "/routine", url.Values{"routineData": {b}, "side": {"b"}, "compareData": {a}, "compareName": {"Routine 1"}}).Body.String()
	for _, want := range []string{`id="routine-skills-b"`, `editSkill(2, &#39;b&#39;)`, `expandedB[1]`, `editingSide === &#39;b&#39;`, "0.1 more than Routine 1"} {
		if !strings.Contains(html, want) {
			t.Errorf("second column is missing %q", want)
		}
	}
	// Skill 1 is the same in both; skill 2 differs; skill 3 has no partner.
	cards := regexp.MustCompile(`class="routine-skill box mb-2[^"]*"`).FindAllString(html, -1)
	if len(cards) != 3 || strings.Contains(cards[0], "differs") || !strings.Contains(cards[1], "differs") || !strings.Contains(cards[2], "differs") {
		t.Errorf("differing cards: %v", cards)
	}

	// Only "a" and "b" are sides; anything else is the first column.
	html = postForm(t, "/routine", url.Values{"routineData": {a}, "side": {"x');alert(1);//"}}).Body.String()
	if strings.Contains(html, "alert") || !strings.Contains(html, `editSkill(0, &#39;a&#39;)`) {
		t.Errorf("an unknown side should be the first column")
	}
}

func TestPageOffersSideBySide(t *testing.T) {
	rec := httptest.NewRecorder()
	routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	html := rec.Body.String()
	for _, want := range []string{`id="compare-with"`, `id="routine-view-b"`, `id="requirement-set-b"`, `href="/compare"`} {
		if !strings.Contains(html, want) {
			t.Errorf("the page is missing %q", want)
		}
	}
	// The skill adder asks which routine to add to while comparing.
	if form := postForm(t, "/skill-form", url.Values{}).Body.String(); !strings.Contains(form, `class="add-to"`) {
		t.Errorf("the skill form should offer a choice of routine to add to")
	}
}

func TestSetRoutineEndpoint(t *testing.T) {
	type response struct {
		Name    string                   `json:"name"`
		Skills  []skills.TrampolineSkill `json:"skills"`
		Matches bool                     `json:"matches"`
	}
	post := func(values url.Values) (int, response) {
		t.Helper()
		rec := postForm(t, "/set-routine", values)
		var got response
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
		}
		return rec.Code, got
	}

	code, got := post(url.Values{"requirementSet": {"builtin:bg-regional-l1-first"}})
	if code != http.StatusOK || got.Name != "BG Regional L1" || len(got.Skills) != 10 || got.Matches || got.Skills[0].Name != "Back Tuck" || got.Skills[9].Name != "Front Pike" {
		t.Fatalf("BG Regional L1: status %d, %+v", code, got)
	}
	routine, _ := json.Marshal(got.Skills)
	if _, again := post(url.Values{"requirementSet": {"builtin:bg-regional-l1-first"}, "routineData": {string(routine)}}); !again.Matches {
		t.Errorf("the loaded routine should match its set routine")
	}

	// The routine view offers to load a set routine that isn't met.
	html := postForm(t, "/routine", url.Values{"routineData": {"[]"}, "requirementSet": {"builtin:bg-regional-l1-first"}, "side": {"b"}}).Body.String()
	if !strings.Contains(html, "Load this set routine") || !strings.Contains(html, "loadSetRoutine(&#39;b&#39;, true)") {
		t.Errorf("an unmet set routine should offer to load it")
	}
	html = postForm(t, "/routine", url.Values{"routineData": {string(routine)}, "requirementSet": {"builtin:bg-regional-l1-first"}}).Body.String()
	if strings.Contains(html, "Load this set routine") {
		t.Errorf("a met set routine has nothing to load")
	}

	for name, set := range map[string]string{"not a set routine": "builtin:fig-ag1-first", "unknown set": "builtin:gone", "nothing": ""} {
		if code, _ := post(url.Values{"requirementSet": {set}}); code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, code)
		}
	}
}

// The picker's shape buttons load the skill in that shape; a jump's shape is
// the jump.
func TestPickerLoadsAShape(t *testing.T) {
	for _, c := range []struct{ key, shape, want string }{
		{"backSomersault", "Pike", "Back Pike"},
		{"backSomersault", "Straight", "Back Straight"},
		{"shapeJump", "Straddle", "Straddle Jump"},
		{"shapeJump", "Pike", "Pike Jump"},
		{"backSomersault", "", "Back Tuck"},         // its usual shape
		{"backSomersault", "sideways", "Back Tuck"}, // not a shape
	} {
		html := postForm(t, "/skill-inputs", url.Values{"load": {"common"}, "commonSkillKey": {c.key}, "shape": {c.shape}}).Body.String()
		if !strings.Contains(html, `<p class="skill-card-name">`+c.want+`</p>`) {
			t.Errorf("%s in %q: want %s", c.key, c.shape, c.want)
		}
	}

	form := postForm(t, "/skill-form", url.Values{}).Body.String()
	for _, want := range []string{">Pike Jump<", ">Straddle Jump<", `class="picker-shapes"`, "&#34;shape&#34;:&#34;Pike&#34;"} {
		if !strings.Contains(form, want) {
			t.Errorf("the picker is missing %q", want)
		}
	}
}

// The skill card's shape buttons say how each shape differs from the one chosen.
func TestSkillCardShapeDifferences(t *testing.T) {
	segments := func(shape string) map[string]string {
		t.Helper()
		html := postForm(t, "/skill-inputs", url.Values{"load": {"common"}, "commonSkillKey": {"backSomersault"}, "shape": {shape}}).Body.String()
		got := map[string]string{}
		for _, m := range regexp.MustCompile(`(?s)<label class="segment shape-segment[^"]*"><input[^>]*value="(\w+)"[^>]*>\s*\w+\s*(?:<span class="segment-modifier">([^<]*)</span>)?`).FindAllStringSubmatch(html, -1) {
			got[m[1]] = m[2]
		}
		return got
	}
	for shape, want := range map[string]map[string]string{
		"Tuck":     {"tuck": "", "pike": "+0.1", "straight": "+0.1"},
		"Pike":     {"tuck": "−0.1", "pike": "", "straight": ""},
		"Straight": {"tuck": "−0.1", "pike": "", "straight": ""},
	} {
		if got := segments(shape); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("Back %s: %v, want %v", shape, got, want)
		}
	}
}

// The sheet's Element heading spans its Name and FIG sub-columns, and stays
// (over FIG alone) when names are hidden.
func TestTariffSheetElementHeading(t *testing.T) {
	html := postSheet(t, `[{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck"}]`).Body.String()
	head := regexp.MustCompile(`(?s)<thead>(.*)</thead>`).FindStringSubmatch(html)[1]
	for _, want := range []string{
		`<th class="element element-with-names" colspan="2">Element</th>`,
		`<th class="element element-without-names">Element</th>`,
		`<th class="col-name">Name</th>`,
		`<th class="fig">FIG</th>`,
		`<th class="num" rowspan="2">No.</th>`,
	} {
		if !strings.Contains(head, want) {
			t.Errorf("heading is missing %s", want)
		}
	}
}

// Every response says which version of the app sent it, and every page carries
// the version it was served with, so an out-of-date page can tell.
func TestAppVersion(t *testing.T) {
	for _, path := range []string{"/", "/tariff-sheet", "/compare", "/requirements"} {
		rec := httptest.NewRecorder()
		routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Header().Get(static.VersionHeader); got != static.Version || got == "" {
			t.Errorf("%s: %s = %q, want %q", path, static.VersionHeader, got, static.Version)
		}
		if body := rec.Body.String(); !strings.Contains(body, `<meta name="app-version" content="`+static.Version+`">`) || !strings.Contains(body, "js/version.js") {
			t.Errorf("%s: the page should carry its version and load version.js", path)
		}
	}
	if rec := postForm(t, "/routine", url.Values{"routineData": {"[]"}}); rec.Header().Get(static.VersionHeader) != static.Version {
		t.Errorf("fragments should carry the version too")
	}
}

// A set routine scores no difficulty and may repeat elements; the routine can
// turn either check back on.
func TestRoutineChecks(t *testing.T) {
	var got struct {
		Skills []skills.TrampolineSkill `json:"skills"`
	}
	rec := postForm(t, "/set-routine", url.Values{"requirementSet": {"builtin:bucs-l4-option-1"}})
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("loading BUCS L4 option 1: %d", rec.Code)
	}
	routine, _ := json.Marshal(got.Skills) // it repeats the tuck jump
	view := func(checks string) string {
		t.Helper()
		return postForm(t, "/routine", url.Values{"routineData": {string(routine)}, "requirementSet": {"builtin:bucs-l4-option-1"}, "checks": {checks}}).Body.String()
	}
	checkbox := regexp.MustCompile(`<input type="checkbox"( checked)? x-on:change="setCheck\(&#39;a&#39;, &#39;(\w+)&#39;`)
	boxes := func(html string) string {
		var out []string
		for _, m := range checkbox.FindAllStringSubmatch(html, -1) {
			out = append(out, m[2]+map[bool]string{true: " on", false: " off"}[m[1] != ""])
		}
		return strings.Join(out, ", ")
	}

	html := view("")
	if strings.Contains(html, "Duplicate") || strings.Contains(html, "duplicate-skill") || !strings.Contains(html, "Difficulty not scored") || strings.Contains(html, "Total Tariff") {
		t.Errorf("a set routine: no repeat warnings, difficulty not scored")
	}
	if got := boxes(html); got != "difficulty off, repeats off" {
		t.Errorf("a set routine's checks: %s", got)
	}

	html = view(`{"difficulty":true,"repeats":true}`)
	if !strings.Contains(html, "Duplicate (Counts Once)") || !strings.Contains(html, "Total Tariff") {
		t.Errorf("with both checks turned back on, repeats are flagged and difficulty scored")
	}
	if got := boxes(html); got != "difficulty on, repeats on" {
		t.Errorf("the routine's own checks: %s", got)
	}

	// Without a set, everything is checked.
	if got := boxes(postForm(t, "/routine", url.Values{"routineData": {string(routine)}}).Body.String()); got != "difficulty on, repeats on" {
		t.Errorf("without a set: %s", got)
	}

	// The sheet follows: no total when difficulty isn't scored.
	sheet := postForm(t, "/tariff-sheet", url.Values{"routineData": {string(routine)}, "requirementSet": {"builtin:bucs-l4-option-1"}}).Body.String()
	if !strings.Contains(sheet, "Difficulty isn't scored for this routine.") || !regexp.MustCompile(`<td class="diff">\s*—\s*</td>`).MatchString(sheet) {
		t.Errorf("the sheet should leave out the total when difficulty isn't scored")
	}
}

// An AG3 first exercise scores only its 2 highest elements; the routine can
// score them all instead.
func TestScoredElements(t *testing.T) {
	// Back tuck 0.5, Barani 0.6, full back 0.7, Rudi 0.8.
	routine := `[{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},
		{"rotation":4,"twist_distribution":[1],"takeoff_position":"Feet","shape":"Tuck"},
		{"rotation":4,"twist_distribution":[2],"takeoff_position":"Feet","shape":"Straight","backward":true},
		{"rotation":4,"twist_distribution":[3],"takeoff_position":"Feet","shape":"Straight"}]`
	values := func(checks string) url.Values {
		return url.Values{"routineData": {routine}, "requirementSet": {"builtin:fig-ag3-first"}, "checks": {checks}}
	}
	html := postForm(t, "/routine", values("")).Body.String()
	if !strings.Contains(html, "Total Tariff: 1.50") || strings.Count(html, `class="scores-toggle is-scoring"`) != 2 || strings.Count(html, `class="scores-toggle`) != 4 ||
		!strings.Contains(html, `<option value="2" selected>`) || !strings.Contains(html, `data-scoring="[2,3]"`) {
		t.Errorf("AG3 first exercise: the full back and Rudi (1.5) score, ticked; every element can be ticked")
	}
	if html := postForm(t, "/routine", values(`{"scored":0}`)).Body.String(); !strings.Contains(html, "Total Tariff: 2.60") || strings.Contains(html, "scores-toggle") {
		t.Errorf("scoring every element: 2.6, no Scores boxes")
	}

	// The coach chose the back tuck and Barani instead.
	chosen := strings.Replace(strings.Replace(routine, `"backward":true}`, `"backward":true,"scores":true}`, 1), `"shape":"Tuck"}`, `"shape":"Tuck","scores":true}`, 1)
	html = postForm(t, "/routine", url.Values{"routineData": {chosen}, "requirementSet": {"builtin:fig-ag3-first"}}).Body.String()
	if !strings.Contains(html, "Total Tariff: 1.10") || !strings.Contains(html, "From the elements you ticked") || !strings.Contains(html, "clearScores(&#39;a&#39;)") {
		t.Errorf("the chosen back tuck and Barani (1.1) score")
	}

	sheet := postForm(t, "/tariff-sheet", values("")).Body.String()
	cells := regexp.MustCompile(`(?s)<td class="diff">\s*([0-9.]*)\s*</td>`).FindAllStringSubmatch(sheet, -1)
	var got []string
	for _, c := range cells {
		got = append(got, c[1])
	}
	if strings.Join(got, ",") != ",,0.7,0.8,1.5" || strings.Contains(sheet, "not-counted") {
		t.Errorf("the sheet shows values for the scoring elements only: %v", got)
	}
}

func TestQRCode(t *testing.T) {
	rec := postForm(t, "/qr", url.Values{"text": {"https://example.test/#share=abc"}})
	svg := rec.Body.String()
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/svg+xml" || !strings.HasPrefix(svg, "<svg") || !strings.Contains(svg, `<path d="M4 4h7v1h-7z`) {
		t.Errorf("a QR code SVG with its finder pattern 4 modules in: %d %.120s", rec.Code, svg)
	}
	if rec := postForm(t, "/qr", url.Values{"text": {""}}); rec.Code != http.StatusBadRequest {
		t.Errorf("nothing to encode: status %d", rec.Code)
	}
	if rec := postForm(t, "/qr", url.Values{"text": {strings.Repeat("x", 5000)}}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "too much for one QR code") {
		t.Errorf("too long: status %d %s", rec.Code, rec.Body)
	}
}

// Both pages can share: the builder its routines, the requirements page its sets.
func TestSharing(t *testing.T) {
	for path, want := range map[string]string{
		"/":             `Share.open('routines', [currentId])`,
		"/requirements": `Share.open('sets', [s.id])`,
	} {
		rec := httptest.NewRecorder()
		routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if body := rec.Body.String(); !strings.Contains(body, "js/share.js") || !strings.Contains(body, want) {
			t.Errorf("%s should load share.js and offer to share", path)
		}
	}
}

// Set (prescribed) routines are listed apart from requirements, by source.
func TestSetRoutinesListedApart(t *testing.T) {
	rec := httptest.NewRecorder()
	routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	page := rec.Body.String()
	groups := map[string]string{}
	for _, m := range regexp.MustCompile(`(?s)<optgroup label="([^"]+)">(.*?)</optgroup>`).FindAllStringSubmatch(page, -1) {
		groups[m[1]] += m[2]
	}
	if !strings.Contains(groups["Set routines · BUCS student championships (2026)"], `value="builtin:bucs-l3-option-1"`) ||
		strings.Contains(groups["BUCS student championships (2026)"], `value="builtin:bucs-l3-option-1"`) ||
		!strings.Contains(groups["BUCS student championships (2026)"], `value="builtin:bucs-l3-second"`) {
		t.Errorf("BUCS L3 option 1 belongs with the set routines, its second exercise with the requirements")
	}
	if !strings.Contains(groups["Set routines · British Gymnastics club &amp; regional pathway (2027)"], ">BG Club L1<") {
		t.Errorf("BG club routines are set routines")
	}
	if _, ok := groups["Set routines · FIG age groups (2025–2028)"]; ok {
		t.Errorf("FIG has no set routines, so no group")
	}

	rec = httptest.NewRecorder()
	routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/requirements", nil))
	if body := rec.Body.String(); !strings.Contains(body, `<h2 class="title is-5">Set routines</h2>`) || !strings.Contains(body, "<li>Back somersault (T)</li>") {
		t.Errorf("the requirements page should list set routines on their own, element by element")
	}
}

func TestLevelPairs(t *testing.T) {
	// First exercise: back tuck 0.5, Barani 0.6, full back 0.7, Rudi 0.8; the full back and Rudi score.
	first := `[{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},
		{"rotation":4,"twist_distribution":[1],"takeoff_position":"Feet","shape":"Tuck"},
		{"rotation":4,"twist_distribution":[2],"takeoff_position":"Feet","shape":"Straight","backward":true},
		{"rotation":4,"twist_distribution":[3],"takeoff_position":"Feet","shape":"Straight"}]`
	// Second exercise: the Rudi again (scored in the first), and a back tuck (didn't score).
	second := `[{"rotation":4,"twist_distribution":[3],"takeoff_position":"Feet","shape":"Straight"},
		{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true}]`
	values := func(level, exercise, own, ownSet, pair, pairSet string) url.Values {
		v := url.Values{"routineData": {own}, "requirementSet": {ownSet}, "level": {level}, "exercise": {exercise}, "pairName": {"Partner"}}
		if pair != "" {
			v.Set("pairData", pair)
			v.Set("pairSet", pairSet)
		}
		return v
	}

	html := postForm(t, "/routine", values("builtin-level:fig-ag3", "2", second, "builtin:fig-ag3-second", first, "builtin:fig-ag3-first")).Body.String()
	if !strings.Contains(html, "Total Tariff: 0.50") || !strings.Contains(html, "Can&#39;t Repeat: Scored In 1st Exercise") {
		t.Errorf("AG3 second exercise: the Rudi's difficulty carried over from the first, so it can't be repeated and only the back tuck (0.5) counts")
	}
	if !strings.Contains(html, `class="is-unmet"`) || !strings.Contains(html, "carried over: elements 3, 4 of the first exercise · repeated at element 1") {
		t.Errorf("AG3 second exercise: the repeat breaks the carry-over requirement")
	}
	if !strings.Contains(html, "FIG AG3 (17–21)</strong> · second exercise") || !strings.Contains(html, "First exercise: Partner") || !strings.Contains(html, "of 5 met") {
		t.Errorf("the level panel names the level, the exercise and the first exercise's routine")
	}

	// Without the first exercise, or at a level without the rule, everything counts.
	if html := postForm(t, "/routine", values("builtin-level:fig-ag3", "2", second, "builtin:fig-ag3-second", "", "")).Body.String(); !strings.Contains(html, "Total Tariff: 1.30") || !strings.Contains(html, "No routine for the first exercise yet") {
		t.Errorf("no first exercise: 1.3, and the panel says so")
	}
	if html := postForm(t, "/routine", values("builtin-level:fig-ag2-junior", "2", second, "builtin:fig-ag2-junior-second", first, "builtin:fig-ag2-junior-first")).Body.String(); !strings.Contains(html, "Total Tariff: 1.30") || strings.Contains(html, "carried over") {
		t.Errorf("AG2: no difficulty carries over from the first exercise, so elements may be repeated")
	}
	// It follows from the first exercise scoring only some elements, at any level.
	if html := postForm(t, "/routine", values("builtin-level:bg-national-17-21", "2", second, "builtin:bg-national-17-21-second", first, "builtin:bg-national-17-21-first")).Body.String(); !strings.Contains(html, "Total Tariff: 0.50") {
		t.Errorf("BG 17–21: two elements carry over, as in AG3")
	}
	ownChecks := values("builtin-level:fig-ag3", "2", second, "builtin:fig-ag3-second", first, "builtin:fig-ag3-first")
	ownChecks.Set("pairChecks", `{"scored":0}`)
	if html := postForm(t, "/routine", ownChecks).Body.String(); !strings.Contains(html, "Total Tariff: 1.30") {
		t.Errorf("when the coach scores every element of the first exercise, nothing carries over")
	}

	// The first exercise is unaffected by the second.
	if html := postForm(t, "/routine", values("builtin-level:fig-ag3", "1", first, "builtin:fig-ag3-first", second, "builtin:fig-ag3-second")).Body.String(); !strings.Contains(html, "Total Tariff: 1.50") || !strings.Contains(html, "Second exercise: Partner") ||
		!strings.Contains(html, "carried over: elements 3, 4 · the second exercise repeats them at element 1") {
		t.Errorf("AG3 first exercise: the full back and Rudi score (1.5) and carry over; the second exercise repeats the Rudi")
	}

	// A custom level, posted as JSON.
	custom := `{"format":1,"name":"Club voluntaries","first":{"options":["builtin:bucs-l1-first"]}}`
	if html := postForm(t, "/routine", values(custom, "1", first, "builtin:bucs-l1-first", "", "")).Body.String(); !strings.Contains(html, "Club voluntaries</strong> · first exercise") {
		t.Errorf("a custom level is named in the panel")
	}
	if html := postForm(t, "/routine", values(`{"format":1,"name":"Broken","first":{"options":[]}}`, "1", first, "", "", "")).Body.String(); !strings.Contains(html, "Couldn't use this level") {
		t.Errorf("a level that doesn't validate is reported")
	}

	// The sheet leaves the repeat's difficulty out.
	sheet := postForm(t, "/tariff-sheet", values("builtin-level:fig-ag3", "2", second, "builtin:fig-ag3-second", first, "builtin:fig-ag3-first")).Body.String()
	if !strings.Contains(sheet, "repeats a 1st-exercise element") {
		t.Errorf("the sheet says why the Rudi adds nothing")
	}
}

func TestViewScreen(t *testing.T) {
	page := getPage(t, "/view")
	for _, want := range []string{"/static/js/view.js?v=", "/static/css/view.css?v=", `id="view-routine"`, `data-hides="hide-fig" checked`, `data-hides="hide-req">`, `id="display"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the view page is missing %q", want)
		}
	}
	if !strings.Contains(getPage(t, "/"), "`/view?routine=${currentId}`") {
		t.Error("the builder should link to the view of the current routine")
	}

	first := `[{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true},
		{"rotation":4,"twist_distribution":[3],"takeoff_position":"Feet","shape":"Straight"}]`
	second := `[{"rotation":4,"twist_distribution":[3],"takeoff_position":"Feet","shape":"Straight","custom_name":"My Rudi"}]`

	// Viewing the second exercise shows the first beside it, first.
	html := postForm(t, "/view", url.Values{
		"routineData": {second}, "requirementSet": {"builtin:fig-ag3-second"}, "routineName": {"Q2"},
		"level": {"builtin-level:fig-ag3"}, "exercise": {"2"},
		"pairData": {first}, "pairSet": {"builtin:fig-ag3-first"}, "pairName": {"Q1"},
	}).Body.String()
	q1, q2 := strings.Index(html, "<h2>Q1</h2>"), strings.Index(html, "<h2>Q2</h2>")
	if q1 < 0 || q2 < 0 || q1 > q2 || !strings.Contains(html, `class="display-column is-other"`) || !strings.Contains(html, "is-several") {
		t.Errorf("the pair shows in exercise order, the other one marked: Q1 at %d, Q2 at %d", q1, q2)
	}
	if !strings.Contains(html, "FIG AG3 (17–21) · second exercise") || !strings.Contains(html, "My Rudi") || !strings.Contains(html, `class="display-row not-counted"`) {
		t.Errorf("the second exercise names its level, shows the coach's label, and strikes the carried-over Rudi")
	}
	if strings.Count(html, `class="display-row is-blank"`) != 17 || !strings.Contains(html, "--rows: 10") {
		t.Errorf("each column has a full exercise of rows")
	}

	// One routine on its own, difficulty not scored.
	html = postForm(t, "/view", url.Values{"routineData": {first}, "routineName": {"Solo"}, "checks": {`{"difficulty":false}`}}).Body.String()
	if strings.Contains(html, "is-several") || !strings.Contains(html, "no difficulty") {
		t.Errorf("a single routine without difficulty")
	}
}

func TestViewShowsSetRoutineOptions(t *testing.T) {
	voluntary := `[{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true}]`
	titles := func(html string) []string {
		var out []string
		for _, m := range regexp.MustCompile(`<h2>([^<]*)</h2>`).FindAllStringSubmatch(html, -1) {
			out = append(out, m[1])
		}
		return out
	}
	base := url.Values{
		"routineData": {voluntary}, "requirementSet": {"builtin:bucs-l3-second"}, "optionRef": {"builtin:bucs-l3-second"},
		"routineName": {"BUCS L3 · second exercise"}, "level": {"builtin-level:bucs-l3"}, "exercise": {"2"},
	}

	// The voluntary alone: both set routine options show before it.
	html := postForm(t, "/view", base).Body.String()
	if got := strings.Join(titles(html), " | "); got != "BUCS L3 · option 1 | BUCS L3 · option 2 | BUCS L3 · second exercise" {
		t.Errorf("columns: %s", got)
	}
	if strings.Count(html, `class="display-column is-option"`) != 2 || !strings.Contains(html, "First exercise · set routine") {
		t.Errorf("the options are marked as set routines for the first exercise")
	}
	if strings.Count(html, `class="display-level"`) != 2 {
		t.Errorf("the voluntary's name already says its level and exercise, so it has no level line")
	}

	// A routine doing option 2: it shows in option 2's place, after option 1.
	paired := url.Values{}
	for k, v := range base {
		paired[k] = v
	}
	paired.Set("pairData", voluntary)
	paired.Set("pairSet", "builtin:bucs-l3-option-2")
	paired.Set("pairOptionRef", "builtin:bucs-l3-option-2")
	paired.Set("pairName", "My option 2")
	if got := strings.Join(titles(postForm(t, "/view", paired).Body.String()), " | "); got != "BUCS L3 · option 1 | My option 2 | BUCS L3 · second exercise" {
		t.Errorf("with a routine doing option 2: %s", got)
	}

	// A custom level's own set routine travels in optionSets.
	custom := url.Values{
		"routineData": {voluntary}, "routineName": {"Vol"}, "optionRef": {"builtin:bucs-l1-second"},
		"level":      {`{"format":1,"name":"Club","first":{"options":["set-mine"]},"second":{"options":["builtin:bucs-l1-second"]}}`},
		"exercise":   {"2"},
		"optionSets": {`{"set-mine":{"format":1,"name":"Our set","rules":[{"type":"sequence","sequence":[{"fig":"4 - o"},{"rotation":{"max":0},"shapes":["tuck"]}]}],"no_difficulty":true,"repeats_allowed":true}}`},
	}
	if got := strings.Join(titles(postForm(t, "/view", custom).Body.String()), " | "); got != "Our set | Vol" {
		t.Errorf("custom level: %s", got)
	}

	if !strings.Contains(getPage(t, "/view"), `data-hides="hide-options" checked`) {
		t.Errorf("the Show menu can hide the options")
	}
}

// A level routine's other exercise may be a set routine whose tab hasn't been
// opened: it's checked as the set routine its requirements describe.
func TestUnopenedSetRoutineTab(t *testing.T) {
	voluntary := `[{"rotation":4,"twist_distribution":[0],"takeoff_position":"Feet","shape":"Tuck","backward":true}]`
	html := postForm(t, "/routine", url.Values{
		"routineData": {voluntary}, "requirementSet": {"builtin:bucs-l7-second"},
		"level": {"builtin-level:bucs-l7"}, "exercise": {"2"},
		"pairData": {""}, "pairSet": {"builtin:bucs-l7-option-2"}, "pairName": {"Set 2"},
	}).Body.String()
	if !strings.Contains(html, "First exercise: Set 2") || !regexp.MustCompile(`(\d+) of (\d+) met`).MatchString(html) {
		t.Fatalf("the level panel should report the set routine")
	}
	panel := html[strings.Index(html, "First exercise: Set 2"):]
	if m := regexp.MustCompile(`(\d+) of (\d+) met`).FindStringSubmatch(panel); m == nil || m[1] != m[2] {
		t.Errorf("Set 2 as prescribed meets all of its requirements: %v", m)
	}
}
