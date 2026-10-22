package web

import (
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Judges offer execution, difficulty and chairing each up to a level of
// their own, and HD at any level; the organiser's note is beside the offer
// form; a seat's dropdown knows who can take it, and a seat given to
// someone who can't says why.
func TestJudgeLevelsByRole(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	officials := admin + "/officials"
	redirected(t, h, officials+"/settings", url.Values{"note": {"  We're short of difficulty judges.  "},
		"panel-trampoline-chair": {"0"}, "panel-trampoline-execution": {"1"}, "panel-trampoline-difficulty": {"0"}, "panel-trampoline-hd": {"1"},
		"panel-trampoline-recorder": {"0"}, "panel-trampoline-marshal": {"0"}})
	page := do(t, h, http.MethodGet, officials, nil).Body.String()
	if !strings.Contains(page, ">We&#39;re short of difficulty judges.</textarea>") || strings.Contains(page, `value="below"`) {
		t.Error("the note is kept, trimmed; the rule of judging below one's own level is gone")
	}
	// Ann judges execution up to BUCS L3 (the easier level), no difficulty,
	// and so HD at any level; Bob judges execution at any level.
	redirected(t, h, officials+"/add", url.Values{"name": {"Ann"}, "judge-trampoline": {"1"}, "execution-trampoline": {"BUCS L3"}, "difficulty-trampoline": {"no"}, "chair-trampoline": {"no"}})
	redirected(t, h, officials+"/add", url.Values{"name": {"Bob"}, "judge-trampoline": {"1"}, "execution-trampoline": {""}, "difficulty-trampoline": {"no"}, "chair-trampoline": {"no"}})
	page = do(t, h, http.MethodGet, officials, nil).Body.String()
	if !strings.Contains(page, "Judge Trampoline (execution up to BUCS L3, and HD)") {
		t.Error("Ann's offer, in words")
	}

	// The organiser's note is beside an individual's offer form.
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	own := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"Fay"}, "level": {"FIG AG3 (17–21)"}, "ex1Skills": {voluntary}, "ex2Skills": {voluntary}}))
	if form := do(t, h, http.MethodGet, own, nil).Body.String(); !strings.Contains(form, "From the organiser") || !strings.Contains(form, "We&#39;re short of difficulty judges.") ||
		!strings.Contains(form, `name="difficulty-trampoline"`) || !strings.Contains(form, "HD at any level") {
		t.Error("the offer form asks for each role's level, with the organiser's note")
	}

	// Planned: Ann may take HD at the harder level, but not execution.
	tt := admin + "/timetable"
	page = do(t, h, http.MethodGet, redirected(t, h, tt+"/plan", url.Values{}), nil).Body.String()
	ann := regexp.MustCompile(`<option value="([^"]+)" data-can="([^"]*)">Ann</option>`).FindStringSubmatch(page)
	if ann == nil {
		t.Fatal("Ann is in the seat dropdown's people")
	}
	easy, hard := seatCode("BUCS L3"), seatCode("FIG AG3 (17–21)")
	can, want := strings.Fields(ann[2]), []string{easy + ":hse", hard + ":hs", "*:hse"} // synchronisation goes with HD, but trampoline has no such seats
	slices.Sort(can)
	slices.Sort(want)
	if !slices.Equal(can, want) {
		t.Errorf("Ann can take HD at both levels, execution only at the easier: %q, want %q", can, want)
	}
	if !strings.Contains(page, `data-need="`+hard+`:e"`) || !strings.Contains(page, `data-need="`+hard+`:h"`) {
		t.Error("each seat says what it needs")
	}
	// And the flights it runs across, with who's busy for them by place in
	// the people's list (Fay competes; Ann and Bob are on the panel).
	span := regexp.MustCompile(`data-need="` + hard + `:e" data-span="([^"]+)" data-holder="`).FindStringSubmatch(page)
	busy := regexp.MustCompile(`<script id="seat-busy" type="application/json">([^<]+)</script>`).FindStringSubmatch(page)
	if span == nil || busy == nil || !strings.Contains(busy[1], `"`+span[1]+`":[`) {
		t.Errorf("who's busy for each seat's flights is on the page: %v %v", span, busy)
	}
	flight := regexp.MustCompile(`name="flight" value="(\d+)">\s*<div class="select is-small"><select name="seat" aria-label="Seat on ` + regexp.QuoteMeta("FIG AG3 (17–21)")).FindStringSubmatch(page)
	if flight == nil {
		t.Fatal("FIG AG3's seat form")
	}
	// Seats are HD then execution (competitions.Roles); the rota gave Ann HD.
	rec := do(t, h, http.MethodPost, tt+"/officials", url.Values{"action": {"seat"}, "flight": {flight[1]}, "seat": {"1"}, "person": {ann[1]}})
	ask := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(ask, "This would break 2 things that hold now:") ||
		!strings.Contains(ask, "Ann can&#39;t take the execution judge seat at FIG AG3 (17–21): their offer: Judge Trampoline (execution up to BUCS L3, and HD)") {
		t.Errorf("giving Ann the execution seat asks first, saying why: %d %s", rec.Code, ask)
	}
	if !strings.Contains(ask, "Give Ann the Execution judge seat") && !strings.Contains(ask, "Give Ann the execution judge seat") {
		t.Error("the change is named")
	}
}
