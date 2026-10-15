package views

import (
	"bytes"
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// paletteOnly are the templates that show states and roles only with the
// chips (the cleanup audit's roll-out, step 2): On the day, the dashboard,
// the club's pages, the timetable's rota and the officials.
var paletteOnly = []string{"day.templ", "dashboard.templ", "clubs.templ", "timetable.templ", "officials.templ"}

// bulmaState is Bulma colour used to show a state: a coloured tag, or
// coloured text.
var bulmaState = regexp.MustCompile(`"tag\b.*\bis-(success|warning|danger)\b|\bhas-text-(success|warning|danger)(-dark)?\b`)

// The converted templates don't go back to Bulma's colours for a state:
// only buttons, which do something rather than show a state, keep them.
func TestStatesUseChips(t *testing.T) {
	for _, old := range []string{
		`<span class="tag is-success is-light">Here</span>`,
		`<span class={ "tag is-light ml-1", templ.KV("is-success", k.Approved), templ.KV("is-warning", !k.Approved) }>{ k.Status }</span>`,
		`<td class={ templ.KV("has-text-danger", r.Video == "missing") } data-label="Video">`,
		`<span class="is-size-7 has-text-warning-dark">{ s }</span>`,
	} {
		if !bulmaState.MatchString(old) {
			t.Errorf("not caught: %s", old)
		}
	}
	for _, name := range paletteOnly {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if bulmaState.MatchString(line) && !strings.Contains(line, "<button") {
				t.Errorf("%s:%d shows a state in Bulma's colours, not with StateChip or WhoChip: %s", name, i+1, strings.TrimSpace(line))
			}
		}
	}
}

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// Every state chip has its own colour class, a mark and the label, so it
// reads without colour; every role chip says the role.
func TestChips(t *testing.T) {
	marks := map[string]bool{}
	for _, s := range []State{NotYet, InProgress, Done, Attention, Stopped} {
		got := render(t, StateChip(s, "Changed & sent"))
		want := `<span class="chip chip-state-` + string(s) + `"><span class="chip-mark" aria-hidden="true">` + s.mark() + `</span>Changed &amp; sent</span>`
		if got != want {
			t.Errorf("%s:\n got %s\nwant %s", s, got, want)
		}
		if marks[s.mark()] {
			t.Errorf("%s shares its mark %q", s, s.mark())
		}
		marks[s.mark()] = true
	}
	for r, word := range map[Role]string{RoleGymnast: "Gymnast", RoleCoach: "Coach", RoleClub: "Club", RoleOrganiser: "Organiser"} {
		got := render(t, WhoChip(r, "Declan O'Neill"))
		if !strings.Contains(got, `class="chip chip-who chip-who-`+string(r)+`"`) || !strings.Contains(got, `<span class="chip-role">`+word+`</span>`) || !strings.Contains(got, "· Declan O&#39;Neill") {
			t.Errorf("%s: %s", r, got)
		}
		if got := render(t, WhoChip(r, "")); strings.Contains(got, "chip-name") {
			t.Errorf("%s with no name: %s", r, got)
		}
	}
}
