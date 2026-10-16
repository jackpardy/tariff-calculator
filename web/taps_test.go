package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// sentAtOnce, as how long a tap waited, is one sent as it was made, without
// the times a kept one carries.
const sentAtOnce time.Duration = -1

// kept is a tap as a phone without signal sends it later: the form, with
// the phone's clock when it was tapped and when it was sent, waited apart.
// The phone's clock is years out, as the server mustn't trust it.
func kept(form url.Values, waited time.Duration) url.Values {
	phone := time.Date(2001, time.January, 1, 3, 0, 0, 0, time.UTC).UnixMilli()
	out := url.Values{}
	for k, v := range form {
		out[k] = v
	}
	out.Set("tapped", strconv.FormatInt(phone, 10))
	out.Set("sent", strconv.FormatInt(phone+waited.Milliseconds(), 10))
	return out
}

// A Started or Finished kept on the phone is recorded when it was tapped,
// by the server's clock; the earliest Started and Finished win, a Finished
// before its Started is refused, as are times in the future and taps over
// 12 hours old; and a tap sent again changes nothing.
func TestFlightTapsKeptOnThePhone(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	admin := created(t, h, newCompetition())
	publishedWith(t, h, admin, "Ann", "Bea", "Cal")
	day := admin + "/day?day=0"
	flight := firstFlightKey(t, h, day)
	mark := func(what string, waited time.Duration) string {
		t.Helper()
		form := url.Values{"flight": {flight}, "what": {what}, "day": {"0"}}
		if waited != sentAtOnce {
			form = kept(form, waited)
		}
		return redirected(t, h, admin+"/day/flight", form)
	}
	times := func() (string, string) {
		t.Helper()
		f := panelOne(t, h, day).Flights[0]
		return f.Started, f.Finished
	}

	clock.set(onTheDay(10, 0))
	if to := mark("start", 10*time.Minute); noticeOf(t, to) != "" {
		t.Errorf("a kept Started is recorded: %s", to)
	}
	area := panelOne(t, h, day)
	if f := area.Flights[0]; f.Started != "09:50" || area.Status != "Running 50 min late" || area.Since != "measured from BUCS L3 started at 09:50" {
		t.Errorf("started when tapped, 09:50, and lateness follows: %+v", area)
	}
	if !historyHas(t, h, admin, "Organiser", "Marked a flight started: BUCS L3 (Panel 1) (tapped at 09:50)") {
		t.Error("the history says when it was tapped")
	}

	// Sent again (its answer lost): nothing changes, and no error page.
	if to := mark("start", 10*time.Minute); !strings.Contains(noticeOf(t, to), "already done that") {
		t.Errorf("the same tap again: %s", to)
	}
	// Another phone's later Started changes nothing; an earlier one wins.
	mark("start", 5*time.Minute)
	if s, _ := times(); s != "09:50" {
		t.Errorf("a later Started doesn't move it: %s", s)
	}
	mark("start", 15*time.Minute)
	if s, _ := times(); s != "09:45" {
		t.Errorf("the earliest Started wins: %s", s)
	}

	// A Finished before the start is refused.
	if to := mark("finish", 20*time.Minute); !strings.Contains(noticeOf(t, to), "earlier than the start (09:45)") {
		t.Errorf("a Finished before its Started: %s", to)
	}
	// Refused: in the future (the phone's clock went back), over 12 hours
	// old, or a time that can't be read.
	if to := mark("finish", -time.Minute); !strings.Contains(noticeOf(t, to), "in the future") {
		t.Errorf("a tap in the future: %s", to)
	}
	if to := mark("finish", 13*time.Hour); !strings.Contains(noticeOf(t, to), "more than 12 hours old") {
		t.Errorf("a tap 13 hours old: %s", to)
	}
	bad := url.Values{"flight": {flight}, "what": {"finish"}, "day": {"0"}, "tapped": {"soon"}, "sent": {"1"}}
	if to := redirected(t, h, admin+"/day/flight", bad); !strings.Contains(noticeOf(t, to), "couldn't be read") {
		t.Errorf("a time that can't be read: %s", to)
	}
	if _, f := times(); f != "" {
		t.Errorf("none of those finished it: %s", f)
	}

	// Sent at once, a tap is now, as ever; then an earlier kept Finished wins.
	mark("finish", sentAtOnce)
	if _, f := times(); f != "10:00" {
		t.Errorf("a tap sent at once is now: %s", f)
	}
	mark("finish", 2*time.Minute)
	if _, f := times(); f != "09:58" {
		t.Errorf("the earliest Finished wins: %s", f)
	}
	if to := mark("finish", sentAtOnce); !strings.Contains(noticeOf(t, to), "already done that") {
		t.Errorf("finished again at once: %s", to)
	}
}

