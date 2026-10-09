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

// scratchSetup is a published timetable where Dara, an individual in tumbling,
// judges BUCS L3, and Finn, another, is entered in tumbling and BUCS L3. It
// returns the server, the organiser's link, the timetable's path, and Finn's
// two entry links (tumbling, then BUCS L3).
func scratchSetup(t *testing.T) (h http.Handler, admin, tt string, finn [2]string) {
	t.Helper()
	h = competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice")
	admin = created(t, h, form)
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
	// Finn enters both: tumbling, then BUCS L3 (each entry has its own link).
	finn[0] = withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"Finn"}, "discipline": {"tumbling"}, "level": {"Novice"}}))
	finn[1] = withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"Finn"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}))
	tt = admin + "/timetable"
	redirected(t, h, tt+"/setup/rules", url.Values{"add": {"1"}, "kind": {"before"}, "must": {"1"}, "event": {"Tumbling Novice"}, "event2": {"BUCS L3"}})
	page := do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()
	if !strings.Contains(page, "Execution judge: Dara") {
		t.Fatalf("Dara has a seat to be scratched from: %s", page)
	}
	redirected(t, h, tt+"/publish", url.Values{"action": {"publish"}})
	return h, admin, tt, finn
}

// A scratched gymnast who officiates that day is called out on the day page,
// with a link to the "what if they leave" tool for those who can use it.
func TestScratchedOfficial(t *testing.T) {
	h, admin, tt, _ := scratchSetup(t)
	day := admin + "/day?day=0"
	page := do(t, h, http.MethodGet, day, nil).Body.String()
	if strings.Contains(page, "is scratched") {
		t.Error("no warning before anyone is scratched")
	}
	entry := checkinEntry(t, page, "Dara")
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {entry}, "status": {"scratched"}, "day": {"0"}})
	page = do(t, h, http.MethodGet, day, nil).Body.String()
	if !strings.Contains(page, "Dara is scratched</strong>") || !strings.Contains(page, "But officiates:") || !strings.Contains(page, "BUCS L3 · Execution judge") {
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
	if !strings.Contains(page, "Dara is scratched</strong>") || !strings.Contains(page, "BUCS L3 · Execution judge") || strings.Contains(page, "What if they leave") {
		t.Errorf("a chair sees the warning, without the link: %s", page)
	}
	// Here, or cleared, there's no warning.
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {entry}, "status": {"here"}, "day": {"0"}})
	if page := do(t, h, http.MethodGet, day, nil).Body.String(); strings.Contains(page, "is scratched</strong>") {
		t.Error("no warning once she's here")
	}
}

