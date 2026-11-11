package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"tariffCalculator/competitions"
	"tariffCalculator/store"
)

// An offer that says "up to" a level the organiser then removes is shown as
// needing attention on the Officials page and on the judge's own page, and
// takes that role at no level.
func TestOfferLevelGone(t *testing.T) {
	h := competitionServer(t)
	form := newCompetition()
	form.Set("tumbling", "Novice\nElite")
	admin := created(t, h, form)
	officials := admin + "/officials"
	redirected(t, h, officials+"/add", url.Values{"name": {"Ann"}, "judge-tumbling": {"1"}, "execution-tumbling": {"Elite"}, "difficulty-tumbling": {"no"}, "chair-tumbling": {"no"}})
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	own := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"Fay"}, "level": {"FIG AG3 (17–21)"}, "ex1Skills": {voluntary}, "ex2Skills": {voluntary}}))
	redirected(t, h, own+"/offer", url.Values{"judge-tumbling": {"1"}, "execution-tumbling": {"Elite"}, "difficulty-tumbling": {"no"}, "chair-tumbling": {"no"}})

	line := "Says up to Elite, which isn&#39;t a level any more: ask them, or set it"
	if strings.Contains(do(t, h, http.MethodGet, officials, nil).Body.String(), "a level any more") || strings.Contains(do(t, h, http.MethodGet, own, nil).Body.String(), "a level any more") {
		t.Error("nothing to say while the level is there")
	}
	redirected(t, h, admin+"/events", url.Values{"tumbling": {"Novice"}})
	if page := do(t, h, http.MethodGet, officials, nil).Body.String(); !strings.Contains(page, line) || strings.Count(page, "a level any more") != 2 {
		t.Errorf("the Officials page says it for Ann and for Fay: %d", strings.Count(page, "a level any more"))
	}
	if page := do(t, h, http.MethodGet, own, nil).Body.String(); !strings.Contains(page, line) {
		t.Error("the judge's own page says it")
	}

	// The seat dropdown's reason, and the rota's: no seat at any level.
	c := competitions.Competition{Tumbling: []string{"Novice"}}
	offer := competitions.Offer{Judge: map[string]competitions.JudgeOffer{competitions.Tumbling: {UpTo: map[string]string{competitions.RoleExecution: "Elite"}}}}
	people := rotaPeople(c, []store.Official{{ID: "o1", Name: "Ann", Offer: offer}}, nil)
	if len(people) != 1 || len(people[0].Gone) != 1 || people[0].Gone[0] != "Elite" || people[0].Can(competitions.RoleExecution, "Novice") {
		t.Errorf("the rota person: %+v", people)
	}
}