// A check-in kept on the phone goes by when it was tapped: the latest tap
// wins, so a kept one older than the gymnast's last mark changes nothing,
// and says so. Officials' check-ins go the same way.
func TestCheckinTapsKeptOnThePhone(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	admin, _, _, _ := scratchSetupOn(t, h, twoFlights)
	day := admin + "/day?day=0"
	page := dayAll(t, h, day)
	eve := checkinEntry(t, page, "Eve")
	checkin := func(status string, waited time.Duration) string {
		t.Helper()
		form := url.Values{"entry": {eve}, "status": {status}, "day": {"0"}}
		if waited != sentAtOnce {
			form = kept(form, waited)
		}
		return redirected(t, h, admin+"/day/checkin", form)
	}
	state := func() string {
		t.Helper()
		for _, a := range dayPageAreas(t, dayAll(t, h, day)) {
			for _, f := range a.Flights {
				if s, ok := f.Gymnasts["Eve"]; ok {
					return s
				}
			}
		}
		t.Fatal("Eve isn't on the day page")
		return ""
	}

	clock.set(onTheDay(10, 0))
	checkin("here", sentAtOnce)
	if to := checkin("scratched", 30*time.Minute); !strings.Contains(noticeOf(t, to), "marked again later, at 10:00") || state() != "here" {
		t.Errorf("a Scratched tapped at 09:30 doesn't undo Here at 10:00: %s, %s", to, state())
	}
	clock.set(onTheDay(10, 10))
	if to := checkin("scratched", 5*time.Minute); noticeOf(t, to) != "" || state() != "scratched" {
		t.Errorf("a Scratched tapped at 10:05 wins: %s, %s", to, state())
	}
	if !historyHas(t, h, admin, "Organiser", "Scratched: Eve (BUCS L3") || !strings.Contains(do(t, h, http.MethodGet, admin+"/history", nil).Body.String(), "(tapped at 10:05)") {
		t.Error("the history says when it was tapped")
	}
	if to := checkin("scratched", 5*time.Minute); !strings.Contains(noticeOf(t, to), "already done that") {
		t.Errorf("sent again: %s", to)
	}

	dara, flight := panelSeat(t, page, "BUCS L3 · flight 1 of 2", "Dara")
	official := func(status string, waited time.Duration) string {
		t.Helper()
		form := url.Values{"person": {dara}, "flight": {flight}, "status": {status}, "day": {"0"}}
		if waited != sentAtOnce {
			form = kept(form, waited)
		}
		return redirected(t, h, admin+"/day/officials", form)
	}
	official("missing", sentAtOnce)
	if to := official("here", 20*time.Minute); !strings.Contains(noticeOf(t, to), "marked again later, at 10:10") {
		t.Errorf("an official's Here tapped before Missing changes nothing: %s", to)
	}
	clock.set(onTheDay(10, 20))
	if to := official("here", 5*time.Minute); noticeOf(t, to) != "" || !historyHas(t, h, admin, "Organiser", "Checked in official: Dara") {
		t.Errorf("an official's later Here wins: %s", to)
	}
	if to := official("here", 13*time.Hour); !strings.Contains(noticeOf(t, to), "more than 12 hours old") {
		t.Errorf("too old: %s", to)
	}
}

