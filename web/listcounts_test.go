package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

// An entry's values for the entries page's filters are those it matches:
// choosing a value shows exactly the entries that have it, so the page's
// count of each choice is what choosing it would show.
func TestDashboardFilterValuesMatch(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	var entries []judged
	for _, club := range []string{"UCD", "individual"} {
		for _, withdrawn := range []bool{false, true} {
			for _, signed := range []bool{false, true} {
				e := judged{club: club, Entry: store.Entry{Withdrawn: withdrawn, Coach: "Jo", Entry: competitions.Entry{Category: "Women"}}}
				if signed {
					e.SignedAt, e.SignedBy, e.CheckedAt = at, "Kim", at
				}
				entries = append(entries, e)
				e.problems, e.video, e.Coach, e.Entry.Entry.Category = []string{"Too short"}, "missing", "", ""
				entries = append(entries, e)
			}
		}
	}
	choices := map[string][]string{
		"club": {"UCD", "individual", "DCU"}, "coach": {"Jo", "Kim", "none", "Al"}, "problems": {"1"},
		"checked": {"yes", "no"}, "signedoff": {"yes", "no"}, "video": {"missing", "OK"}, "category": {"Women", "Men"},
	}
	for i, j := range entries {
		values := j.filterValues()
		for key, options := range choices {
			for _, v := range options {
				if got, want := slices.Contains(values[key], v), j.matches(url.Values{key: {v}}); got != want {
					t.Errorf("entry %d: has %s=%s is %v, but choosing it shows it: %v", i, key, v, got, want)
				}
			}
		}
	}
}

