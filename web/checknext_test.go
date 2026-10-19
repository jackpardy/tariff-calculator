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

// Mark checked and next opens the next entry to check, in the same level
// first; after the last, the entries page says so. Video OK waits for a video.
func TestCheckAndNext(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Ann", "Bea"} {
		redirected(t, h, enter, bucsEntry(name))
	}
	page := do(t, h, http.MethodGet, admin, nil).Body.String()
	ids := regexp.MustCompile(`href="`+regexp.QuoteMeta(admin)+`/entries/([^"#]+)"`).FindAllStringSubmatch(page, -1)
	if len(ids) < 2 {
		t.Fatalf("two entries: %s", page)
	}
	first, second := ids[0][1], ids[1][1]
	if first == second {
		second = ids[2][1]
	}
	to := redirected(t, h, admin+"/entries/"+first+"/check", url.Values{"checked": {"1"}, "next": {"1"}})
	if !strings.Contains(to, "/entries/"+second+"?notice=") && !strings.Contains(to, "/entries/"+second+"#") && !strings.Contains(to, "/entries/"+second) {
		t.Errorf("on to the next: %s", to)
	}
	if to := redirected(t, h, admin+"/entries/"+second+"/check", url.Values{"checked": {"1"}, "next": {"1"}}); !strings.Contains(to, "last+to+check") {
		t.Errorf("after the last: %s", to)
	}
	if to := redirected(t, h, admin+"/entries/"+first+"/video", url.Values{"review": {"ok"}}); !strings.Contains(to, "no+video+yet") {
		t.Errorf("no video to say OK to: %s", to)
	}
}

// In a browser: a search typed on the entries page goes with an entry opened
// from it, and with removing; ← Entries comes back to the entry's row, its
// level opened; Sent is hidden until chosen.
func TestBackToTheEntryInABrowser(t *testing.T) {
	ctx := browser(t)
	s := newTestServer(t)
	admin := created(t, s.h, newCompetition())
	enter := pathIn(t, do(t, s.h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Ann Ryan", "Bea Kelly", "Cat Ryan"} {
		redirected(t, s.h, enter, bucsEntry(name))
	}
	cat := entryIDs(t, do(t, s.h, http.MethodGet, admin, nil).Body.String(), admin)["Cat Ryan"]
	srv := httptest.NewServer(s.h)
	defer srv.Close()

	var link, remove string
	var sent bool
	step(t, ctx, "searching the entries page",
		chromedp.Navigate(srv.URL+admin+"?club=individual"),
		chromedp.WaitReady(`#entries`, chromedp.ByQuery),
		chromedp.Evaluate(`{const s = document.querySelector('[data-list-search]'); s.value = 'cat'; s.dispatchEvent(new Event('input', {bubbles: true}))}`, nil),
		chromedp.Evaluate(`{const a = [...document.querySelectorAll('#entries td.comp-stack-title > a')].find((a) => a.textContent === 'Cat Ryan'); a.dispatchEvent(new MouseEvent('contextmenu', {bubbles: true})); a.getAttribute('href')}`, &link),
		chromedp.Evaluate(`document.getElementById('remove-form').getAttribute('action')`, &remove),
		chromedp.Evaluate(`[...document.querySelectorAll('table.comp-entries th')].find((th) => th.textContent.trim() === 'Sent').hidden`, &sent),
	)
	if link != admin+"/entries/"+cat+"?club=individual&q=cat" || remove != admin+"/remove?club=individual&q=cat" {
		t.Errorf("the search goes with the entry and removing: %s, %s", link, remove)
	}
	if !sent {
		t.Error("Sent is hidden until chosen")
	}

	var picked, open bool
	step(t, ctx, "coming back to Cat",
		chromedp.Navigate(srv.URL+admin+"#entry-"+cat),
		chromedp.WaitReady(`#entries`, chromedp.ByQuery),
		chromedp.Evaluate(`!!document.querySelector('tr.comp-picked') && document.querySelector('tr.comp-picked td.comp-stack-title a').textContent === 'Cat Ryan'`, &picked),
		chromedp.Evaluate(`document.querySelector('tr.comp-picked').closest('details').open`, &open),
	)
	if !picked || !open {
		t.Errorf("back at Cat's row (%v), her level open (%v)", picked, open)
	}
}

// entryIDs are the entries the entries page links to, by gymnast.
func entryIDs(t *testing.T, page, admin string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`href="`+regexp.QuoteMeta(admin)+`/entries/([^"?#]+)[^"]*">([^<]+)</a>`).FindAllStringSubmatch(page, -1) {
		out[m[2]] = m[1]
	}
	return out
}

