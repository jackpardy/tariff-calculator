package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"tariffCalculator/requirements"
)

// everyKindOfRule uses every rule type and every matcher field.
const everyKindOfRule = `{"format":1,"name":"Everything","description":"All of it","source":"The tests","rules":[
	{"type":"count","label":"A back somersault","match":{"direction":"backward","rotation":{"min":4,"max":7}},"min":1},
	{"type":"count","match":{"twist":{"min":2}},"min":2,"max":3},
	{"type":"every","match":{"shapes":["tuck","pike"],"takeoff":["feet"],"landing":["feet","seat"]}},
	{"type":"elements","min":10,"max":10},
	{"type":"difficulty","min":1.5,"max":3.5},
	{"type":"position","position":10,"match":{"tariff":{"min":0.5,"max":1.2}}},
	{"type":"sequence","sequence":[{"fig":"4 - o"},{"rotation":{"max":0},"landing":["back"]}]},
	{"type":"separate","each":[{"label":"Landing on the front","landing":["front"]},{"twist":{"min":3},"rotation":{"max":5}}]},
	{"type":"different"},
	{"type":"difficulty","cap":1.7},
	{"type":"includes","options":[[{"label":"¾ to front or back","rotation":{"min":3,"max":3},"landing":["front","back"]},{"rotation":{"min":5,"max":5}}],[{"rotation":{"min":4,"max":4},"twist":{"min":2}}]]}
]}`