// The owner's example: looking for a judge from a club for a level, a level
// nobody from the club judges says so, greyed out, and taking a filter off
// by its chip puts the list back.
func TestListFilterCountsInABrowser(t *testing.T) {
	ctx := browser(t)
	s := newTestServer(t, atTime(dayBefore))
	admin := created(t, s.h, newCompetition())
	aClub(t, s.h, admin, "UCD", true, "Ann Byrne")
	aClub(t, s.h, admin, "DCU", true, "Cara Doyle")
	// BUCS L3 is the first trampoline level, FIG AG3 the second.
	redirected(t, s.h, admin+"/officials/add", url.Values{"name": {"Seán Ó Briain"}, "club": {"UCD"}, "judge-trampoline": {"1"}, "execution-trampoline": {"BUCS L3"}, "difficulty-trampoline": {"BUCS L3"}})
	redirected(t, s.h, admin+"/officials/add", url.Values{"name": {"Dara Kane"}, "club": {"DCU"}, "judge-trampoline": {"1"}, "execution-trampoline": {"FIG AG3 (17–21)"}, "difficulty-trampoline": {"FIG AG3 (17–21)"}})
	srv := httptest.NewServer(s.h)
	defer srv.Close()

	choose := func(key, value string) chromedp.Action {
		return chromedp.Evaluate(`{const f = document.querySelector('[data-list-filter="`+key+`"]'); f.value = `+strconv.Quote(value)+`; f.dispatchEvent(new Event('change', {bubbles: true}))}`, nil)
	}
	// option is a choice's text and whether it's greyed out.
	option := func(key, value string) string {
		t.Helper()
		var got string
		step(t, ctx, "reading "+key+"="+value, chromedp.Evaluate(`{const o = [...document.querySelector('[data-list-filter="`+key+`"]').options].find((o) => o.value === `+strconv.Quote(value)+`); o.textContent + (o.disabled ? ' greyed' : '')}`, &got))
		return got
	}
	var chips, count string
	read := func(name string) {
		t.Helper()
		step(t, ctx, name,
			chromedp.Evaluate(`[...document.querySelectorAll('[data-list-chips] .comp-list-chip')].map((c) => c.textContent.trim()).join(', ')`, &chips),
			chromedp.Evaluate(`document.querySelector('[data-list-count]').textContent`, &count))
	}

	step(t, ctx, "opening People", chromedp.Navigate(srv.URL+admin+"/people"), chromedp.WaitVisible(`[data-list-search]`, chromedp.ByQuery))
	if got := option("club", "UCD"); got != "UCD (2)" {
		t.Errorf("UCD has Ann and Seán: %q", got)
	}
	step(t, ctx, "choosing UCD", choose("club", "UCD"))
	if got := option("judges", "trampoline:2"); got != "FIG AG3 (17–21) (0) greyed" {
		t.Errorf("nobody from UCD judges FIG AG3, greyed out: %q", got)
	}
	if got := option("judges", "trampoline:1"); got != "BUCS L3 (1)" {
		t.Errorf("Seán judges BUCS L3: %q", got)
	}
	if got := option("club", "DCU"); got != "DCU (2)" {
		t.Errorf("another club still counts its own: %q", got)
	}
	step(t, ctx, "choosing BUCS L3", choose("judges", "trampoline:1"))
	read("reading the chips")
	if chips != "Club: UCD, Can judge: BUCS L3" || count != "1 of 4 people" {
		t.Errorf("the chips and count: %q, %q", chips, count)
	}
	// A choice that leaves nobody stays as it is while chosen, so it can be
	// seen and taken off.
	step(t, ctx, "choosing FIG AG3", choose("judges", "trampoline:2"))
	if got := option("judges", "trampoline:2"); got != "FIG AG3 (17–21) (0)" {
		t.Errorf("the choice made isn't greyed out: %q", got)
	}
	step(t, ctx, "searching", chromedp.Evaluate(`{const s = document.querySelector('[data-list-search]'); s.value = 'dara'; s.dispatchEvent(new Event('input', {bubbles: true}))}`, nil))
	if got := option("club", "DCU"); got != "DCU (1)" {
		t.Errorf("the counts follow the search: %q", got)
	}
	step(t, ctx, "taking UCD off by its chip", chromedp.Click(`[data-list-chips] .comp-list-chip`, chromedp.ByQuery))
	read("reading the chips again")
	if chips != "Can judge: FIG AG3 (17–21)" || count != "1 of 4 people" {
		t.Errorf("UCD's chip took it off: %q, %q", chips, count)
	}
	var club string
	step(t, ctx, "reading the club", chromedp.Evaluate(`document.querySelector('[data-list-filter="club"]').value`, &club))
	if club != "" {
		t.Errorf("the club filter is off: %q", club)
	}

	// The entries page's filters are the server's: each choice's count is
	// what the server shows when it's chosen.
	step(t, ctx, "opening the entries for UCD", chromedp.Navigate(srv.URL+admin+"?club=UCD"), chromedp.WaitVisible(`[data-list-search]`, chromedp.ByQuery))
	var options []string
	step(t, ctx, "reading the choices", chromedp.Evaluate(`[...document.querySelectorAll('[data-list-filter] option')].filter((o) => o.value !== '').map((o) => o.parentElement.closest('select').name + '=' + o.value + '=' + o.textContent + (o.disabled ? '=greyed' : ''))`, &options))
	if len(options) < 6 {
		t.Fatalf("the entries' choices: %v", options)
	}
	shown := regexp.MustCompile(`data-list-count aria-live="polite">(\d+)`)
	for _, o := range options {
		parts := strings.Split(o, "=")
		q := url.Values{"club": {"UCD"}}
		q.Set(parts[0], parts[1])
		m := shown.FindStringSubmatch(do(t, s.h, http.MethodGet, admin+"?"+q.Encode(), nil).Body.String())
		if m == nil {
			t.Fatalf("no count for %s", q.Encode())
		}
		if !strings.HasSuffix(parts[2], "("+m[1]+")") || (m[1] == "0") != (len(parts) == 4) {
			t.Errorf("%s=%s says %q (greyed: %v), and the server shows %s", parts[0], parts[1], parts[2], len(parts) == 4, m[1])
		}
	}
	if got := option("checked", "yes"); got != "Checked (0) greyed" {
		t.Errorf("nothing's checked: %q", got)
	}
	read("reading the entries' chips")
	if chips != "Club: UCD" || count != "1 of 2 entries" {
		t.Errorf("the entries' chips and count: %q, %q", chips, count)
	}
	step(t, ctx, "taking UCD off the entries", chromedp.Click(`[data-list-chips] .comp-list-chip`, chromedp.ByQuery), chromedp.WaitNotPresent(`[data-list-chips] .comp-list-chip`, chromedp.ByQuery))
	read("reading the entries again")
	if chips != "" || count != "2 entries" {
		t.Errorf("the chip's link shows every entry: %q, %q", chips, count)
	}
}