// Checking entries with the entries page filtered keeps the filters: an
// entry's links carry them, Mark checked and next goes to the next entry
// they show (in their sort), ← Entries goes back to the list as it was at
// the entry's row, and removing entries comes back to it filtered.
func TestCheckingKeepsFilters(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Ann Ryan", "Bea Kelly", "Cat Ryan", "Dee Ryan"} {
		redirected(t, h, enter, bucsEntry(name))
	}
	ids := entryIDs(t, do(t, h, http.MethodGet, admin, nil).Body.String(), admin)
	if len(ids) != 4 {
		t.Fatalf("four entries: %v", ids)
	}
	ann, bea, cat, dee := ids["Ann Ryan"], ids["Bea Kelly"], ids["Cat Ryan"], ids["Dee Ryan"]

	// The entries page's links carry its filters and sort.
	list := "?q=ryan&sort=gymnast"
	page := do(t, h, http.MethodGet, admin+list, nil).Body.String()
	if !strings.Contains(page, `href="`+admin+`/entries/`+ann+`?q=ryan&amp;sort=gymnast"`) {
		t.Error("an entry's link carries the filters and sort")
	}
	if !strings.Contains(page, `action="`+admin+`/remove?q=ryan&amp;sort=gymnast" data-list-carry`) {
		t.Error("removing carries the filters")
	}

	// The entry's page: back to the list as it was, at the entry, and its
	// forms carry the filters.
	page = do(t, h, http.MethodGet, admin+"/entries/"+ann+list, nil).Body.String()
	if !strings.Contains(page, `href="`+admin+`?q=ryan&amp;sort=gymnast#entry-`+ann+`">← Entries</a>`) {
		t.Error("← Entries goes back to the filtered list, at the entry")
	}
	if !strings.Contains(page, `action="`+admin+`/entries/`+ann+`/check?q=ryan&amp;sort=gymnast"`) {
		t.Error("the check form carries the filters")
	}

	// Mark checked and next skips Bea (not a Ryan), keeping the filters.
	to := redirected(t, h, admin+"/entries/"+ann+"/check"+list, url.Values{"checked": {"1"}, "next": {"1"}})
	if !strings.HasPrefix(to, admin+"/entries/"+cat+"?notice=") || !strings.Contains(to, "&q=ryan&sort=gymnast#check") {
		t.Errorf("on to Cat, the next Ryan, filtered: %s", to)
	}
	// Not checked: the entry just checked drops out of the filter, but the
	// next is still the one after it.
	to = redirected(t, h, admin+"/entries/"+cat+"/check?checked=no", url.Values{"checked": {"1"}, "next": {"1"}})
	if !strings.HasPrefix(to, admin+"/entries/"+dee+"?") || !strings.Contains(to, "checked=no") {
		t.Errorf("on to Dee, the next not checked: %s", to)
	}
	if page := do(t, h, http.MethodGet, admin+"/entries/"+cat+"?checked=no", nil).Body.String(); !strings.Contains(page, `href="`+admin+`?checked=no#entry-`+cat+`"`) {
		t.Error("back to the not-checked list")
	}
	// After Dee, the Ryans are all checked: Bea is left, but not shown.
	to = redirected(t, h, admin+"/entries/"+dee+"/check"+list, url.Values{"checked": {"1"}, "next": {"1"}})
	if !strings.HasPrefix(to, admin+"?notice=") || !strings.Contains(to, "last+to+check+of+those+shown") || !strings.Contains(to, "&q=ryan&sort=gymnast") {
		t.Errorf("the last Ryan to check, back to the filtered list: %s", to)
	}
	// Without filters, Bea is next.
	to = redirected(t, h, admin+"/entries/"+dee+"/check", url.Values{"checked": {"1"}, "next": {"1"}})
	if !strings.HasPrefix(to, admin+"/entries/"+bea+"?notice=") || strings.Contains(to, "q=") {
		t.Errorf("on to Bea, unfiltered: %s", to)
	}

	// The filter checked=no isn't read as the posted checked: Save the note
	// unmarks, Mark checked marks.
	redirected(t, h, admin+"/entries/"+bea+"/check?checked=yes", url.Values{"checked": {"0"}})
	if page := do(t, h, http.MethodGet, admin+"?checked=yes&q=bea", nil).Body.String(); strings.Contains(page, "Tick Bea Kelly") {
		t.Error("Bea isn't checked")
	}

	// Removing comes back to the filtered list.
	to = redirected(t, h, admin+"/remove"+list, url.Values{"entry": {bea}, "action": {"hold"}})
	if !strings.HasPrefix(to, admin+"?notice=") || !strings.Contains(to, "&q=ryan&sort=gymnast") {
		t.Errorf("back to the filtered list after removing: %s", to)
	}
}