// Who's up taps kept on the phone carry the gymnast and routine the page
// showed, so one sent again, or after the flight moved on, changes nothing.
func TestUpTapsKeptOnThePhone(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	admin := created(t, h, newCompetition())
	publishedWith(t, h, admin, "Ann", "Bea", "Cal")
	day := admin + "/day?day=0"
	flight := firstFlightKey(t, h, day)
	clock.set(onTheDay(9, 30))
	redirected(t, h, admin+"/day/flight", kept(url.Values{"flight": {flight}, "what": {"start"}, "day": {"0"}}, 25*time.Minute))
	first := url.Values{"flight": {flight}, "what": {"next"}, "day": {"0"}} // before the first tap: no gymnast
	if to := redirected(t, h, admin+"/day/up", kept(first, 20*time.Minute)); noticeOf(t, to) != "" {
		t.Errorf("the first Next gymnast: %s", to)
	}
	on := panelOne(t, h, day).Flights[0].Up
	if to := redirected(t, h, admin+"/day/up", kept(first, 20*time.Minute)); noticeOf(t, to) != "" || panelOne(t, h, day).Flights[0].Up != on {
		t.Errorf("sent again, it changes nothing and says nothing: %s", to)
	}
	if to := redirected(t, h, admin+"/day/up", kept(first, 13*time.Hour)); !strings.Contains(noticeOf(t, to), "more than 12 hours old") {
		t.Errorf("too old: %s", to)
	}
}

// The organiser, and helpers who can mark flights, can put a flight's start
// and finish right afterwards, as clock times on its day; the history says
// who did, and lateness follows the times put right.
func TestCorrectFlightTimes(t *testing.T) {
	h, clock := competitionServerAt(t, dayBefore)
	admin := created(t, h, newCompetition())
	publishedWith(t, h, admin, "Ann", "Bea", "Cal")
	day := admin + "/day?day=0"
	flight := firstFlightKey(t, h, day)
	correct := func(link, started, finished string) string {
		t.Helper()
		return redirected(t, h, link+"/day/times", url.Values{"flight": {flight}, "started": {started}, "finished": {finished}, "day": {"0"}})
	}
	if page := dayAll(t, h, day); !strings.Contains(page, `action="`+admin+`/day/times"`) || !strings.Contains(page, ">Correct the times</summary>") {
		t.Error("the organiser can correct the times")
	}

	clock.set(onTheDay(12, 0))
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {"start"}, "day": {"0"}})
	if to := correct(admin, "09:10", "09:40"); noticeOf(t, to) != "" || !strings.HasSuffix(to, "#"+flightID(flight)) {
		t.Errorf("corrected, back to the flight: %s", to)
	}
	area := panelOne(t, h, day)
	if f := area.Flights[0]; f.Started != "09:10" || f.Finished != "09:40" || area.Status != "Running 15 min late" || area.Since != "measured from BUCS L3 finished at 09:40" {
		t.Errorf("the times put right, and lateness follows them: %+v", area)
	}
	if !historyHas(t, h, admin, "Organiser", "Corrected a flight's times: BUCS L3 (Panel 1): started 09:10, finished 09:40") {
		t.Error("the history says who corrected the times")
	}

	// The same again changes nothing, and isn't recorded again.
	correct(admin, "09:10", "09:40")
	if n := strings.Count(do(t, h, http.MethodGet, admin+"/history", nil).Body.String(), "Corrected a flight&#39;s times"); n != 1 {
		t.Errorf("recorded once: %d", n)
	}

	// Refused, plainly, changing nothing.
	for _, c := range []struct{ started, finished, why string }{
		{"09:40", "09:10", "before the started time"},
		{"", "09:40", "Give a started time too"},
		{"09:10", "12:30", "can't be later than now"},
		{"9h", "", "as a clock time"},
	} {
		if to := correct(admin, c.started, c.finished); !strings.Contains(noticeOf(t, to), c.why) {
			t.Errorf("%s–%s: %s", c.started, c.finished, to)
		}
	}
	if f := panelOne(t, h, day).Flights[0]; f.Started != "09:10" || f.Finished != "09:40" {
		t.Errorf("unchanged: %+v", f)
	}

	// A chair can; a card checker can't. Clearing both clears them.
	chair := madeLink(t, h, admin, "Chris", "chair")
	correct(chair, "09:05", "")
	if f := panelOne(t, h, day).Flights[0]; f.Started != "09:05" || f.Finished != "" || !historyHas(t, h, admin, "Chris", "Corrected a flight's times: BUCS L3 (Panel 1): started 09:05, not finished") {
		t.Errorf("a chair corrects the times: %+v", f)
	}
	cards := madeLink(t, h, admin, "Cara", "cards")
	if rec := do(t, h, http.MethodPost, cards+"/day/times", url.Values{"flight": {flight}, "started": {"09:00"}}); rec.Code != http.StatusForbidden {
		t.Errorf("a card checker can't: %d", rec.Code)
	}
	correct(admin, "", "")
	if area := panelOne(t, h, day); area.Flights[0].Started != "" || area.Status != "" {
		t.Errorf("cleared: %+v", area)
	}
	if rec := do(t, h, http.MethodPost, admin+"/day/times", url.Values{"flight": {"9|Nowhere|X||1"}, "started": {"09:00"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("not a flight: %d", rec.Code)
	}
}

// The queue of taps on the phone keeps them in order, stops without a
// signal, drops one the server answers or keeps failing on, and goes
// through who's up as the server does, run in node against a fake phone
// (testdata/tapstest.mjs). It skips without node.
func TestTapQueue(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node to run the tap queue in")
	}
	out, err := exec.Command(node, "testdata/tapstest.mjs", "../static/js/taps.js").CombinedOutput()
	if err != nil {
		t.Errorf("%v\n%s", err, out)
	}
}

