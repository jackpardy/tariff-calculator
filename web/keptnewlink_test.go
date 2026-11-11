package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// A coach's requirements kept on their page are still there on the new link
// the comp sec gives them (ADR 0013): a lost phone loses nothing.
func TestKeptSurvivesANewCoachLink(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	ucd := aClub(t, h, admin, "UCD", false, "Ciarán Ó Briain")
	club := ucd.page
	declan := pathIn(t, do(t, h, http.MethodPost, club+"/coaches", url.Values{"name": {"Declan"}}).Body.String(), "/clubs/coach/")

	set := `{"format":1,"name":"Grade 1","rules":[{"type":"count","min":1,"match":{"rotation":{"min":4,"max":4},"direction":"backward"}}]}`
	if rec := do(t, h, http.MethodPost, declan+"/requirements", url.Values{"items": {`[{"id":"set-1","kind":"set","data":` + set + `}]`}}); rec.Code != http.StatusOK {
		t.Fatalf("keeping: %d %s", rec.Code, rec.Body)
	}
	newLink := formAction(t, do(t, h, http.MethodGet, club+"/coaches", nil).Body.String(), club+"/coaches/")
	fresh := pathIn(t, do(t, h, http.MethodPost, newLink, url.Values{"anyway": {"1"}}).Body.String(), "/clubs/coach/")
	if fresh == declan {
		t.Fatal("a new link")
	}
	if page := do(t, h, http.MethodGet, fresh+"/requirements", nil).Body.String(); !strings.Contains(unescape(page), "Grade 1") {
		t.Errorf("the new link's Requirements tab lists the set: %s", page)
	}
}
