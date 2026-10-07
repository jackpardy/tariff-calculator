package main

import (
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPairedSynchro(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form["level"] = []string{"builtin-level:bucs-l3", "builtin-level:bucs-l4"}
	form["synchroLevel"] = []string{"builtin-level:bucs-l3", "builtin-level:bucs-l4"}
	form.Set("synchroPairs", "BUCS L3 + BUCS L4")
	admin := created(t, h, form)
	dash := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(dash, ">Synchro BUCS L3/L4 ") || !strings.Contains(dash, "BUCS L3 + BUCS L4</textarea>") {
		t.Error("one synchro event for both levels, and the pairing to change")
	}
	enter := pathIn(t, dash, "/competitions/enter/")
	if page := do(t, h, http.MethodGet, enter+"?discipline=synchro", nil).Body.String(); !strings.Contains(page, ">BUCS L3/L4 (doing BUCS L3)</option>") || !strings.Contains(page, ">BUCS L3/L4 (doing BUCS L4)</option>") {
		t.Error("a pair chooses which level of the event they do")
	}

	// A competes at L3 and B at L4: as a pair they do L4, the easier.
	redirected(t, h, enter, url.Values{"gymnast": {"A"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})
	b := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"B"}, "level": {"BUCS L4"}, "ex1Option": {"builtin:bucs-l4-option-1"}, "ex2Option": {"builtin:bucs-l4-second"}, "ex2Skills": {voluntary}}))
	pair := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"A"}, "discipline": {"synchro"}, "level": {"BUCS L3"}, "partnerName": {"B"},
		"ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}))
	redirected(t, h, pathIn(t, do(t, h, http.MethodGet, pair, nil).Body.String(), "/competitions/partner/"), url.Values{"link": {"http://example.com" + b}})

	dash = html.UnescapeString(do(t, h, http.MethodGet, admin, nil).Body.String())
	wrong := "A (BUCS L3) and B (BUCS L4) compete individually, so as a pair they do BUCS L4 in synchro, not BUCS L3"
	if !strings.Contains(dash, ">Synchro BUCS L3/L4 ") || !strings.Contains(dash, wrong) {
		t.Errorf("the pair entered at the wrong level is flagged: %s", dash)
	}
	if page := do(t, h, http.MethodGet, pair, nil).Body.String(); !strings.Contains(page, "(doing BUCS L3)") {
		t.Error("the entry says which level the pair does")
	}

	// Changed to L4, it's right.
	redirected(t, h, pair, url.Values{"gymnast": {"A"}, "discipline": {"synchro"}, "level": {"BUCS L4"}, "partnerName": {"B"},
		"ex1Option": {"builtin:bucs-l4-option-1"}, "ex2Option": {"builtin:bucs-l4-second"}, "ex2Skills": {voluntary}})
	dash = html.UnescapeString(do(t, h, http.MethodGet, admin, nil).Body.String())
	if strings.Contains(dash, "as a pair they do") {
		t.Error("the pair at the easier level is fine")
	}

	// Pairing a level not ticked is refused.
	if loc := redirected(t, h, admin+"/events", url.Values{"synchroLevel": {"builtin-level:bucs-l3"}, "synchroPairs": {"BUCS L3 + BUCS L5"}}); !strings.Contains(loc, "weren%27t+changed") && !strings.Contains(loc, "weren't+changed") {
		t.Errorf("pairing a level not ticked: %s", loc)
	}
}
