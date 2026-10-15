package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// The timetable carries the flights a gymnast can move to once, not once per
// gymnast, and a move with no flight chosen changes nothing.
func TestMoveOptionsOnce(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Ann", "Bea", "Cal", "Dan"} {
		redirected(t, h, enter, url.Values{"gymnast": {name}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	}
	tt := admin + "/timetable"
	redirected(t, h, tt+"/plan", url.Values{})
	page := do(t, h, http.MethodGet, tt, nil).Body.String()
	template := regexp.MustCompile(`(?s)<template id="move-options">(.*?)</template>`).FindStringSubmatch(page)
	if template == nil || !strings.Contains(template[1], "<option value=") || !strings.Contains(page, "js/moves.js") {
		t.Fatalf("the flights to move to, once, with the script that fills the dropdowns: %s", page)
	}
	selects := regexp.MustCompile(`(?s)<select name="to"[^>]*data-moves>(.*?)</select>`).FindAllStringSubmatch(page, -1)
	if len(selects) < 4 {
		t.Fatalf("a move dropdown per gymnast: %d", len(selects))
	}
	for _, s := range selects {
		if strings.Count(s[1], "<option") != 2 || !strings.Contains(s[1], `<option value="" disabled selected>Choose a flight</option>`) {
			t.Errorf("a dropdown holds only its placeholder and Take out until it's opened: %s", s[1])
		}
	}

	// Left on "Choose a flight" (nothing sent): nothing changes.
	entry := regexp.MustCompile(`name="entry" value="([^"]+)"`).FindStringSubmatch(page)[1]
	if to := redirected(t, h, tt+"/entry", url.Values{"entry": {entry}}); !strings.Contains(to, "Choose+a+flight") {
		t.Errorf("a move with no flight chosen says so: %s", to)
	}
	if after := do(t, h, http.MethodGet, tt, nil).Body.String(); strings.Contains(after, "Not in the timetable") && !strings.Contains(page, "Not in the timetable") {
		t.Error("nobody was taken out")
	}
	// Choosing "Take out of the timetable" (sent empty) still takes them out.
	redirected(t, h, tt+"/entry", url.Values{"entry": {entry}, "to": {""}})
	if after := do(t, h, http.MethodGet, tt, nil).Body.String(); after == page {
		t.Error("taking someone out changes the timetable")
	}
}
