package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"tariffCalculator/requirements"
)

// levelEditor posts to the level editor and returns the level it would save,
// the number of problems and the page.
func levelEditor(t *testing.T, form url.Values) (requirements.Level, string, string) {
	t.Helper()
	rec := postForm(t, "/requirements/level-editor", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	m := regexp.MustCompile(`data-level-json="([^"]*)"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no data-level-json in %s", body)
	}
	var level requirements.Level
	if err := json.Unmarshal([]byte(unescape(m[1])), &level); err != nil {
		t.Fatal(err)
	}
	problems := regexp.MustCompile(`data-problems="(\d+)"`).FindStringSubmatch(body)[1]
	return level, problems, body
}

func unescape(s string) string {
	return strings.NewReplacer("&#34;", `"`, "&quot;", `"`, "&amp;", "&", "&lt;", "<", "&gt;", ">", "&#39;", "'").Replace(s)
}

func TestLevelEditor(t *testing.T) {
	custom := `[{"id":"set-mine","name":"My voluntary","set_routine":false}]`

	// Opening a level shows its choices, the user's own requirements among them.
	level, problems, body := levelEditor(t, url.Values{
		"level":  {`{"format":1,"name":"Novice","first":{"options":["builtin:bucs-l5-option-1","builtin:bucs-l5-option-2"]},"second":{"options":["set-mine"]}}`},
		"custom": {custom},
	})
	if problems != "0" || level.Name != "Novice" || len(level.First.Options) != 2 || level.Second == nil || level.Second.Options[0] != "set-mine" {
		t.Errorf("opened level: %+v (%s problems)", level, problems)
	}
	for _, want := range []string{`<option value="builtin:bucs-l5-option-2" selected>`, `<option value="set-mine" selected>My voluntary</option>`, `name="second_same" value="1">`} {
		if !strings.Contains(body, want) {
			t.Errorf("the editor is missing %q", want)
		}
	}

	// The form: two set routines, then the same as the first ticked.
	form := url.Values{
		"name": {"Club"}, "custom": {custom},
		"first.n": {"2"}, "first.0": {"builtin:bg-club-l1"}, "first.1": {""},
		"second_same": {"1"}, "second.n": {"1"}, "second.0": {"set-mine"},
		"action": {"delete:first:1"},
	}
	level, problems, _ = levelEditor(t, form)
	if problems != "0" || level.Second != nil || len(level.First.Options) != 1 || level.First.Options[0] != "builtin:bg-club-l1" {
		t.Errorf("both exercises the club set routine: %+v (%s problems)", level, problems)
	}

	// Unticking "the same" gives the second exercise a choice to make.
	form.Del("second_same")
	form.Del("second.n")
	form.Set("action", "")
	if level, problems, body = levelEditor(t, form); level.Second == nil || len(level.Second.Options) != 1 || problems == "0" || !strings.Contains(body, "the second exercise has a choice still to make") {
		t.Errorf("unticked: %+v (%s problems)", level.Second, problems)
	}

	// Adding a choice, and requirements no longer saved.
	form.Set("action", "add:first")
	if level, _, _ = levelEditor(t, form); len(level.First.Options) != 3 {
		t.Errorf("add:first: %v", level.First.Options)
	}
	form.Set("first.0", "set-gone")
	form.Set("action", "")
	if _, _, body = levelEditor(t, form); !strings.Contains(body, "no longer saved here") {
		t.Errorf("requirements no longer saved are reported")
	}

	if rec := postForm(t, "/requirements/level-editor", url.Values{"level": {"not json"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("a level that isn't JSON: status %d", rec.Code)
	}
}

func TestRequirementsPageListsLevels(t *testing.T) {
	body := getPage(t, "/requirements")
	for _, want := range []string{"Built-in levels", "Your levels", `duplicateBuiltinLevel(&#39;builtin-level:bucs-l3&#39;)`, "First exercise: BUCS L3 · option 1 or BUCS L3 · option 2", "Both exercises: BG Club L1", `id="level-data"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the requirements page is missing %q", want)
		}
	}
	body = getPage(t, "/")
	for _, want := range []string{`<optgroup label="BUCS student championships (2026)">`, `<option value="builtin-level:bucs-l3">BUCS L3</option>`, `id="level-data"`, "setMode('levels')", "startVoluntaryFrom(&#39;a&#39;)", "linkVoluntary(&#39;b&#39;, $event.target.value)"} {
		if !strings.Contains(body, want) {
			t.Errorf("the calculator is missing %q", want)
		}
	}
}

func getPage(t *testing.T, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, rec.Code)
	}
	return rec.Body.String()
}
