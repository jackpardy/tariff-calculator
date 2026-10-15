package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestDashboardNavigation(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("signoff", "1")
	admin := created(t, h, form)
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Dara O'Néill", "Ann Ryan", "Bea Kelly"} {
		redirected(t, h, enter, bucsEntry(name))
	}
	shows := func(query string) string {
		t.Helper()
		return do(t, h, http.MethodGet, admin+"?"+query, nil).Body.String()
	}

	// Levels start collapsed, with buttons to expand or collapse them all;
	// filtering opens the levels with something shown.
	if page := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(page, `data-level="BUCS L3" data-remember data-list-group>`) || !strings.Contains(page, "Expand all") || !strings.Contains(page, "Collapse all") {
		t.Error("levels start collapsed")
	}
	if !strings.Contains(shows("q=oneill"), `data-level="BUCS L3" open`) {
		t.Error("a search opens the level it finds someone in")
	}

	// Search ignores accents and case.
	page := shows("q=oneill")
	if !strings.Contains(page, `aria-label="Tick Dara O&#39;Néill"`) || strings.Contains(page, `aria-label="Tick Ann Ryan"`) || !strings.Contains(page, "1 of 3 entries") {
		t.Error("searching finds Dara only")
	}
	// The levels open and close, so there's no list of them to jump to; the
	// entries come before the competition's facts and counts.
	if !strings.Contains(page, "(1 of 3)") || strings.Contains(page, `href="#level-0"`) || strings.Contains(page, "Jump to") || !strings.Contains(page, `id="level-0"`) {
		t.Error("each level says how many are shown, with no jump list")
	}
	if e, about := strings.Index(page, `<div id="entries">`), strings.Index(page, `class="comp-dash-about`); e < 0 || about < e || strings.Index(page, "comp-counts") < e {
		t.Error("the entries come first, the facts and counts after them")
	}
	// Its many filters are under a Filters button, counting those on.
	if f := shows("checked=yes&signedoff=no"); !strings.Contains(f, `<summary class="button is-small comp-opens-button">Filters <span class="tag is-link is-light ml-1" data-list-filters-on>2</span>`) {
		t.Error("the filters are under a button saying 2 are on")
	}
	// The search and filters are the shared list tools, sent to the server.
	for _, want := range []string{`data-list-tools="entries" data-noun="entries" data-total="3" data-form`, `placeholder="Find a name"`, `name="q" value="oneill"`,
		`<select aria-label="Club" data-list-filter="club" name="club">`, `<div id="entries">`} {
		if !strings.Contains(page, want) {
			t.Errorf("the dashboard's tools have %s", want)
		}
	}
	if rows := listRows(t, page, "entries"); len(rows) != 1 || rows["Dara O'Néill"] == nil {
		t.Errorf("one row, Dara's, searchable by name: %v", rows)
	}
	// Print and CSV what's shown: one set of buttons, following the search,
	// and a button in each level for its cards.
	if strings.Count(page, `href="`+admin+`/cards?q=oneill"`) != 1 || strings.Count(page, `href="`+admin+`/entries.csv?q=oneill"`) != 1 {
		t.Error("print and download what's shown, once each")
	}
	if !strings.Contains(page, ">Print this level's cards</a>") || !strings.Contains(page, `class="button is-small" href="`+admin+`/cards?level=BUCS+L3&amp;q=oneill" data-list-carry`) {
		t.Error("a button prints the level's cards")
	}
	if csv := do(t, h, http.MethodGet, admin+"/entries.csv?q=ryan", nil).Body.String(); !strings.Contains(csv, "Ann Ryan") || strings.Contains(csv, "Bea Kelly") {
		t.Errorf("the CSV of what's shown: %s", csv)
	}

	// Checked or not, signed off or not, no coach.
	ann := regexp.MustCompile(`href="(` + regexp.QuoteMeta(admin) + `/entries/[^"]+)"`).FindStringSubmatch(shows("q=ann"))[1]
	redirected(t, h, ann+"/check", url.Values{"checked": {"1"}})
	if page := shows("checked=yes"); !strings.Contains(page, `aria-label="Tick Ann Ryan"`) || strings.Contains(page, `aria-label="Tick Bea Kelly"`) {
		t.Error("only Ann is checked")
	}
	if page := shows("checked=no&signedoff=no&coach=none"); strings.Contains(page, `aria-label="Tick Ann Ryan"`) || !strings.Contains(page, `aria-label="Tick Bea Kelly"`) {
		t.Error("not checked, not signed off, no coach: Bea and Dara")
	}

	// Sorted by gymnast: Ann, Bea, Dara; by club (as they came) otherwise.
	page = shows("sort=gymnast")
	if a, b, d := strings.Index(page, "Tick Ann Ryan"), strings.Index(page, "Tick Bea Kelly"), strings.Index(page, "Tick Dara"); !(a < b && b < d) {
		t.Errorf("by gymnast: %d %d %d", a, b, d)
	}
	if !strings.Contains(shows("sort=gymnast"), `href="`+admin+`"`) {
		t.Error("clear goes back to everything")
	}
}
