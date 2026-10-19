package views

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// builderPages are the Routine Builder's and the Requirements page's
// templates, which have a look of their own; every other template is a
// competition or club page and keeps to the shared look
// (knowledge/architecture/ui.md).
var builderPages = map[string]bool{
	"page.templ": true, "form.templ": true, "routine.templ": true, "sets.templ": true, "chooser.templ": true,
	"compare.templ": true, "view.templ": true, "sheet.templ": true, "judging.templ": true,
	"levels.templ": true, "leveleditor.templ": true, "kept.templ": true,
}

// competitionPages are the templates of the competition and club pages.
func competitionPages(t *testing.T) []string {
	t.Helper()
	all, err := filepath.Glob("*.templ")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range all {
		if !builderPages[f] {
			out = append(out, f)
		}
	}
	return out
}

// buttonRoles are the buttons' looks, by role: the main action (is-primary,
// medium for the day's big taps), an ordinary one, Change opening a form in
// place (comp-opens-button), remove or delete (red text), a quiet one
// (is-text), and a tap setting a state, in the state's colours.
var buttonRoles = map[string]bool{
	"button is-primary": true, "button is-small is-primary": true, "button is-primary is-medium": true,
	"button": true, "button is-small": true,
	"button is-small comp-opens-button":       true,
	"button is-small is-text has-text-danger": true, "button is-text has-text-danger": true,
	"button is-small is-text": true, "button is-text": true,
	"button is-small is-state-done": true, "button is-small is-state-stopped": true,
	"button is-small is-danger": true, // deleting a whole competition or club, and nothing else
}

// buttonClass is a button's classes in a template, written plainly or with
// templ.KV (whose optional class is taken as on).
var buttonClass = regexp.MustCompile(`class="(button(?: [^"]*)?)"|class=\{ "(button[^"]*)"((?:, templ\.KV\("[^"]+", [^)]+\))*) \}`)

// helpers are classes for spacing or a page's own layout, not the look.
var helpers = regexp.MustCompile(`^(m[trblxy]?|p[trblxy]?)-\d$|^comp-(list-clear|inline-button|day-open)$`)

// Every button on the competition and club pages has one of the roles' looks:
// not Bulma's green, blue or yellow for a main action, nor a full-width one.
func TestButtonsHaveARole(t *testing.T) {
	kv := regexp.MustCompile(`templ\.KV\("([^"]+)"`)
	for _, name := range competitionPages(t) {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			for _, m := range buttonClass.FindAllStringSubmatch(line, -1) {
				classes := m[1] + m[2]
				for _, k := range kv.FindAllStringSubmatch(m[3], -1) {
					classes += " " + k[1]
				}
				var look []string
				for _, c := range strings.Fields(classes) {
					if !helpers.MatchString(c) {
						look = append(look, c)
					}
				}
				if got := strings.Join(look, " "); !buttonRoles[got] {
					t.Errorf("%s:%d: a button looks like %q, not one of the roles (ui.md)", name, i+1, got)
				}
			}
		}
	}
}

// A notice after a change is views.notice (green, with ×), never a
// notification written out on its own.
func TestNoticesAreShared(t *testing.T) {
	old := regexp.MustCompile(`class="notification is-(success|info) is-light[^"]*">\{ \w+\.Notice \}`)
	for _, name := range competitionPages(t) {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if m := old.FindString(string(src)); m != "" {
			t.Errorf("%s writes its own notice: %s", name, m)
		}
	}
	got := render(t, notice("Saved."))
	if !strings.Contains(got, `class="notification is-success is-light comp-notice"`) || !strings.Contains(got, `class="delete"`) || !strings.Contains(got, "Saved.") {
		t.Errorf("a notice: %s", got)
	}
	if got := render(t, notice("")); got != "" {
		t.Errorf("no notice: %q", got)
	}
}

// A way back names where it goes: never just "← Back".
func TestBackLinksSayWhere(t *testing.T) {
	bare := regexp.MustCompile(`>← Back<|"← Back"`)
	for _, name := range competitionPages(t) {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if bare.Match(src) {
			t.Errorf("%s has a way back that doesn't say where", name)
		}
	}
}
