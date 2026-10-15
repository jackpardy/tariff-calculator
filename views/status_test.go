package views

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

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
