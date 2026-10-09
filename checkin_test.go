package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// checkinEntry finds a gymnast's entry id in the day page's check-in lists.
func checkinEntry(t *testing.T, page, name string) string {
	t.Helper()
	m := regexp.MustCompile(`(?s)comp-checkin-name">\s*<span[^>]*>` + regexp.QuoteMeta(name) + `</span>.*?name="entry" value="([^"]+)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no check-in row for %s: %s", name, page)
	}
	return html2text(m[1])
}

func TestCheckinAndScratches(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	own := map[string]string{}
	for _, name := range []string{"Ann", "Bea", "Cal"} {
		own[name] = withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {name}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}))
	}
	redirected(t, h, admin+"/timetable/plan", url.Values{})
	redirected(t, h, admin+"/timetable/publish", url.Values{"action": {"publish"}})

	day := admin + "/day?day=0"
	page := do(t, h, http.MethodGet, day, nil).Body.String()
	if !strings.Contains(page, "Check-in · 0 of 3 here</summary>") {
		t.Errorf("nothing marked yet: %s", page)
	}
	if !strings.Contains(page, ">Here</button>") || !strings.Contains(page, ">Scratched</button>") || strings.Contains(page, ">Clear</button>") {
		t.Error("Here and Scratched are offered, Clear isn't until something is marked")
	}
	ann, bea := checkinEntry(t, page, "Ann"), checkinEntry(t, page, "Bea")
	mark := func(link, entry, status string) string {
		t.Helper()
		return redirected(t, h, link+"/day/checkin", url.Values{"entry": {entry}, "status": {status}, "day": {"0"}})
	}

	if to := mark(admin, ann, "here"); !strings.HasPrefix(to, day) || strings.Contains(to, "notice=") {
		t.Errorf("back to the day: %s", to)
	}
	mark(admin, bea, "scratched")
	page = do(t, h, http.MethodGet, day, nil).Body.String()
	if !strings.Contains(page, "Check-in · 1 of 3 here, 1 scratched</summary>") {
		t.Errorf("the counts: %s", page)
	}
	if !strings.Contains(page, `<span class="comp-scratched">Bea</span>`) || strings.Contains(page, `<span class="comp-scratched">Ann</span>`) ||
		!strings.Contains(page, `<span class="tag is-success is-light">Here</span>`) || !strings.Contains(page, `<span class="tag is-danger is-light">Scratched</span>`) {
		t.Errorf("each gymnast's state: %s", page)
	}
	if !strings.Contains(page, ">Clear</button>") {
		t.Error("Clear is offered once something is marked")
	}
	// Doing it again does nothing, and says so.
	if to := mark(admin, bea, "scratched"); !strings.Contains(to, "notice=") {
		t.Errorf("already scratched: %s", to)
	}
	if to := mark(admin, checkinEntry(t, page, "Cal"), ""); !strings.Contains(to, "notice=") {
		t.Errorf("nothing to clear: %s", to)
	}

	// Scratched gymnasts are struck through on the printed sheets, and no one else.
	for _, sheet := range []string{"scores", "marshal", "judges"} {
		print := do(t, h, http.MethodGet, admin+"/timetable/print?sheet="+sheet, nil).Body.String()
		if !strings.Contains(print, `<s class="comp-scratched">Bea</s> (scratched)`) || strings.Contains(print, `<s class="comp-scratched">Ann</s>`) || strings.Contains(print, `<s class="comp-scratched">Cal</s>`) {
			t.Errorf("the %s sheet strikes out Bea only: %s", sheet, print)
		}
	}
	// They see it on their own page.
	if mine := do(t, h, http.MethodGet, own["Bea"]+"/day", nil).Body.String(); !strings.Contains(mine, "Scratched · ") {
		t.Errorf("Bea's page says she's scratched: %s", mine)
	}
	if mine := do(t, h, http.MethodGet, own["Ann"]+"/day", nil).Body.String(); strings.Contains(mine, "Scratched") {
		t.Error("Ann's page doesn't")
	}

	// Clear takes it back.
	mark(admin, bea, "")
	page = do(t, h, http.MethodGet, day, nil).Body.String()
	if !strings.Contains(page, "Check-in · 1 of 3 here</summary>") || strings.Contains(page, "comp-scratched") {
		t.Errorf("cleared: %s", page)
	}
	if print := do(t, h, http.MethodGet, admin+"/timetable/print?sheet=scores", nil).Body.String(); strings.Contains(print, "(scratched)") {
		t.Error("no one struck through once cleared")
	}
	mark(admin, bea, "scratched")

	// Not a mark, or not a gymnast in a flight.
	if rec := do(t, h, http.MethodPost, admin+"/day/checkin", url.Values{"entry": {ann}, "status": {"late"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("not a status: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, admin+"/day/checkin", url.Values{"entry": {"nobody"}, "status": {"here"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("not an entry: %d", rec.Code)
	}

	// The history says what was done, in words, once each.
	history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String()
	for _, want := range []string{"Checked in: Ann (BUCS L3", "Scratched: Bea (BUCS L3", "Cleared check-in: Bea"} {
		if !strings.Contains(history, want) {
			t.Errorf("the history has %q", want)
		}
	}
	if n := strings.Count(history, "Scratched: Bea"); n != 2 {
		t.Errorf("the scratch that did nothing isn't recorded: %d", n)
	}
	if strings.Contains(history, "Cleared check-in: Cal") {
		t.Error("a clear that did nothing isn't recorded")
	}

	// Who can: chairs and the timetable link, not the cards link.
	chair := madeLink(t, h, admin, "Chris", "chair")
	mark(chair, checkinEntry(t, page, "Cal"), "here")
	if history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String(); !strings.Contains(history, "Checked in: Cal") || !strings.Contains(history, "Chris") {
		t.Error("the history says the chair checked Cal in")
	}
	if page := do(t, h, http.MethodGet, chair+"/day?day=0", nil).Body.String(); !strings.Contains(page, "Check-in · 2 of 3 here, 1 scratched</summary>") {
		t.Errorf("a chair sees the check-in: %s", page)
	}
	timetable := madeLink(t, h, admin, "Tina", "timetable")
	mark(timetable, ann, "scratched")
	cards := madeLink(t, h, admin, "Carl", "cards")
	if rec := do(t, h, http.MethodPost, cards+"/day/checkin", url.Values{"entry": {ann}, "status": {"here"}}); rec.Code != http.StatusForbidden {
		t.Errorf("a cards link can't check anyone in: %d", rec.Code)
	}
}

// A scratched gymnast who officiates that day is called out on the day page,
// with a link to the "what if they leave" tool for those who can use it.
func TestScratchedOfficial(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin := created(t, h, form)
	officials := admin + "/officials"
	redirected(t, h, officials+"/settings", url.Values{
		"panel-trampoline-chair": {"1"}, "panel-trampoline-execution": {"2"}, "panel-trampoline-difficulty": {"0"}, "panel-trampoline-recorder": {"0"}, "panel-trampoline-marshal": {"0"},
		"panel-tumbling-chair": {"1"}, "panel-tumbling-execution": {"1"}, "panel-tumbling-difficulty": {"0"}, "panel-tumbling-recorder": {"0"}, "panel-tumbling-marshal": {"0"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Mary"}, "judge-trampoline": {"1"}, "chair-trampoline": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Tom"}, "judge-trampoline": {"1"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Ann"}, "judge-tumbling": {"1"}, "chair-tumbling": {"1"}})
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	redirected(t, h, enter, url.Values{"gymnast": {"Eve"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	// Dara competes in tumbling and judges trampoline: tumbling goes first
	// (as in TestOfficialsRota, where she is seated as an execution judge).
	dara := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"Dara"}, "discipline": {"tumbling"}, "level": {"Novice"}}))
	redirected(t, h, dara+"/offer", url.Values{"judge-trampoline": {"1"}})
	tt := admin + "/timetable"
	redirected(t, h, tt+"/setup/rules", url.Values{"add": {"1"}, "kind": {"before"}, "must": {"1"}, "event": {"Tumbling Novice"}, "event2": {"BUCS L3"}})
	page := do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()
	if !strings.Contains(page, "Execution judge: Dara") {
		t.Fatalf("Dara has a seat to be scratched from: %s", page)
	}
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}})

	day := admin + "/day?day=0"
	page = do(t, h, http.MethodGet, day, nil).Body.String()
	if strings.Contains(page, "is scratched but officiates") {
		t.Error("no warning before anyone is scratched")
	}
	entry := checkinEntry(t, page, "Dara")
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {entry}, "status": {"scratched"}, "day": {"0"}})
	page = do(t, h, http.MethodGet, day, nil).Body.String()
	if !strings.Contains(page, "Dara is scratched but officiates:") || !strings.Contains(page, "BUCS L3 · Execution judge") {
		t.Fatalf("the warning names the seat: %s", page)
	}
	leave := regexp.MustCompile(`href="([^"]*timetable/leave\?[^"]*)">What if they leave`).FindStringSubmatch(page)
	if leave == nil {
		t.Fatalf("a link to the what-if tool: %s", page)
	}
	if to := html2text(leave[1]); !strings.HasPrefix(to, tt+"/leave?") || !strings.Contains(to, "person=i%3Adara") || !strings.Contains(to, "day=0") {
		t.Errorf("the link is for Dara on that day: %s", to)
	} else if rec := do(t, h, http.MethodGet, to, nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Dara") {
		t.Errorf("the what-if page opens for her: %d", rec.Code)
	}

	// A chair sees the warning but isn't sent to a page they can't use.
	chair := madeLink(t, h, admin, "Chris", "chair")
	page = do(t, h, http.MethodGet, chair+"/day?day=0", nil).Body.String()
	if !strings.Contains(page, "Dara is scratched but officiates:") || strings.Contains(page, "What if they leave") {
		t.Errorf("a chair sees the warning, without the link: %s", page)
	}
	// Here, or cleared, there's no warning.
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {entry}, "status": {"here"}, "day": {"0"}})
	if page := do(t, h, http.MethodGet, day, nil).Body.String(); strings.Contains(page, "is scratched but officiates") {
		t.Error("no warning once she's here")
	}
}
