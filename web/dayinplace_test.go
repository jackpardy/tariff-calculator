package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// Ids made from text are safe in a page and a #fragment, and different text
// never makes the same id.
func TestIdPart(t *testing.T) {
	for in, want := range map[string]string{
		"Panel 1":            "Panel_201",
		"0|Panel 1|BUCS L3|": "0_7cPanel_201_7cBUCS_20L3_7c",
		"a-b":                "a-b",
		"a_b":                "a_5fb",
		"é":                  "_c3_a9",
	} {
		if got := idPart(in); got != want {
			t.Errorf("idPart(%q) = %q, want %q", in, got, want)
		}
	}
	if idPart("a b") == idPart("a_b") || idPart("a_20b") == idPart("a b") {
		t.Error("different text keeps different ids")
	}
}

// The day page gives its changing parts stable ids and its lists a key, so the
// page can swap them for fresh ones and open the same lists again; and each
// button sends the browser back to the flight it was in.
func TestDayPageKeepsPlace(t *testing.T) {
	h := competitionServer(t)
	admin, _, _, _ := scratchSetupOn(t, h, twoFlights)
	day := admin + "/day?day=0"
	get := func() string { return do(t, h, http.MethodGet, day, nil).Body.String() }
	page := get()

	for _, id := range []string{"day-notice", "day-warnings", "day-areas", areaID("Panel 1")} {
		if !strings.Contains(page, `id="`+id+`"`) {
			t.Errorf("the page has no %s: %s", id, page)
		}
	}
	if !strings.Contains(page, `id="area-Panel_201"`) {
		t.Error("an area's id is made from its name")
	}
	_, flight1 := panelSeat(t, page, "BUCS L3 · flight 1 of 2", "Dara")
	box := flightID(flight1)
	if !strings.Contains(page, `class="box comp-flight" id="`+box+`"`) {
		t.Errorf("the flight has a box with an id: %s", page)
	}
	for _, key := range []string{`data-key="checkin:` + flight1 + `"`, `data-key="panel:` + flight1 + `"`} {
		if !strings.Contains(page, key) {
			t.Errorf("the lists carry a key, missing %s", key)
		}
	}
	if !regexp.MustCompile(`data-key="notify:official:` + regexp.QuoteMeta(flight1) + `:[^"]+"`).MatchString(page) {
		t.Error("a Notify list carries a key too")
	}
	if strings.Contains(page, `data-key="notify:official:`+flight1+`:"`) {
		t.Error("whose key names the person")
	}
	if !strings.Contains(page, `id="day-areas" data-day-page`) {
		t.Error("the page says it is the day page, for its script")
	}

	// Each button goes back to the flight it was in (the fragment follows the notice).
	back := func(to string) {
		t.Helper()
		if !strings.HasPrefix(to, day) {
			t.Errorf("back to the day: %s", to)
		}
		if !strings.HasSuffix(to, "#"+box) {
			t.Errorf("back to the flight's box %s: %s", box, to)
		}
	}
	dara, _ := panelSeat(t, page, "BUCS L3 · flight 1 of 2", "Dara")
	official := url.Values{"person": {dara}, "flight": {flight1}, "status": {"missing"}, "day": {"0"}}
	back(redirected(t, h, admin+"/day/officials", official))
	back(redirected(t, h, admin+"/day/officials", official)) // already done: with a notice
	notify := url.Values{"person": {dara}, "flight": {flight1}, "as": {"official"}, "day": {"0"}}
	if to := redirected(t, h, admin+"/day/notify", notify); !strings.Contains(to, "&notice=") {
		t.Errorf("no one ticked: %s", to)
	} else {
		back(to)
	}
	flight := url.Values{"flight": {flight1}, "what": {"start"}, "day": {"0"}}
	back(redirected(t, h, admin+"/day/flight", flight))
	back(redirected(t, h, admin+"/day/flight", flight))

	// A gymnast's check-in goes back to the box they are in.
	eve := checkinEntry(t, page, "Eve")
	to := redirected(t, h, admin+"/day/checkin", url.Values{"entry": {eve}, "status": {"here"}, "day": {"0"}})
	id := to[strings.Index(to, "#")+1:]
	page = get()
	inBox := page[strings.Index(page, `id="`+id+`"`):]
	if next := strings.Index(inBox[1:], `class="box comp-flight"`); next > 0 {
		inBox = inBox[:next]
	}
	if !strings.HasPrefix(to, day) || !strings.Contains(inBox, ">Eve<") {
		t.Errorf("Eve's check-in goes back to her flight's box: %s", to)
	}
	if to := redirected(t, h, admin+"/day/checkin", url.Values{"entry": {eve}, "status": {"here"}, "day": {"0"}}); !strings.Contains(to, "&notice=") || !strings.HasSuffix(to, "#"+id) {
		t.Errorf("already here, with a notice: %s", to)
	}

	// Clearing a scratch warning goes back to the warnings.
	redirected(t, h, admin+"/day/checkin", url.Values{"entry": {checkinEntry(t, page, "Dara")}, "status": {"scratched"}, "day": {"0"}})
	page = get()
	m := regexp.MustCompile(`(?s)id="day-warnings">.*?name="person" value="([^"]+)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("a scratched official is warned about: %s", page)
	}
	clear := url.Values{"person": {unescape(m[1])}, "kind": {"officiating"}, "day": {"0"}}
	if to := redirected(t, h, admin+"/day/clear", clear); !strings.HasPrefix(to, day) || !strings.HasSuffix(to, "#day-warnings") {
		t.Errorf("back to the warnings: %s", to)
	}
	if to := redirected(t, h, admin+"/day/clear", clear); !strings.Contains(to, "&notice=") || !strings.HasSuffix(to, "#day-warnings") {
		t.Errorf("back to the warnings, with a notice: %s", to)
	}
}

// Tapping Here on the day page in a real browser changes the page in place:
// no reload, the list stays open and the page stays where it was scrolled to;
// the notice shows when someone got there first; and a reload keeps the list
// open.
func TestDayPageUpdatesInPlace(t *testing.T) {
	ctx := browser(t)
	h := competitionServer(t)
	admin, _, _, _ := scratchSetupOn(t, h, twoFlights)
	srv := httptest.NewServer(h)
	defer srv.Close()

	const (
		checkin = `details[data-key^="checkin:"]`
		here    = checkin + ` button[value="here"]`
	)
	var scrolled, scrolledAfter float64
	var open, sameDoc, added bool
	var notice string
	step(t, ctx, "opening the day page",
		chromedp.Navigate(srv.URL+admin+"/day?day=0"),
		chromedp.WaitVisible(`#day-areas`, chromedp.ByQuery),
		// Make the page tall, so there is somewhere to scroll to, and mark it
		// so a reload would be noticed.
		chromedp.Evaluate(`document.getElementById('day-notice').insertAdjacentHTML('beforebegin', '<div style="height:1500px"></div>'); window.sameDocument = true`, nil),
	)
	step(t, ctx, "opening the check-in list and scrolling to Here",
		chromedp.Click(checkin+` summary`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('`+here+`').scrollIntoView({block: 'center'}); window.scrollY`, &scrolled),
	)
	step(t, ctx, "tapping Here and waiting for its tag",
		chromedp.Click(here, chromedp.ByQuery),
		chromedp.WaitVisible(checkin+` .chip-state-done`, chromedp.ByQuery),
	)
	step(t, ctx, "reading the page after the tap",
		chromedp.Evaluate(`window.scrollY`, &scrolledAfter),
		chromedp.Evaluate(`document.querySelector('`+checkin+`').open`, &open),
		chromedp.Evaluate(`window.sameDocument === true`, &sameDoc),
	)
	if !open {
		t.Error("the check-in list stays open")
	}
	if !sameDoc {
		t.Error("the page was reloaded, not updated in place")
	}
	if history := do(t, h, http.MethodGet, admin+"/history", nil).Body.String(); !strings.Contains(history, "Checked in: ") {
		t.Errorf("the tap is in the history, as a plain form post is: %s", history)
	}
	if scrolledAfter != scrolled {
		t.Errorf("the page moved from %v to %v", scrolled, scrolledAfter)
	}

	// Doing the same thing again from the old button says someone has done it:
	// the page's notice shows it, and the list is still open.
	step(t, ctx, "putting back a stale Here button",
		// Put back a Here button for a gymnast who is already here, as a
		// stale page would have it.
		chromedp.Evaluate(`(() => {
			const form = document.querySelector('`+checkin+` form');
			form.insertAdjacentHTML('beforeend', '<button class="button" type="submit" name="status" value="here" id="stale">Here</button>');
			return true;
		})()`, &added),
		chromedp.Evaluate(`document.querySelector('`+checkin+`').getBoundingClientRect().top`, &scrolled),
	)
	step(t, ctx, "tapping the stale button and waiting for the notice",
		chromedp.Click(`#stale`, chromedp.ByQuery),
		chromedp.WaitVisible(`#day-notice .notification`, chromedp.ByQuery),
	)
	step(t, ctx, "reading the notice",
		chromedp.Evaluate(`document.querySelector('`+checkin+`').getBoundingClientRect().top`, &scrolledAfter),
		chromedp.Text(`#day-notice`, &notice, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('`+checkin+`').open && window.sameDocument === true`, &open),
	)
	if !strings.Contains(notice, "Someone has already done that for") {
		t.Errorf("the notice: %q", notice)
	}
	if !open {
		t.Error("the list is still open, and the page wasn't reloaded")
	}
	if scrolledAfter != scrolled {
		t.Errorf("the notice moved the flight on the screen from %v to %v", scrolled, scrolledAfter)
	}

	// A reload keeps the list open for this tab.
	step(t, ctx, "reloading the day page",
		chromedp.Reload(),
		chromedp.WaitVisible(`#day-areas`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('`+checkin+`').open`, &open),
	)
	if !open {
		t.Error("a reload keeps the list open")
	}
}
