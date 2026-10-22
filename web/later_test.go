package web

import (
	"net/http"
	"strings"
	"testing"

	"tariffCalculator/views"
)

// Settings is the organiser's last section, the competition's settings and
// its history; feedback about the app isn't there, it goes to the
// maintainer (feedback.go).
func TestSettingsInTheMenu(t *testing.T) {
	settings := orgMenu("/x", views.Access{Settings: true}, menuState{}, menuEntries)
	if last := settings[len(settings)-1]; last.Label != "Settings" || len(last.Items) != 2 || last.Badge != "" {
		t.Errorf("Settings: %+v", last)
	}
	for _, s := range orgMenu("/x", views.Access{}, menuState{}, menuEntries) {
		if s.Label == "Settings" || s.Label == "Feedback" {
			t.Errorf("a link without the settings has no %s", s.Label)
		}
	}
}

// A link that leads nowhere says so, with a way on, and marks itself for the
// browser to forget.
func TestDeadLinkPage(t *testing.T) {
	h := competitionServer(t)
	rec := do(t, h, http.MethodGet, "/competitions/admin/nothingHere", nil)
	page := rec.Body.String()
	if rec.Code != http.StatusNotFound || !strings.Contains(page, "data-dead-link") || !strings.Contains(page, `href="/competitions"`) {
		t.Errorf("a dead link: %d %s", rec.Code, page)
	}
}
