package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// A person keeps their own requirements and levels on their page (ADR
// 0013): Save to my page posts them, the JSON gives them back (to the
// Requirements page and the builder), their Requirements tab lists them and
// takes them off; someone else's link sees none of them; and an
// individual's link keeps them too.
func TestKeptRequirements(t *testing.T) {
	h := competitionServer(t)
	admin := created(t, h, newCompetition())
	club := aClub(t, h, admin, "UCD", false, "A. Murphy", "B. Byrne")
	ann, bea := club.members["A. Murphy"], club.members["B. Byrne"]

	set := `{"format":1,"name":"Grade 1","rules":[{"type":"count","min":1,"match":{"rotation":{"min":4,"max":4},"direction":"backward"}}]}`
	level := `{"format":1,"name":"Novice","first":{"options":["set-1"]},"second":{"options":["builtin:bucs-l7-second"]}}`
	keep := func(link, items string) map[string]any {
		t.Helper()
		rec := do(t, h, http.MethodPost, link+"/requirements", url.Values{"items": {items}})
		if rec.Code != http.StatusOK {
			t.Fatalf("keeping: %d %s", rec.Code, rec.Body)
		}
		var answer map[string]any
		json.Unmarshal(rec.Body.Bytes(), &answer)
		return answer
	}
	if got := keep(ann, `[{"id":"set-1","kind":"set","data":`+set+`},{"id":"level-1","kind":"level","data":`+level+`},{"id":"x","kind":"set","data":{"rules":[]}}]`); got["kept"] != 2.0 || len(got["refused"].([]any)) != 1 {
		t.Errorf("two kept, the nameless one refused: %v", got)
	}

	var kept struct {
		Sets, Levels []struct {
			ID        string
			Data      json.RawMessage
			Described []string
		}
	}
	json.Unmarshal(do(t, h, http.MethodGet, ann+"/requirements.json", nil).Body.Bytes(), &kept)
	if len(kept.Sets) != 1 || kept.Sets[0].ID != "set-1" || !strings.Contains(string(kept.Sets[0].Data), "Grade 1") || len(kept.Sets[0].Described) != 1 || len(kept.Levels) != 1 {
		t.Errorf("read back: %+v", kept)
	}
	if body := do(t, h, http.MethodGet, bea+"/requirements.json", nil).Body.String(); strings.Contains(body, "Grade 1") {
		t.Errorf("another member sees none of them: %s", body)
	}

	// Saved again from the browser: changed, not added.
	keep(ann, `[{"id":"set-1","kind":"set","data":`+strings.Replace(set, "Grade 1", "Grade 1 (2027)", 1)+`}]`)
	page := do(t, h, http.MethodGet, ann+"/requirements", nil).Body.String()
	if got := currentTab(t, page); got != "Requirements" {
		t.Errorf("the member's Requirements tab is %q", got)
	}
	for _, want := range []string{"Grade 1 (2027)", "Novice", "First exercise: Grade 1 (2027)", "Second exercise: BUCS L7", "data-keep-from-browser", "Take off"} {
		if !strings.Contains(unescape(page), want) {
			t.Errorf("the Requirements tab should have %q", want)
		}
	}
	if strings.Count(page, "Grade 1 (2027)") < 1 || strings.Contains(page, ">Grade 1<") {
		t.Error("saved again, it changed in place")
	}

	// Taken off from the tab.
	location := redirected(t, h, ann+"/requirements/set/set-1/remove", url.Values{"back": {"1"}})
	if notice := noticeOf(t, location); notice != "Taken off your page." {
		t.Errorf("taken off: %q", notice)
	}
	json.Unmarshal(do(t, h, http.MethodGet, ann+"/requirements.json", nil).Body.Bytes(), &kept)
	if len(kept.Sets) != 0 || len(kept.Levels) != 1 {
		t.Errorf("after taking it off: %+v", kept)
	}

	// Someone entering on their own keeps them on their personal page.
	enter := pathIn(t, do(t, h, http.MethodGet, admin+"/settings", nil).Body.String(), "/competitions/enter/")
	entry := withoutQuery(redirected(t, h, enter, bucsEntry("Ivy")))
	personal := pathIn(t, do(t, h, http.MethodGet, entry, nil).Body.String(), "/competitions/gymnast/")
	keep(personal, `[{"id":"set-9","kind":"set","data":`+set+`}]`)
	if page := do(t, h, http.MethodGet, personal+"/requirements", nil).Body.String(); !strings.Contains(page, "Grade 1") || currentTab(t, page) != "Requirements" {
		t.Errorf("an individual's Requirements tab: %s", page)
	}
}