// In a real browser, taps made without signal are kept on the phone, shown
// as waiting with Finished and who's up ready to tap, and sent when the
// signal is back, at the times they were tapped.
func TestTapsWithoutSignalInABrowser(t *testing.T) {
	ctx := browser(t)
	h, clock := competitionServerAt(t, dayBefore)
	admin := created(t, h, newCompetition())
	publishedWith(t, h, admin, "Ann", "Bea", "Cal")
	clock.set(onTheDay(10, 0))
	srv := httptest.NewServer(h)
	defer srv.Close()
	day := admin + "/day?day=0"

	var waiting, next, onNow string
	var finished bool
	step(t, ctx, "opening Panel 1's page and losing the signal",
		chromedp.Navigate(srv.URL+day+"&area=Panel+1"),
		chromedp.WaitVisible(`#day-areas`, chromedp.ByQuery),
		// The phone's clock, stood still so the test says how long the taps wait.
		chromedp.Evaluate(`window.phoneNow = Date.now(); Date.now = () => window.phoneNow; true`, nil),
		network.Enable(),
		network.EmulateNetworkConditions(true, 0, -1, -1),
	)
	step(t, ctx, "tapping Started and Next gymnast without signal",
		chromedp.Click(`form[data-tap^="flight:"] button[value="start"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#day-taps`, chromedp.ByQuery),
		chromedp.WaitVisible(`.comp-up button[value="next"]`, chromedp.ByQuery),
		chromedp.Evaluate(`!!document.querySelector('form[data-tap^="flight:"] button[value="finish"]:not([hidden])')`, &finished),
		chromedp.Text(`.comp-up p`, &next, chromedp.ByQuery),
		chromedp.Click(`.comp-up button[value="next"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`.comp-up + [data-tap-note], .comp-up [data-tap-note]`, chromedp.ByQuery),
		chromedp.Text(`.comp-up`, &onNow, chromedp.ByQuery),
		chromedp.Text(`#day-taps`, &waiting, chromedp.ByQuery),
	)
	if !finished {
		t.Error("Finished is offered once Started is waiting")
	}
	if !strings.HasPrefix(next, "First up") || !strings.Contains(onNow, "On now") || !strings.Contains(onNow, "Cal") {
		t.Errorf("who's up goes on without signal: %q, then %q", next, onNow)
	}
	if !strings.Contains(waiting, "Waiting to send: 2 taps") {
		t.Errorf("the count of taps waiting: %q", waiting)
	}
	if f := panelOne(t, h, day).Flights[0]; f.Started != "" {
		t.Errorf("nothing reached the server yet: %+v", f)
	}

	// Ten minutes later by the phone's clock, the signal comes back.
	step(t, ctx, "the signal coming back, and the taps sent",
		chromedp.Evaluate(`window.phoneNow += 10 * 60 * 1000; true`, nil),
		network.EmulateNetworkConditions(false, 0, -1, -1),
		chromedp.WaitNotVisible(`#day-taps`, chromedp.ByQuery),
		chromedp.WaitNotPresent(`[data-tap-note]`, chromedp.ByQuery),
	)
	f := panelOne(t, h, day).Flights[0]
	if f.Started != "09:50" || f.Up != "Cal" || f.UpLabel != "On now" {
		t.Errorf("the taps landed at the time they were tapped, 09:50: %+v", f)
	}
	if !historyHas(t, h, admin, "Organiser", "Marked a flight started: BUCS L3 (Panel 1) (tapped at 09:50)") {
		t.Error("the history says when it was tapped")
	}
	var started string
	step(t, ctx, "reading the page", chromedp.Text(`.comp-flight p:nth-of-type(3)`, &started, chromedp.ByQuery))
	if !strings.Contains(started, "09:50") {
		t.Errorf("the page shows the server's times: %q", started)
	}
}

// Back on the flight's last routine, without signal: who's up goes back,
// Next gymnast is offered again in Finished's place, and the tap is sent
// once the signal comes back (it once threw, and nothing more was sent).
func TestBackFromTheLastRoutineWithoutSignalInABrowser(t *testing.T) {
	ctx := browser(t)
	h, clock := competitionServerAt(t, dayBefore)
	admin := created(t, h, newCompetition())
	publishedWith(t, h, admin, "Ann", "Bea", "Cal")
	clock.set(onTheDay(10, 0))
	day := admin + "/day?day=0"
	flight := panelOne(t, h, day).Flights[0].Key
	redirected(t, h, admin+"/day/flight", url.Values{"flight": {flight}, "what": {"start"}, "day": {"0"}})
	for range 20 {
		if panelOne(t, h, day).Flights[0].Last {
			break
		}
		tapUp(t, h, admin, flight, "next")
	}
	if !panelOne(t, h, day).Flights[0].Last {
		t.Fatal("never reached the last routine")
	}
	last := panelOne(t, h, day).Flights[0].Up
	srv := httptest.NewServer(h)
	defer srv.Close()

	var errs []string
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*runtime.EventExceptionThrown); ok {
			errs = append(errs, e.ExceptionDetails.Error())
		}
	})
	var nextShown, finishHidden bool
	step(t, ctx, "losing the signal on the last routine",
		chromedp.Navigate(srv.URL+day+"&area=Panel+1"),
		chromedp.WaitVisible(`.comp-up-last`, chromedp.ByQuery),
		runtime.Enable(),
		network.Enable(),
		network.EmulateNetworkConditions(true, 0, -1, -1),
	)
	step(t, ctx, "tapping Back",
		chromedp.Click(`.comp-up form[data-tap^="up:"] button[value="back"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#day-taps`, chromedp.ByQuery),
		chromedp.Evaluate(`!!document.querySelector('.comp-up form[data-tap^="up:"] button[value="next"]:not([hidden])')`, &nextShown),
		chromedp.Evaluate(`!!document.querySelector('.comp-up form[data-tap^="flight:"]')?.hidden`, &finishHidden),
	)
	if len(errs) > 0 {
		t.Errorf("no errors: %v", errs)
	}
	if !nextShown || !finishHidden {
		t.Errorf("Next gymnast again in Finished's place: next %v, finish hidden %v", nextShown, finishHidden)
	}
	step(t, ctx, "the signal coming back, and Back sent",
		network.EmulateNetworkConditions(false, 0, -1, -1),
		chromedp.WaitNotVisible(`#day-taps`, chromedp.ByQuery),
	)
	if f := panelOne(t, h, day).Flights[0]; f.Up == last || f.Last {
		t.Errorf("back on the server: %+v", f)
	}
}
