package web

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
	custom := `[{"id":"set-mine","name":"My voluntary","set_routine":false},{"id":"set-ours","name":"Our set","set_routine":true}]`

	// Opening a level works out its structure and offers each slot what fits it.
	level, problems, body := levelEditor(t, url.Values{
		"level":  {`{"format":1,"name":"Novice","first":{"options":["builtin:bucs-l5-option-1","set-ours"]},"second":{"options":["set-mine"]}}`},
		"custom": {custom},
	})
	if problems != "0" || level.Name != "Novice" || len(level.First.Options) != 2 || level.Second == nil || level.Second.Options[0] != "set-mine" {
		t.Errorf("opened level: %+v (%s problems)", level, problems)
	}
	for _, want := range []string{
		`name="structure" value="set-voluntary" checked`,
		`<option value="builtin:bucs-l5-option-2">`, `<option value="set-ours" selected>Our set</option>`, // set routine slots: set routines
		`<option value="set-mine" selected>My voluntary</option>`, `<option value="builtin:bucs-l5-second">`, // the voluntary slot: requirements
		"Option 2", "+ Add another set routine",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the editor is missing %q", want)
		}
	}
	first := body[strings.Index(body, `name="first.0"`):]
	first = first[:strings.Index(first, "</select>")]
	if strings.Contains(first, "bucs-l5-second") || strings.Contains(first, "set-mine") {
		t.Error("a set routine slot offers only set routines")
	}

	for _, tc := range []struct {
		structure string
		want      string
		first     []string
		second    []string // nil: no second exercise
	}{
		// Set routine then a voluntary: the sets stay, the voluntary moves to the second.
		{"set-voluntary", "set-voluntary", []string{"builtin:bg-club-l1", "set-ours"}, []string{"builtin:bucs-l1-second"}},
		// One voluntary: the voluntary already chosen carries over.
		{"voluntary", "voluntary", []string{"builtin:bucs-l1-second"}, nil},
		// Two voluntaries: the first has none yet, the second keeps its own.
		{"two-voluntaries", "two-voluntaries", []string{""}, []string{"builtin:bucs-l1-second"}},
		// A set routine for both: the sets stay, no second exercise.
		{"set", "set", []string{"builtin:bg-club-l1", "set-ours"}, nil},
	} {
		form := url.Values{
			"name": {"Club"}, "custom": {custom}, "structure": {tc.structure},
			"first.n": {"2"}, "first.0": {"builtin:bg-club-l1"}, "first.1": {"set-ours"},
			"second.n": {"1"}, "second.0": {"builtin:bucs-l1-second"},
		}
		level, _, body := levelEditor(t, form)
		if !strings.Contains(body, `name="structure" value="`+tc.want+`" checked`) {
			t.Errorf("%s: not shown as chosen", tc.structure)
		}
		if strings.Join(level.First.Options, ",") != strings.Join(tc.first, ",") {
			t.Errorf("%s: first exercise %v, want %v", tc.structure, level.First.Options, tc.first)
		}
		if (level.Second == nil) != (tc.second == nil) || level.Second != nil && strings.Join(level.Second.Options, ",") != strings.Join(tc.second, ",") {
			t.Errorf("%s: second exercise %+v, want %v", tc.structure, level.Second, tc.second)
		}
	}

	// Adding and removing set routine options.
	form := url.Values{"name": {"Club"}, "custom": {custom}, "structure": {"set"}, "first.n": {"1"}, "first.0": {"builtin:bg-club-l1"}, "action": {"add:first"}}
	if level, _, _ := levelEditor(t, form); len(level.First.Options) != 2 {
		t.Errorf("add:first: %v", level.First.Options)
	}
	form.Set("first.0", "set-gone")
	form.Set("action", "")
	if _, _, body := levelEditor(t, form); !strings.Contains(body, "the first exercise has a choice still to make") {
		t.Errorf("a set routine no longer saved can't be kept in a set routine slot")
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