// checkinEntryIn finds a gymnast's entry id in one flight's check-in list.
func checkinEntryIn(t *testing.T, page, flight, name string) string {
	t.Helper()
	m := regexp.MustCompile(`(?s)<strong>` + regexp.QuoteMeta(flight) + `</strong>.*?comp-checkin-name">\s*<span[^>]*>` + regexp.QuoteMeta(name) + `</span>.*?name="entry" value="([^"]+)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no check-in row for %s in %s: %s", name, flight, page)
	}
	return html2text(m[1])
}

// A scratched person's warnings, for officiating and for competing, can be
// cleared by the desk, by the person themselves, or by checking in another of
// their entries.
func TestScratchClearing(t *testing.T) {
	h, admin, _, finn := scratchSetup(t)
	day := admin + "/day?day=0"
	get := func(path string) string { return do(t, h, http.MethodGet, path, nil).Body.String() }
	scratch := func(flight, name string) {
		t.Helper()
		redirected(t, h, admin+"/day/checkin", url.Values{"entry": {checkinEntryIn(t, get(day), flight, name)}, "status": {"scratched"}, "day": {"0"}})
	}

	// Dara officiates: one list, and its button.
	scratch("Tumbling Novice", "Dara")
	page := get(day)
	if !strings.Contains(page, "Dara is scratched</strong>") || !strings.Contains(page, "Still officiating</button>") || strings.Contains(page, "Still competing") || strings.Contains(page, "But is still entered in") {
		t.Errorf("Dara officiates and has nothing else to compete in: %s", page)
	}
	// A chair clears it; the history says so, once.
	chair := madeLink(t, h, admin, "Chris", "chair")
	clear := url.Values{"person": {"i:dara"}, "kind": {"officiating"}, "day": {"0"}}
	if to := redirected(t, h, chair+"/day/clear", clear); !strings.HasPrefix(to, chair+"/day?day=0") || strings.Contains(to, "notice=") {
		t.Errorf("back to the day: %s", to)
	}
	if page := get(day); strings.Contains(page, "Dara is scratched") {
		t.Errorf("cleared, Dara isn't listed: %s", page)
	}
	if to := redirected(t, h, admin+"/day/clear", clear); !strings.Contains(to, "notice=") {
		t.Errorf("already cleared: %s", to)
	}
	history := get(admin + "/history")
	if !strings.Contains(history, "Cleared Dara&#39;s scratch warning (officiating)") || strings.Count(history, "Cleared Dara") != 1 || !strings.Contains(history, "Chris") {
		t.Errorf("the history says the chair cleared it, once: %s", history)
	}

	// Not a kind, not a person who's scratched, not a link that can.
	if rec := do(t, h, http.MethodPost, admin+"/day/clear", url.Values{"person": {"i:dara"}, "kind": {"whatever"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("not a kind: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, admin+"/day/clear", url.Values{"person": {"i:finn"}, "kind": {"competing"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("Finn isn't scratched yet: %d", rec.Code)
	}
	cards := madeLink(t, h, admin, "Carl", "cards")
	if rec := do(t, h, http.MethodPost, cards+"/day/clear", clear); rec.Code != http.StatusForbidden {
		t.Errorf("a cards link can't clear a warning: %d", rec.Code)
	}

	// Finn is scratched from tumbling but is in BUCS L3 that day: a competing
	// warning, not an officiating one.
	scratch("Tumbling Novice", "Finn")
	page = get(day)
	if !strings.Contains(page, "Finn is scratched</strong>") || !strings.Contains(page, "But is still entered in:") || !strings.Contains(page, "BUCS L3 · Panel 1 · 09:14") ||
		!strings.Contains(page, "Still competing</button>") || strings.Contains(page, "Still officiating") || strings.Contains(page, "But officiates") {
		t.Errorf("Finn is still competing: %s", page)
	}
	if m := get(finn[0] + "/day"); !strings.Contains(m, "You're scratched from Tumbling Novice. If you're still here for your other events, tell the organisers:") || !strings.Contains(m, "I'm here for the rest") {
		t.Errorf("Finn's page offers to tell the organisers: %s", m)
	}
	if m := get(finn[1] + "/day"); !strings.Contains(m, "I'm here for the rest") {
		t.Error("so does his other entry's page")
	}

	// Checking in his other entry clears it without storing anything...
	bucs := checkinEntryIn(t, page, "BUCS L3", "Finn")
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {bucs}, "status": {"here"}, "day": {"0"}})
	if page := get(day); strings.Contains(page, "Finn is scratched") {
		t.Errorf("he's here for something: %s", page)
	}
	if m := get(finn[0] + "/day"); strings.Contains(m, "here for the rest") {
		t.Error("and his page stops asking")
	}
	// ... so the warning comes back if that's taken back.
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {bucs}, "status": {""}, "day": {"0"}})
	if page := get(day); !strings.Contains(page, "Finn is scratched</strong>") {
		t.Errorf("the warning is back: %s", page)
	}

	// Finn says he's here, from his own link: it clears both kinds.
	to := redirected(t, h, finn[0]+"/here", url.Values{})
	if !strings.HasPrefix(to, finn[0]+"/day?notice=") {
		t.Errorf("back to his page: %s", to)
	}
	mine := get(to)
	if !strings.Contains(mine, "Thanks: the organisers can see you&#39;re here.") || strings.Contains(mine, "here for the rest") {
		t.Errorf("he's thanked, and not asked again: %s", mine)
	}
	if page := get(day); strings.Contains(page, "Finn is scratched") {
		t.Errorf("the desk no longer sees Finn: %s", page)
	}
}
