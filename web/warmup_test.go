package web

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"tariffCalculator/views"
)

// Warm-up over (roadmap 15): after Started, the tap that comes next; the
// flight says Warming up until it's tapped, then Competing; Undo takes it
// back, and the history says who tapped it.
func TestWarmUpOverTap(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	admin := created(t, h, newCompetition())
	publishedWith(t, h, admin, "Ann", "Bea", "Cal")
	day := admin + "/day?day=0"
	flight := firstFlightKey(t, h, day)
	mark := func(what string) string {
		t.Helper()
		return redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {what}, "day": {"0"}})
	}

	clock.set(onTheDay(9, 0))
	mark("start")
	f := panelOne(t, h, day).Flights[0]
	if f.State != "Warming up" || !slices.Equal(f.Buttons[:2], []string{"Warm-up over", "Finished"}) || !slices.Contains(f.Buttons, "Undo") || f.WarmUpOver != "" {
		t.Errorf("started: warming up, with Warm-up over to tap: %+v", f)
	}
	if card := dayAll(t, h, day); !strings.Contains(card, `value="warm-up">Warm-up over</button>`) {
		t.Error("the overview's card offers Warm-up over")
	}

	clock.set(onTheDay(9, 12))
	if to := mark("warm-up"); noticeOf(t, to) != "" {
		t.Errorf("Warm-up over: %s", to)
	}
	f = panelOne(t, h, day).Flights[0]
	if f.State != "Competing" || f.WarmUpOver != "09:12" || f.Started != "09:00" || slices.Contains(f.Buttons, "Warm-up over") || !slices.Contains(f.Buttons, "Finished") {
		t.Errorf("competing from 09:12: %+v", f)
	}
	if !historyHas(t, h, admin, "Organiser", "Marked a flight's warm-up over: BUCS L3 (Panel 1)") {
		t.Error("the history says who tapped it")
	}
	if to := mark("warm-up"); !strings.Contains(noticeOf(t, to), "already done that") {
		t.Errorf("tapped again: %s", to)
	}

	// Undo takes back the latest: the warm-up, then the start.
	page := dayAll(t, h, day+"&area=Panel+1")
	if !strings.Contains(page, `value="undo-warm-up">Undo</button>`) {
		t.Error("Undo takes back Warm-up over")
	}
	mark("undo-warm-up")
	if f := panelOne(t, h, day).Flights[0]; f.WarmUpOver != "" || f.Started != "09:00" || f.State != "Warming up" || !slices.Contains(f.Buttons, "Warm-up over") {
		t.Errorf("undone, warming up again: %+v", f)
	}
	clock.set(onTheDay(9, 14))
	mark("warm-up")
	clock.set(onTheDay(9, 30))
	mark("finish")
	area := panelOne(t, h, day)
	if f := area.Flights[0]; f.State != "Finished" || f.WarmUpOver != "09:14" || f.Finished != "09:30" {
		t.Errorf("finished: %+v", f)
	}
	// Lateness is measured as ever; competing against its plan (09:10) beside it.
	if area.Status != "Running 5 min late" || area.Since != "measured from BUCS L3 finished at 09:30 · competing began 09:14, 4 min late" {
		t.Errorf("competing against the plan: %+v", area)
	}
	if to := mark("undo-warm-up"); noticeOf(t, to) != "" || panelOne(t, h, day).Flights[0].WarmUpOver != "09:14" {
		t.Errorf("a finished flight's Warm-up over stays: %s", to)
	}

	// The day's real times, beside Setup's: 14 minutes' warm-up and 16 of
	// competing, over 3 gymnasts.
	if page := dayAll(t, h, day); !strings.Contains(page, ">Real times today</summary>") ||
		!strings.Contains(page, "Trampoline: warm-up 4.7 min a competitor (set 0), competing 5.3 (set 5), from 1 flight") {
		t.Errorf("the real times: %s", page)
	}
	if page := dayAll(t, h, day+"&area=Panel+1"); strings.Contains(page, "Real times today") {
		t.Error("not on an area's page")
	}
}

