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
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	for _, name := range []string{"Dara O'Néill", "Ann Ryan", "Bea Kelly"} {
		redirected(t, h, enter, url.Values{"gymnast": {name}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	}
	shows := func(query string) string {
		t.Helper()
		return do(t, h, http.MethodGet, admin+"?"+query, nil).Body.String()
	}

	// Levels start collapsed, with buttons to expand or collapse them all;
	// filtering opens the levels with something shown.
	if page := do(t, h, http.MethodGet, admin, nil).Body.String(); !strings.Contains(page, `data-level="BUCS L3" data-remember>`) || !strings.Contains(page, "Expand all") || !strings.Contains(page, "Collapse all") {
		t.Error("levels start collapsed")
	}
	if !strings.Contains(shows("q=oneill"), `data-level="BUCS L3" open`) {
		t.Error("a search opens the level it finds someone in")
	}

	// Search ignores accents and case.
	page := shows("q=oneill")
	if !strings.Contains(page, `aria-label="Tick Dara O&#39;Néill"`) || strings.Contains(page, `aria-label="Tick Ann Ryan"`) || !strings.Contains(page, "1 of 3 shown") {
		t.Error("searching finds Dara only")
	}
	if !strings.Contains(page, "(1 of 3)") || !strings.Contains(page, `href="#level-0"`) || !strings.Contains(page, `id="level-0"`) {
		t.Error("each level says how many are shown, and can be jumped to")
	}
	// Print and CSV what's shown.
	if !strings.Contains(page, admin+"/cards?q=oneill") {
		t.Error("print what's shown")
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
