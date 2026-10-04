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
		if barani == nil || !strings.Contains(barani[0], "Barani Tuck") || !strings.Contains(barani[0], "0.6") || !strings.Contains(barani[1], "&#34;load&#34;:&#34;common&#34;") {
			t.Errorf("the picker should have a Barani Tuck (0.6) button that loads it: %v", barani)
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
		for _, want := range []string{"Update skill", "Cancel", `name="editIndex" value="2"`, `name="builder_open" value="1"`} {
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
		if !strings.Contains(html, "Shape doesn't change this skill.") || !strings.Contains(html, `<input type="hidden" name="shape" value="straight">`) {
			t.Errorf("a full back should keep its shape hidden and say why")
		}
		html = inputs(t, skillFormValues("4", "feet", "pike", "0"))
		if !strings.Contains(html, `value="pike" checked`) || strings.Contains(html, "Shape doesn't change") {
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
		for _, want := range []string{`value="Opener"`, `name="editIndex" value="3"`, "Update skill", `name="builder_open" value="1"`} {
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
	if len(links) != 7 {
		t.Errorf("found %d asset links, want 7 (2 CSS, 5 JS)", len(links))
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
	for _, want := range []string{`hx-post="/tariff-sheet"`, `hx-trigger="load"`, "RoutineStore.current(RoutineStore.load()).skills", "/static/js/routines.js?v=", `onclick="window.print()"`, "/static/css/sheet.css?v=", "/static/js/htmx.min.js?v=",
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
	if n := strings.Count(html, "<tr") - 2; n != 11 { // minus header and footer rows
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