// Warm-up over can be skipped: Finished, and who's up, work without it, and
// the warm-up is left unmeasured. Once who's up has reached a gymnast the
// flight is competing, and Warm-up over is no longer offered.
func TestWarmUpOverSkipped(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	admin := created(t, h, newCompetition())
	publishedWith(t, h, admin, "Ann", "Bea", "Cal")
	day := admin + "/day?day=0"
	flight := firstFlightKey(t, h, day)
	mark := func(what string) string {
		t.Helper()
		return redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {what}, "day": {"0"}})
	}

	clock.set(onTheDay(9, 0))
	mark("start")
	clock.set(onTheDay(9, 25))
	if to := mark("finish"); noticeOf(t, to) != "" {
		t.Errorf("finished without it: %s", to)
	}
	area := panelOne(t, h, day)
	if f := area.Flights[0]; f.Finished != "09:25" || f.WarmUpOver != "" || area.Since != "measured from BUCS L3 finished at 09:25" {
		t.Errorf("finished, the warm-up unmeasured: %+v", area)
	}
	if strings.Contains(dayAll(t, h, day), "Real times today") {
		t.Error("nothing measured, no real times")
	}
	mark("undo-finish")
	if f := panelOne(t, h, day).Flights[0]; !slices.Contains(f.Buttons, "Warm-up over") {
		t.Errorf("Undo, and it's offered again: %+v", f)
	}

	clock.set(onTheDay(9, 26))
	tapUp(t, h, admin, flight, "next")
	f := panelOne(t, h, day).Flights[0]
	if f.State != "Competing" || f.WarmUpOver != "" || slices.Contains(f.Buttons, "Warm-up over") || f.UpLabel != "On now" {
		t.Errorf("who's up moved on: competing, without Warm-up over: %+v", f)
	}
	if page := dayAll(t, h, day+"&area=Panel+1"); !strings.Contains(page, `value="undo-start">Undo</button>`) {
		t.Error("Undo still takes back the start")
	}
}

