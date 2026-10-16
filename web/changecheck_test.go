package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// A manual change that breaks something asks first, offering fixes; one
// that breaks nothing is made at once.
func TestManualChangeAsksFirst(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, bucsEntry("Ann"))
	tt := admin + "/timetable"
	redirected(t, h, tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "add": {"1"}})
	redirected(t, h, tt+"/setup/areas", url.Values{"area-0-name": {"Panel 1"}, "area-0-discipline": {"trampoline"}, "area-1-name": {"Panel 2"}, "area-1-discipline": {"trampoline"}})
	redirected(t, h, tt+"/setup/rules", url.Values{"add": {"1"}, "kind": {"area"}, "event": {"BUCS L3"}, "area": {"Panel 1"}})
	page := do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()
	flight := regexp.MustCompile(`name="flight" value="(\d+)"`).FindStringSubmatch(page)[1]

	// To Panel 2 breaks the (prefer) rule: asked. Where it is now, Panel 1,
	// isn't offered as a fix: that's no move.
	move := url.Values{"flight": {flight}, "action": {"move"}, "to": {"0:Panel 2"}}
	rec := do(t, h, http.MethodPost, tt+"/flight", move)
	ask := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(ask, "Before you change it") || !strings.Contains(ask, "Move BUCS L3 to") ||
		!strings.Contains(ask, "Breaks the rule: BUCS L3 on Panel 1 (prefer)") || strings.Contains(ask, `value="0:Panel 1"`) {
		t.Fatalf("asked first, not offering where it is: %d %s", rec.Code, ask)
	}
	if strings.Contains(do(t, h, http.MethodGet, tt, nil).Body.String(), "Changes not published") {
		t.Error("nothing changed yet")
	}
	// Made anyway.
	move.Set("anyway", "1")
	if to := redirected(t, h, tt+"/flight", move); !strings.Contains(noticeOf(t, to), "Moved BUCS L3") {
		t.Errorf("made anyway: %s", to)
	}
	// Back to Panel 1 mends it: no question.
	if to := redirected(t, h, tt+"/flight", url.Values{"flight": {flight}, "action": {"move"}, "to": {"0:Panel 1"}}); !strings.Contains(noticeOf(t, to), "Moved BUCS L3") {
		t.Errorf("made at once: %s", to)
	}
}
