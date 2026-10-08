package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestLimitsAndWaitingLists(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	var links []string
	for _, name := range []string{"Ann Ryan", "Bea Kelly", "Cat Doyle"} {
		links = append(links, withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {name}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}})))
	}
	if loc := redirected(t, h, admin+"/limits", url.Values{"limit-0": {"two"}}); !strings.Contains(loc, "Nothing+was+changed") {
		t.Errorf("a limit must be a number: %s", loc)
	}
	// BUCS L3 is the first event; it takes 2.
	if loc := redirected(t, h, admin+"/limits", url.Values{"limit-0": {"2"}}); !strings.Contains(loc, "1+entry+on+a+waiting+list") {
		t.Errorf("the organiser hears who's waiting: %s", loc)
	}
	page := do(t, h, http.MethodGet, admin, nil).Body.String()
	if !strings.Contains(page, "2 of 2 places") || !strings.Contains(page, "Waiting lists") || !strings.Contains(page, `aria-label="Let in Cat Doyle"`) {
		t.Fatalf("BUCS L3 full, Cat waiting: %s", page)
	}
	if strings.Contains(page, `aria-label="Tick Cat Doyle"`) {
		t.Error("Cat isn't in the level's table")
	}
	if got := do(t, h, http.MethodGet, links[2], nil).Body.String(); !strings.Contains(got, "On the waiting list for BUCS L3: 1st") {
		t.Error("Cat sees she's first on the waiting list")
	}
	if strings.Contains(do(t, h, http.MethodGet, admin+"/entries.csv", nil).Body.String(), "Cat Doyle") {
		t.Error("waiting entries aren't exported")
	}

	// Bea withdraws: Cat is in.
	if rec := do(t, h, http.MethodPost, links[1]+"/withdraw", url.Values{"confirm": {"1"}}); !strings.Contains(rec.Body.String(), "Entry withdrawn") {
		t.Fatal("Bea withdraws")
	}
	if got := do(t, h, http.MethodGet, links[2], nil).Body.String(); strings.Contains(got, "waiting list") {
		t.Error("Cat moves up when Bea withdraws")
	}
	if page := do(t, h, http.MethodGet, admin, nil).Body.String(); strings.Contains(page, "Waiting lists") || !strings.Contains(page, `aria-label="Tick Cat Doyle"`) {
		t.Error("Cat is in the table now")
	}
	if hist := do(t, h, http.MethodGet, admin+"/history", nil).Body.String(); !strings.Contains(hist, "Changed the limits") {
		t.Error("the history")
	}
}