// Two phones without signal, the same rules as Started: the earliest
// Warm-up over counts; it's refused without a Started, before the start or
// after the finish, as a Finished before it is; and by a link that can't tap.
func TestWarmUpOverKeptAndRefused(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	admin := created(t, h, newCompetition())
	publishedWith(t, h, admin, "Ann", "Bea", "Cal")
	day := admin + "/day?day=0"
	flight := firstFlightKey(t, h, day)
	mark := func(link, what string, waited time.Duration) string {
		t.Helper()
		form := url.Values{"flight": {flight}, "what": {what}, "day": {"0"}}
		if waited != sentAtOnce {
			form = kept(form, waited)
		}
		return redirected(t, h, link+"/day/flight", form)
	}
	warmUp := func() string { t.Helper(); return panelOne(t, h, day).Flights[0].WarmUpOver }

	clock.set(onTheDay(9, 30))
	if to := mark(admin, "warm-up", sentAtOnce); !strings.Contains(noticeOf(t, to), "BUCS L3 hasn't started, so Warm-up over wasn't recorded.") || warmUp() != "" {
		t.Errorf("without a Started: %s", to)
	}
	if historyHas(t, h, admin, "Organiser", "Marked a flight's warm-up over") {
		t.Error("a refused tap isn't in the history")
	}
	mark(admin, "start", 30*time.Minute) // 09:00
	if to := mark(admin, "warm-up", 35*time.Minute); !strings.Contains(noticeOf(t, to), "earlier than the start (09:00)") {
		t.Errorf("before the start: %s", to)
	}
	if to := mark(admin, "warm-up", 20*time.Minute); noticeOf(t, to) != "" || warmUp() != "09:10" || !historyHas(t, h, admin, "Organiser", "Marked a flight's warm-up over: BUCS L3 (Panel 1) (tapped at 09:10)") {
		t.Errorf("kept, with when it was tapped: %s, %s", to, warmUp())
	}
	if to := mark(admin, "warm-up", 15*time.Minute); !strings.Contains(noticeOf(t, to), "already done that") || warmUp() != "09:10" {
		t.Errorf("another phone's later one: %s", to)
	}
	if to := mark(admin, "warm-up", 25*time.Minute); noticeOf(t, to) != "" || warmUp() != "09:05" {
		t.Errorf("the earliest counts: %s, %s", to, warmUp())
	}
	if to := mark(admin, "finish", 27*time.Minute); !strings.Contains(noticeOf(t, to), "earlier than the warm-up was over (09:05)") {
		t.Errorf("a Finished before it: %s", to)
	}
	mark(admin, "undo-warm-up", sentAtOnce)
	mark(admin, "finish", 10*time.Minute) // 09:20
	if to := mark(admin, "warm-up", 5*time.Minute); !strings.Contains(noticeOf(t, to), "later than the finish (09:20)") {
		t.Errorf("after the finish: %s", to)
	}

	// A flight that's gone, or a mark that isn't one, is refused as ever.
	if rec := do(t, h, http.MethodPost, admin+"/day/flight", url.Values{"flight": {"gone"}, "what": {"warm-up"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("a flight that's gone: %d", rec.Code)
	}

	// A chair may tap it, as Started; a card checker may not.
	mark(admin, "undo-finish", sentAtOnce)
	chair := madeLink(t, h, admin, "Chris", "chair")
	if to := mark(chair, "warm-up", sentAtOnce); noticeOf(t, to) != "" || !historyHas(t, h, admin, "Chris", "Marked a flight's warm-up over") {
		t.Errorf("a chair taps it: %s", to)
	}
	cards := madeLink(t, h, admin, "Cara", "cards")
	if rec := do(t, h, http.MethodPost, cards+"/day/flight", url.Values{"flight": {flight}, "what": {"undo-warm-up"}}); rec.Code != http.StatusForbidden {
		t.Errorf("a card checker can't: %d", rec.Code)
	}
}

// Correct the times takes Warm-up over too, optional, between the others.
func TestCorrectWarmUpOver(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	admin := created(t, h, newCompetition())
	publishedWith(t, h, admin, "Ann", "Bea", "Cal")
	day := admin + "/day?day=0"
	flight := firstFlightKey(t, h, day)
	correct := func(started, warmUp, finished string) string {
		t.Helper()
		return redirected(t, h, admin+"/day/times", url.Values{"flight": {flight}, "started": {started}, "warmUpOver": {warmUp}, "finished": {finished}, "day": {"0"}})
	}
	if page := dayAll(t, h, day); !strings.Contains(page, `name="warmUpOver"`) {
		t.Error("Correct the times has Warm-up over")
	}
	clock.set(onTheDay(12, 0))
	correct("09:00", "09:12", "09:40")
	if f := panelOne(t, h, day).Flights[0]; f.Started != "09:00" || f.WarmUpOver != "09:12" || f.Finished != "09:40" {
		t.Errorf("all three: %+v", f)
	}
	if !historyHas(t, h, admin, "Organiser", "Corrected a flight's times: BUCS L3 (Panel 1): started 09:00, warm-up over 09:12, finished 09:40") {
		t.Error("the history has all three")
	}
	correct("09:00", "", "09:40")
	if f := panelOne(t, h, day).Flights[0]; f.WarmUpOver != "" || f.Finished != "09:40" {
		t.Errorf("without it: %+v", f)
	}
	if !historyHas(t, h, admin, "Organiser", "Corrected a flight's times: BUCS L3 (Panel 1): started 09:00, finished 09:40") {
		t.Error("the history without it")
	}
	for _, c := range []struct{ started, warmUp, finished, why string }{
		{"09:10", "09:05", "", "warm-up over time is before the started time"},
		{"09:00", "09:30", "09:20", "finished time is before the warm-up over time"},
		{"", "09:10", "", "a flight's warm-up can't be over without starting"},
		{"09:00", "12:30", "", "can't be later than now"},
		{"09:00", "9h", "", "Give the warm-up over time as a clock time"},
	} {
		if to := correct(c.started, c.warmUp, c.finished); !strings.Contains(noticeOf(t, to), c.why) {
			t.Errorf("%s, %s, %s: %s", c.started, c.warmUp, c.finished, to)
		}
	}
}

// The same two words on the venue screen, a gymnast's and an official's own
// page, and the organiser's page of a person.
func TestWarmingUpAndCompetingShown(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	admin, _, finn, dara := scratchSetupOn(t, h, nil)
	day := admin + "/day?day=0"
	page := dayAll(t, h, day)
	_, flight := panelSeat(t, page, "BUCS L3", "Dara")
	finnKey := personKeyIn(t, page, admin, "Finn")
	mark := func(what string) {
		t.Helper()
		redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {what}, "day": {"0"}})
	}
	shows := func(stage string) {
		t.Helper()
		if s := dayAll(t, h, admin+"/screen?day=0"); !strings.Contains(s, "· "+stage) {
			t.Errorf("the venue screen says %s: %s", stage, s)
		}
		want := chip(t, views.InProgress, stage)
		if m := dayAll(t, h, finn[1]+"/day"); !strings.Contains(m, want) {
			t.Errorf("Finn's page says %s: %s", stage, m)
		}
		if m := dayAll(t, h, dara+"/day"); !strings.Contains(m, "On your panel now:") || !strings.Contains(m, want) {
			t.Errorf("Dara's page says her panel is %s: %s", stage, m)
		}
		if m := dayAll(t, h, admin+"/people/"+url.PathEscape(finnKey)); !strings.Contains(m, want) {
			t.Errorf("Finn's page for the organiser says %s: %s", stage, m)
		}
	}

	clock.set(onTheDay(10, 0))
	if m := dayAll(t, h, finn[1]+"/day"); strings.Contains(m, "Warming up") || strings.Contains(m, "Competing") {
		t.Error("nothing said before it starts")
	}
	mark("start")
	shows("Warming up")
	mark("warm-up")
	shows("Competing")
	mark("finish")
	if m := dayAll(t, h, dara+"/day"); strings.Contains(m, "On your panel now:") {
		t.Error("nothing once it's finished")
	}
}