// editorFor renders the editor for a set given as JSON.
func editorFor(t *testing.T, setJSON string) *html.Node {
	t.Helper()
	rec := postForm(t, "/requirements/editor", url.Values{"set": {setJSON}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	doc, err := html.Parse(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// submitted is what a browser would submit for the editor form in doc.
func submitted(doc *html.Node) url.Values {
	values := url.Values{}
	attr := func(n *html.Node, key string) (string, bool) {
		for _, a := range n.Attr {
			if a.Key == key {
				return a.Val, true
			}
		}
		return "", false
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			name, named := attr(n, "name")
			switch {
			case !named:
			case n.Data == "input":
				value, _ := attr(n, "value")
				kind, _ := attr(n, "type")
				_, checked := attr(n, "checked")
				if kind != "checkbox" || checked {
					values.Add(name, value)
				}
			case n.Data == "select":
				var options []*html.Node
				for o := range n.Descendants() {
					if o.Type == html.ElementNode && o.Data == "option" {
						options = append(options, o)
					}
				}
				chosen := options[0] // a select with nothing selected submits its first option
				for _, o := range options {
					if _, selected := attr(o, "selected"); selected {
						chosen = o
					}
				}
				value, _ := attr(chosen, "value")
				values.Add(name, value)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return values
}

// editorSet is the set the editor in doc holds (its data-set-json).
func editorSet(t *testing.T, doc *html.Node) (requirements.Set, string) {
	t.Helper()
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode || n.Data != "form" {
			continue
		}
		var setJSON, problems string
		for _, a := range n.Attr {
			switch a.Key {
			case "data-set-json":
				setJSON = a.Val
			case "data-problems":
				problems = a.Val
			}
		}
		var set requirements.Set
		if err := json.Unmarshal([]byte(setJSON), &set); err != nil {
			t.Fatalf("data-set-json: %v", err)
		}
		return set, problems
	}
	t.Fatal("no editor form")
	return requirements.Set{}, ""
}

func canonical(t *testing.T, set requirements.Set) string {
	t.Helper()
	data, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSetEditorRoundTrip(t *testing.T) {
	want, err := requirements.Parse([]byte(everyKindOfRule))
	if err != nil {
		t.Fatal(err)
	}
	doc := editorFor(t, everyKindOfRule)
	if got, problems := editorSet(t, doc); canonical(t, got) != canonical(t, want) || problems != "0" {
		t.Fatalf("editor holds %s (%s problems), want %s", canonical(t, got), problems, canonical(t, want))
	}

	// Submitting the form unchanged gives back the same set.
	rec := postForm(t, "/requirements/editor", submitted(doc))
	doc, err = html.Parse(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got, problems := editorSet(t, doc); canonical(t, got) != canonical(t, want) || problems != "0" {
		t.Errorf("after submitting the form: %s (%s problems), want %s", canonical(t, got), problems, canonical(t, want))
	}
}

// Every built-in set comes back unchanged from the editor, so duplicating one
// and saving it keeps it exactly.
func TestSetEditorKeepsBuiltins(t *testing.T) {
	for _, b := range requirements.Builtins() {
		data, err := json.Marshal(b.Set)
		if err != nil {
			t.Fatal(err)
		}
		rec := postForm(t, "/requirements/editor", submitted(editorFor(t, string(data))))
		doc, err := html.Parse(rec.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got, problems := editorSet(t, doc); canonical(t, got) != canonical(t, b.Set) || problems != "0" {
			t.Errorf("%s: after the editor: %s (%s problems), want %s", b.ID, canonical(t, got), problems, canonical(t, b.Set))
		}
	}
}

func TestSetEditorActions(t *testing.T) {
	form := submitted(editorFor(t, everyKindOfRule))
	edit := func(changes map[string]string) requirements.Set {
		t.Helper()
		values := url.Values{}
		for k, v := range form {
			values[k] = append([]string(nil), v...)
		}
		for k, v := range changes {
			values.Set(k, v)
		}
		got, _ := editorSet(t, func() *html.Node {
			doc, err := html.Parse(postForm(t, "/requirements/editor", values).Body)
			if err != nil {
				t.Fatal(err)
			}
			return doc
		}())
		return got
	}
	types := func(set requirements.Set) string {
		var names []string
		for _, r := range set.Rules {
			names = append(names, r.Type)
		}
		return strings.Join(names, " ")
	}

	cases := []struct {
		name    string
		changes map[string]string
		want    string
	}{
		{"add a rule", map[string]string{"add": "difficulty"}, "count count every elements difficulty position sequence separate different difficulty includes difficulty"},
		{"remove a rule", map[string]string{"action": "delete:0"}, "count every elements difficulty position sequence separate different difficulty includes"},
		{"move a rule up", map[string]string{"action": "up:2"}, "count every count elements difficulty position sequence separate different difficulty includes"},
		{"move a rule down", map[string]string{"action": "down:5"}, "count count every elements difficulty sequence position separate different difficulty includes"},
		{"the first rule can't move up", map[string]string{"action": "up:0"}, "count count every elements difficulty position sequence separate different difficulty includes"},
		{"an unknown action does nothing", map[string]string{"action": "explode:1"}, "count count every elements difficulty position sequence separate different difficulty includes"},
		{"an out of range rule does nothing", map[string]string{"action": "delete:99"}, "count count every elements difficulty position sequence separate different difficulty includes"},
	}
	for _, c := range cases {
		if got := types(edit(c.changes)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}

	if got := edit(map[string]string{"action": "add-element:7"}).Rules[7].Each; len(got) != 3 {
		t.Errorf("adding a requirement: %d requirements, want 3", len(got))
	}
	if got := edit(map[string]string{"action": "delete-element:7:0"}).Rules[7].Each; len(got) != 1 || got[0].Twist == nil {
		t.Errorf("removing the first requirement left %+v", got)
	}
	includes := func(action string) [][]requirements.Matcher {
		t.Helper()
		return edit(map[string]string{"action": action}).Rules[10].Options
	}
	if got := includes("add-option:10"); len(got) != 3 || len(got[2]) != 1 {
		t.Errorf("adding an option: %d options", len(got))
	}
	if got := includes("delete-option:10:0"); len(got) != 1 || got[0][0].Twist == nil {
		t.Errorf("removing the first option left %+v", got)
	}
	if got := includes("add-step:10:1"); len(got[1]) != 2 {
		t.Errorf("adding a step: option 2 has %d elements, want 2", len(got[1]))
	}
	if got := includes("delete-step:10:0:0"); len(got[0]) != 1 || got[0][0].Rotation == nil || *got[0][0].Rotation.Min != 5 {
		t.Errorf("removing the first step of option 1 left %+v", got[0])
	}
	if got := includes("delete-step:10:9:0"); len(got) != 2 {
		t.Errorf("an out of range option does nothing: %d options", len(got))
	}
	if got := edit(map[string]string{"action": "add-element:6"}).Rules[6].Sequence; len(got) != 3 {
		t.Errorf("adding an element: %d elements, want 3", len(got))
	}
	if got := edit(map[string]string{"action": "delete-element:6:0"}).Rules[6].Sequence; len(got) != 1 || got[0].Landing[0] != "back" {
		t.Errorf("removing the first element left %+v", got)
	}

	// Changing a rule's kind starts it from that kind's defaults, keeping its
	// wording and matcher.
	retyped := edit(map[string]string{"r0.type": "every"}).Rules[0]
	if retyped.Min != nil || retyped.Label != "A back somersault" || retyped.Match.Direction != "backward" {
		t.Errorf("count changed to every: %+v", retyped)
	}
	retyped = edit(map[string]string{"r2.type": "sequence"}).Rules[2]
	if len(retyped.Sequence) != 1 || retyped.Match != nil {
		t.Errorf("every changed to a set routine: %+v", retyped)
	}
	retyped = edit(map[string]string{"r3.type": "count"}).Rules[3]
	if retyped.Match == nil || retyped.Min == nil || *retyped.Min != 1 {
		t.Errorf("elements changed to count: %+v", retyped)
	}
}

func TestSetEditorReportsProblems(t *testing.T) {
	cases := map[string]url.Values{
		"a number that isn't one": {"name": {"x"}, "rules": {"1"}, "r0.type": {"elements"}, "r0.was": {"elements"}, "r0.min": {"ten"}, "r0.max": {"10"}},
		"min over max":            {"name": {"x"}, "rules": {"1"}, "r0.type": {"elements"}, "r0.was": {"elements"}, "r0.min": {"10"}, "r0.max": {"8"}},
		"no name":                 {"name": {""}, "rules": {"0"}},
	}
	for name, form := range cases {
		doc, err := html.Parse(postForm(t, "/requirements/editor", form).Body)
		if err != nil {
			t.Fatal(err)
		}
		if _, problems := editorSet(t, doc); problems == "0" {
			t.Errorf("%s: no problems reported", name)
		}
	}

	if rec := postForm(t, "/requirements/editor", url.Values{"set": {"{not json"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("unreadable JSON: status %d, want 400", rec.Code)
	}
	// An imported set with problems still opens, so they can be fixed.
	doc := editorFor(t, `{"name":"Imported","rules":[{"type":"count","match":{}}]}`)
	if set, problems := editorSet(t, doc); set.Format != requirements.Format || problems == "0" {
		t.Errorf("imported set: format %d, %s problems", set.Format, problems)
	}
}

func TestRequirementsPage(t *testing.T) {
	rec := httptest.NewRecorder()
	routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/requirements", nil))
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	for _, want := range []string{`id="builtin-sets"`, "requirementsPage()", "requirements.js", "fig-ag1-first", "FIG age groups"} {
		if !strings.Contains(body, want) {
			t.Errorf("page is missing %q", want)
		}
	}
}
