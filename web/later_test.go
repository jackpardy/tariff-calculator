package web

import (
	"net/http"
	"strings"
	"testing"

	"tariffCalculator/views"
)

// Feedback is under Settings, the tab saying how much is new; a link that
// reads feedback without the settings has a Feedback tab, not "History".
func TestFeedbackInTheMenu(t *testing.T) {
	m := menuState{FeedbackNew: 2}
	settings := orgMenu("/x", views.Access{Settings: true, Feedback: true}, m, menuEntries)
	if last := settings[len(settings)-1]; last.Label != "Settings" || last.Badge != "2 new feedback" {
		t.Errorf("Settings, its badge naming feedback: %+v", last)
	}
	helper := orgMenu("/x", views.Access{Feedback: true}, m, menuEntries)
	if last := helper[len(helper)-1]; last.Label != "Feedback" || len(last.Items) != 1 {
		t.Errorf("a Feedback tab: %+v", last)
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
